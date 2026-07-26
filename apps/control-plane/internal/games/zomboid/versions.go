// versions.go — "เวอร์ชัน" ของเกมนี้คือ **Steam branch** ไม่ใช่เลขเวอร์ชัน
//
// SteamCMD ติดตั้งบิลด์ล่าสุดของ branch ที่เลือกเสมอ (ปักหมุดเลขเวอร์ชันไม่ได้เหมือน jar
// ของ Minecraft) — field `game_version` จึงเก็บชื่อ branch แล้วส่งต่อให้ agent ใส่ใน -beta
// ซึ่งแปลว่าค่านี้ **ต้องเป็น allow-list เสมอ** (มันเดินทางไปเป็น argv ของ steamcmd)
package zomboid

import "context"

// branches = Steam branch ที่รองรับ — **ต้องตรงกับ allow-list ฝั่ง node-agent**
// public = branch ปกติที่ Steam เสิร์ฟให้ทุกคน, unstable = branch ทดสอบของผู้พัฒนา
var branches = []string{"public", "unstable"}

func listBranches(context.Context, string) ([]string, error) {
	// ไม่ต้องถาม upstream: รายการ branch เป็นค่าคงที่ของเกม (Steam ไม่มี API สาธารณะ
	// ให้ query branch ของ app แบบ anonymous อยู่แล้ว)
	out := make([]string, len(branches))
	copy(out, branches)
	return out, nil
}

func validBranch(_, version string) bool {
	for _, b := range branches {
		if b == version {
			return true
		}
	}
	return false
}
