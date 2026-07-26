package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	"github.com/game-manager/node-agent/internal/games"
)

const (
	// grace period ตอน stop: รอเกม save state หลัง stop word ก่อน fallback SIGTERM
	gracefulWait = 30 * time.Second
	sigtermWait  = 10 // วินาที — ให้ docker ส่ง SIGKILL ต่อเองถ้า SIGTERM ไม่พอ
)

// DockerRunner คุม instance ผ่าน Docker Engine API ของ host (ผ่าน /var/run/docker.sock)
// container ของ instance ถูกสร้างเป็น sibling ของ agent container ไม่ใช่ลูก — path bind mount
// จึงต้องเป็น path ฝั่ง host เสมอ (GM_DATA_DIR mount ด้วย path เดียวกันทั้งสองฝั่ง)
type DockerRunner struct {
	cli     *client.Client
	network string
	// games = ตัวหา game definition ของแต่ละ instance — runner ไม่รู้จักเกมใด ๆ เอง
	// (container port, env ของ launch script, คำสั่ง stop มาจาก definition ของ instance นั้น)
	games games.InstanceLookup

	mu sync.Mutex
	// จำว่า server ไหนถูก "สั่ง" stop/kill — ใช้แยก STOPPED กับ ERRORED ตอน die event
	stopRequested map[string]struct{}
}

func NewDockerRunner(cli *client.Client, network string, gl games.InstanceLookup) *DockerRunner {
	return &DockerRunner{
		cli:           cli,
		network:       network,
		games:         gl,
		stopRequested: make(map[string]struct{}),
	}
}

// containerName — prefix ของ brand เพื่อให้ container ของ panel แยกออกจากของอื่นบน host
// ชื่อนี้ถูกใช้เป็น network alias ด้วย (proxy variant อ้างถึง backend ด้วยชื่อคงที่)
func containerName(id string) string { return "game-manager-" + id }

func (r *DockerRunner) markStopRequested(id string) {
	r.mu.Lock()
	r.stopRequested[id] = struct{}{}
	r.mu.Unlock()
}

// ConsumeStopRequested เช็คแล้วเคลียร์ flag ในครั้งเดียว — die event ถัดไป
// ที่ไม่มีคำสั่งค้างต้องถูกตีความเป็น crash ตามปกติ
func (r *DockerRunner) ConsumeStopRequested(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.stopRequested[id]
	delete(r.stopRequested, id)
	return ok
}

func (r *DockerRunner) Start(ctx context.Context, cfg ServerConfig) error {
	name := containerName(cfg.ID)

	// instance บน disk บอกเองว่าเป็นเกมอะไร (.gamemanager/meta.json) — job start ไม่ได้พกมา
	def, _, ok := r.games.DefinitionFor(cfg.ID)
	if !ok {
		return fmt.Errorf("server %s references a game this agent does not support", cfg.ID)
	}

	if insp, err := r.cli.ContainerInspect(ctx, name); err == nil {
		if insp.State != nil && insp.State.Running {
			// โดน redeliver ซ้ำหรือ user กด start ซ้อน — ถือว่าสำเร็จแล้ว
			return nil
		}
		if err := r.removeContainer(ctx, name); err != nil {
			return fmt.Errorf("remove stale container: %w", err)
		}
	} else if !client.IsErrNotFound(err) {
		return fmt.Errorf("inspect container: %w", err)
	}

	// docker daemon สร้าง bind source ที่หายไปให้เองเป็น root-owned dir
	// — กันเคสยังไม่ provision ด้วยการเช็คก่อน
	if _, err := os.Stat(cfg.WorkDir); err != nil {
		return fmt.Errorf("server directory %s not found (server not provisioned?): %w", cfg.WorkDir, err)
	}

	if err := games.EnsureRuntimeImage(ctx, r.cli, cfg.Image); err != nil {
		return err
	}

	pidsLimit := int64(512)
	memoryBytes := int64(cfg.MemoryMB) * 1024 * 1024
	// HOME ชี้เข้า data dir: image hardened ของเราตั้ง HOME ให้แล้ว แต่ base eclipse-temurin ที่ pull มา cache
	// ไม่ได้ตั้ง — modded server บางตัวเขียน cache ลง $HOME ต้องชี้เข้า data dir ที่ write ได้
	env := append(def.LaunchEnv(cfg.MemoryMB), "HOME="+games.ContainerDataDir)

	config := &container.Config{
		Image: cfg.Image,
		// สั่ง launch.sh ตรง ๆ ไม่พึ่ง CMD ของ image — runtime image ที่ agent auto-pull
		// (eclipse-temurin re-tag) ไม่มี CMD ของเรา entrypoint เลย exec ว่างแล้ว exit 0 ทันที
		// (เฉพาะ image ที่ build ด้วย make runtime-images ถึงจะมี CMD launch.sh) ตั้งตรงนี้ครอบทั้งสองแบบ
		Cmd:        []string{"/bin/sh", games.ContainerDataDir + "/" + games.PanelDir + "/" + games.LaunchScriptName},
		Env:        env,
		User:       "1000:1000",
		WorkingDir: games.ContainerDataDir,
		OpenStdin:  true,
		Tty:        false,
		Labels: map[string]string{
			LabelManagedBy: LabelManagedByValue,
			LabelServerID:  cfg.ID,
			LabelProject:   LabelProjectValue,
		},
	}
	hostConfig := &container.HostConfig{
		Binds:       []string{cfg.WorkDir + ":" + games.ContainerDataDir},
		CapDrop:     strslice.StrSlice{"ALL"},
		SecurityOpt: []string{"no-new-privileges"},
		// agent เป็นเจ้าของ lifecycle เอง — docker restart เองจะทำ state ใน DB เพี้ยน
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
		Resources: container.Resources{
			Memory: memoryBytes,
			// MemorySwap = Memory คือปิด swap — game server ที่โดน swap อาการแย่กว่าโดน OOM kill
			MemorySwap: memoryBytes,
			PidsLimit:  &pidsLimit,
		},
	}
	if cfg.Port > 0 {
		// port ที่ server ฟังใน container มาจาก game definition ของ instance นั้น
		gamePort := nat.Port(strconv.Itoa(def.ContainerPort) + "/tcp")
		config.ExposedPorts = nat.PortSet{gamePort: struct{}{}}
		hostConfig.PortBindings = nat.PortMap{
			gamePort: []nat.PortBinding{{HostPort: strconv.Itoa(cfg.Port)}},
		}
	}
	netConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			// alias game-manager-{id} ให้ proxy instance อ้างถึง backend ด้วยชื่อคงที่ใน network เดียวกัน
			r.network: {Aliases: []string{name}},
		},
	}

	if _, err := r.cli.ContainerCreate(ctx, config, hostConfig, netConfig, nil, name); err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	r.mu.Lock()
	delete(r.stopRequested, cfg.ID)
	r.mu.Unlock()
	if err := r.cli.ContainerStart(ctx, name, container.StartOptions{}); err != nil {
		// create สำเร็จแต่ start ไม่ขึ้น — ลบ Created container ที่ค้างทิ้งก่อน return
		// ไม่งั้น start รอบถัดไปเจอ stale container ต้องเสียรอบ remove เอง
		if rmErr := r.removeContainer(ctx, name); rmErr != nil {
			log.Printf("remove container after failed start: server=%s err=%v", cfg.ID, rmErr)
		}
		return classifyStartError(err, cfg.Port)
	}
	log.Printf("container started: server=%s game=%s image=%s memory_mb=%d host_port=%d",
		cfg.ID, def.ID, cfg.Image, cfg.MemoryMB, cfg.Port)
	return nil
}

// classifyStartError แปลง error ดิบจาก ContainerStart เป็นข้อความที่ user อ่านรู้เรื่อง
// เคสเจอบ่อยสุดคือ host port ถูกใช้อยู่แล้วโดยของนอก game-manager — DB unique (node_id, host_port)
// จับไม่ได้เพราะมันเช็คแค่ server ที่ panel จัดการ ไม่รู้จักโปรเซสอื่นบน host หรือ port ที่ OS จองไว้
// (เช่น winnat/Hyper-V excluded range บน Windows/WSL ที่คืน 500 จาก /forwards/expose).
// docker แต่ละ backend ขึ้นข้อความคนละแบบ (Linux: "port is already allocated" /
// Docker Desktop: "ports are not available") จึง match หลาย substring เป็น lowercase
func classifyStartError(err error, port int) error {
	msg := strings.ToLower(err.Error())
	portInUse := strings.Contains(msg, "port is already allocated") ||
		strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "ports are not available") ||
		strings.Contains(msg, "/forwards/expose") ||
		strings.Contains(msg, "bind for")
	if portInUse && port > 0 {
		return fmt.Errorf("host port %d is already in use on this node (another process or the OS is holding it — e.g. a reserved port range on Windows/WSL); free that port or set a different host port for this server: %w", port, err)
	}
	return fmt.Errorf("start container: %w", err)
}

func (r *DockerRunner) Stop(id string, graceful bool) error {
	ctx := context.Background()
	name := containerName(id)

	insp, err := r.cli.ContainerInspect(ctx, name)
	if client.IsErrNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}

	r.markStopRequested(id)

	if insp.State != nil && insp.State.Running {
		stopped := false
		if graceful {
			word := r.stopCommand(id)
			if err := r.writeStdin(ctx, name, word+"\n"); err != nil {
				log.Printf("write stop command failed: server=%s err=%v (falling back to SIGTERM)", id, err)
			} else {
				stopped = r.waitNotRunning(ctx, name, gracefulWait)
			}
		}
		if !stopped {
			timeout := sigtermWait
			if err := r.cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout}); err != nil && !client.IsErrNotFound(err) {
				return fmt.Errorf("stop container: %w", err)
			}
		}
	}

	// ลบเฉพาะ container — เก็บ directory ไว้ start รอบถัดไป
	return r.removeContainer(ctx, name)
}

func (r *DockerRunner) Kill(id string) error {
	ctx := context.Background()
	name := containerName(id)
	r.markStopRequested(id)
	if err := r.cli.ContainerKill(ctx, name, "SIGKILL"); err != nil && !client.IsErrNotFound(err) {
		// container มีอยู่แต่หยุดแล้ว → daemon ตอบ conflict — remove ต่อได้เลย
		log.Printf("container kill: server=%s err=%v (removing anyway)", id, err)
	}
	return r.removeContainer(ctx, name)
}

// Cleanup ลบ container game-manager-{id} ทิ้งโดยไม่แตะ directory (data ยังอยู่)
// die handler เรียกตัวนี้เก็บกวาด container ที่ crash แล้วค้างเป็น Exited —
// เฉพาะเคส crash (ERRORED) เท่านั้น; Stop()/Kill() ลบ container ให้เองอยู่แล้ว
func (r *DockerRunner) Cleanup(id string) error {
	return r.removeContainer(context.Background(), containerName(id))
}

func (r *DockerRunner) AttachConsole(id string) (io.ReadWriteCloser, error) {
	hijack, err := r.cli.ContainerAttach(context.Background(), containerName(id), container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("attach container: %w", err)
	}
	pr, pw := io.Pipe()
	go func() {
		// Tty=false → stream เป็น multiplexed frame ต้อง demux ด้วย stdcopy
		// stdout/stderr รวมเข้า pipe เดียว — console ฝั่ง web ไม่แยกอยู่แล้ว
		_, err := stdcopy.StdCopy(pw, pw, hijack.Reader)
		pw.CloseWithError(err)
	}()
	return &attachConn{hijack: hijack, pr: pr}, nil
}

type attachConn struct {
	hijack types.HijackedResponse
	pr     *io.PipeReader
}

func (a *attachConn) Read(p []byte) (int, error)  { return a.pr.Read(p) }
func (a *attachConn) Write(p []byte) (int, error) { return a.hijack.Conn.Write(p) }
func (a *attachConn) Close() error {
	a.hijack.Close()
	return a.pr.Close()
}

func (r *DockerRunner) Stats(id string) (ResourceStats, error) {
	// อ่านแบบ stream 2 frame แทน OneShot — OneShot มี PreCPUStats=0 ทำให้ CPU%
	// (คำนวณจาก delta ระหว่าง 2 sample) เพี้ยนไปสูงผิดปกติ frame ที่สองมี PreCPUStats
	// จาก frame แรกครบ จึงคำนวณ delta ได้ถูก (frame ห่างกัน ~1s ตาม docker)
	resp, err := r.cli.ContainerStats(context.Background(), containerName(id), true)
	if err != nil {
		return ResourceStats{}, err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	var s container.StatsResponse
	if err := dec.Decode(&s); err != nil {
		return ResourceStats{}, err
	}
	if err := dec.Decode(&s); err != nil {
		return ResourceStats{}, err
	}
	stats := ResourceStats{
		MemoryMB:      int(s.MemoryStats.Usage / (1024 * 1024)),
		MemoryLimitMB: int(s.MemoryStats.Limit / (1024 * 1024)),
	}
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	if cpuDelta > 0 && sysDelta > 0 {
		stats.CPUPercent = cpuDelta / sysDelta * float64(s.CPUStats.OnlineCPUs) * 100
	}
	// counter สะสมของ network/block-io — sum ทุก interface / ทุก device
	// (แปลงเป็น rate/sec ที่ serverstats จาก delta ระหว่าง sample)
	for _, n := range s.Networks {
		stats.NetRxBytes += n.RxBytes
		stats.NetTxBytes += n.TxBytes
	}
	for _, e := range s.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(e.Op) {
		case "read":
			stats.DiskReadBytes += e.Value
		case "write":
			stats.DiskWrBytes += e.Value
		}
	}
	return stats, nil
}

// stopCommand คืนคำที่เขียนเข้า stdin เพื่อสั่งปิดอย่างสุภาพ — เชื่อ meta.json ที่ provision
// เขียนไว้ก่อน (instance เก่าที่ยังไม่มี field `game` ก็ยังหยุดถูก) แล้ว fallback ไป definition
// อ่านอะไรไม่ได้เลย = เกม default + variant ว่าง ซึ่งให้คำสั่งมาตรฐานของเกมนั้น
func (r *DockerRunner) stopCommand(id string) string {
	def, meta, ok := r.games.DefinitionFor(id)
	if meta.StopCommand != "" {
		return meta.StopCommand
	}
	if !ok {
		def = r.games.Registry.Default()
	}
	if def == nil {
		return ""
	}
	return def.StopCommand(meta.Variant)
}

// writeStdin เปิด attach ชั่วคราวเฉพาะ stdin เพื่อส่งคำสั่งเดียว
// (docker อนุญาต attach ซ้อนกับ session ของ console manager ได้)
func (r *DockerRunner) writeStdin(ctx context.Context, name, s string) error {
	hijack, err := r.cli.ContainerAttach(ctx, name, container.AttachOptions{Stream: true, Stdin: true})
	if err != nil {
		return err
	}
	defer hijack.Close()
	_, err = hijack.Conn.Write([]byte(s))
	return err
}

func (r *DockerRunner) waitNotRunning(ctx context.Context, name string, timeout time.Duration) bool {
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	waitCh, errCh := r.cli.ContainerWait(wctx, name, container.WaitConditionNotRunning)
	select {
	case <-waitCh:
		return true
	case err := <-errCh:
		return client.IsErrNotFound(err)
	}
}

func (r *DockerRunner) removeContainer(ctx context.Context, name string) error {
	err := r.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	if err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("remove container: %w", err)
	}
	return nil
}
