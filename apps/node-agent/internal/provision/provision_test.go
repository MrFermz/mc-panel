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
			t.Errorf("javaTagForMC(%q) = %q, want %q", c.version, got, c.want)
		}
	}
}

func TestLaunchScript_ExecsJavaLast(t *testing.T) {
	// java ต้องถูก exec เป็นคำสั่งสุดท้ายเสมอเพื่อเป็น PID 1 (รับ stdin/SIGTERM ตรง)
	for _, serverType := range []string{"vanilla", "paper", "fabric", "velocity", "forge"} {
		script := launchScript(serverType)
		if !strings.HasPrefix(script, "#!/bin/sh\n") {
			t.Errorf("%s: launch script must start with a shebang", serverType)
		}
		if !strings.Contains(script, "exec ") {
			t.Errorf("%s: launch script must contain exec", serverType)
		}
		if !strings.Contains(script, "${MC_MEMORY_MB") {
			t.Errorf("%s: launch script must read MC_MEMORY_MB from env", serverType)
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
