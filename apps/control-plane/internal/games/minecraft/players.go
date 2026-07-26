// players.go — กติกาผู้เล่นของ Minecraft (games.PlayerSpec ของเกมนี้):
// identity ผ่าน Mojang, whitelist.json ที่ panel เป็นเจ้าของ, ไฟล์ state ของ MC ที่อ่านมา merge,
// คำสั่ง moderation ผ่าน console และ playtime จาก world stats
package minecraft

import (
	"encoding/json"
	"regexp"

	"github.com/google/uuid"

	"github.com/game-manager/control-plane/internal/games"
)

// ไฟล์ผู้เล่นที่ root ของ server dir
const (
	whitelistFileName = "whitelist.json"
	usercacheFileName = "usercache.json"
	opsFileName       = "ops.json"
	bannedFileName    = "banned-players.json"
)

const (
	// defaultLevelName = ชื่อ world default ของ MC (server.properties key `level-name`)
	defaultLevelName = "world"
	// ticksPerSecond ของ MC — stat เวลาเล่นเก็บเป็น tick
	ticksPerSecond = 20
)

// safeUsernameRe — ชื่อที่จะถูกต่อเข้าไปในคำสั่ง console ต้องไม่มี whitespace/newline
// ไม่งั้นเป็น command injection เข้า server console ได้ตรง ๆ (WriteInput ต่อ "\n" ท้ายคำสั่ง)
var safeUsernameRe = regexp.MustCompile(`^[A-Za-z0-9_.*-]{1,32}$`)

func playerSpec(avatars games.AvatarFetcher) games.PlayerSpec {
	return games.PlayerSpec{
		IdentityService:     "Mojang",
		UsernameRule:        "username must be 3-16 characters of A-Z, a-z, 0-9, or underscore",
		ValidateUsername:    isValidUsername,
		ConsoleSafeUsername: safeUsernameRe.MatchString,
		Lookup:              lookupProfile,
		Avatar:              avatars,
		Allowlist: games.AllowlistSpec{
			FileName: whitelistFileName,
			// ต้อง white-list=true ใน server.properties ถึงจะ enforce จริง
			EnabledKey: "white-list",
			Encode:     encodeWhitelist,
			// ให้ผลทันทีโดยไม่ต้อง restart (best-effort — ส่งเฉพาะตอน running)
			ReloadCommand: "whitelist reload",
		},
		StateFiles: []games.StateFile{
			{Path: usercacheFileName, Flag: games.FlagSeen, Decode: decodePlayerFile},
			{Path: opsFileName, Flag: games.FlagOp, Decode: decodePlayerFile},
			{Path: bannedFileName, Flag: games.FlagBanned, Decode: decodePlayerFile},
		},
		// allow-list ของคำสั่ง — ห้ามรับคำสั่งดิบจาก client
		Actions: map[string]string{
			"op":     "op",
			"deop":   "deop",
			"kick":   "kick",
			"ban":    "ban",
			"pardon": "pardon",
		},
		Playtime: &games.PlaytimeSpec{
			SaveNameKey:     "level-name",
			DefaultSaveName: defaultLevelName,
			Path:            playtimePath,
			Decode:          decodePlaytime,
		},
	}
}

// isValidUsername: 3-16 ตัว [A-Za-z0-9_] ตามกติกา Minecraft (เช็คก่อนยิง Mojang)
func isValidUsername(s string) bool {
	if len(s) < 3 || len(s) > 16 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}

// whitelistEntry คือ shape ที่ Minecraft อ่านจาก whitelist.json
type whitelistEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

func encodeWhitelist(entries []games.AllowlistEntry) ([]byte, error) {
	out := make([]whitelistEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, whitelistEntry{UUID: e.UUID.String(), Name: e.Username})
	}
	return json.MarshalIndent(out, "", "  ")
}

// mcPlayerEntry = subset ที่พอสำหรับ usercache/ops/banned (ทุกไฟล์มี name+uuid)
type mcPlayerEntry struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

func decodePlayerFile(content []byte) ([]games.PlayerRef, error) {
	var entries []mcPlayerEntry
	if err := json.Unmarshal(content, &entries); err != nil {
		return nil, err
	}
	refs := make([]games.PlayerRef, 0, len(entries))
	for _, e := range entries {
		refs = append(refs, games.PlayerRef{UUID: e.UUID, Name: e.Name})
	}
	return refs, nil
}

// playtimePath — stats เป็นไฟล์ละคนใต้ world dir ของ server
func playtimePath(worldName string, playerUUID uuid.UUID) string {
	return worldName + "/stats/" + playerUUID.String() + ".json"
}

// mcStatsFile = shape ของ world/stats/{uuid}.json เท่าที่ต้องใช้
// play_time (1.17+) กับ play_one_minute (เวอร์ชันเก่า) เก็บ tick เหมือนกัน คนละ key
type mcStatsFile struct {
	Stats struct {
		Custom map[string]int64 `json:"minecraft:custom"`
	} `json:"stats"`
}

func decodePlaytime(content []byte) int64 {
	var f mcStatsFile
	if err := json.Unmarshal(content, &f); err != nil {
		return 0
	}
	ticks := f.Stats.Custom["minecraft:play_time"]
	if ticks == 0 {
		ticks = f.Stats.Custom["minecraft:play_one_minute"]
	}
	return ticks / ticksPerSecond
}
