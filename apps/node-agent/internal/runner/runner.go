package runner

import (
	"context"
	"io"
)

// Label ที่ agent ติดให้ทุก container ที่ตัวเองจัดการ — events/reconcile/heartbeat
// ใช้ filter ชุดเดียวกันนี้ ห้าม container อื่นบนเครื่องปนเข้ามา
const (
	LabelManagedBy      = "mc.managed_by"
	LabelManagedByValue = "mc-panel-agent"
	LabelServerID       = "mc.server_id"
	LabelProject        = "project"
	LabelProjectValue   = "mc-panel"
)

// ServerConfig คือค่าที่ใช้ start instance หนึ่งตัว
type ServerConfig struct {
	ID             string
	StartupCommand string // ใช้เฉพาะ native mode (docker mode ใช้ CMD ใน image)
	MemoryMB       int
	WorkDir        string // เช่น /data/servers/{id}/ — ต้องเป็น dir เดียวกันไม่ว่าจะรัน native หรือ docker
	Port           int    // host port; 0 = ไม่ expose (เข้าถึงผ่าน velocity ใน network เดียวกัน)
	Image          string // docker image เช่น mcpanel/mc-runtime:21 — control plane เป็นคน map java version
}

// ResourceStats คือ resource usage ที่ agent รายงานกลับ
type ResourceStats struct {
	CPUPercent    float64
	MemoryMB      int
	MemoryLimitMB int
	DiskMB        int
}

// Runner คือ interface กลางที่ NativeRunner และ DockerRunner ต้อง implement
// ส่วนที่เหลือของ agent (console streaming, stop/restart logic) เรียกผ่าน interface นี้
// โดยไม่ต้องรู้ว่าข้างล่างเป็นโหมดไหน
type Runner interface {
	Start(ctx context.Context, cfg ServerConfig) error
	// Stop ต้องเขียน stop word เข้า stdin ก่อนเสมอ (native และ docker เหมือนกัน)
	// แล้วรอ grace period ก่อน fallback ไป force kill
	Stop(id string, graceful bool) error
	Kill(id string) error
	AttachConsole(id string) (io.ReadWriteCloser, error)
	Stats(id string) (ResourceStats, error)
}
