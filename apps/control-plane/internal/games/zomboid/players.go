// players.go — กติกาผู้เล่นของเกมนี้ (games.PlayerSpec)
//
// PZ ไม่มี identity service สาธารณะให้ verify ชื่อ และไม่มีไฟล์รายชื่อให้ panel เป็นเจ้าของ:
// บัญชีผู้เล่นทั้งหมดอยู่ใน DB ของตัวเกมเอง จัดการผ่านคำสั่ง console เท่านั้น
// definition นี้จึงเว้น Lookup/Avatar/Allowlist/StateFiles/Playtime ไว้ทั้งหมด —
// httpapi ตอบ endpoint ที่ต้องใช้ของพวกนั้นเป็น 409 unsupported ให้เอง (ดู helpers.go)
// สิ่งที่ทำได้จริงคือ **คำสั่ง moderation ผ่าน console** ซึ่งอยู่ใน Actions
package zomboid

import (
	"regexp"

	"github.com/game-manager/control-plane/internal/games"
)

// usernameRe — ชื่อบัญชีของ PZ ถูกต่อเข้าไปในคำสั่ง console ตรง ๆ จึงห้ามมี whitespace
// หรืออักขระที่ทำให้ parser ของเกมแตกคำสั่งผิด (เจตนาเดียวกับ ConsoleSafeUsername ของ Minecraft)
var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)

func playerSpec() games.PlayerSpec {
	return games.PlayerSpec{
		UsernameRule:        "username must be 1-32 characters: letters, digits, underscore, dot or dash",
		ValidateUsername:    usernameRe.MatchString,
		ConsoleSafeUsername: usernameRe.MatchString,
		// คำสั่ง moderation ของ PZ พิมพ์ตรง ๆ ใน console ของ server (ไม่มี / นำหน้า)
		Actions: map[string]string{
			"op":     "grantadmin",
			"deop":   "removeadmin",
			"kick":   "kickuser",
			"ban":    "banuser",
			"pardon": "unbanuser",
		},
	}
}
