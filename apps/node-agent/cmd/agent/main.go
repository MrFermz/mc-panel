// node-agent — daemon ประจำเครื่องที่รัน Minecraft instance
//
// ลำดับ boot: ต่อ docker → เปิด gRPC stream หา control plane (retry backoff)
// → ส่ง Hello รอ Welcome เพื่อรู้ node_id → reconcile container ที่มีอยู่
// → เปิด NATS job consumer + heartbeat + docker events watcher
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/nats-io/nats.go"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/mc-panel/node-agent/internal/console"
	"github.com/mc-panel/node-agent/internal/events"
	"github.com/mc-panel/node-agent/internal/filemanager"
	"github.com/mc-panel/node-agent/internal/grpcclient"
	"github.com/mc-panel/node-agent/internal/heartbeat"
	"github.com/mc-panel/node-agent/internal/jobs"
	"github.com/mc-panel/node-agent/internal/mcstate"
	"github.com/mc-panel/node-agent/internal/provision"
	"github.com/mc-panel/node-agent/internal/reconcile"
	"github.com/mc-panel/node-agent/internal/runner"
	"github.com/mc-panel/node-agent/internal/serverstats"
	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"
)

const agentVersion = "0.1.0"

type config struct {
	agentToken         string
	controlPlaneGRPC   string
	natsURL            string
	mcDataDir          string
	mcNetwork          string
	runtimeImagePrefix string
}

func loadConfig() (config, error) {
	cfg := config{
		agentToken:         os.Getenv("AGENT_TOKEN"),
		controlPlaneGRPC:   os.Getenv("CONTROL_PLANE_GRPC"),
		natsURL:            os.Getenv("NATS_URL"),
		mcDataDir:          os.Getenv("MC_DATA_DIR"),
		mcNetwork:          envOr("MC_NETWORK", "mcpanel-servers"),
		runtimeImagePrefix: envOr("MC_RUNTIME_IMAGE_PREFIX", "mcpanel/mc-runtime"),
	}
	if cfg.agentToken == "" {
		return cfg, errors.New("AGENT_TOKEN is required")
	}
	if cfg.controlPlaneGRPC == "" {
		return cfg, errors.New("CONTROL_PLANE_GRPC is required")
	}
	if cfg.natsURL == "" {
		return cfg, errors.New("NATS_URL is required")
	}
	if cfg.mcDataDir == "" {
		return cfg, errors.New("MC_DATA_DIR is required")
	}
	if !filepath.IsAbs(cfg.mcDataDir) {
		// path นี้ถูกส่งต่อให้ docker daemon ทำ bind mount จากมุมมอง host
		// relative path จะชี้ผิดที่แบบเงียบ ๆ — บังคับ absolute ตั้งแต่ต้น
		return cfg, fmt.Errorf("MC_DATA_DIR must be an absolute path, got %q", cfg.mcDataDir)
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("node-agent fatal: %v", err)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("node-agent starting: version=%s data_dir=%s", agentVersion, cfg.mcDataDir)

	if err := os.MkdirAll(cfg.mcDataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer dockerCli.Close()
	if err := waitDocker(ctx, dockerCli); err != nil {
		return err
	}
	log.Printf("docker daemon connected")

	// compose สร้างเฉพาะ network ที่มี service ใช้ — วง MC มีแต่ container ที่ agent
	// สร้างเอง เลยต้อง ensure เองที่นี่ (ครอบคลุมทั้งโหมด compose และ node เดี่ยว)
	if err := ensureNetwork(ctx, dockerCli, cfg.mcNetwork); err != nil {
		return err
	}

	dockerRunner := runner.NewDockerRunner(dockerCli, cfg.mcDataDir, cfg.mcNetwork)
	grpcCli := grpcclient.New(cfg.controlPlaneGRPC, cfg.agentToken, buildHello(cfg.mcDataDir))
	// tracker อ่านสถานะในเกมจาก console (ผู้เล่นออนไลน์/TPS) แล้ว serverstats แนบไปกับ stats
	// สองตัวอ้างถึงกัน: Manager ต้องมี observer ตั้งแต่สร้าง / tracker เขียน stdin ผ่าน Manager
	mcTracker := mcstate.NewTracker()
	consoles := console.NewManager(dockerRunner, grpcCli, mcTracker)
	mcTracker.SetWriter(consoles)
	defer consoles.DetachAll()

	// ตั้ง handler ก่อน Run เพื่อไม่พลาด ConsoleInput ที่มาทันทีหลัง Welcome
	grpcCli.OnConsoleInput(func(serverID, command string) {
		if err := consoles.WriteInput(serverID, command); err != nil {
			log.Printf("console input failed: server=%s err=%v", serverID, err)
		}
	})

	files := filemanager.NewManager(cfg.mcDataDir)
	grpcCli.OnFileRequest(func(req *agentv1.FileRequest) *agentv1.FileResponse {
		return handleFileRequest(files, req)
	})

	grpcDone := make(chan error, 1)
	go func() { grpcDone <- grpcCli.Run(ctx) }()

	log.Printf("waiting for control plane welcome: addr=%s", cfg.controlPlaneGRPC)
	var nodeID string
	welcomed := make(chan error, 1)
	go func() {
		id, err := grpcCli.WaitForNodeID(ctx)
		nodeID = id
		welcomed <- err
	}()
	select {
	case err := <-welcomed:
		if err != nil {
			return err
		}
	case err := <-grpcDone:
		// client จบก่อนได้ Welcome (เช่น address ผิดรูปแบบ) — อย่าค้างรอเงียบ ๆ
		return fmt.Errorf("grpc client stopped before welcome: %w", err)
	}
	log.Printf("registered with control plane: node_id=%s", nodeID)

	// เปิด events watcher ก่อน reconcile — กัน start/die event หล่นหายในช่องว่าง
	// ระหว่าง list container กับตอน subscribe
	go events.Watch(ctx, dockerCli, dockerRunner, grpcCli, consoles, dockerRunner, consoles)

	if err := reconcile.Run(ctx, dockerCli, grpcCli, consoles); err != nil {
		return err
	}

	nc, err := nats.Connect(cfg.natsURL,
		nats.Name("mc-panel-agent-"+nodeID),
		nats.MaxReconnects(-1),
		nats.RetryOnFailedConnect(true),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	defer nc.Drain()
	log.Printf("nats connected")

	prov := provision.New(dockerCli, cfg.mcDataDir, cfg.runtimeImagePrefix)
	consumer, err := jobs.NewConsumer(nc, nodeID, jobs.NewHandler(dockerRunner, prov, cfg.mcDataDir))
	if err != nil {
		return fmt.Errorf("create jobs consumer: %w", err)
	}
	jobsDone := make(chan error, 1)
	go func() { jobsDone <- consumer.Run(ctx) }()

	go heartbeat.Run(ctx, dockerCli, grpcCli, cfg.mcDataDir)
	go serverstats.Run(ctx, dockerCli, dockerRunner, grpcCli, mcTracker)

	log.Printf("node-agent ready: node_id=%s", nodeID)

	select {
	case <-ctx.Done():
		log.Printf("shutdown signal received")
	case err := <-grpcDone:
		if ctx.Err() == nil {
			return fmt.Errorf("grpc client stopped unexpectedly: %w", err)
		}
	case err := <-jobsDone:
		if ctx.Err() == nil {
			return fmt.Errorf("jobs consumer stopped unexpectedly: %w", err)
		}
	}
	return nil
}

func waitDocker(ctx context.Context, cli *client.Client) error {
	// docker daemon อาจยังไม่พร้อมตอน compose เพิ่ง start — retry สั้น ๆ พอ
	// (compose ตั้ง restart: unless-stopped อยู่แล้วถ้าพังจริง)
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		if _, lastErr = cli.Ping(ctx); lastErr == nil {
			return nil
		}
		log.Printf("docker ping failed: attempt=%d err=%v", attempt, lastErr)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("docker daemon unreachable: %w", lastErr)
}

func ensureNetwork(ctx context.Context, cli *client.Client, name string) error {
	if _, err := cli.NetworkInspect(ctx, name, network.InspectOptions{}); err == nil {
		return nil
	}
	_, err := cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver:     "bridge",
		Attachable: true,
		Labels:     map[string]string{"project": "mc-panel"},
	})
	if err != nil {
		// สอง agent boot พร้อมกันอาจแข่งกันสร้าง — ตรวจซ้ำก่อนถือว่า fail จริง
		if _, inspectErr := cli.NetworkInspect(ctx, name, network.InspectOptions{}); inspectErr == nil {
			return nil
		}
		return fmt.Errorf("create mc network %q: %w", name, err)
	}
	log.Printf("mc network created: %s", name)
	return nil
}

// handleFileRequest แปลง FileRequest.op → เรียก filemanager ops → ประกอบ FileResponse
// RequestId ต้องตรงกับ request เสมอ เพื่อให้ control-plane จับคู่ response ได้
func handleFileRequest(files *filemanager.Manager, req *agentv1.FileRequest) *agentv1.FileResponse {
	resp := &agentv1.FileResponse{RequestId: req.GetRequestId()}
	serverID := req.GetServerId()

	fail := func(err error) *agentv1.FileResponse {
		resp.Success = false
		resp.Error = err.Error()
		return resp
	}

	switch op := req.Op.(type) {
	case *agentv1.FileRequest_List:
		infos, err := files.List(serverID, op.List.GetPath())
		if err != nil {
			return fail(err)
		}
		entries := make([]*agentv1.FileEntry, 0, len(infos))
		for _, fi := range infos {
			entries = append(entries, &agentv1.FileEntry{
				Name:        fi.Name,
				IsDir:       fi.IsDir,
				Size:        fi.Size,
				ModTimeUnix: fi.ModTimeUnix,
			})
		}
		resp.Entries = entries
	case *agentv1.FileRequest_Read:
		content, truncated, err := files.Read(serverID, op.Read.GetPath())
		if err != nil {
			return fail(err)
		}
		resp.Content = content
		resp.Truncated = truncated
	case *agentv1.FileRequest_Write:
		if err := files.Write(serverID, op.Write.GetPath(), op.Write.GetContent()); err != nil {
			return fail(err)
		}
	case *agentv1.FileRequest_WriteChunk:
		if err := files.WriteChunk(serverID, op.WriteChunk.GetPath(), op.WriteChunk.GetContent(), op.WriteChunk.GetFirst(), op.WriteChunk.GetLast()); err != nil {
			return fail(err)
		}
	case *agentv1.FileRequest_Mkdir:
		if err := files.Mkdir(serverID, op.Mkdir.GetPath()); err != nil {
			return fail(err)
		}
	case *agentv1.FileRequest_Delete:
		if err := files.Delete(serverID, op.Delete.GetPath()); err != nil {
			return fail(err)
		}
	case *agentv1.FileRequest_Rename:
		if err := files.Rename(serverID, op.Rename.GetFrom(), op.Rename.GetTo()); err != nil {
			return fail(err)
		}
	default:
		return fail(errors.New("unknown file operation"))
	}

	resp.Success = true
	return resp
}

func buildHello(dataDir string) *agentv1.Hello {
	hello := &agentv1.Hello{
		AgentVersion: agentVersion,
		Os:           runtime.GOOS,
		Arch:         runtime.GOARCH,
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		hello.TotalRamMb = int64(vm.Total / (1024 * 1024))
	}
	if du, err := disk.Usage(dataDir); err == nil {
		hello.TotalDiskMb = int64(du.Total / (1024 * 1024))
	}
	return hello
}
