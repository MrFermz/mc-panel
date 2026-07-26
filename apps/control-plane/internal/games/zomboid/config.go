// config.go — catalog ของไฟล์ config ของเกมนี้ (games.ConfigSpec)
package zomboid

import (
	"github.com/game-manager/control-plane/internal/games"
)

// configFileName = ไฟล์ config ของ instance (relative ต่อ jail ของ server)
// PZ ไม่ได้เก็บ config ไว้ที่ root ของ install dir แต่เก็บใน cache dir ของมันเอง —
// agent เป็นคนสั่งเกมด้วย `-cachedir=/data/zomboid -servername gm` ตอน start
// **path นี้ต้องตรงกับ zomboid.ConfigPath ฝั่ง node-agent**
const configFileName = "zomboid/Server/gm.ini"

func intPtr(n int) *int { return &n }

// configSpec: curated set ของ key ที่ให้แก้ผ่าน UI (key อื่นในไฟล์เก็บไว้ verbatim)
func configSpec() games.ConfigSpec {
	return games.ConfigSpec{
		FileName: configFileName,
		// PZ เขียนไฟล์ ini ทับตอน save/shutdown เหมือน Minecraft — แก้ตอนรันอยู่จะหาย
		EditableWhileRunning: false,
		// ไฟล์ ini ของ PZ เป็น key=value บรรทัดละคู่ ไม่มี section — ใช้ format กลางได้
		Format: games.KeyValueFormat{},
		Fields: []games.ConfigField{
			{Key: "PublicName", Label: "Server Name", Type: "string", Default: "Project Zomboid Server"},
			{Key: "PublicDescription", Label: "Description", Type: "string", Default: ""},
			{Key: "Public", Label: "List Publicly", Type: "bool", Default: "false"},
			{Key: "Open", Label: "Open (no account required)", Type: "bool", Default: "true"},
			{Key: "Password", Label: "Server Password", Type: "string", Default: ""},
			{Key: "MaxPlayers", Label: "Max Players", Type: "int", Min: intPtr(1), Max: intPtr(100), Default: "16"},
			{Key: "PVP", Label: "PvP", Type: "bool", Default: "true"},
			{Key: "PauseEmpty", Label: "Pause When Empty", Type: "bool", Default: "true"},
			{Key: "GlobalChat", Label: "Global Chat", Type: "bool", Default: "true"},
			{Key: "ServerWelcomeMessage", Label: "Welcome Message", Type: "string", Default: "Welcome to Project Zomboid!"},
			{Key: "Map", Label: "Map", Type: "string", Default: "Muldraugh, KY"},
			{Key: "SaveWorldEveryMinutes", Label: "Autosave (minutes)", Type: "int", Min: intPtr(0), Max: intPtr(1440), Default: "0"},
			{Key: "SleepAllowed", Label: "Allow Sleeping", Type: "bool", Default: "false"},
			{Key: "MaxAccountsPerUser", Label: "Max Accounts Per User", Type: "int", Min: intPtr(0), Max: intPtr(100), Default: "0"},
			// UPnP ไม่มีประโยชน์ในนี้ (port map ถูกกำหนดโดย agent ตอนสร้าง container)
			// และเปิดไว้ทำให้ server พยายามเจาะ router ของ host เอง
			{Key: "UPnP", Label: "UPnP", Type: "bool", Default: "false"},
		},
	}
}
