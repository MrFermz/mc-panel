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
	"context"
	"errors"
	"regexp"

	"github.com/google/uuid"

	"github.com/mc-panel/control-plane/internal/games"
	"github.com/mc-panel/control-plane/internal/mojang"
	"github.com/mc-panel/control-plane/internal/playerface"
)

// ID ต้องตรงกับ id ฝั่ง node-agent — เดินทางไปกับ job payload และเก็บใน servers.game
const ID = "minecraft"

// defaultHostPort = port มาตรฐานของ MC server (ใช้เป็นจุดเริ่มไล่หา host port ว่าง)
const defaultHostPort = 25565

// minMemoryMB = ต่ำสุดที่ยอมให้ตั้ง — ต่ำกว่านี้ JVM ยัง start ไม่ขึ้น
const minMemoryMB = 256

// maxVersionLen = ความยาวสูงสุดของ mc_version ที่รับจาก user
const maxVersionLen = 50

// detectedVersionRe กัน garbage/injection ก่อนเขียน mc_version ที่ agent detect มาจาก jar จริง —
// เผื่อ release (1.20.1), snapshot (23w13a), pre/rc (1.20-pre1) แต่ปฏิเสธค่าเพี้ยนยาว ๆ /
// มีอักขระแปลก (ค่านี้ถูกเขียนทับของเดิมโดยไม่มีคนยืนยัน จึงเข้มกว่าค่าที่ user กรอกเอง)
var detectedVersionRe = regexp.MustCompile(`^[0-9][0-9A-Za-z._-]{0,31}$`)

// Deps = บริการภายนอกที่ definition ต้องใช้ — inject จาก cmd/server เพื่อไม่ให้ package นี้
// ผูกกับ store โดยตรง (playerface cache ใช้ Postgres เป็น storage ของ skin)
type Deps struct {
	Faces *playerface.Cache
}

// New สร้าง definition หนึ่งตัว (มี state ภายในคือ cache ของรายการเวอร์ชัน จึงต้องสร้างครั้งเดียว
// แล้วใช้ร่วมกันทั้ง process)
func New(deps Deps) *games.Definition {
	vs := newVersionService()

	return &games.Definition{
		ID:    ID,
		Label: "Minecraft",
		Variants: []games.Variant{
			{ID: "vanilla", Label: "Vanilla", NeedsEULA: true},
			{ID: "paper", Label: "Paper", NeedsEULA: true},
			{ID: "fabric", Label: "Fabric", NeedsEULA: true},
			{ID: "forge", Label: "Forge", NeedsEULA: true},
			// velocity เป็น proxy ไม่รัน Mojang server jar — ไม่มี EULA ให้ยอมรับ
			{ID: "velocity", Label: "Velocity (proxy)", NeedsEULA: false},
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
		Players: playerSpec(deps.Faces),
	}
}

// lookupProfile ห่อ mojang.Lookup ให้คืน error กลางของ package games
// (handler จึงไม่ต้องรู้จัก package mojang)
func lookupProfile(ctx context.Context, username string) (games.Profile, error) {
	p, err := mojang.Lookup(ctx, username)
	if errors.Is(err, mojang.ErrNotFound) {
		return games.Profile{}, games.ErrPlayerNotFound
	}
	if err != nil {
		return games.Profile{}, err
	}
	return games.Profile{UUID: p.UUID, Username: p.Username}, nil
}

// faceFetcher ห่อ playerface.Cache ให้คืน error กลางของ package games
func faceFetcher(faces *playerface.Cache) func(context.Context, uuid.UUID) ([]byte, error) {
	if faces == nil {
		return nil
	}
	return func(ctx context.Context, id uuid.UUID) ([]byte, error) {
		png, err := faces.Face(ctx, id)
		if errors.Is(err, playerface.ErrNoSkin) {
			return nil, games.ErrNoFace
		}
		return png, err
	}
}
