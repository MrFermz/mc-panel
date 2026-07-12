// Package provision สร้าง directory + ดาวน์โหลด server jar จาก official source
// + เขียน config/launch script ของ instance ใหม่ (ยังไม่ start)
//
// ทุกขั้นตอนต้อง idempotent — job โดน redeliver ซ้ำได้เสมอ ขั้นที่เสร็จแล้วให้ข้าม
package provision

import (
	"context"
	"encoding/json"
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
	"github.com/mc-panel/node-agent/internal/filemanager"
)

const (
	// uid/gid ที่ MC container รัน (User: 1000:1000) — ไฟล์ทั้งหมดต้องเป็นของ user นี้
	mcUID = 1000
	mcGID = 1000

	userAgent = "mc-panel-agent/0.1.0 (https://github.com/mc-panel)"
)

type Spec struct {
	ServerType string
	MCVersion  string
	AcceptEULA bool
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

	if spec.AcceptEULA {
		// เขียนเฉพาะเมื่อ user ติ๊กยอมรับเอง — ระบบห้าม default eula ให้เด็ดขาด
		if err := os.WriteFile(filepath.Join(dir, "eula.txt"), []byte("eula=true\n"), 0o644); err != nil {
			return "", fmt.Errorf("write eula.txt: %w", err)
		}
	}

	if spec.ServerType != "velocity" {
		propsPath := filepath.Join(dir, "server.properties")
		if _, err := os.Stat(propsPath); errors.Is(err, fs.ErrNotExist) {
			// port ใน container ตายตัว 25565 เสมอ — host port ไปกำหนดที่ PortBindings ตอน start
			if err := os.WriteFile(propsPath, []byte("server-port=25565\n"), 0o644); err != nil {
				return "", fmt.Errorf("write server.properties: %w", err)
			}
		}
	}

	mcpanelDir := filepath.Join(dir, ".mcpanel")
	if err := os.MkdirAll(mcpanelDir, 0o755); err != nil {
		return "", fmt.Errorf("create .mcpanel directory: %w", err)
	}

	stopCommand := "stop"
	if spec.ServerType == "velocity" {
		stopCommand = "end"
	}
	meta, err := json.MarshalIndent(map[string]string{
		"server_type":  spec.ServerType,
		"mc_version":   spec.MCVersion,
		"stop_command": stopCommand,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(mcpanelDir, "meta.json"), append(meta, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write meta.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(mcpanelDir, "launch.sh"), []byte(launchScript(spec.ServerType)), 0o755); err != nil {
		return "", fmt.Errorf("write launch.sh: %w", err)
	}

	p.chownRecursive(dir)
	log.Printf("server provisioned: server=%s type=%s version=%s", serverID, spec.ServerType, spec.MCVersion)
	return detail, nil
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
