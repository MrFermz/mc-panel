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

// HeapMB แปลง memory_mb ที่ user จัดสรร (= hard limit ของทั้ง container) เป็น -Xmx ของ JVM
// JVM กินนอก heap อีกมาก (metaspace, code cache, thread stacks, direct buffers, GC overhead)
// วัดจริง: Paper 1.21 heap 1G โดน OOM kill ที่ limit 1.25x ตอน world-gen เลยกันไว้ ~1/3
// (limit ≈ 1.5x heap) แต่ไม่เกิน 2GB เพื่อไม่ให้เครื่องใหญ่เสียเปล่า และไม่เกินครึ่งของ limit
// เพื่อให้ instance เล็ก (256MB ซึ่งเป็นขั้นต่ำที่ API ยอม) ยังเหลือ heap พอ start ได้
func HeapMB(memoryMB int) int {
	reserve := memoryMB / 3
	if reserve < 256 {
		reserve = 256
	}
	if reserve > 2048 {
		reserve = 2048
	}
	if reserve > memoryMB/2 {
		reserve = memoryMB / 2
	}
	return memoryMB - reserve
}

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
// Net*/Disk* เป็น counter สะสมดิบ (cumulative bytes) — rate/sec คำนวณจาก delta
// ระหว่าง sample ที่ชั้นบน (serverstats) ไม่ใช่ที่นี่
type ResourceStats struct {
	CPUPercent    float64
	MemoryMB      int
	MemoryLimitMB int
	DiskMB        int
	NetRxBytes    uint64
	NetTxBytes    uint64
	DiskReadBytes uint64
	DiskWrBytes   uint64
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
