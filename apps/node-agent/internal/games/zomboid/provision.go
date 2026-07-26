// provision.go — ติดตั้ง artifact ของเกมผ่าน **SteamCMD** (ที่มาแบบ official ของเกมนี้)
//
// ต่างจาก Minecraft ที่โหลด jar ผ่าน HTTP ตรง ๆ: Steam จ่ายไฟล์ผ่าน protocol ของตัวเอง
// จึงต้องรัน steamcmd ใน one-off container (agent ไม่มี steamcmd และต้องการ isolation
// เท่า container ของ instance) โดย +force_install_dir ชี้เข้า dir ของ server ตัวนั้น
//
// app 380870 = "Project Zomboid Dedicated Server" ซึ่ง Steam เปิดให้ **login anonymous**
// (คนละ app กับตัวเกม 108600 ที่ต้องเป็นเจ้าของ) — ห้ามใส่ credential ของใครลงไปเด็ดขาด
package zomboid

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/game-manager/node-agent/internal/games"
)

const (
	// steamAppID = Project Zomboid Dedicated Server (โหลดแบบ anonymous ได้)
	steamAppID = "380870"
	// steamCmdPath = ที่อยู่ของ steamcmd ใน runtime image ของเรา (ดู runtime.Dockerfile)
	steamCmdPath = "/steamcmd/steamcmd.sh"

	// launcherName = ไฟล์ที่ Steam ติดตั้งมาให้และ launch script เรียกใช้
	launcherName = "start-server.sh"

	// steamInstallTimeout — app นี้ใหญ่หลาย GB. เพดานนี้ต้องไม่เกิน reapThreshold ของ
	// control-plane (30 นาที) ไม่งั้น job ถูก reap เป็น failed ทั้งที่ agent ยังโหลดอยู่
	steamInstallTimeout = 29 * time.Minute
)

// branches = Steam branch ที่ยอมให้เลือกเป็น `game_version` — ต้องเป็น allow-list เสมอ
// เพราะค่านี้ถูกต่อเข้าไปใน argv ของ steamcmd (payload มาจาก NATS ห้ามเชื่อว่า validate มาแล้ว)
// **ต้องตรงกับรายการฝั่ง control-plane**
var branches = map[string]bool{
	"public":   true,
	"unstable": true,
}

// provision = games.Definition.Provision
func provision(ctx context.Context, env games.ProvisionEnv) (string, error) {
	if !branches[env.Version] {
		return "", fmt.Errorf("unsupported steam branch %q", env.Version)
	}

	// PZ ถาม admin password ทาง stdin ตอน start ครั้งแรกถ้าไม่ได้ส่งมาทาง argv —
	// server จะค้างรอคำตอบตลอดกาล จึงต้องมีรหัสไว้ก่อนเสมอ (launch.sh อ่านไฟล์นี้)
	if err := ensureAdminPassword(env.Dir); err != nil {
		return "", err
	}

	image := runtimeImage(env.RuntimeImageNamespace, env.Variant, env.Version)
	if err := games.EnsureRuntimeImage(ctx, env.Docker, image, imageSource(image)); err != nil {
		return "", err
	}
	// steamcmd รันเป็น uid 1000 ต้องเขียน dir ของ server ได้
	env.Chown()

	if err := runSteamCmd(ctx, env, image); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(env.Dir, launcherName)); err != nil {
		return "", fmt.Errorf("steamcmd finished but %s is missing in %s", launcherName, env.Dir)
	}

	return fmt.Sprintf("steam app %s branch=%s (admin password in %s/%s)",
		steamAppID, env.Version, games.PanelDir, adminPasswordFile), nil
}

// steamCmdArgs ประกอบคำสั่งของ steamcmd — branch ถูก validate ด้วย allow-list มาแล้ว
// (+login anonymous เท่านั้น: panel ไม่รับ credential ของ Steam ที่ไหนเลย)
func steamCmdArgs(branch string) []string {
	args := []string{
		steamCmdPath,
		"+force_install_dir", games.ContainerDataDir,
		"+login", "anonymous",
		"+app_update", steamAppID,
	}
	// public = branch ปกติ ไม่ต้องระบุ (ระบุแล้ว steamcmd บาง build บ่น)
	if branch != "public" {
		args = append(args, "-beta", branch)
	}
	return append(args, "validate", "+quit")
}

// runSteamCmd รัน steamcmd ใน one-off container ที่ bind เฉพาะ dir ของ server ตัวนี้
func runSteamCmd(ctx context.Context, env games.ProvisionEnv, image string) error {
	name := "gm-provision-" + env.ServerID
	// container ค้างจากรอบก่อนที่ crash — ลบทิ้งก่อน (idempotent)
	if err := env.Docker.ContainerRemove(ctx, name, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("remove stale provision container: %w", err)
	}

	config := &container.Config{
		Image:      image,
		User:       "1000:1000",
		WorkingDir: games.ContainerDataDir,
		Cmd:        steamCmdArgs(env.Version),
		// HOME ต้องเขียนได้: steamcmd เก็บ state ของตัวเอง (~/.steam, ~/Steam) ไว้ที่นั่น
		Env: []string{"HOME=" + games.ContainerDataDir},
		// ไม่ติด gamemanager.managed_by — events watcher จะได้ไม่รายงาน container นี้เป็น server
		Labels: map[string]string{"project": "game-manager"},
	}
	hostConfig := &container.HostConfig{
		Binds: []string{env.Dir + ":" + games.ContainerDataDir},
		// steamcmd ต้องคุยกับ Steam CDN — ใช้ default bridge (NAT ออก internet ได้)
		// isolation อื่นคงเดิม: user 1000, bind เฉพาะ dir ของ server นี้, cap-drop ALL, no-new-privileges
		NetworkMode: "bridge",
		CapDrop:     []string{"ALL"},
		SecurityOpt: []string{"no-new-privileges"},
	}
	if _, err := env.Docker.ContainerCreate(ctx, config, hostConfig, nil, nil, name); err != nil {
		return fmt.Errorf("create provision container: %w", err)
	}
	defer func() {
		if err := env.Docker.ContainerRemove(context.Background(), name, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
			log.Printf("remove provision container failed: %v", err)
		}
	}()

	if err := env.Docker.ContainerStart(ctx, name, container.StartOptions{}); err != nil {
		return fmt.Errorf("start provision container: %w", err)
	}
	log.Printf("steamcmd running: server=%s app=%s branch=%s image=%s",
		env.ServerID, steamAppID, env.Version, image)

	wctx, cancel := context.WithTimeout(ctx, steamInstallTimeout)
	defer cancel()
	waitCh, errCh := env.Docker.ContainerWait(wctx, name, container.WaitConditionNotRunning)
	select {
	case res := <-waitCh:
		if res.StatusCode != 0 {
			return fmt.Errorf("steamcmd exited with code %d: %s", res.StatusCode, containerLogTail(env, name))
		}
	case err := <-errCh:
		return fmt.Errorf("wait for steamcmd: %w", err)
	}
	return nil
}

// ensureAdminPassword สร้างรหัส admin ของเกมครั้งแรกครั้งเดียว แล้ววางไว้ใน PanelDir
// ให้ launch script อ่าน — idempotent: มีอยู่แล้วใช้ของเดิม (ไม่งั้น provision ซ้ำจะเปลี่ยน
// รหัสใต้มือ user)
func ensureAdminPassword(dir string) error {
	panelDir := filepath.Join(dir, games.PanelDir)
	if err := os.MkdirAll(panelDir, 0o755); err != nil {
		return fmt.Errorf("create %s directory: %w", games.PanelDir, err)
	}
	path := filepath.Join(panelDir, adminPasswordFile)
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("generate admin password: %w", err)
	}
	// base64 ของ 15 byte = 20 ตัวอักษรพอดี ไม่มี padding และไม่มีอักขระที่ทำให้ shell/ini เพี้ยน
	pw := base64.RawURLEncoding.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(pw+"\n"), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", adminPasswordFile, err)
	}
	return nil
}

func containerLogTail(env games.ProvisionEnv, name string) string {
	rc, err := env.Docker.ContainerLogs(context.Background(), name, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "20",
	})
	if err != nil {
		return "(logs unavailable)"
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, io.LimitReader(rc, 64*1024)); err != nil {
		return "(logs unavailable)"
	}
	return strings.TrimSpace(buf.String())
}
