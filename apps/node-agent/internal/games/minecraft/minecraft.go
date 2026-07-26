// Package minecraft = game definition ของ Minecraft ฝั่ง node-agent
//
// ทุกอย่างที่ "เป็น Minecraft" ในชั้น runtime รวมอยู่ที่นี่: variant, ที่มาของ jar
// (Mojang/PaperMC/FabricMC/Forge maven), launch script, คำสั่ง stop, eula.txt/server.properties,
// การเดาเวอร์ชันจาก jar และการอ่านผู้เล่นออนไลน์/TPS จาก console
//
// เพิ่มเกมใหม่ = เพิ่ม package แบบเดียวกันนี้แล้วลงทะเบียนใน registry ที่ cmd/agent
package minecraft

import (
	"github.com/game-manager/node-agent/internal/games"
)

// ID ต้องตรงกับ id ฝั่ง control-plane — เดินทางมากับ job payload และเก็บใน .gamemanager/meta.json
const ID = "minecraft"

// containerPort = port ที่ MC server ฟังใน container เสมอ (host port map เข้ามาที่นี่)
const containerPort = 25565

// New สร้าง definition ของเกมนี้ (ไม่มี state — สร้างกี่ครั้งก็ได้ แต่ลงทะเบียนครั้งเดียวพอ)
func New() *games.Definition {
	return &games.Definition{
		ID:            ID,
		Variants:      []string{"vanilla", "paper", "fabric", "forge", "velocity"},
		ContainerPort: containerPort,
		StopCommand:   stopCommand,
		LaunchScript:  launchScript,
		LaunchEnv:     launchEnv,
		SeedFiles:     seedFiles,
		Provision:     provision,
		RuntimeImage:  runtimeImage,
		Import: games.ImportSpec{
			Ext: ".jar",
			// เดา jar หลักจากชื่อที่คุ้น (server jar มักมีคำพวกนี้อยู่ในชื่อ)
			NameHints:     []string{"paper", "purpur", "spigot", "vanilla", "fabric-server", "minecraft_server", "craftbukkit", "server"},
			MainArtifact:  mainArtifact,
			DetectVersion: detectVersion,
		},
		Console: consoleSpec(),
	}
}
