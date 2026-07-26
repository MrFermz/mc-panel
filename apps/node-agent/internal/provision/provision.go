// Package provision สร้าง directory ของ instance ใหม่ แล้วให้ game definition เป็นคน
// โหลด artifact จาก official source + บอกว่าไฟล์ config/launch script ต้องเป็นอะไร
//
// package นี้ไม่รู้จักเกมใด ๆ: มันคุมเรื่อง filesystem (jail, chown, แตก zip อย่างปลอดภัย,
// เขียน .gamemanager/) ส่วน "โหลดอะไรจากไหน / รันยังไง" มาจาก internal/games ทั้งหมด
//
// ทุกขั้นตอนต้อง idempotent — job โดน redeliver ซ้ำได้เสมอ ขั้นที่เสร็จแล้วให้ข้าม
package provision

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
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

	// maxImportUncompressed กัน zip-bomb — รวมขนาดหลัง decompress ของทุก entry ต้องไม่เกินนี้
	// (เผื่อ save/mod ใหญ่ได้จริงหลาย GB แต่มีเพดานกัน decompress bomb ที่พอง disk เต็ม)
	maxImportUncompressed = 8 << 30 // 8 GiB
)

// Spec = input ของ CreateServer (Game ว่าง = เกม default — job เก่าที่ค้างใน stream)
type Spec struct {
	Game          string
	Variant       string
	GameVersion   string
	AcceptLicense bool
}

// ImportSpec คือ input ของ ImportServer — zip ถูก stage ไว้แล้วที่ ArchivePath (relative ต่อ jail)
type ImportSpec struct {
	Game          string
	Variant       string
	GameVersion   string
	AcceptLicense bool
	ArchivePath   string
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
// meta.json/launch.sh ถูกเขียนทับเสมอ (WriteFile truncate) — ตอน import จึงมั่นใจได้ว่า
// panel คุม launch ไม่ว่า zip จะมี .gamemanager เดิมติดมาหรือไม่
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

// ImportServer แตก zip ที่ถูก stage ไว้ใน jail ของ server แล้ว provision โดยไม่โหลด artifact
// (artifact/save/config มาจาก zip ที่ user อัปโหลด) — ทุก path ที่แตะ filesystem ผ่าน SafeJoin,
// ไม่ materialize symlink, มี size cap กัน disk-fill/zip-bomb
func (p *Provisioner) ImportServer(ctx context.Context, serverID string, spec ImportSpec) (detectedVersion string, err error) {
	def, err := p.resolveGame(spec.Game, spec.Variant)
	if err != nil {
		return "", err
	}

	dir, err := p.serverDir(serverID)
	if err != nil {
		return "", err
	}

	// staged zip ถูกเขียนเข้ามาใน jail แล้วผ่าน chunked write — path มาจากภายนอก ต้อง SafeJoin
	archivePath, err := filemanager.SafeJoin(dir, spec.ArchivePath)
	if err != nil {
		return "", fmt.Errorf("archive path validation failed: %w", err)
	}
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("open import archive: %w", err)
	}

	var (
		fileCount int
		totalIn   int64
	)
	extractErr := func() error {
		var written int64
		for _, f := range zr.File {
			// zip-slip guard — ชื่อ entry มาจาก zip ที่ไม่เชื่อถือ ต้องอยู่ใต้ jail เท่านั้น
			target, err := filemanager.SafeJoin(dir, f.Name)
			if err != nil {
				return fmt.Errorf("unsafe entry %q: %w", f.Name, err)
			}
			// ข้าม staged zip เอง เผื่อมันโผล่อยู่ใน archive (จะได้ไม่ทับ/วนลูป)
			if target == archivePath {
				continue
			}
			// ปฏิเสธ symlink — ถ้า materialize ไว้ operation ทีหลังอาจ escape jail ผ่านมัน
			if f.Mode()&os.ModeSymlink != 0 {
				log.Printf("import: skipping symlink entry: server=%s name=%s", serverID, f.Name)
				continue
			}
			if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return err
				}
				chownBestEffort(target)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			written, err = extractFile(f, target, written)
			if err != nil {
				return err
			}
			fileCount++
			totalIn += int64(f.UncompressedSize64)
		}
		return nil
	}()
	if cerr := zr.Close(); cerr != nil && extractErr == nil {
		extractErr = cerr
	}
	if extractErr != nil {
		return "", fmt.Errorf("extract import archive: %w", extractErr)
	}

	// เอา staged zip ออกหลังแตกเสร็จ — ไม่ให้ค้างเปลือง disk / โผล่ใน file manager
	if err := os.Remove(archivePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("remove staged archive: %w", err)
	}

	// launch script รัน artifact ชื่อตายตัวตาม definition — zip ที่ user อัปโหลดมักตั้งชื่ออื่น
	// (เช่น paper-1.21.1.jar) ต้อง normalize ชื่อตาม Import.MainArtifact ไม่งั้น start crash
	// originalName = ชื่อไฟล์เดิมของ artifact หลัก (ใช้ fallback เดา version)
	renamedTo, originalName := normalizeMainArtifact(dir, def, spec.Variant, serverID)

	// เดาเวอร์ชัน best-effort เพื่อ pre-fill panel — สุดท้าย fallback ค่าที่ user กรอก
	artifactPath := ""
	if renamedTo != "" {
		artifactPath = filepath.Join(dir, renamedTo)
	}
	detectedVersion = def.Import.DetectVersion(artifactPath, originalName)
	if detectedVersion == "" {
		detectedVersion = spec.GameVersion
	}

	// meta.json ต้องสะท้อน version จริงที่ detect ได้ (ไม่ใช่ค่าที่ user เดา)
	if err := p.writePanelFiles(dir, def, spec.Variant, detectedVersion, spec.AcceptLicense); err != nil {
		return "", err
	}

	p.chownRecursive(dir)
	log.Printf("server imported: server=%s game=%s type=%s version=%s files=%d bytes=%d",
		serverID, def.ID, spec.Variant, detectedVersion, fileCount, totalIn)
	return detectedVersion, nil
}

// normalizeMainArtifact เปลี่ยนชื่อ artifact หลักที่ root ให้ตรงกับที่ launch script คาดหวัง
// variant ที่ definition บอกว่าไม่มี main artifact (เช่น variant ที่ใช้ run script) จะถูกข้าม
// คืน (ชื่อไฟล์ target ที่ใช้จริง, ชื่อไฟล์เดิม) เพื่อไปเดา version ต่อ
func normalizeMainArtifact(dir string, def *games.Definition, variant, serverID string) (target, originalName string) {
	target = def.Import.MainArtifact(variant)
	if target == "" {
		return "", ""
	}

	// มี target อยู่แล้ว = zip ตั้งชื่อถูกมาแต่แรก, ไม่ต้อง rename แต่ยังคืนชื่อไว้เดา version
	if _, err := os.Stat(filepath.Join(dir, target)); err == nil {
		return target, target
	}

	candidates := rootFilesWithExt(dir, def.Import.Ext)
	if len(candidates) == 0 {
		// อาจเป็น setup ที่ไม่มี artifact ที่ root — ไม่ fail ที่นี่
		// ปล่อยให้ start เป็นคน surface error จริงทีหลัง
		log.Printf("import: no root %s to rename to %s: server=%s", def.Import.Ext, target, serverID)
		return "", ""
	}

	pick := pickMainArtifact(dir, candidates, def.Import.NameHints)
	if err := os.Rename(filepath.Join(dir, pick), filepath.Join(dir, target)); err != nil {
		log.Printf("import: rename %s to %s failed: server=%s err=%v", pick, target, serverID, err)
		return "", pick
	}
	log.Printf("import: renamed main artifact %s to %s: server=%s", pick, target, serverID)
	return target, pick
}

// rootFilesWithExt คืนชื่อไฟล์นามสกุลที่กำหนดที่ root ของ server dir (non-recursive)
func rootFilesWithExt(dir, ext string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ext) {
			out = append(out, e.Name())
		}
	}
	return out
}

// pickMainArtifact เดาไฟล์หลักจากหลายไฟล์: ไฟล์เดียวเลือกเลย, หลายไฟล์เลือกจากชื่อที่คุ้น
// (hint ของเกม) ไม่มีเลยเลือกไฟล์ใหญ่สุด (artifact หลักมักใหญ่กว่า plugin/lib)
func pickMainArtifact(dir string, candidates, hints []string) string {
	if len(candidates) == 1 {
		return candidates[0]
	}
	for _, hint := range hints {
		for _, c := range candidates {
			if strings.Contains(strings.ToLower(c), hint) {
				return c
			}
		}
	}
	largest, largestSize := candidates[0], int64(-1)
	for _, c := range candidates {
		if fi, err := os.Stat(filepath.Join(dir, c)); err == nil && fi.Size() > largestSize {
			largest, largestSize = c, fi.Size()
		}
	}
	return largest
}

// extractFile แตก 1 regular entry ไปที่ target โดยคุมขนาดสะสม (written) กัน zip-bomb
// คืน written ที่อัปเดตแล้ว — เกิน maxImportUncompressed เมื่อไรถือว่า fail ทั้ง import
func extractFile(f *zip.File, target string, written int64) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return written, err
	}
	defer rc.Close()

	dst, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return written, err
	}

	// อ่านได้ถึงเพดานที่เหลือ +1 ไบต์ เพื่อจับกรณีเกิน limit จริง (ไม่พึ่ง UncompressedSize64
	// ที่ header อาจโกหก) — LimitReader ตัดที่ remaining+1 แล้วเช็คว่าเขียนเกินหรือไม่
	remaining := int64(maxImportUncompressed) - written
	if remaining < 0 {
		remaining = 0
	}
	n, copyErr := io.Copy(dst, io.LimitReader(rc, remaining+1))
	if closeErr := dst.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return written, copyErr
	}
	written += n
	if written > int64(maxImportUncompressed) {
		return written, errors.New("import archive too large (uncompressed size cap exceeded)")
	}
	chownBestEffort(target)
	return written, nil
}

// chownBestEffort โอน ownership ของ 1 path ให้ uid 1000 — fail ได้บน dev host ที่ไม่ใช่ root
func chownBestEffort(path string) {
	if err := os.Lchown(path, mcUID, mcGID); err != nil {
		log.Printf("chown %s to %d:%d failed: %v (continuing)", path, mcUID, mcGID, err)
	}
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
