package provision

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/mc-panel/node-agent/internal/runner"
)

const (
	forgePromotionsURL = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"
	forgeMavenBase     = "https://maven.minecraftforge.net/net/minecraftforge/forge"

	// ตั้งชื่อไม่ให้ match glob forge-*.jar ใน launch.sh — ถ้าลบไม่สำเร็จจะได้ไม่ถูกรันแทน server jar
	forgeInstallerName = "installer.jar"

	// installer แตกไฟล์ library หลายร้อยไฟล์ — เครื่อง/disk ช้ากินเวลาได้หลายนาที
	forgeInstallTimeout = 15 * time.Minute
)

func (p *Provisioner) provisionForge(ctx context.Context, dir, serverID, mcVersion string) (string, error) {
	var promos struct {
		Promos map[string]string `json:"promos"`
	}
	if err := p.fetchJSON(ctx, forgePromotionsURL, &promos); err != nil {
		return "", err
	}
	forgeBuild := promos.Promos[mcVersion+"-recommended"]
	if forgeBuild == "" {
		forgeBuild = promos.Promos[mcVersion+"-latest"]
	}
	if forgeBuild == "" {
		return "", fmt.Errorf("forge has no promoted build for mc version %q", mcVersion)
	}
	fullVersion := mcVersion + "-" + forgeBuild
	detail := "forge " + fullVersion

	if forgeInstalled(dir) {
		// redeliver หลัง install สำเร็จแล้ว — ห้ามรัน installer ซ้ำ
		return detail + " (already installed)", nil
	}

	installerURL := fmt.Sprintf("%s/%s/forge-%s-installer.jar", forgeMavenBase, fullVersion, fullVersion)
	installerPath := filepath.Join(dir, forgeInstallerName)
	// maven ของ forge ไม่แจก checksum ผ่าน API — โหลดจาก official host ตรง ๆ
	if err := p.downloadFile(ctx, installerURL, installerPath, "", ""); err != nil {
		return "", err
	}
	p.chownRecursive(dir)

	image := p.runtimeImagePrefix + ":" + javaTagForMC(mcVersion)
	if err := p.runForgeInstaller(ctx, serverID, image); err != nil {
		return "", err
	}

	os.Remove(installerPath)
	os.Remove(installerPath + ".log")

	if !forgeInstalled(dir) {
		return "", fmt.Errorf("forge installer finished but produced neither run.sh nor forge-*.jar in %s", dir)
	}
	return detail, nil
}

func forgeInstalled(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "run.sh")); err == nil {
		return true
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "forge-*.jar"))
	return len(matches) > 0
}

// runForgeInstaller รัน installer ใน one-off container (image mc-runtime ตัวเดียว
// กับที่จะใช้รันจริง) — agent เองไม่มี java และต้องการ isolation เท่า MC container
//
// bind mount ต้องใช้ path แบบไม่ resolve symlink เพราะ docker daemon มองจากฝั่ง host
// (MC_DATA_DIR ถูก mount ด้วย path เดียวกันทั้งสองฝั่งตาม docker-compose)
func (p *Provisioner) runForgeInstaller(ctx context.Context, serverID, image string) error {
	bindDir := filepath.Join(p.dataDir, serverID)
	// installer ใช้ runtime image ตัวเดียวกับที่จะรัน server จริง — ensure ไว้ก่อน
	// (reuse cache ถ้ามี, ไม่มีก็ pull+cache) เพื่อไม่ต้อง make runtime-images ล่วงหน้า
	if err := runner.EnsureRuntimeImage(ctx, p.docker, image); err != nil {
		return err
	}

	name := "mc-provision-" + serverID
	// container ค้างจากรอบก่อนที่ crash — ลบทิ้งก่อน (idempotent)
	if err := p.docker.ContainerRemove(ctx, name, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("remove stale provision container: %w", err)
	}

	config := &container.Config{
		Image:      image,
		User:       "1000:1000",
		WorkingDir: "/mc",
		Cmd:        []string{"java", "-jar", forgeInstallerName, "--installServer"},
		// ไม่ติด mc.managed_by — events watcher จะได้ไม่รายงาน container นี้เป็น server
		Labels: map[string]string{"project": "mc-panel"},
	}
	hostConfig := &container.HostConfig{
		Binds: []string{bindDir + ":/mc"},
		// forge installer ยุคใหม่ต้องโหลด vanilla server jar + libraries จาก maven ตอน
		// --installServer จึงต้องมี egress — ใช้ default bridge (NAT ออก internet ได้)
		// isolation อื่นคงเดิม: user 1000, bind เฉพาะ dir ของ server นี้, cap-drop ALL, no-new-privileges
		NetworkMode: "bridge",
		CapDrop:     []string{"ALL"},
		SecurityOpt: []string{"no-new-privileges"},
	}
	if _, err := p.docker.ContainerCreate(ctx, config, hostConfig, nil, nil, name); err != nil {
		return fmt.Errorf("create provision container: %w", err)
	}
	defer func() {
		if err := p.docker.ContainerRemove(context.Background(), name, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
			log.Printf("remove provision container failed: %v", err)
		}
	}()

	if err := p.docker.ContainerStart(ctx, name, container.StartOptions{}); err != nil {
		return fmt.Errorf("start provision container: %w", err)
	}
	log.Printf("forge installer running: server=%s image=%s", serverID, image)

	wctx, cancel := context.WithTimeout(ctx, forgeInstallTimeout)
	defer cancel()
	waitCh, errCh := p.docker.ContainerWait(wctx, name, container.WaitConditionNotRunning)
	select {
	case res := <-waitCh:
		if res.StatusCode != 0 {
			return fmt.Errorf("forge installer exited with code %d: %s", res.StatusCode, p.containerLogTail(name))
		}
	case err := <-errCh:
		return fmt.Errorf("wait for forge installer: %w", err)
	}
	return nil
}

func (p *Provisioner) containerLogTail(name string) string {
	rc, err := p.docker.ContainerLogs(context.Background(), name, container.LogsOptions{
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

// latestJavaTag = Java ใหม่สุดที่มี runtime image (ต้องตรงกับ control-plane image.go)
const latestJavaTag = "25"

// javaTagForMC — mapping เดียวกับ control-plane (jobs/image.go DockerImage):
// MC <= 1.16.5 → java 8, 1.17–1.20.4 → java 17, 1.20.5–1.21.x → java 21,
// calendar version (26.x…) และ parse ไม่ได้ → java ใหม่สุด (Java backward-compatible)
func javaTagForMC(mcVersion string) string {
	parts := strings.Split(strings.TrimSpace(mcVersion), ".")
	if len(parts) < 2 {
		return latestJavaTag
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return latestJavaTag
	}
	// major != 1 = calendar versioning ตั้งแต่ 2025 — ต้องการ Java ใหม่สุด
	if major != 1 {
		return latestJavaTag
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return latestJavaTag
	}
	patch := 0
	if len(parts) >= 3 {
		// patch แบบมี suffix (เช่น pre-release) ให้ parse เท่าที่ parse ได้
		if n, err := strconv.Atoi(strings.TrimFunc(parts[2], func(r rune) bool { return r < '0' || r > '9' })); err == nil {
			patch = n
		}
	}
	switch {
	case minor <= 16:
		return "8"
	case minor < 20:
		return "17"
	case minor == 20 && patch <= 4:
		return "17"
	default:
		return "21"
	}
}
