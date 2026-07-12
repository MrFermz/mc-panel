package jobs

import (
	"strconv"
	"strings"
)

const runtimeImagePrefix = "mcpanel/mc-runtime"

// latestJavaTag = Java ใหม่สุดที่เรามี runtime image ให้ (ดู make runtime-images)
// ใช้กับ calendar version (25.x/26.x…) และ fallback — Java backward-compatible
// ดังนั้น jar ที่ target Java เก่ากว่ารันบน JVM ใหม่นี้ได้ปกติ (ปลอดภัยเป็น default)
const latestJavaTag = "25"

// DockerImage เลือก Java runtime image ตามชนิดและเวอร์ชันของ server:
//
//	velocity            -> :25 (velocity รองรับ Java ใหม่เสมอ)
//	MC calendar (26.x…) -> :25 (รุ่นใหม่ตั้งแต่ 2025 ต้องการ Java 25)
//	MC 1.20.5 - 1.21.x  -> :21
//	MC 1.17 - 1.20.4    -> :17
//	MC เก่ากว่า 1.17     -> :8 (รุ่นเก่าพังบน Java ใหม่ ต้องใช้ Java เดิม)
//
// เวอร์ชันที่ parse ไม่ได้ (snapshot ฯลฯ) fallback :25 (ใหม่สุด = ปลอดภัยกับ jar ใหม่)
func DockerImage(serverType, mcVersion string) string {
	if serverType == "velocity" {
		return runtimeImagePrefix + ":" + latestJavaTag
	}

	major, minor, patch, ok := parseMCVersion(mcVersion)
	if !ok {
		return runtimeImagePrefix + ":" + latestJavaTag
	}
	// major != 1 = calendar versioning (Mojang เปลี่ยนเป็น YY.N ตั้งแต่ 2025)
	// รุ่นเหล่านี้ต้องการ Java ใหม่สุด — เก่าสุดที่ยืนยันคือ 26.2 ใช้ Java 25
	if major != 1 {
		return runtimeImagePrefix + ":" + latestJavaTag
	}

	switch {
	case minor > 20 || (minor == 20 && patch >= 5):
		return runtimeImagePrefix + ":21"
	case minor >= 17:
		return runtimeImagePrefix + ":17"
	default:
		return runtimeImagePrefix + ":8"
	}
}

// parseMCVersion รองรับ "1.21", "1.20.4" — ตัวเลขล้วนคั่นด้วยจุดเท่านั้น
// ("1.20.5-rc1" หรือ "24w14a" ถือว่า parse ไม่ได้ ให้ caller ตัดสินใจ fallback)
func parseMCVersion(v string) (major, minor, patch int, ok bool) {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, false
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	major, minor = nums[0], nums[1]
	if len(nums) == 3 {
		patch = nums[2]
	}
	return major, minor, patch, true
}
