package filemanager

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testServerID = "srv-1"

// newTestManager สร้าง mcDataDir ชั่วคราว + root ของ server ให้พร้อมใช้
func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dataDir := t.TempDir()
	jail := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(jail, 0o755); err != nil {
		t.Fatalf("mkdir jail: %v", err)
	}
	return NewManager(dataDir), jail
}

func TestListOrdersDirsFirstThenAlpha(t *testing.T) {
	m, jail := newTestManager(t)
	mustWrite(t, filepath.Join(jail, "b.txt"), "b")
	mustWrite(t, filepath.Join(jail, "a.txt"), "a")
	if err := os.Mkdir(filepath.Join(jail, "zdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(jail, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries, err := m.List(testServerID, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := make([]string, len(entries))
	for i, e := range entries {
		got[i] = e.Name
	}
	want := []string{"adir", "zdir", "a.txt", "b.txt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestListOnFileErrors(t *testing.T) {
	m, jail := newTestManager(t)
	mustWrite(t, filepath.Join(jail, "f.txt"), "x")
	if _, err := m.List(testServerID, "f.txt"); err == nil {
		t.Fatal("expected error listing a file")
	}
}

func TestListTraversalRejected(t *testing.T) {
	m, _ := newTestManager(t)
	if _, err := m.List(testServerID, "../../etc"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestReadTruncates(t *testing.T) {
	m, jail := newTestManager(t)
	big := bytes.Repeat([]byte("A"), maxFileSize+100)
	mustWriteBytes(t, filepath.Join(jail, "big.bin"), big)

	content, truncated, err := m.Read(testServerID, "big.bin")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncated=true")
	}
	if len(content) != maxFileSize {
		t.Fatalf("content len = %d, want %d", len(content), maxFileSize)
	}
}

func TestReadSmallFile(t *testing.T) {
	m, jail := newTestManager(t)
	mustWrite(t, filepath.Join(jail, "s.txt"), "hello")
	content, truncated, err := m.Read(testServerID, "s.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if truncated || string(content) != "hello" {
		t.Fatalf("got %q truncated=%v", content, truncated)
	}
}

func TestReadDirErrors(t *testing.T) {
	m, jail := newTestManager(t)
	if err := os.Mkdir(filepath.Join(jail, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Read(testServerID, "d"); err == nil {
		t.Fatal("expected error reading a directory")
	}
}

func TestReadNotFound(t *testing.T) {
	m, _ := newTestManager(t)
	if _, _, err := m.Read(testServerID, "nope.txt"); err == nil || err.Error() != "not found" {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestWriteCreatesParent(t *testing.T) {
	m, jail := newTestManager(t)
	if err := m.Write(testServerID, "config/sub/server.properties", []byte("k=v")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(jail, "config", "sub", "server.properties"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "k=v" {
		t.Fatalf("content = %q", got)
	}
}

func TestWriteTooLarge(t *testing.T) {
	m, _ := newTestManager(t)
	big := bytes.Repeat([]byte("A"), maxFileSize+1)
	if err := m.Write(testServerID, "x.bin", big); err == nil {
		t.Fatal("expected too large error")
	}
}

func TestMkdir(t *testing.T) {
	m, jail := newTestManager(t)
	if err := m.Mkdir(testServerID, "plugins/data"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	info, err := os.Stat(filepath.Join(jail, "plugins", "data"))
	if err != nil || !info.IsDir() {
		t.Fatalf("dir not created: %v", err)
	}
}

func TestDeleteRootRejected(t *testing.T) {
	m, jail := newTestManager(t)
	for _, p := range []string{"", ".", "/"} {
		if err := m.Delete(testServerID, p); err == nil {
			t.Fatalf("expected root delete %q to be rejected", p)
		}
	}
	// root ต้องยังอยู่
	if _, err := os.Stat(jail); err != nil {
		t.Fatalf("root was removed: %v", err)
	}
}

func TestDelete(t *testing.T) {
	m, jail := newTestManager(t)
	mustWrite(t, filepath.Join(jail, "gone.txt"), "x")
	if err := m.Delete(testServerID, "gone.txt"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(jail, "gone.txt")); !os.IsNotExist(err) {
		t.Fatal("file still exists")
	}
}

func TestDeleteNotFound(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.Delete(testServerID, "nope.txt"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestRename(t *testing.T) {
	m, jail := newTestManager(t)
	mustWrite(t, filepath.Join(jail, "old.txt"), "data")
	if err := m.Rename(testServerID, "old.txt", "renamed/new.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(jail, "renamed", "new.txt"))
	if err != nil || string(got) != "data" {
		t.Fatalf("rename result wrong: %v %q", err, got)
	}
}

func TestRenameRootRejected(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.Rename(testServerID, "", "x"); err == nil {
		t.Fatal("expected root rename rejected")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustWriteBytes(t, path, []byte(content))
}

func mustWriteBytes(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
