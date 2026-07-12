package provision

import (
	"strings"
	"testing"
)

func TestJavaTagForMC(t *testing.T) {
	cases := []struct {
		version string
		want    string
	}{
		{"1.8.9", "8"},
		{"1.12.2", "8"},
		{"1.16.5", "8"},
		{"1.17", "17"},
		{"1.18.2", "17"},
		{"1.20.4", "17"},
		{"1.20.5", "21"},
		{"1.20.6", "21"},
		{"1.21", "21"},
		{"1.21.4", "21"},
		// calendar versioning (26.x…) และ parse ไม่ได้ -> java ใหม่สุด
		{"26.2", "25"},
		{"25.0", "25"},
		{"weird", "25"},
	}
	for _, c := range cases {
		if got := javaTagForMC(c.version); got != c.want {
			t.Errorf("javaTagForMC(%q) = %q ต้องการ %q", c.version, got, c.want)
		}
	}
}

func TestLaunchScript_ExecsJavaLast(t *testing.T) {
	// java ต้องถูก exec เป็นคำสั่งสุดท้ายเสมอเพื่อเป็น PID 1 (รับ stdin/SIGTERM ตรง)
	for _, serverType := range []string{"vanilla", "paper", "fabric", "velocity", "forge"} {
		script := launchScript(serverType)
		if !strings.HasPrefix(script, "#!/bin/sh\n") {
			t.Errorf("%s: launch script ต้องขึ้นต้นด้วย shebang", serverType)
		}
		if !strings.Contains(script, "exec ") {
			t.Errorf("%s: launch script ต้องมี exec", serverType)
		}
		if !strings.Contains(script, "${MC_MEMORY_MB") {
			t.Errorf("%s: launch script ต้องอ่าน MC_MEMORY_MB จาก env", serverType)
		}
	}

	if !strings.Contains(launchScript("velocity"), "velocity.jar") {
		t.Error("velocity ต้องรัน velocity.jar")
	}
	if !strings.Contains(launchScript("paper"), "server.jar nogui") {
		t.Error("paper ต้องรัน server.jar nogui")
	}
	forge := launchScript("forge")
	if !strings.Contains(forge, "run.sh") || !strings.Contains(forge, "forge-*.jar") {
		t.Error("forge ต้องรองรับทั้ง run.sh (ใหม่) และ forge-*.jar (เก่า)")
	}
	if !strings.Contains(forge, "user_jvm_args.txt") {
		t.Error("forge ใหม่ต้องเขียน jvm args ลง user_jvm_args.txt")
	}
}
