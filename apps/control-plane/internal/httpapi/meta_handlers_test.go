package httpapi

import (
	"testing"

	"github.com/game-manager/control-plane/internal/games"
	"github.com/game-manager/control-plane/internal/store"
)

// เกมสมมติสองตัว: กิน port เดียว กับกิน 2 port ติดกัน — ไม่ผูกกับเกมจริง
// (กติกา "port ที่ suggest ต้องเว้นช่วงให้พอ" เป็นของ handler ไม่ใช่ของเกมใดเกมหนึ่ง)
func testRegistry() *games.Registry {
	return games.NewRegistry(
		&games.Definition{ID: "single", DefaultHostPort: 100},
		&games.Definition{ID: "double", DefaultHostPort: 200, HostPortSpan: 2},
	)
}

func TestNextFreeHostPort(t *testing.T) {
	a := &API{games: testRegistry()}
	reg := testRegistry()
	single, _ := reg.Get("single")
	double, _ := reg.Get("double")

	tests := []struct {
		name  string
		def   *games.Definition
		usage []store.HostPortUsage
		want  int
	}{
		{"empty node", single, nil, 100},
		{"skips taken", single, []store.HostPortUsage{{Port: 100, Game: "single"}}, 101},
		{
			// instance ของเกมที่กิน 2 port จองทั้ง 200 และ 201 ทั้งที่ DB เก็บแค่ 200
			name:  "multi-port instance reserves the next port too",
			def:   double,
			usage: []store.HostPortUsage{{Port: 200, Game: "double"}},
			want:  202,
		},
		{
			// เกมที่กินช่วงต้องไม่เลือกช่วงที่คร่อม port ของเกมอื่น
			name:  "span must fit entirely",
			def:   double,
			usage: []store.HostPortUsage{{Port: 201, Game: "single"}},
			want:  202,
		},
		{
			// เกมที่กิน port เดียวต้องไม่ไปนั่งทับ port รองของ instance ที่กินช่วง
			name:  "single-port game avoids a reserved second port",
			def:   single,
			usage: []store.HostPortUsage{{Port: 100, Game: "double"}},
			want:  102,
		},
	}

	for _, tt := range tests {
		if got := a.nextFreeHostPort(tt.def, tt.usage); got != tt.want {
			t.Errorf("%s: nextFreeHostPort = %d, want %d", tt.name, got, tt.want)
		}
	}
}
