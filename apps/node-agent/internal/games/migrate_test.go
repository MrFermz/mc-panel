package games

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/game-manager/node-agent/internal/filemanager"
)

// writeLegacyInstance สร้าง instance ตาม layout เดิม ({DataDir}/{server_id})
func writeLegacyInstance(t *testing.T, dataDir, serverID, game string) {
	t.Helper()
	dir := filepath.Join(dataDir, serverID)
	if err := os.MkdirAll(filepath.Join(dir, PanelDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteInstanceMeta(dir, InstanceMeta{Game: game, Variant: "vanilla"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server.properties"), []byte("motd=hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateLegacyLayout(t *testing.T) {
	dataDir := t.TempDir()
	layout := filemanager.Layout{DataDir: dataDir}
	reg := NewRegistry(&Definition{ID: DefaultID})

	writeLegacyInstance(t, dataDir, "srv-1", DefaultID)
	// instance ที่ provision ไว้ก่อนมี field `game` ใน meta.json → ต้องตกไปเกม default
	writeLegacyInstance(t, dataDir, "srv-2", "")
	// dir ที่ไม่ใช่ของ panel (ไม่มี meta.json) ต้องไม่ถูกแตะ
	if err := os.MkdirAll(filepath.Join(dataDir, "not-an-instance"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyLayout(layout, reg); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, id := range []string{"srv-1", "srv-2"} {
		dir, err := layout.Find(id)
		if err != nil {
			t.Fatalf("find %s after migrate: %v", id, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "server.properties")); err != nil {
			t.Errorf("%s: data file missing after migrate: %v", id, err)
		}
		if _, err := os.Stat(filepath.Join(dataDir, id)); !os.IsNotExist(err) {
			t.Errorf("%s: legacy directory still exists", id)
		}
	}
	if _, err := os.Stat(filepath.Join(dataDir, "not-an-instance")); err != nil {
		t.Errorf("unrelated directory was moved: %v", err)
	}

	// รันซ้ำต้องไม่พังและไม่ย้ายอะไรอีก (job/boot ซ้ำได้เสมอ)
	if err := MigrateLegacyLayout(layout, reg); err != nil {
		t.Fatalf("migrate twice: %v", err)
	}
	if _, err := layout.Find("srv-1"); err != nil {
		t.Fatalf("find after second migrate: %v", err)
	}
}

func TestMigrateLegacySkipsUnknownGame(t *testing.T) {
	dataDir := t.TempDir()
	layout := filemanager.Layout{DataDir: dataDir}
	reg := NewRegistry(&Definition{ID: DefaultID})

	writeLegacyInstance(t, dataDir, "srv-9", "unknown-game")
	if err := MigrateLegacyLayout(layout, reg); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "srv-9")); err != nil {
		t.Fatalf("instance of unknown game must be left in place: %v", err)
	}
}
