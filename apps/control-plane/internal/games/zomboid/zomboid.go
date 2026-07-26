// Package zomboid = game definition ของ Project Zomboid ฝั่ง control-plane
//
// เกมนี้เป็นตัวพิสูจน์ว่า abstraction ของ "เกม" รองรับเกมที่ **ไม่ได้โหลด artifact ผ่าน HTTP**
// (ติดตั้งผ่าน SteamCMD — ดู provision ฝั่ง node-agent) และเกมที่ **ไม่มี identity service
// ของตัวเอง** (ไม่มี uuid/รูปผู้เล่น/ไฟล์ allowlist ให้ panel เป็นเจ้าของ)
//
// เพิ่มเกมใหม่ = เพิ่ม package แบบเดียวกันนี้แล้วลงทะเบียนใน registry ที่ cmd/server
package zomboid

import (
	"github.com/game-manager/control-plane/internal/games"
)

// ID ต้องตรงกับ id ฝั่ง node-agent — เดินทางไปกับ job payload และเก็บใน servers.game
const ID = "zomboid"

const (
	// defaultHostPort = port มาตรฐานของ PZ (ใช้เป็นจุดเริ่มไล่หา host port ว่าง)
	defaultHostPort = 16261
	// hostPortSpan = จำนวน host port ที่ instance หนึ่งกิน — PZ ต้องเปิดสอง port ติดกัน
	// (agent map host_port → 16261/udp และ host_port+1 → 16262/udp)
	hostPortSpan = 2

	// minMemoryMB — PZ เป็นเกม JVM ที่กินหน่วยความจำหนักกว่า Minecraft มาก
	// ต่ำกว่านี้ start ขึ้นก็จริงแต่ตายตอนโหลด map
	minMemoryMB = 2048

	// maxVersionLen — `game_version` ของเกมนี้คือชื่อ Steam branch (ดู versions.go)
	maxVersionLen = 32
)

// New สร้าง definition ของเกมนี้ (ไม่มี state — สร้างครั้งเดียวแล้วใช้ร่วมทั้ง process)
func New() *games.Definition {
	return &games.Definition{
		ID:    ID,
		Label: "Project Zomboid",
		// ไม่มีข้อตกลงให้ user กดยอมรับก่อนสร้าง (dedicated server เป็น app ที่ Steam
		// เปิดให้โหลดแบบ anonymous) — ไม่มี variant ไหนตั้ง RequiresLicense
		LicenseName: "",
		Variants: []games.Variant{
			{ID: "vanilla", Label: "Vanilla", RequiresLicense: false},
		},
		DefaultHostPort: defaultHostPort,
		HostPortSpan:    hostPortSpan,
		MinMemoryMB:     minMemoryMB,
		Version: games.VersionSpec{
			MaxLen:       maxVersionLen,
			List:         listBranches,
			Valid:        validBranch,
			RuntimeImage: RuntimeImage,
		},
		Config:  configSpec(),
		Players: playerSpec(),
	}
}
