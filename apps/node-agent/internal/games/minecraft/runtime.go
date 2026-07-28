// runtime.go — การ map เวอร์ชันของเกม → Java runtime image
// **ต้องให้ผลตรงกับฝั่ง control-plane** (internal/games/minecraft/runtime.go ของ control-plane)
// ซึ่งเป็นคนเลือก image ให้ job start_server — ที่นี่ใช้กับ tool ระหว่าง provision (forge installer)
package minecraft

import (
	"strconv"
	"strings"

	"github.com/game-manager/node-agent/internal/games"
)

// latestJavaTag = Java ใหม่สุดที่มี runtime image (ต้องตรงกับ control-plane)
const latestJavaTag = "25"

// imageName = ชื่อ image ของเกมนี้ใต้ namespace ของ agent (ต้องตรงกับ control-plane)
const imageName = "runtime-java"

// runtimeImage = games.Definition.RuntimeImage — namespace มาจาก config ของ agent
func runtimeImage(namespace, variant, version string) string {
	return namespace + "/" + imageName + ":" + javaTagFor(variant, version)
}

// imageSource = games.Definition.ImageSource — เกมนี้รันบน JVM ล้วน ไม่ต้อง build image เอง:
// pull JRE ของ Adoptium ตาม java version ที่อยู่ใน tag ท้าย ':' แล้ว tag ซ้ำเป็นชื่อของเรา
// (ผลลัพธ์เท่ากับ image ที่ build ด้วย make runtime-images เว้นแต่ HOME/CMD ที่ runner ตั้งเองอยู่แล้ว)
//
// ⚠️ รับเฉพาะ ref ที่เป็น runtime image **ของเกมนี้จริง ๆ** (`{ns}/runtime-java:{เลข java}`) —
// ref ของเกมอื่นที่หลุดมาถึงที่นี่ (เช่นตอนหา definition ของ instance ไม่เจอแล้วตกมาที่เกม default)
// ต้องคืน zero value เพื่อให้ EnsureRuntimeImage บอกตรง ๆ ว่าเตรียม image ให้ไม่ได้
// ไม่ใช่ไปลาก tag ของเกมอื่นมาต่อเป็นชื่อ image ของ Adoptium (เช่น `eclipse-temurin:1-jre`)
func imageSource(imageRef string) games.ImageSource {
	repo, tag, ok := strings.Cut(imageRef, ":")
	if !ok || tag == "" || !strings.HasSuffix(repo, "/"+imageName) {
		return games.ImageSource{}
	}
	if _, err := strconv.Atoi(tag); err != nil {
		return games.ImageSource{}
	}
	return games.ImageSource{
		PullFrom: "docker.io/library/eclipse-temurin:" + tag + "-jre",
	}
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
