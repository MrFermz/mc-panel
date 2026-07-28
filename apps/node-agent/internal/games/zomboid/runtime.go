// runtime.go — runtime image ของเกมนี้
//
// ต่างจาก Minecraft ตรงที่ไม่มี image สำเร็จรูปจาก upstream ให้ pull (ไม่มีใครแจก image
// ที่มี SteamCMD อย่างเป็นทางการ และเราไม่ใช้ image ของ third-party) — agent จึง build
// เองจาก Dockerfile ที่ definition นี้ถือไว้ แล้ว cache ไว้ใช้ร่วมกันทุก instance บน node
package zomboid

import (
	_ "embed"

	"github.com/game-manager/node-agent/internal/games"
)

// imageName/imageTag = ชื่อ image ใต้ namespace ของ agent — **ต้องตรงกับ control-plane**
// tag เป็นเลขรุ่นของ Dockerfile: แก้ Dockerfile แล้วต้อง bump ที่นี่ + ฝั่ง control-plane
// ไม่งั้น node ที่ build ไปแล้วจะใช้ image เก่าต่อไปตลอด (EnsureRuntimeImage เจอ cache แล้วข้าม)
const (
	imageName = "runtime-steam"
	imageTag  = "1"
)

//go:embed runtime.Dockerfile
var runtimeDockerfile string

// runtimeImage = games.Definition.RuntimeImage — เกมนี้ใช้ image เดียวทุก variant/version
// (เวอร์ชันของเกมมาจาก Steam branch ไม่ได้เปลี่ยน runtime)
func runtimeImage(namespace, _, _ string) string {
	return namespace + "/" + imageName + ":" + imageTag
}

// imagePlatform = สถาปัตยกรรมที่ image นี้ต้องเป็นเสมอ: SteamCMD ของ Valve เป็น binary x86
// อย่างเดียว (ไม่เคยมี build ของ ARM) และ artifact ของเกมที่มันโหลดมาก็เป็น x86_64 —
// node ที่เป็น ARM (Apple Silicon ฯลฯ) ต้อง build/รัน image นี้ผ่าน emulation
// ไม่งั้น `dpkg --add-architecture i386` + apt-get update จะล้มตั้งแต่ตอน build
const imagePlatform = "linux/amd64"

// imageSource = games.Definition.ImageSource — ไม่มี base ให้ pull จึงให้ agent build เอง
func imageSource(string) games.ImageSource {
	return games.ImageSource{Dockerfile: runtimeDockerfile, Platform: imagePlatform}
}
