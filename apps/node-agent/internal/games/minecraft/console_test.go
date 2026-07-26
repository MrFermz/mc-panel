package minecraft

import (
	"testing"

	"github.com/game-manager/node-agent/internal/games"
)

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

// parser ต้อง map บรรทัดจริงของ MC เป็น event ที่ tracker เข้าใจ (ตัว tracker เทสต์แยก
// ที่ internal/gamestate บน parser ตัวนี้)
func TestParseConsoleLineKinds(t *testing.T) {
	cases := []struct {
		line string
		want games.EventKind
	}{
		{`[12:34:56] [Server thread/INFO]: Done (12.345s)! For help, type "help"`, games.EventReady},
		{"[12:34:56] [Server thread/INFO]: There are 1 of a max of 20 players online: CreeperKing", games.EventRoster},
		{"[12:34:56 INFO]: §6TPS from last 1m, 5m, 15m: §a19.98, §a20.0, §a20.0", games.EventMetric},
		{"[12:34:56] [Server thread/INFO]: Unknown or incomplete command, see below for error", games.EventUnknownCommand},
		{"[12:34:56] [Server thread/INFO]: Steve_Builder joined the game", games.EventJoin},
		{"[12:34:56] [Server thread/INFO]: Steve_Builder left the game", games.EventLeave},
		{"[12:34:56] [Server thread/INFO]: Preparing spawn area: 42%", games.EventNone},
	}
	for _, c := range cases {
		if got := parseConsoleLine(c.line).Kind; got != c.want {
			t.Errorf("parseConsoleLine(%q).Kind = %v, want %v", c.line, got, c.want)
		}
	}
}
