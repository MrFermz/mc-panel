// Package provision สร้าง directory ของ instance ใหม่ แล้วให้ game definition เป็นคน
// โหลด artifact จาก official source + บอกว่าไฟล์ config/launch script ต้องเป็นอะไร
//
// package นี้ไม่รู้จักเกมใด ๆ: มันคุมเรื่อง filesystem (jail, chown, เขียน .gamemanager/)
// ส่วน "โหลดอะไรจากไหน / รันยังไง" มาจาก internal/games ทั้งหมด
//
// ทุกขั้นตอนต้อง idempotent — job โดน redeliver ซ้ำได้เสมอ ขั้นที่เสร็จแล้วให้ข้าม
package provision

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/client"

	"github.com/game-manager/node-agent/internal/filemanager"
	"github.com/game-manager/node-agent/internal/games"
)

const (
	// uid/gid ที่ container ของ instance รัน (User: 1000:1000) — ไฟล์ทั้งหมดต้องเป็นของ user นี้
	mcUID = 1000
	mcGID = 1000
)

// Spec = input ของ CreateServer (Game ว่าง = เกม default — job เก่าที่ค้างใน stream)
type Spec struct {
	Game          string
	Variant       string
	GameVersion   string
	AcceptLicense bool
}

type Provisioner struct {
	docker             *client.Client
	dataDir            string
	runtimeImagePrefix string
	games              *games.Registry
	http               *http.Client
}

func New(docker *client.Client, dataDir, runtimeImagePrefix string, gr *games.Registry) *Provisioner {
	return &Provisioner{
		docker:             docker,
		dataDir:            dataDir,
		runtimeImagePrefix: runtimeImagePrefix,
		games:              gr,
		http: &http.Client{
			// artifact/installer ใหญ่ได้หลายร้อย MB บน connection ช้า — timeout รวมต้องยาว
			// แต่ connect/header ต้องสั้นเพื่อ fail เร็วเมื่อ upstream ล่ม
			Timeout: 10 * time.Minute,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
	}
}

// serverDir validate server id แล้วคืน path จริงใต้ GM_DATA_DIR
// id มาจาก NATS message — ห้ามเชื่อว่าเป็น UUID เสมอ ต้องผ่าน SafeJoin ก่อนแตะ filesystem
func (p *Provisioner) serverDir(serverID string) (string, error) {
	if serverID == "" || strings.ContainsAny(serverID, "/\\") || serverID == "." || serverID == ".." {
		return "", fmt.Errorf("invalid server id %q", serverID)
	}
	dir, err := filemanager.SafeJoin(p.dataDir, serverID)
	if err != nil {
		return "", fmt.Errorf("server path validation failed: %w", err)
	}
	return dir, nil
}

// resolveGame หา definition + เช็คว่า variant ที่ขอมามีจริงในเกมนั้น
// (payload มาจาก NATS — ห้ามเชื่อว่าตรงกับที่ control-plane validate ไว้แล้ว)
func (p *Provisioner) resolveGame(game, variant string) (*games.Definition, error) {
	def, ok := p.games.Resolve(game)
	if !ok {
		return nil, fmt.Errorf("unsupported game %q", game)
	}
	if !def.HasVariant(variant) {
		return nil, fmt.Errorf("unsupported variant %q for game %q", variant, def.ID)
	}
	return def, nil
}

func (p *Provisioner) CreateServer(ctx context.Context, serverID string, spec Spec) (string, error) {
	def, err := p.resolveGame(spec.Game, spec.Variant)
	if err != nil {
		return "", err
	}

	dir, err := p.serverDir(serverID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create server directory: %w", err)
	}
	// tool ของเกมที่รันเป็น uid 1000 (installer ของบาง variant) ต้องเขียน dir ได้ — chown ก่อนเริ่มโหลด
	p.chownRecursive(dir)

	detail, err := def.Provision(ctx, p.provisionEnv(def, serverID, dir, spec.Variant, spec.GameVersion))
	if err != nil {
		return "", err
	}

	if err := p.writePanelFiles(dir, def, spec.Variant, spec.GameVersion, spec.AcceptLicense); err != nil {
		return "", err
	}

	p.chownRecursive(dir)
	log.Printf("server provisioned: server=%s game=%s type=%s version=%s",
		serverID, def.ID, spec.Variant, spec.GameVersion)
	return detail, nil
}

func (p *Provisioner) provisionEnv(def *games.Definition, serverID, dir, variant, version string) games.ProvisionEnv {
	return games.ProvisionEnv{
		ServerID:           serverID,
		Dir:                dir,
		Variant:            variant,
		Version:            version,
		HTTP:               p.http,
		Docker:             p.docker,
		RuntimeImagePrefix: p.runtimeImagePrefix,
		Chown:              func() { p.chownRecursive(dir) },
	}
}

// writePanelFiles เขียนไฟล์ที่ panel เป็นเจ้าของ: seed config ของเกม (license/config เริ่มต้น)
// + .gamemanager/meta.json + .gamemanager/launch.sh
// meta.json/launch.sh ถูกเขียนทับเสมอ (WriteFile truncate) — panel คุม launch เสมอ
// ไม่ว่าจะมี .gamemanager เดิมค้างอยู่ใน dir หรือไม่
func (p *Provisioner) writePanelFiles(dir string, def *games.Definition, variant, version string, acceptLicense bool) error {
	// seed file ของเกม — path มาจาก definition (ค่าคงที่ในโค้ด) ยังผ่าน SafeJoin ไว้อีกชั้น
	// เพื่อไม่ให้ definition ที่เขียนพลาดหลุดออกนอก jail ได้เลย
	for _, f := range def.SeedFiles(variant, acceptLicense) {
		target, err := filemanager.SafeJoin(dir, f.Path)
		if err != nil {
			return fmt.Errorf("seed file %q: %w", f.Path, err)
		}
		if !f.Overwrite {
			if _, err := os.Stat(target); err == nil {
				continue
			} else if !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("stat seed file %q: %w", f.Path, err)
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create dir for seed file %q: %w", f.Path, err)
		}
		if err := os.WriteFile(target, f.Content, f.Mode); err != nil {
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
	}

	panelDir := filepath.Join(dir, games.PanelDir)
	if err := os.MkdirAll(panelDir, 0o755); err != nil {
		return fmt.Errorf("create %s directory: %w", games.PanelDir, err)
	}

	if err := games.WriteInstanceMeta(dir, games.InstanceMeta{
		Game:        def.ID,
		Variant:     variant,
		GameVersion: version,
		StopCommand: def.StopCommand(variant),
	}); err != nil {
		return fmt.Errorf("write %s: %w", games.MetaFileName, err)
	}
	if err := os.WriteFile(filepath.Join(panelDir, games.LaunchScriptName), []byte(def.LaunchScript(variant)), 0o755); err != nil {
		return fmt.Errorf("write %s: %w", games.LaunchScriptName, err)
	}
	return nil
}

// DeleteServer ลบ directory ทั้งหมดของ server — ผู้เรียกต้อง stop/remove container ก่อน
func (p *Provisioner) DeleteServer(serverID string) error {
	dir, err := p.serverDir(serverID)
	if err != nil {
		return err
	}
	// กันพลาดชั้นสุดท้าย: ต้องไม่ใช่ตัว data dir เอง (SafeJoin คืน path ที่ resolve แล้ว)
	if resolved, err := filepath.EvalSymlinks(p.dataDir); err == nil && dir == resolved {
		return errors.New("refusing to delete data dir root")
	}
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove server directory: %w", err)
	}
	log.Printf("server directory removed: server=%s", serverID)
	return nil
}

// chownRecursive โอนทุกไฟล์ให้ uid 1000 — fail ได้บน dev host ที่ไม่ใช่ root (เช่น mac)
// ซึ่งไม่เป็นไรเพราะ Docker Desktop จัดการ ownership ของ bind mount เอง
func (p *Provisioner) chownRecursive(root string) {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, mcUID, mcGID)
	})
	if err != nil {
		log.Printf("chown %s to %d:%d failed: %v (continuing)", root, mcUID, mcGID, err)
	}
}
