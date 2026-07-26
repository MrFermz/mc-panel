// Package minecraft = game definition ของ Minecraft ฝั่ง control-plane
//
// ทุกอย่างที่ "เป็น Minecraft" ในชั้น control-plane รวมอยู่ที่นี่ที่เดียว: variant
// (vanilla/paper/fabric/forge/velocity), กติกาเวอร์ชัน + การ map Java runtime image,
// catalog ของ server.properties, และกติกาผู้เล่น (Mojang, whitelist.json, ops/banned/usercache,
// คำสั่ง op/kick/ban, playtime จาก world stats)
//
// เพิ่มเกมใหม่ = เพิ่ม package แบบเดียวกันนี้แล้วลงทะเบียนใน registry ที่ cmd/server
package minecraft

import (
	"regexp"

	"github.com/game-manager/control-plane/internal/games"
)

// ID ต้องตรงกับ id ฝั่ง node-agent — เดินทางไปกับ job payload และเก็บใน servers.game
const ID = "minecraft"

// defaultHostPort = port มาตรฐานของ MC server (ใช้เป็นจุดเริ่มไล่หา host port ว่าง)
const defaultHostPort = 25565

// minMemoryMB = ต่ำสุดที่ยอมให้ตั้ง — ต่ำกว่านี้ JVM ยัง start ไม่ขึ้น
const minMemoryMB = 256

// maxVersionLen = ความยาวสูงสุดของ game_version ที่รับจาก user
const maxVersionLen = 50

// detectedVersionRe กัน garbage/injection ก่อนเขียน game_version ที่ agent detect มาจาก jar จริง —
// เผื่อ release (1.20.1), snapshot (23w13a), pre/rc (1.20-pre1) แต่ปฏิเสธค่าเพี้ยนยาว ๆ /
// มีอักขระแปลก (ค่านี้ถูกเขียนทับของเดิมโดยไม่มีคนยืนยัน จึงเข้มกว่าค่าที่ user กรอกเอง)
var detectedVersionRe = regexp.MustCompile(`^[0-9][0-9A-Za-z._-]{0,31}$`)

// Deps = บริการภายนอกที่ definition ต้องใช้ — inject จาก cmd/server เพื่อไม่ให้ package นี้
// ผูกกับ store โดยตรง (ชั้น cache ของรูปผู้เล่นเป็นของกลางที่ internal/avatarcache
// ส่วนวิธีดึงรูปจริงคือ FetchAvatar ในไฟล์ avatar.go ของ package นี้)
type Deps struct {
	Avatars games.AvatarFetcher
}

// New สร้าง definition หนึ่งตัว (มี state ภายในคือ cache ของรายการเวอร์ชัน จึงต้องสร้างครั้งเดียว
// แล้วใช้ร่วมกันทั้ง process)
func New(deps Deps) *games.Definition {
	vs := newVersionService()

	return &games.Definition{
		ID:          ID,
		Label:       "Minecraft",
		LicenseName: "Minecraft EULA",
		Variants: []games.Variant{
			{ID: "vanilla", Label: "Vanilla", RequiresLicense: true},
			{ID: "paper", Label: "Paper", RequiresLicense: true},
			{ID: "fabric", Label: "Fabric", RequiresLicense: true},
			{ID: "forge", Label: "Forge", RequiresLicense: true},
			// velocity เป็น proxy ไม่รัน server jar ของ Mojang — ไม่มี EULA ให้ยอมรับ
			{ID: "velocity", Label: "Velocity (proxy)", RequiresLicense: false},
		},
		DefaultHostPort: defaultHostPort,
		MinMemoryMB:     minMemoryMB,
		Version: games.VersionSpec{
			MaxLen:        maxVersionLen,
			List:          vs.list,
			ValidDetected: detectedVersionRe.MatchString,
			RuntimeImage:  RuntimeImage,
		},
		Config:  configSpec(),
		Players: playerSpec(deps.Avatars),
	}
}
