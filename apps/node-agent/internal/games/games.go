// Package games นิยาม "เกม" ที่ node-agent รันได้ (game definition ฝั่ง runtime)
//
// ความรู้เฉพาะเกมทุกอย่างที่ agent ต้องใช้ — variant ที่ provision ได้, ที่มาของ artifact,
// launch script, คำสั่งปิด, ไฟล์ config ที่ panel เขียนให้,
// และวิธีอ่านสถานะในเกมจาก console — อยู่ใน Definition ตัวเดียว ส่วนที่เหลือของ agent
// (provision/runner/gamestate/jobs) ทำงานผ่าน Registry นี้เท่านั้น
// **ห้าม switch ตามชื่อ variant กระจายอยู่ในโค้ดอื่นอีก**
//
// คู่ขนานกับ package ชื่อเดียวกันฝั่ง control-plane ซึ่งถือความรู้ฝั่ง API/validation —
// สอง app แยก module กันจึงไม่ import ข้ามกัน (ดู CLAUDE.md เรื่อง submodules)
// สิ่งที่ต้องตรงกันสองฝั่งคือ **id ของเกมและ variant** ซึ่งเดินทางมากับ job payload
package games

import (
	"context"
	"io/fs"
)

// DefaultID = เกมที่ใช้เมื่อ payload/meta.json ไม่ได้ระบุ (job เก่าที่ค้างใน stream และ
// instance ที่ provision ไว้ก่อนมี field นี้)
const DefaultID = "minecraft"

// Definition = ทุกอย่างที่ agent ต้องรู้เกี่ยวกับเกมหนึ่งเกม
type Definition struct {
	// ID ต้องตรงกับ id ฝั่ง control-plane
	ID string
	// Variants = variant ที่ provision/รันได้ (ชื่อเดียวกับ variant ใน API)
	Variants []string

	// ContainerPort = port ที่ server ฟังอยู่ใน container (map ไป host port ตอน start)
	ContainerPort int

	// StopCommand = คำที่เขียนเข้า stdin เพื่อสั่งปิดอย่างสุภาพ (ต่าง variant ต่างคำได้)
	StopCommand func(variant string) string
	// LaunchScript = เนื้อของ .gamemanager/launch.sh ที่ container รัน
	LaunchScript func(variant string) string
	// LaunchEnv = env ที่ container ต้องมีให้ launch script ใช้ (เช่น heap size)
	// คืนเป็น "KEY=VALUE" ตามรูปแบบของ docker
	LaunchEnv func(memoryMB int) []string
	// SeedFiles = ไฟล์ config ที่ panel เขียนให้ตอน provision (license/ค่าเริ่มต้นของ config)
	SeedFiles func(variant string, acceptLicense bool) []SeedFile

	// Provision โหลด artifact ของ variant/version ลง dir ของ server (idempotent)
	// คืน detail สั้น ๆ ที่จะเดินทางกลับไปกับ JobResult
	Provision func(ctx context.Context, env ProvisionEnv) (detail string, err error)
	// RuntimeImage = image ที่ tool ของเกมต้องใช้ (เช่น installer ของบาง variant) — prefix มาจาก
	// config ของ agent เพื่อให้ override ได้ ต้องให้ผลตรงกับที่ control-plane เลือกไว้
	RuntimeImage func(prefix, variant, version string) string

	// Console = วิธีอ่านสถานะในเกม (ผู้เล่นออนไลน์/metric) จาก console
	Console ConsoleSpec
}

// HasVariant = variant นี้อยู่ในเกมนี้ไหม
func (d *Definition) HasVariant(id string) bool {
	for _, v := range d.Variants {
		if v == id {
			return true
		}
	}
	return false
}

// SeedFile = ไฟล์ที่ panel เขียนให้ตอน provision
type SeedFile struct {
	// Path relative ต่อ dir ของ server
	Path    string
	Content []byte
	Mode    fs.FileMode
	// Overwrite=false → เขียนเฉพาะเมื่อไฟล์ยังไม่มี (ห้ามทับ config ที่ user แก้ไว้)
	Overwrite bool
}

// ---------- console ----------

// ConsoleSpec = วิธีอ่าน "สถานะในเกม" ที่ container stats มองไม่เห็น (ผู้เล่นออนไลน์, metric)
// เกมส่วนใหญ่ไม่มี API ให้ถาม จึงต้องยิงคำสั่งเข้า console แล้วอ่าน reply
type ConsoleSpec struct {
	// RosterCommand = คำสั่งขอรายชื่อผู้เล่น + max players (source of truth ของแต่ละรอบ)
	// "" = เกมนี้ไม่มีวิธีถาม (tracker จะไม่ยิงอะไรเลย)
	RosterCommand string
	// MetricCommand = คำสั่งขอ metric ประจำเกม (minecraft = `tps`) — "" = ไม่มี
	// รองรับการปิดถาวรเมื่อ server ตอบว่าไม่รู้จักคำสั่งนี้ (variant ที่ไม่รองรับ)
	MetricCommand string
	// Parse อ่าน 1 บรรทัดดิบจาก console แล้วบอกว่าเป็น event อะไร
	// ต้องเร็ว: ทุกบรรทัดของทุก server วิ่งผ่านที่นี่
	Parse func(line string) Event
}

type EventKind int

const (
	// EventNone = บรรทัดที่ไม่เกี่ยวข้อง
	EventNone EventKind = iota
	// EventReady = server ประกาศว่า start เสร็จ พร้อมรับคำสั่งแล้ว
	EventReady
	// EventRoster = reply ของ RosterCommand (รายชื่อ + max players)
	EventRoster
	// EventMetric = reply ของ MetricCommand
	EventMetric
	// EventJoin / EventLeave = ผู้เล่นเข้า/ออกระหว่างรอบ
	EventJoin
	EventLeave
	// EventUnknownCommand = server ไม่รู้จักคำสั่งที่เพิ่งยิงไป
	EventUnknownCommand
)

// Event = ผลของการอ่าน console 1 บรรทัด
type Event struct {
	Kind EventKind
	// Names/MaxPlayers ใช้กับ EventRoster (MaxPlayers = 0 แปลว่าไม่รู้ ให้คงค่าเดิม)
	Names      []string
	MaxPlayers int
	// Metric ใช้กับ EventMetric
	Metric float64
	// Name ใช้กับ EventJoin/EventLeave
	Name string
}

// ---------- registry ----------

// Registry = เกมทั้งหมดที่ agent นี้รันได้ (immutable หลังสร้าง)
type Registry struct {
	byID map[string]*Definition
}

func NewRegistry(defs ...*Definition) *Registry {
	r := &Registry{byID: make(map[string]*Definition, len(defs))}
	for _, d := range defs {
		r.byID[d.ID] = d
	}
	return r
}

// Get คืน definition ตาม id แบบตรงตัว
func (r *Registry) Get(id string) (*Definition, bool) {
	d, ok := r.byID[id]
	return d, ok
}

// Resolve เหมือน Get แต่ id ว่างแปลว่า DefaultID — ใช้กับ job/meta.json ที่ยังไม่มี field `game`
func (r *Registry) Resolve(id string) (*Definition, bool) {
	if id == "" {
		id = DefaultID
	}
	return r.Get(id)
}

// Default คืนเกม DefaultID — nil ถ้าไม่ได้ลงทะเบียนไว้
func (r *Registry) Default() *Definition { return r.byID[DefaultID] }
