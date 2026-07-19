package mcstate

import (
	"testing"
	"time"
)

// newAttached สร้าง tracker ที่ "attach" server ไว้แล้วโดยไม่เริ่ม poll loop
// (poll ยิงคำสั่งจริงผ่าน writer — เทสต์สนใจแค่การอ่านบรรทัด)
func newAttached(id string) (*Tracker, *serverState) {
	t := NewTracker()
	st := &serverState{online: make(map[string]struct{}), stop: make(chan struct{})}
	t.servers[id] = st
	return t, st
}

func TestObserveLineJoinLeave(t *testing.T) {
	tr, _ := newAttached("s1")

	tr.ObserveLine("s1", "[12:34:56] [Server thread/INFO]: Steve_Builder joined the game")
	tr.ObserveLine("s1", "[12:34:57] [Server thread/INFO]: zBlazeQueen joined the game")
	if got := tr.Snapshot("s1").Online; len(got) != 2 {
		t.Fatalf("want 2 online, got %v", got)
	}

	tr.ObserveLine("s1", "[12:35:00] [Server thread/INFO]: Steve_Builder left the game")
	got := tr.Snapshot("s1").Online
	if len(got) != 1 || got[0] != "zBlazeQueen" {
		t.Fatalf("want [zBlazeQueen], got %v", got)
	}
}

// บรรทัดแชท/plugin ที่ลงท้ายเหมือนกันต้องไม่ถูกนับเป็นผู้เล่น
func TestObserveLineIgnoresNonPlayerLines(t *testing.T) {
	tr, _ := newAttached("s1")
	for _, line := range []string{
		"[12:34:56] [Server thread/INFO]: <Steve> a friend joined the game",
		"[12:34:56] [Server thread/INFO]: a name far longer than any real minecraft account joined the game",
	} {
		tr.ObserveLine("s1", line)
	}
	if got := tr.Snapshot("s1").Online; len(got) != 0 {
		t.Fatalf("want nobody online, got %v", got)
	}
}

func TestObserveLineListReply(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		wantNames int
		wantMax   int
	}{
		{
			name:      "vanilla with players",
			line:      "[12:34:56] [Server thread/INFO]: There are 3 of a max of 20 players online: Notch_Fan22, Steve_Builder, CreeperKing",
			wantNames: 3,
			wantMax:   20,
		},
		{
			name:      "vanilla empty",
			line:      "[12:34:56] [Server thread/INFO]: There are 0 of a max of 20 players online:",
			wantNames: 0,
			wantMax:   20,
		},
		{
			name:      "slash form",
			line:      "[12:34:56 INFO]: There are 2/50 players online: Alex_the_Alchemist, zBlazeQueen",
			wantNames: 2,
			wantMax:   50,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr, _ := newAttached("s1")
			tr.ObserveLine("s1", tc.line)
			snap := tr.Snapshot("s1")
			if len(snap.Online) != tc.wantNames {
				t.Fatalf("names: want %d, got %v", tc.wantNames, snap.Online)
			}
			if snap.MaxPlayers != tc.wantMax {
				t.Fatalf("max: want %d, got %d", tc.wantMax, snap.MaxPlayers)
			}
		})
	}
}

// `list` เป็น source of truth — ต้องทับ set เดิมทั้งชุด ไม่ใช่ merge
func TestListReplyReplacesOnlineSet(t *testing.T) {
	tr, _ := newAttached("s1")
	tr.ObserveLine("s1", "[12:00:00] [Server thread/INFO]: GhostPlayer joined the game")
	tr.ObserveLine("s1", "[12:00:01] [Server thread/INFO]: There are 1 of a max of 20 players online: CreeperKing")

	got := tr.Snapshot("s1").Online
	if len(got) != 1 || got[0] != "CreeperKing" {
		t.Fatalf("want [CreeperKing], got %v", got)
	}
}

func TestObserveLineTPSPaper(t *testing.T) {
	tr, st := newAttached("s1")
	st.tpsSentAt = time.Now()

	// paper แทรก color code — ต้องถูกตัดก่อน parse
	shown := tr.ObserveLine("s1", "[12:34:56 INFO]: §6TPS from last 1m, 5m, 15m: §a19.98, §a20.0, §a20.0")
	if shown {
		t.Error("reply to a tracker-issued command must be hidden from the user's console")
	}
	if tps := tr.Snapshot("s1").TPS; tps != 19.98 {
		t.Fatalf("want tps 19.98, got %v", tps)
	}
	if st.tpsUnsupported {
		t.Error("paper supports tps — should not be marked unsupported")
	}
}

// vanilla ไม่มีคำสั่ง tps — probe ครั้งเดียวแล้วต้องเลิกถามถาวร
func TestUnknownCommandMarksTPSUnsupported(t *testing.T) {
	tr, st := newAttached("s1")
	st.tpsSentAt = time.Now()

	shown := tr.ObserveLine("s1", "[12:34:56] [Server thread/INFO]: Unknown or incomplete command, see below for error")
	if shown {
		t.Error("probe error must not appear in the user's console")
	}
	if !st.tpsUnsupported {
		t.Fatal("want tpsUnsupported after unknown command")
	}
	if tps := tr.Snapshot("s1").TPS; tps != 0 {
		t.Fatalf("want tps 0 for unsupported, got %v", tps)
	}
}

// user พิมพ์ `list` เองต้องเห็น reply ตามปกติ (นอกหน้าต่างของคำสั่งที่ tracker ยิง)
func TestUserTypedListReplyStaysVisible(t *testing.T) {
	tr, st := newAttached("s1")
	st.listSentAt = time.Now().Add(-time.Minute)

	shown := tr.ObserveLine("s1", "[12:34:56] [Server thread/INFO]: There are 1 of a max of 20 players online: CreeperKing")
	if !shown {
		t.Error("reply to a user-issued command must not be hidden")
	}
}

func TestSnapshotUnknownServer(t *testing.T) {
	tr := NewTracker()
	if snap := tr.Snapshot("nope"); len(snap.Online) != 0 || snap.TPS != 0 || snap.MaxPlayers != 0 {
		t.Fatalf("want zero snapshot, got %+v", snap)
	}
}

// detach = session จบ — สถานะต้องหายไป ไม่ค้างให้ dashboard โชว์ผู้เล่นของรอบที่แล้ว
func TestOnDetachClearsState(t *testing.T) {
	tr, _ := newAttached("s1")
	tr.ObserveLine("s1", "[12:00:00] [Server thread/INFO]: CreeperKing joined the game")
	tr.OnDetach("s1")

	if got := tr.Snapshot("s1").Online; len(got) != 0 {
		t.Fatalf("want empty after detach, got %v", got)
	}
}

func TestMessageOf(t *testing.T) {
	cases := map[string]string{
		"[12:34:56] [Server thread/INFO]: hello": "hello",
		"[12:34:56 INFO]: hello":                 "hello",
		"no prefix at all":                       "no prefix at all",
	}
	for in, want := range cases {
		if got := messageOf(in); got != want {
			t.Errorf("messageOf(%q) = %q, want %q", in, got, want)
		}
	}
}

// regression: console.Manager ต้องเรียก OnAttach ก่อนสตาร์ท pump เสมอ
// ถ้า container ตายทันที (attach ผ่านแต่ EOF เลย) ลำดับกลับด้านจะทำให้ OnDetach
// วิ่งตอน tracker ยังไม่รู้จัก server → state ที่ OnAttach สร้างทีหลังไม่มีใครล้าง
// (poll loop ค้างถาวร + รายชื่อผู้เล่นค้างของ session ที่ตายไปแล้ว)
func TestAttachThenImmediateDetachLeavesNoState(t *testing.T) {
	tr := NewTracker()

	tr.OnAttach("s1")
	tr.ObserveLine("s1", "[12:00:00] [Server thread/INFO]: CreeperKing joined the game")
	tr.OnDetach("s1")

	if got := tr.Snapshot("s1"); len(got.Online) != 0 {
		t.Fatalf("state left over after detach: %v", got.Online)
	}
	// attach รอบใหม่ต้องสร้าง state ได้อีก (ของเก่าถูกลบออกจาก map จริง)
	tr.OnAttach("s1")
	tr.ObserveLine("s1", "[12:00:10] [Server thread/INFO]: Steve_Builder joined the game")
	got := tr.Snapshot("s1").Online
	if len(got) != 1 || got[0] != "Steve_Builder" {
		t.Fatalf("re-attach did not work: %v", got)
	}
	tr.OnDetach("s1")
}

// server ที่เคยรายงาน TPS ได้แล้ว (paper) ห้ามถูกปิดเพราะ user พิมพ์คำสั่งผิด
// ในช่วง reply window ของ probe — ไม่งั้น TPS หายไปทั้ง session
func TestUserTypoDoesNotDisableTPSOnPaper(t *testing.T) {
	tr, st := newAttached("s1")
	st.tpsSentAt = time.Now()
	tr.ObserveLine("s1", "[12:00:00 INFO]: TPS from last 1m, 5m, 15m: 19.98, 20.0, 20.0")

	st.tpsSentAt = time.Now()
	tr.ObserveLine("s1", "[12:00:01] [Server thread/INFO]: Unknown or incomplete command, see below for error")

	if st.tpsUnsupported {
		t.Fatal("a server that previously reported TPS should not be marked unsupported")
	}
	if tps := tr.Snapshot("s1").TPS; tps != 19.98 {
		t.Fatalf("want tps 19.98, got %v", tps)
	}
}
