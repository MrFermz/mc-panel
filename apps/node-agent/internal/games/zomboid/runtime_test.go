package zomboid

import (
	"strings"
	"testing"
)

// control-plane เป็นคนบอก image ให้ job start_server ส่วน agent เป็นคน build/หา image นั้น
// สองฝั่งคนละ module จึงเทียบตรง ๆ ไม่ได้ — ต้องตรึงค่าที่คาดหวังไว้ทั้งสองฝั่ง
// (ฝั่งนู้นอยู่ที่ internal/games/zomboid/runtime_test.go ของ control-plane)
const controlPlaneImageRef = "game-manager/runtime-steam:1"

func TestRuntimeImageMatchesControlPlane(t *testing.T) {
	if got := runtimeImage("game-manager", "vanilla", "public"); got != controlPlaneImageRef {
		t.Errorf("runtimeImage = %q, want %q", got, controlPlaneImageRef)
	}
	// branch/variant ไม่เปลี่ยน runtime ของเกมนี้ (JVM มากับ artifact ที่ Steam ติดตั้ง)
	if got := runtimeImage("game-manager", "vanilla", "unstable"); got != controlPlaneImageRef {
		t.Errorf("runtimeImage(unstable) = %q, want %q", got, controlPlaneImageRef)
	}
}

// ไม่มี image สำเร็จรูปให้ pull — agent ต้อง build เองเสมอ ไม่งั้น start จะพังบน node ใหม่
func TestImageSourceBuildsLocally(t *testing.T) {
	src := imageSource(controlPlaneImageRef)
	if src.PullFrom != "" {
		t.Errorf("zomboid must not pull a third-party image, got %q", src.PullFrom)
	}
	if !strings.Contains(src.Dockerfile, "steamcmd_linux.tar.gz") {
		t.Error("embedded Dockerfile must install SteamCMD from Valve")
	}
	if !strings.Contains(src.Dockerfile, "lib32gcc-s1") {
		t.Error("embedded Dockerfile must install the 32-bit libs SteamCMD needs")
	}
}
