package filemanager

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// SafeJoin กัน path traversal — clean path, เช็คว่าอยู่ใต้ jailRoot, resolve symlink
// ใช้ทุกครั้งก่อนแตะ filesystem จริงจาก path ที่มาจาก client
//
// path ปลายทางอาจยังไม่มีจริง (เช่นกำลังจะสร้างไฟล์ใหม่) — จึง resolve symlink
// จาก ancestor ที่ลึกที่สุดที่มีอยู่จริงแทน แล้วต่อส่วนที่เหลือกลับเข้าไป
// คืน path ที่ resolve แล้ว ผู้เรียกต้องใช้ path นี้เท่านั้นในการแตะ filesystem
func SafeJoin(jailRoot, userPath string) (string, error) {
	// jail เองอาจเป็น symlink (เช่น /tmp บน mac ชี้ไป /private/tmp) — resolve ก่อน
	// ไม่งั้นเทียบ prefix กับ path ที่ resolve แล้วจะไม่ตรงกันเอง
	jail, err := filepath.EvalSymlinks(jailRoot)
	if err != nil {
		return "", errors.New("jail root not accessible")
	}

	cleaned := filepath.Clean(filepath.Join(jail, userPath))
	if !isWithin(jail, cleaned) {
		return "", errors.New("path traversal detected")
	}

	// หา ancestor ที่ลึกที่สุดที่มีอยู่จริง เก็บ component ที่เหลือไว้ต่อกลับทีหลัง
	existing := cleaned
	var pending []string
	for {
		if _, statErr := os.Lstat(existing); statErr == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		pending = append([]string{filepath.Base(existing)}, pending...)
		existing = parent
	}

	resolvedBase, err := filepath.EvalSymlinks(existing)
	if err != nil {
		// entry มีอยู่แต่ resolve ไม่ได้ (เช่น dangling symlink) — ปฏิเสธไว้ก่อน
		return "", errors.New("cannot resolve path inside jail")
	}
	resolved := filepath.Join(append([]string{resolvedBase}, pending...)...)
	if !isWithin(jail, resolved) {
		return "", errors.New("symlink escapes jail")
	}
	return resolved, nil
}

// isWithin เทียบผ่าน filepath.Rel แล้วดู ".." เป๊ะ ๆ — เช็คด้วย HasPrefix เฉย ๆ
// จะ false positive กับไฟล์ชื่อขึ้นต้นด้วยจุดสองจุด เช่น "..data"
func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
