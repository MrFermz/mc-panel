package httpapi

import (
	"slices"
	"testing"

	"github.com/game-manager/control-plane/internal/games"
	"github.com/game-manager/control-plane/internal/store"
)

// catalog ต้องโตตาม registry เอง — เพิ่มเกมใหม่แล้วต้องได้ capability ของเกมนั้น
// โดยไม่ต้องไปแก้ baseCapabilityCatalog (ดู CLAUDE.md กฎข้อ 0)
func TestCapabilityCatalogIncludesGames(t *testing.T) {
	a := &API{games: games.NewRegistry(
		&games.Definition{ID: "alpha", Label: "Alpha"},
		&games.Definition{ID: "beta", Label: "Beta"},
	)}

	catalog := a.capabilityCatalog()
	if len(catalog) != len(baseCapabilityCatalog)+2 {
		t.Fatalf("catalog size = %d, want %d", len(catalog), len(baseCapabilityCatalog)+2)
	}

	idx := slices.IndexFunc(catalog, func(c capabilityMeta) bool { return c.Key == "games.alpha" })
	if idx < 0 {
		t.Fatal("games.alpha missing from catalog")
	}
	if got := catalog[idx].Group; got != capGamesGroup {
		t.Errorf("group = %q, want %q", got, capGamesGroup)
	}
	if got := catalog[idx].Action; got != "alpha" {
		t.Errorf("action = %q, want the game id", got)
	}

	// key ของเกมที่ไม่ได้ลงทะเบียนต้องถูกปฏิเสธเหมือน key มั่ว ๆ ทั่วไป
	if a.validateCapabilities([]string{"games.gamma"}) {
		t.Error("games.gamma accepted although the game is not registered")
	}
	if !a.validateCapabilities([]string{"games.beta", capServersCreate}) {
		t.Error("valid capability keys rejected")
	}
}

// สิทธิ์ต่อเกมเป็น global-only เหมือน servers.create — grant ในชั้น server ไม่ได้
func TestGameCapabilityIsNotServerScoped(t *testing.T) {
	if validateServerCapabilities([]string{gameCapKey("alpha")}) {
		t.Error("games.* must not be grantable per server")
	}
}

func TestHasGameCapability(t *testing.T) {
	admin := &store.User{IsAdmin: true}
	if !hasCapability(admin, gameCapKey("alpha")) {
		t.Error("admin must pass every game capability")
	}
	user := &store.User{Capabilities: []string{capServersCreate, gameCapKey("alpha")}}
	if !hasCapability(user, gameCapKey("alpha")) {
		t.Error("granted game capability not detected")
	}
	if hasCapability(user, gameCapKey("beta")) {
		t.Error("ungranted game capability must not pass")
	}
}
