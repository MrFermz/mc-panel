// runtime.go — การ map เวอร์ชันของเกม → Java runtime image
// **ต้องให้ผลตรงกับฝั่ง control-plane** (internal/games/minecraft/runtime.go ของ control-plane)
// ซึ่งเป็นคนเลือก image ให้ job start_server — ที่นี่ใช้กับ tool ระหว่าง provision (forge installer)
package minecraft

import (
	"strconv"
	"strings"
)

// latestJavaTag = Java ใหม่สุดที่มี runtime image (ต้องตรงกับ control-plane)
const latestJavaTag = "25"

// runtimeImage = games.Definition.RuntimeImage — prefix มาจาก config ของ agent
func runtimeImage(prefix, variant, version string) string {
	return prefix + ":" + javaTagFor(variant, version)
}

// javaTagFor — mapping เดียวกับ control-plane:
// velocity → java ใหม่สุด, MC <= 1.16.5 → java 8, 1.17–1.20.4 → java 17,
// 1.20.5–1.21.x → java 21, calendar version (26.x…) และ parse ไม่ได้ → java ใหม่สุด
// (Java backward-compatible: jar เก่ารันบน JVM ใหม่ได้ ปลอดภัยเป็น default)
func javaTagFor(variant, gameVersion string) string {
	if variant == "velocity" {
		return latestJavaTag
	}
	parts := strings.Split(strings.TrimSpace(gameVersion), ".")
	if len(parts) < 2 {
		return latestJavaTag
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return latestJavaTag
	}
	// major != 1 = calendar versioning ตั้งแต่ 2025 — ต้องการ Java ใหม่สุด
	if major != 1 {
		return latestJavaTag
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return latestJavaTag
	}
	patch := 0
	if len(parts) >= 3 {
		// patch แบบมี suffix (เช่น pre-release) ให้ parse เท่าที่ parse ได้
		if n, err := strconv.Atoi(strings.TrimFunc(parts[2], func(r rune) bool { return r < '0' || r > '9' })); err == nil {
			patch = n
		}
	}
	switch {
	case minor <= 16:
		return "8"
	case minor < 20:
		return "17"
	case minor == 20 && patch <= 4:
		return "17"
	default:
		return "21"
	}
}
