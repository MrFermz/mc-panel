package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

const (
	mcPort = "25565/tcp"
	// grace period ตอน stop: รอ save world หลัง stop word ก่อน fallback SIGTERM
	gracefulWait = 30 * time.Second
	sigtermWait  = 10 // วินาที — ให้ docker ส่ง SIGKILL ต่อเองถ้า SIGTERM ไม่พอ
)

// DockerRunner คุม instance ผ่าน Docker Engine API ของ host (ผ่าน /var/run/docker.sock)
// MC container ถูกสร้างเป็น sibling ของ agent container ไม่ใช่ลูก — path bind mount
// จึงต้องเป็น path ฝั่ง host เสมอ (MC_DATA_DIR mount ด้วย path เดียวกันทั้งสองฝั่ง)
type DockerRunner struct {
	cli     *client.Client
	dataDir string
	network string

	mu sync.Mutex
	// จำว่า server ไหนถูก "สั่ง" stop/kill — ใช้แยก STOPPED กับ ERRORED ตอน die event
	stopRequested map[string]struct{}
}

func NewDockerRunner(cli *client.Client, dataDir, network string) *DockerRunner {
	return &DockerRunner{
		cli:           cli,
		dataDir:       dataDir,
		network:       network,
		stopRequested: make(map[string]struct{}),
	}
}

func containerName(id string) string { return "mc-" + id }

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

	if err := EnsureRuntimeImage(ctx, r.cli, cfg.Image); err != nil {
		return err
	}

	pidsLimit := int64(512)
	// MemoryMB ของ user = Xmx (heap) — แต่ JVM กินนอก heap อีกมาก
	// (metaspace, code cache, thread stacks, direct buffers, GC)
	// วัดจริง: Paper 1.21 heap 1G โดน OOM kill ที่ limit 1.25x ตอน world-gen
	// เลยเผื่อ max(50%, 768MB) แต่ไม่เกิน 2GB เพื่อไม่ให้เครื่องใหญ่เปลืองเกินเหตุ
	overheadMB := int64(cfg.MemoryMB) / 2
	if overheadMB < 768 {
		overheadMB = 768
	}
	if overheadMB > 2048 {
		overheadMB = 2048
	}
	memoryBytes := (int64(cfg.MemoryMB) + overheadMB) * 1024 * 1024

	config := &container.Config{
		Image: cfg.Image,
		// HOME=/mc: image hardened ของเราตั้ง HOME ให้แล้ว แต่ base eclipse-temurin ที่ pull มา cache
		// ไม่ได้ตั้ง — modded server บางตัวเขียน cache ลง $HOME ต้องชี้เข้า /mc ที่ write ได้
		Env:        []string{fmt.Sprintf("MC_MEMORY_MB=%d", cfg.MemoryMB), "HOME=/mc"},
		User:       "1000:1000",
		WorkingDir: "/mc",
		OpenStdin:  true,
		Tty:        false,
		Labels: map[string]string{
			LabelManagedBy: LabelManagedByValue,
			LabelServerID:  cfg.ID,
			LabelProject:   LabelProjectValue,
		},
	}
	hostConfig := &container.HostConfig{
		Binds:       []string{filepath.Join(r.dataDir, cfg.ID) + ":/mc"},
		CapDrop:     strslice.StrSlice{"ALL"},
		SecurityOpt: []string{"no-new-privileges"},
		// agent เป็นเจ้าของ lifecycle เอง — docker restart เองจะทำ state ใน DB เพี้ยน
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
		Resources: container.Resources{
			Memory: memoryBytes,
			// MemorySwap = Memory คือปิด swap — MC server ที่โดน swap อาการแย่กว่าโดน OOM kill
			MemorySwap: memoryBytes,
			PidsLimit:  &pidsLimit,
		},
	}
	if cfg.Port > 0 {
		config.ExposedPorts = nat.PortSet{mcPort: struct{}{}}
		hostConfig.PortBindings = nat.PortMap{
			mcPort: []nat.PortBinding{{HostPort: strconv.Itoa(cfg.Port)}},
		}
	}
	netConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			// alias mc-{id} ให้ velocity อ้างถึง backend ด้วยชื่อคงที่ใน network เดียวกัน
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
		return fmt.Errorf("start container: %w", err)
	}
	log.Printf("container started: server=%s image=%s memory_mb=%d host_port=%d", cfg.ID, cfg.Image, cfg.MemoryMB, cfg.Port)
	return nil
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

// Cleanup ลบ container mc-{id} ทิ้งโดยไม่แตะ directory (data ยังอยู่)
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
	return stats, nil
}

// stopCommand อ่าน stop word จาก meta.json ที่ provision เขียนไว้
// ("end" สำหรับ velocity, "stop" สำหรับที่เหลือ) — อ่านไม่ได้ให้ใช้ "stop"
func (r *DockerRunner) stopCommand(id string) string {
	b, err := os.ReadFile(filepath.Join(r.dataDir, id, ".mcpanel", "meta.json"))
	if err != nil {
		return "stop"
	}
	var meta struct {
		StopCommand string `json:"stop_command"`
	}
	if json.Unmarshal(b, &meta) != nil || meta.StopCommand == "" {
		return "stop"
	}
	return meta.StopCommand
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
