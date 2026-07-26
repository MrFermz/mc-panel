// forge.go — forge ต่างจาก variant อื่นตรงที่ไม่มี jar สำเร็จรูปให้โหลด ต้องรัน installer
// ของ forge เองใน one-off container ก่อน (agent ไม่มี java และต้องการ isolation เท่า MC container)
package minecraft

import (
	"bytes"
	"context"
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
	forgePromotionsURL = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"
	forgeMavenBase     = "https://maven.minecraftforge.net/net/minecraftforge/forge"

	// ตั้งชื่อไม่ให้ match glob forge-*.jar ใน launch.sh — ถ้าลบไม่สำเร็จจะได้ไม่ถูกรันแทน server jar
	forgeInstallerName = "installer.jar"

	// installer แตกไฟล์ library หลายร้อยไฟล์ — เครื่อง/disk ช้ากินเวลาได้หลายนาที
	forgeInstallTimeout = 15 * time.Minute
)

func provisionForge(ctx context.Context, env games.ProvisionEnv) (string, error) {
	var promos struct {
		Promos map[string]string `json:"promos"`
	}
	if err := env.FetchJSON(ctx, forgePromotionsURL, &promos); err != nil {
		return "", err
	}
	forgeBuild := promos.Promos[env.Version+"-recommended"]
	if forgeBuild == "" {
		forgeBuild = promos.Promos[env.Version+"-latest"]
	}
	if forgeBuild == "" {
		return "", fmt.Errorf("forge has no promoted build for mc version %q", env.Version)
	}
	fullVersion := env.Version + "-" + forgeBuild
	detail := "forge " + fullVersion

	if forgeInstalled(env.Dir) {
		// redeliver หลัง install สำเร็จแล้ว — ห้ามรัน installer ซ้ำ
		return detail + " (already installed)", nil
	}

	installerURL := fmt.Sprintf("%s/%s/forge-%s-installer.jar", forgeMavenBase, fullVersion, fullVersion)
	installerPath := filepath.Join(env.Dir, forgeInstallerName)
	// maven ของ forge ไม่แจก checksum ผ่าน API — โหลดจาก official host ตรง ๆ
	if err := env.Download(ctx, installerURL, installerPath, "", ""); err != nil {
		return "", err
	}
	// installer รันเป็น uid 1000 ต้องเขียน dir ได้
	env.Chown()

	image := runtimeImage(env.RuntimeImagePrefix, env.Variant, env.Version)
	if err := runForgeInstaller(ctx, env, image); err != nil {
		return "", err
	}

	os.Remove(installerPath)
	os.Remove(installerPath + ".log")

	if !forgeInstalled(env.Dir) {
		return "", fmt.Errorf("forge installer finished but produced neither run.sh nor forge-*.jar in %s", env.Dir)
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
// (GM_DATA_DIR ถูก mount ด้วย path เดียวกันทั้งสองฝั่งตาม docker-compose)
func runForgeInstaller(ctx context.Context, env games.ProvisionEnv, image string) error {
	// installer ใช้ runtime image ตัวเดียวกับที่จะรัน server จริง — ensure ไว้ก่อน
	// (reuse cache ถ้ามี, ไม่มีก็ pull+cache) เพื่อไม่ต้อง make runtime-images ล่วงหน้า
	if err := games.EnsureRuntimeImage(ctx, env.Docker, image); err != nil {
		return err
	}

	name := "mc-provision-" + env.ServerID
	// container ค้างจากรอบก่อนที่ crash — ลบทิ้งก่อน (idempotent)
	if err := env.Docker.ContainerRemove(ctx, name, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("remove stale provision container: %w", err)
	}

	config := &container.Config{
		Image:      image,
		User:       "1000:1000",
		WorkingDir: games.ContainerDataDir,
		Cmd:        []string{"java", "-jar", forgeInstallerName, "--installServer"},
		// ไม่ติด gamemanager.managed_by — events watcher จะได้ไม่รายงาน container นี้เป็น server
		Labels: map[string]string{"project": "game-manager"},
	}
	hostConfig := &container.HostConfig{
		Binds: []string{env.Dir + ":" + games.ContainerDataDir},
		// forge installer ยุคใหม่ต้องโหลด vanilla server jar + libraries จาก maven ตอน
		// --installServer จึงต้องมี egress — ใช้ default bridge (NAT ออก internet ได้)
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
	log.Printf("forge installer running: server=%s image=%s", env.ServerID, image)

	wctx, cancel := context.WithTimeout(ctx, forgeInstallTimeout)
	defer cancel()
	waitCh, errCh := env.Docker.ContainerWait(wctx, name, container.WaitConditionNotRunning)
	select {
	case res := <-waitCh:
		if res.StatusCode != 0 {
			return fmt.Errorf("forge installer exited with code %d: %s", res.StatusCode, containerLogTail(env, name))
		}
	case err := <-errCh:
		return fmt.Errorf("wait for forge installer: %w", err)
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
