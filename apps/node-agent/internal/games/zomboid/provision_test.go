package zomboid

import (
	"strings"
	"testing"
)

// branch เดินทางไปเป็น argv ของ steamcmd — allow-list คือด่านเดียวที่กันค่าจาก NATS
// (control-plane validate มาแล้ว แต่ agent ห้ามเชื่อ payload)
func TestBranchAllowList(t *testing.T) {
	for _, ok := range []string{"public", "unstable"} {
		if !branches[ok] {
			t.Errorf("branch %q should be allowed", ok)
		}
	}
	for _, bad := range []string{"", "PUBLIC", "public;rm -rf /", "-beta", "../etc"} {
		if branches[bad] {
			t.Errorf("branch %q must not be allowed", bad)
		}
	}
}

func TestSteamCmdArgs(t *testing.T) {
	got := strings.Join(steamCmdArgs("public"), " ")
	// public เป็น branch ปกติ — ห้ามส่ง -beta ไปด้วย
	if strings.Contains(got, "-beta") {
		t.Errorf("public branch must not pass -beta: %s", got)
	}
	for _, want := range []string{"+login anonymous", "+app_update 380870", "+force_install_dir /data", "validate", "+quit"} {
		if !strings.Contains(got, want) {
			t.Errorf("steamCmdArgs(public) missing %q: %s", want, got)
		}
	}

	got = strings.Join(steamCmdArgs("unstable"), " ")
	if !strings.Contains(got, "-beta unstable") {
		t.Errorf("steamCmdArgs(unstable) missing -beta: %s", got)
	}
}

// ห้ามมี credential ของ Steam โผล่ในคำสั่งไม่ว่าทางไหน — panel ไม่รับ login ของใครทั้งนั้น
func TestSteamCmdNeverLogsIn(t *testing.T) {
	for _, branch := range []string{"public", "unstable"} {
		args := steamCmdArgs(branch)
		for i, a := range args {
			if a == "+login" && args[i+1] != "anonymous" {
				t.Fatalf("steamcmd must only log in anonymously, got %q", args[i+1])
			}
		}
	}
}

func TestHeapMB(t *testing.T) {
	tests := map[int]int{
		2048:  1366, // กันไว้ 1/3
		1024:  683,
		512:   256,  // floor 256 แต่ต้องไม่เกินครึ่ง
		8192:  6144, // เพดาน reserve 2GB
		16384: 14336,
	}
	for limit, want := range tests {
		if got := heapMB(limit); got != want {
			t.Errorf("heapMB(%d) = %d, want %d", limit, got, want)
		}
		if heapMB(limit) >= limit {
			t.Errorf("heapMB(%d) must leave room for non-heap memory", limit)
		}
	}
}
