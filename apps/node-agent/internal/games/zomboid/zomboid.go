// Package zomboid = game definition ของ Project Zomboid ฝั่ง node-agent
//
// เกมนี้คือตัวพิสูจน์เส้นทาง "ติดตั้ง artifact ผ่าน SteamCMD": The Indie Stone แจก
// dedicated server เป็น **app แยกที่โหลดแบบ anonymous ได้** (app 380870) ต่างจากตัวเกม
// ที่ต้องเป็นเจ้าของ — agent จึงโหลดผ่าน SteamCMD ใน one-off container แทน HTTP ตรง ๆ
// แบบ Minecraft (ดู provision.go)
//
// เกมนี้รันบน JVM ที่ Steam แจกมาพร้อม server (jre64/) ไม่ใช่ runtime image ของเรา —
// image ที่ต้องการจึงเป็น image ที่มี SteamCMD + lib ที่ตัวเกมต้องใช้ (ดู runtime.go)
package zomboid

import (
	"github.com/game-manager/node-agent/internal/games"
)

// ID ต้องตรงกับ id ฝั่ง control-plane — เดินทางมากับ job payload และเก็บใน .gamemanager/meta.json
const ID = "zomboid"

const (
	// gamePort/udpPort = port ที่ PZ ฟังใน container เสมอ (ตั้งไว้ในไฟล์ ini ที่ seed ให้)
	// PZ ต้องใช้สอง port: port หลักสำหรับ handshake + อีก port สำหรับ traffic ของเกม
	gamePort = 16261
	udpPort  = 16262

	// cacheDir = ที่เก็บ config/save ของ PZ ใน jail (ปกติเกมใช้ ~/Zomboid) —
	// path นี้ถูกส่งให้เกมผ่าน -cachedir ตอน start
	cacheDir = "zomboid"
	// serverName = ชื่อ server ของ PZ ที่ panel ใช้ตายตัว — เป็นตัวกำหนดชื่อไฟล์ config
	serverName = "gm"
)

// ConfigPath = ไฟล์ config ของ instance (relative ต่อ jail) ที่เกิดจาก cacheDir+serverName
// **ต้องตรงกับ Config.FileName ฝั่ง control-plane** ซึ่งเป็นคนอ่าน/เขียนไฟล์นี้ผ่าน file stream
const ConfigPath = cacheDir + "/Server/" + serverName + ".ini"

// New สร้าง definition ของเกมนี้ (ไม่มี state — สร้างกี่ครั้งก็ได้ แต่ลงทะเบียนครั้งเดียวพอ)
func New() *games.Definition {
	return &games.Definition{
		ID:       ID,
		Variants: []string{"vanilla"},
		Ports: []games.Port{
			{Container: gamePort, Protocol: "udp", HostOffset: 0},
			{Container: udpPort, Protocol: "udp", HostOffset: 1},
		},
		StopCommand:  stopCommand,
		LaunchScript: launchScript,
		LaunchEnv:    launchEnv,
		SeedFiles:    seedFiles,
		Provision:    provision,
		RuntimeImage: runtimeImage,
		ImageSource:  imageSource,
		Console:      consoleSpec(),
	}
}
