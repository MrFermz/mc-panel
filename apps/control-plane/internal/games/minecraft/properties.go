// properties.go — catalog + ไวยากรณ์ของ server.properties (games.ConfigSpec ของเกมนี้)
package minecraft

import (
	"github.com/game-manager/control-plane/internal/games"
)

// server.properties เป็นไฟล์ text ที่ root ของ server instance — จัดการผ่าน file manager
// stream เดียวกัน (gate ด้วย cap settings.view/edit ต่อ server)
const propertiesFileName = "server.properties"

func intPtr(n int) *int { return &n }

// configSpec: curated set ของ key ที่ให้แก้ผ่าน UI (key อื่นใน server.properties เก็บไว้
// verbatim — mergeProperties ไม่แตะ)
func configSpec() games.ConfigSpec {
	return games.ConfigSpec{
		FileName: propertiesFileName,
		// MC เขียนทับ server.properties ตอน shutdown — แก้ตอนรันอยู่จะถูก overwrite หายทันที
		EditableWhileRunning: false,
		// server.properties เป็น key=value ตรง ๆ — ใช้ format กลางของ games ได้เลย
		Format: games.KeyValueFormat{},
		Fields: []games.ConfigField{
			{Key: "gamemode", Label: "Game Mode", Type: "enum", Options: []string{"survival", "creative", "adventure", "spectator"}, Default: "survival"},
			{Key: "difficulty", Label: "Difficulty", Type: "enum", Options: []string{"peaceful", "easy", "normal", "hard"}, Default: "easy"},
			{Key: "hardcore", Label: "Hardcore", Type: "bool", Default: "false"},
			{Key: "pvp", Label: "PvP", Type: "bool", Default: "true"},
			{Key: "max-players", Label: "Max Players", Type: "int", Min: intPtr(1), Max: intPtr(2147483647), Default: "20"},
			{Key: "motd", Label: "MOTD", Type: "string", Default: "A Minecraft Server"},
			{Key: "online-mode", Label: "Online Mode", Type: "bool", Default: "true"},
			{Key: "white-list", Label: "Whitelist", Type: "bool", Default: "false"},
			{Key: "enforce-whitelist", Label: "Enforce Whitelist", Type: "bool", Default: "false"},
			{Key: "spawn-protection", Label: "Spawn Protection", Type: "int", Min: intPtr(0), Default: "16"},
			{Key: "view-distance", Label: "View Distance", Type: "int", Min: intPtr(3), Max: intPtr(32), Default: "10"},
			{Key: "simulation-distance", Label: "Simulation Distance", Type: "int", Min: intPtr(3), Max: intPtr(32), Default: "10"},
			{Key: "level-name", Label: "Level Name", Type: "string", Default: "world"},
			{Key: "level-seed", Label: "Level Seed", Type: "string", Default: ""},
			{Key: "level-type", Label: "Level Type", Type: "enum", Options: []string{"minecraft:normal", "minecraft:flat", "minecraft:large_biomes", "minecraft:amplified"}, Default: "minecraft:normal"},
			{Key: "allow-nether", Label: "Allow Nether", Type: "bool", Default: "true"},
			{Key: "allow-flight", Label: "Allow Flight", Type: "bool", Default: "false"},
			{Key: "enable-command-block", Label: "Enable Command Block", Type: "bool", Default: "false"},
			{Key: "spawn-monsters", Label: "Spawn Monsters", Type: "bool", Default: "true"},
			{Key: "spawn-animals", Label: "Spawn Animals", Type: "bool", Default: "true"},
			{Key: "spawn-npcs", Label: "Spawn NPCs", Type: "bool", Default: "true"},
			{Key: "generate-structures", Label: "Generate Structures", Type: "bool", Default: "true"},
			{Key: "force-gamemode", Label: "Force Game Mode", Type: "bool", Default: "false"},
			{Key: "player-idle-timeout", Label: "Player Idle Timeout", Type: "int", Min: intPtr(0), Default: "0"},
			{Key: "max-world-size", Label: "Max World Size", Type: "int", Min: intPtr(1), Max: intPtr(29999984), Default: "29999984"},
		},
	}
}
