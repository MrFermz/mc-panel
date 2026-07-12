// Package provision สร้าง directory + ดาวน์โหลด server jar จาก official source
// + เขียน config/launch script ของ instance ใหม่ (ยังไม่ start)
//
// ทุกขั้นตอนต้อง idempotent — job โดน redeliver ซ้ำได้เสมอ ขั้นที่เสร็จแล้วให้ข้าม
package provision

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/mc-panel/node-agent/internal/filemanager"
)

const (
	// uid/gid ที่ MC container รัน (User: 1000:1000) — ไฟล์ทั้งหมดต้องเป็นของ user นี้
	mcUID = 1000
	mcGID = 1000

	userAgent = "mc-panel-agent/0.1.0 (https://github.com/mc-panel)"

	// maxImportUncompressed กัน zip-bomb — รวมขนาดหลัง decompress ของทุก entry ต้องไม่เกินนี้
	// (เผื่อ world/mod ใหญ่ได้จริงหลาย GB แต่มีเพดานกัน decompress bomb ที่พอง disk เต็ม)
	maxImportUncompressed = 8 << 30 // 8 GiB
)

type Spec struct {
	ServerType string
	MCVersion  string
	AcceptEULA bool
}

// ImportSpec คือ input ของ ImportServer — zip ถูก stage ไว้แล้วที่ ArchivePath (relative ต่อ jail)
type ImportSpec struct {
	ServerType  string
	MCVersion   string
	AcceptEULA  bool
	ArchivePath string
}

type Provisioner struct {
	docker             *client.Client
	dataDir            string
	runtimeImagePrefix string
	http               *http.Client
}

func New(docker *client.Client, dataDir, runtimeImagePrefix string) *Provisioner {
	return &Provisioner{
		docker:             docker,
		dataDir:            dataDir,
		runtimeImagePrefix: runtimeImagePrefix,
		http: &http.Client{
			// jar/installer ใหญ่ได้หลายร้อย MB บน connection ช้า — timeout รวมต้องยาว
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

// serverDir validate server id แล้วคืน path จริงใต้ MC_DATA_DIR
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

func (p *Provisioner) CreateServer(ctx context.Context, serverID string, spec Spec) (string, error) {
	dir, err := p.serverDir(serverID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create server directory: %w", err)
	}
	// forge installer รันเป็น uid 1000 ต้องเขียน dir ได้ — chown ก่อนเริ่มโหลด
	p.chownRecursive(dir)

	var detail string
	switch spec.ServerType {
	case "vanilla":
		detail, err = p.provisionVanilla(ctx, dir, spec.MCVersion)
	case "paper":
		detail, err = p.provisionPaperProject(ctx, dir, "paper", spec.MCVersion, "server.jar")
	case "velocity":
		detail, err = p.provisionPaperProject(ctx, dir, "velocity", spec.MCVersion, "velocity.jar")
	case "fabric":
		detail, err = p.provisionFabric(ctx, dir, spec.MCVersion)
	case "forge":
		detail, err = p.provisionForge(ctx, dir, serverID, spec.MCVersion)
	default:
		return "", fmt.Errorf("unsupported server_type %q", spec.ServerType)
	}
	if err != nil {
		return "", err
	}

	if err := writePanelFiles(dir, spec.ServerType, spec.MCVersion, spec.AcceptEULA); err != nil {
		return "", err
	}

	p.chownRecursive(dir)
	log.Printf("server provisioned: server=%s type=%s version=%s", serverID, spec.ServerType, spec.MCVersion)
	return detail, nil
}

// writePanelFiles เขียน config ที่ panel เป็นเจ้าของ: eula.txt (ถ้า user ยอมรับ),
// server.properties (ถ้ายังไม่มี, non-velocity), .mcpanel/meta.json + launch.sh
// meta.json/launch.sh ถูกเขียนทับเสมอ (WriteFile truncate) — ตอน import จึงมั่นใจได้ว่า
// panel คุม launch ไม่ว่า zip จะมี .mcpanel เดิมติดมาหรือไม่
func writePanelFiles(dir, serverType, mcVersion string, acceptEULA bool) error {
	if acceptEULA {
		// เขียนเฉพาะเมื่อ user ติ๊กยอมรับเอง — ระบบห้าม default eula ให้เด็ดขาด
		if err := os.WriteFile(filepath.Join(dir, "eula.txt"), []byte("eula=true\n"), 0o644); err != nil {
			return fmt.Errorf("write eula.txt: %w", err)
		}
	}

	if serverType != "velocity" {
		propsPath := filepath.Join(dir, "server.properties")
		if _, err := os.Stat(propsPath); errors.Is(err, fs.ErrNotExist) {
			// port ใน container ตายตัว 25565 เสมอ — host port ไปกำหนดที่ PortBindings ตอน start
			if err := os.WriteFile(propsPath, []byte("server-port=25565\n"), 0o644); err != nil {
				return fmt.Errorf("write server.properties: %w", err)
			}
		}
	}

	mcpanelDir := filepath.Join(dir, ".mcpanel")
	if err := os.MkdirAll(mcpanelDir, 0o755); err != nil {
		return fmt.Errorf("create .mcpanel directory: %w", err)
	}

	stopCommand := "stop"
	if serverType == "velocity" {
		stopCommand = "end"
	}
	meta, err := json.MarshalIndent(map[string]string{
		"server_type":  serverType,
		"mc_version":   mcVersion,
		"stop_command": stopCommand,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(mcpanelDir, "meta.json"), append(meta, '\n'), 0o644); err != nil {
		return fmt.Errorf("write meta.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(mcpanelDir, "launch.sh"), []byte(launchScript(serverType)), 0o755); err != nil {
		return fmt.Errorf("write launch.sh: %w", err)
	}
	return nil
}

// ImportServer แตก zip ที่ถูก stage ไว้ใน jail ของ server แล้ว provision โดยไม่โหลด jar
// (jar/world/config มาจาก zip ที่ user อัปโหลด) — ทุก path ที่แตะ filesystem ผ่าน SafeJoin,
// ไม่ materialize symlink, มี size cap กัน disk-fill/zip-bomb
func (p *Provisioner) ImportServer(ctx context.Context, serverID string, spec ImportSpec) (detectedVersion string, err error) {
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

	// launch script รัน `java -jar server.jar` (หรือ velocity.jar) ตายตัว — zip ที่ user
	// อัปโหลดมักมี jar ชื่ออื่น (paper-1.21.1.jar ฯลฯ) ต้อง normalize ชื่อ ไม่งั้น start crash
	// jarName = ชื่อไฟล์เดิมของ jar หลัก (ใช้ fallback เดา version จากชื่อ)
	renamedTo, jarName := p.normalizeServerJar(dir, spec.ServerType, serverID)

	// เดา mc_version best-effort เพื่อ pre-fill panel — เชื่อ version.json ใน jar ก่อน,
	// ไม่ได้ค่อย parse จากชื่อไฟล์เดิม, สุดท้าย fallback ค่าที่ user กรอก
	detectedVersion = p.detectMCVersion(dir, renamedTo, jarName)
	if detectedVersion == "" {
		detectedVersion = spec.MCVersion
	}

	// meta.json ต้องสะท้อน version จริงที่ detect ได้ (ไม่ใช่ค่าที่ user เดา)
	if err := writePanelFiles(dir, spec.ServerType, detectedVersion, spec.AcceptEULA); err != nil {
		return "", err
	}

	p.chownRecursive(dir)
	log.Printf("server imported: server=%s type=%s version=%s files=%d bytes=%d",
		serverID, spec.ServerType, detectedVersion, fileCount, totalIn)
	return detectedVersion, nil
}

// normalizeServerJar เปลี่ยนชื่อ jar หลักที่ root เป็น server.jar (หรือ velocity.jar)
// ให้ตรงกับ launch script. forge ใช้ run.sh/forge-*.jar อยู่แล้ว — ข้ามไป.
// คืน (ชื่อไฟล์ target ที่ใช้จริง, ชื่อไฟล์เดิมของ jar หลัก) เพื่อไปเดา version ต่อ
func (p *Provisioner) normalizeServerJar(dir, serverType, serverID string) (target, originalName string) {
	switch serverType {
	case "forge":
		// forge จัดการผ่าน run.sh / forge-*.jar ใน launchScript — ไม่แตะ
		return "", ""
	case "velocity":
		target = "velocity.jar"
	default:
		target = "server.jar"
	}

	// มี target อยู่แล้ว = zip ตั้งชื่อถูกมาแต่แรก, ไม่ต้อง rename แต่ยังคืนชื่อไว้เดา version
	if _, err := os.Stat(filepath.Join(dir, target)); err == nil {
		return target, target
	}

	jars := rootJars(dir)
	if len(jars) == 0 {
		// อาจเป็น setup ที่ไม่มี jar (velocity บาง config) — ไม่ fail ที่นี่
		// ปล่อยให้ start เป็นคน surface error จริงทีหลัง
		log.Printf("import: no root jar to rename to %s: server=%s", target, serverID)
		return "", ""
	}

	pick := pickMainJar(dir, jars)
	if err := os.Rename(filepath.Join(dir, pick), filepath.Join(dir, target)); err != nil {
		log.Printf("import: rename %s to %s failed: server=%s err=%v", pick, target, serverID, err)
		return "", pick
	}
	log.Printf("import: renamed main jar %s to %s: server=%s", pick, target, serverID)
	return target, pick
}

// rootJars คืนชื่อ *.jar ที่ root ของ server dir (non-recursive)
func rootJars(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var jars []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ".jar") {
			jars = append(jars, e.Name())
		}
	}
	return jars
}

// pickMainJar เดา jar หลักจากหลายไฟล์: jar เดียวเลือกเลย, หลายไฟล์เลือกจากชื่อที่คุ้น
// (paper/purpur/...) ไม่มีเลยเลือกไฟล์ใหญ่สุด (server jar มักใหญ่กว่า plugin/lib)
func pickMainJar(dir string, jars []string) string {
	if len(jars) == 1 {
		return jars[0]
	}
	knownHints := []string{"paper", "purpur", "spigot", "vanilla", "fabric-server", "minecraft_server", "craftbukkit", "server"}
	for _, hint := range knownHints {
		for _, j := range jars {
			if strings.Contains(strings.ToLower(j), hint) {
				return j
			}
		}
	}
	// fallback: ไฟล์ใหญ่สุด — server jar มักใหญ่กว่า plugin/mod jar ที่เผลอวางไว้ root
	largest, largestSize := jars[0], int64(-1)
	for _, j := range jars {
		if fi, err := os.Stat(filepath.Join(dir, j)); err == nil && fi.Size() > largestSize {
			largest, largestSize = j, fi.Size()
		}
	}
	return largest
}

var versionTokenRe = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// detectMCVersion เดา version จาก jar: version.json ใน jar ก่อน แล้ว fallback ชื่อไฟล์เดิม
func (p *Provisioner) detectMCVersion(dir, target, originalName string) string {
	if target != "" {
		if v := versionFromJarManifest(filepath.Join(dir, target)); v != "" {
			return v
		}
	}
	// fallback: token version ในชื่อไฟล์เดิม เช่น paper-1.21.1.jar
	if originalName != "" {
		if v := versionTokenRe.FindString(originalName); v != "" {
			return v
		}
	}
	return ""
}

// versionFromJarManifest เปิด jar เป็น zip อ่าน version.json ที่ root → คืน id/name
// (vanilla/paper/fabric bundle ไฟล์นี้มา) — อ่านไม่ได้/ไม่มีให้คืน ""
func versionFromJarManifest(jarPath string) string {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return ""
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "version.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return ""
		}
		// version.json เล็ก (ไม่กี่ร้อย byte) — จำกัดขนาดกันไฟล์ผิดปกติ
		b, err := io.ReadAll(io.LimitReader(rc, 1<<20))
		rc.Close()
		if err != nil {
			return ""
		}
		var vj struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal(b, &vj) != nil {
			return ""
		}
		if vj.ID != "" {
			return vj.ID
		}
		return vj.Name
	}
	return ""
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

// launchScript — java ต้องเป็น process สุดท้ายผ่าน exec เสมอ เพื่อให้เป็น PID 1
// (รับ stdin จาก docker attach และ SIGTERM ตรง ไม่ผ่าน shell)
func launchScript(serverType string) string {
	const header = "#!/bin/sh\ncd /mc\n"
	const mem = "${MC_MEMORY_MB:-1024}"
	switch serverType {
	case "velocity":
		return header + "exec java -Xms" + mem + "M -Xmx" + mem + "M -jar velocity.jar\n"
	case "forge":
		// forge ใหม่ (>=1.17) ได้ run.sh + อ่าน jvm args จาก user_jvm_args.txt
		// forge เก่าได้ jar ชื่อ forge-{mc}-{build}.jar รันตรง ๆ
		return header +
			"if [ -f run.sh ]; then\n" +
			"  echo \"-Xms" + mem + "M -Xmx" + mem + "M\" > user_jvm_args.txt\n" +
			"  exec sh run.sh nogui\n" +
			"fi\n" +
			"exec java -Xms" + mem + "M -Xmx" + mem + "M -jar forge-*.jar nogui\n"
	default: // vanilla / paper / fabric
		return header + "exec java -Xms" + mem + "M -Xmx" + mem + "M -jar server.jar nogui\n"
	}
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
