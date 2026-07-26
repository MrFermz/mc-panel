package filemanager

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLayoutDirPutsInstanceUnderGame(t *testing.T) {
	dataDir := t.TempDir()
	l := Layout{DataDir: dataDir}

	got, err := l.Dir("minecraft", "srv-1")
	if err != nil {
		t.Fatalf("dir: %v", err)
	}
	want := filepath.Join(dataDir, "minecraft", "srv-1")
	// t.TempDir บน mac อยู่ใต้ symlink /var → SafeJoin คืน path ที่ resolve แล้ว
	if resolved, err := filepath.EvalSymlinks(dataDir); err == nil {
		want = filepath.Join(resolved, "minecraft", "srv-1")
	}
	if got != want {
		t.Fatalf("dir = %q, want %q", got, want)
	}
}

func TestLayoutDirRejectsTraversal(t *testing.T) {
	l := Layout{DataDir: t.TempDir()}
	cases := []struct{ game, id string }{
		{"minecraft", ".."},
		{"minecraft", "../escape"},
		{"minecraft", ""},
		{"..", "srv-1"},
		{"mine/craft", "srv-1"},
		{"minecraft", `srv\1`},
	}
	for _, c := range cases {
		if _, err := l.Dir(c.game, c.id); err == nil {
			t.Errorf("dir(%q, %q) = nil error, want rejection", c.game, c.id)
		}
	}
}

func TestLayoutFindScansGameDirs(t *testing.T) {
	dataDir := t.TempDir()
	l := Layout{DataDir: dataDir}
	jail := filepath.Join(dataDir, "othergame", "srv-2")
	if err := os.MkdirAll(jail, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "minecraft"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := l.Find("srv-2")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(jail); err == nil {
		jail = resolved
	}
	if got != jail {
		t.Fatalf("find = %q, want %q", got, jail)
	}
}

func TestLayoutFindMissingInstance(t *testing.T) {
	dataDir := t.TempDir()
	l := Layout{DataDir: dataDir}

	// ยังไม่มี dir ชั้นเกมเลย (node ที่เพิ่ง boot)
	if _, err := l.Find("srv-1"); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("find on empty data dir = %v, want ErrInstanceNotFound", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "minecraft"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Find("srv-1"); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("find = %v, want ErrInstanceNotFound", err)
	}
}

func TestManagerRejectsUnknownServer(t *testing.T) {
	m, _ := newTestManager(t)
	if _, err := m.List("srv-does-not-exist", ""); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("list unknown server = %v, want ErrInstanceNotFound", err)
	}
}
