package zomboid

import "testing"

// อีกครึ่งของสัญญานี้อยู่ที่ internal/games/zomboid/runtime_test.go ของ node-agent
// (คนละ module จึง import ข้ามไม่ได้ ต้องตรึงค่าเดียวกันไว้ทั้งสองฝั่ง)
const agentImageRef = "game-manager/runtime-steam:1"

func TestRuntimeImageMatchesAgent(t *testing.T) {
	if got := RuntimeImage("vanilla", "public"); got != agentImageRef {
		t.Errorf("RuntimeImage = %q, want %q", got, agentImageRef)
	}
}

// branch เดินทางไปเป็น argv ของ steamcmd ฝั่ง agent — allow-list ต้องตรงกันสองฝั่ง
func TestValidBranch(t *testing.T) {
	for _, ok := range []string{"public", "unstable"} {
		if !validBranch("vanilla", ok) {
			t.Errorf("branch %q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "beta", "public ", "PUBLIC", "public;id"} {
		if validBranch("vanilla", bad) {
			t.Errorf("branch %q must be rejected", bad)
		}
	}
}
