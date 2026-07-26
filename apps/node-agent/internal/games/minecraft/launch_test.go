package minecraft

import (
	"strings"
	"testing"
)

func TestJavaTagFor(t *testing.T) {
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
		if got := javaTagFor("vanilla", c.version); got != c.want {
			t.Errorf("javaTagFor(%q) = %q, want %q", c.version, got, c.want)
		}
	}
	// velocity ไม่ผูกกับเวอร์ชัน MC — ใช้ Java ใหม่สุดเสมอ (ตรงกับ control-plane)
	if got := javaTagFor("velocity", "3.4.0"); got != latestJavaTag {
		t.Errorf("javaTagFor(velocity) = %q, want %q", got, latestJavaTag)
	}
}

func TestLaunchScript_ExecsJavaLast(t *testing.T) {
	// java ต้องถูก exec เป็นคำสั่งสุดท้ายเสมอเพื่อเป็น PID 1 (รับ stdin/SIGTERM ตรง)
	for _, variant := range []string{"vanilla", "paper", "fabric", "velocity", "forge"} {
		script := launchScript(variant)
		if !strings.HasPrefix(script, "#!/bin/sh\n") {
			t.Errorf("%s: launch script must start with a shebang", variant)
		}
		if !strings.Contains(script, "exec ") {
			t.Errorf("%s: launch script must contain exec", variant)
		}
		if !strings.Contains(script, "${GM_MEMORY_MB") {
			t.Errorf("%s: launch script must read GM_MEMORY_MB from env", variant)
		}
	}

	if !strings.Contains(launchScript("velocity"), "velocity.jar") {
		t.Error("velocity must run velocity.jar")
	}
	if !strings.Contains(launchScript("paper"), "server.jar nogui") {
		t.Error("paper must run server.jar nogui")
	}
	forge := launchScript("forge")
	if !strings.Contains(forge, "run.sh") || !strings.Contains(forge, "forge-*.jar") {
		t.Error("forge must support both run.sh (new) and forge-*.jar (old)")
	}
	if !strings.Contains(forge, "user_jvm_args.txt") {
		t.Error("new forge must write jvm args to user_jvm_args.txt")
	}
}
