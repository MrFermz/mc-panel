package httpapi

import "strings"

// ชื่อที่ระบบจองไว้ — กันคนสร้างบัญชีที่ "ดูเหมือนของระบบ" แล้วเอาไปหลอกแอดมินคนอื่น
// หรือทำให้ audit log อ่านแล้วแยกไม่ออกว่าเป็นคนหรือเป็นระบบ
//
// enforce ที่ **HTTP handler เท่านั้น** ไม่ใช่ที่ store — seed ตอน boot กับ CLI
// `-reset-admin-password` เรียก store ตรง ๆ จึงยังตั้งชื่อ `admin` (ค่า default ของ
// ADMIN_USERNAME) ได้ตามเดิม. ระบบจองชื่อของตัวเองได้ คนอื่นจองไม่ได้
//
// key ทุกตัวต้องเป็นรูป **normalized** แล้ว (พิมพ์เล็ก + ไม่มี `.`, `_`, `-`)
var reservedUsernames = map[string]bool{
	// อำนาจ/ตัวตนระดับระบบ
	"admin": true, "administrator": true, "root": true, "superuser": true,
	"superadmin": true, "sysadmin": true, "system": true, "staff": true,
	"official": true, "security": true,
	// ชื่อ role ในระบบนี้ — ปลอมเป็นชื่อ role ทำให้อ่าน access list แล้วสับสน
	"owner": true, "operator": true, "moderator": true, "viewer": true,
	// ชื่อ component ของ panel
	"mcpanel": true, "panel": true, "controlplane": true, "nodeagent": true,
	"agent": true, "node": true, "console": true, "api": true,
	"daemon": true, "service": true, "bot": true, "webhook": true,
	// ช่องทางติดต่อ — ใช้หลอกให้คนเชื่อว่าเป็นทีมงาน
	"support": true, "helpdesk": true, "noreply": true, "postmaster": true,
	"webmaster": true, "abuse": true, "billing": true,
	// placeholder ที่อาจถูกตีความผิดว่าเป็น "ไม่มีคน"
	"anonymous": true, "guest": true, "nobody": true, "everyone": true,
	"deleted": true, "unknown": true, "null": true, "undefined": true, "none": true,
}

// normalizeUsername ตัด separator ที่ตาคนมองข้ามได้ (`a-d-m-i-n`, `admin_`, `A.D.M.I.N`
// ล้วน normalize เป็น `admin`) แล้วเทียบแบบตรงตัวเป๊ะ ไม่ใช่ substring —
// ชื่อปกติอย่าง `nodeman` จึงไม่โดนบล็อกเพราะมี `node` อยู่ข้างใน
func normalizeUsername(username string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(username) {
		if r == '.' || r == '_' || r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isReservedUsername: ชื่อที่เหลือแต่ separator (`___`) ผ่าน regex แต่ normalize แล้วว่าง —
// นับเป็นสงวนด้วย ไม่ให้มีบัญชีที่ชื่อแทบมองไม่เห็น
func isReservedUsername(username string) bool {
	n := normalizeUsername(username)
	return n == "" || reservedUsernames[n]
}
