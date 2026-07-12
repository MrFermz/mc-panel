package filemanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeJoin_BlocksTraversal(t *testing.T) {
	jail := t.TempDir()

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"normal file", "server.jar", false},
		{"nested dir", "plugins/example.jar", false},
		{"simple traversal", "../../../etc/passwd", true},
		{"traversal after clean", "plugins/../../secret", true},
		// ชื่อไฟล์ขึ้นต้นด้วย ".." เป็น path ปกติ ห้ามโดนตีเป็น traversal
		{"file named ..data", "..data", false},
		{"nested file named ..data", "plugins/..data", false},
		// path ที่ยังไม่มีจริง (จะสร้างใหม่) ต้องผ่านได้
		{"nonexistent nested path", "world/region/r.0.0.mca", false},
		{"jail root itself", ".", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := SafeJoin(jail, c.input)
			if c.wantErr && err == nil {
				t.Errorf("คาดว่าจะ error แต่ไม่ error สำหรับ input: %s", c.input)
			}
			if !c.wantErr && err != nil {
				t.Errorf("ไม่คาดว่าจะ error แต่ error: %v", err)
			}
		})
	}
}

func TestSafeJoin_ReturnsResolvedPathInsideJail(t *testing.T) {
	jail := t.TempDir()

	got, err := SafeJoin(jail, "newdir/newfile.txt")
	if err != nil {
		t.Fatalf("ไม่คาดว่าจะ error: %v", err)
	}
	// jail จาก t.TempDir บน mac อยู่ใต้ /var ที่เป็น symlink — ผลลัพธ์ต้องอยู่ใต้ jail ที่ resolve แล้ว
	resolvedJail, err := filepath.EvalSymlinks(jail)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(resolvedJail, "newdir", "newfile.txt")
	if got != want {
		t.Errorf("ได้ %q ต้องการ %q", got, want)
	}
}

func TestSafeJoin_BlocksSymlinkEscape(t *testing.T) {
	jail := t.TempDir()
	outside := t.TempDir()

	// symlink อยู่ใน jail แต่ชี้ออกไปข้างนอก
	link := filepath.Join(jail, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	t.Run("symlink itself", func(t *testing.T) {
		if _, err := SafeJoin(jail, "link"); err == nil {
			t.Error("คาดว่าจะ block symlink ที่ชี้ออกนอก jail")
		}
	})

	// ช่องโหว่เดิม: ไฟล์ปลายทางยังไม่มีจริง EvalSymlinks ทั้งเส้นจะ fail
	// แล้วโค้ดเก่าปล่อยผ่านโดยไม่เช็ค symlink ของ ancestor เลย
	t.Run("new file under escaping symlink", func(t *testing.T) {
		if _, err := SafeJoin(jail, "link/newfile.txt"); err == nil {
			t.Error("คาดว่าจะ block การสร้างไฟล์ใหม่ผ่าน symlink ที่ชี้ออกนอก jail")
		}
	})

	t.Run("existing file under escaping symlink", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := SafeJoin(jail, "link/secret.txt"); err == nil {
			t.Error("คาดว่าจะ block การอ่านไฟล์ผ่าน symlink ที่ชี้ออกนอก jail")
		}
	})
}

func TestSafeJoin_BlocksNestedSymlinkEscape(t *testing.T) {
	jail := t.TempDir()
	outside := t.TempDir()

	// symlink ซ้อน symlink: inner -> middle -> นอก jail
	middle := filepath.Join(jail, "middle")
	if err := os.Symlink(outside, middle); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(jail, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(sub, "inner")
	if err := os.Symlink(middle, inner); err != nil {
		t.Fatal(err)
	}

	if _, err := SafeJoin(jail, "sub/inner/newfile.txt"); err == nil {
		t.Error("คาดว่าจะ block nested symlink ที่สุดท้ายชี้ออกนอก jail")
	}
}

func TestSafeJoin_AllowsSymlinkInsideJail(t *testing.T) {
	jail := t.TempDir()

	target := filepath.Join(jail, "real")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(jail, "alias")); err != nil {
		t.Fatal(err)
	}

	got, err := SafeJoin(jail, "alias/file.txt")
	if err != nil {
		t.Fatalf("symlink ที่ชี้อยู่ใน jail ต้องผ่านได้: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("real", "file.txt")) {
		t.Errorf("ต้องคืน path ที่ resolve แล้ว ได้ %q", got)
	}
}
