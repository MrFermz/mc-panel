package filemanager

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
)

const (
	// maxFileSize จำกัดขนาดไฟล์ที่ read/write ผ่าน file manager — path นี้ผ่าน gRPC stream
	// ที่ใช้ร่วมกับ realtime data (console/heartbeat) การส่งไฟล์ใหญ่จะไปเบียด channel อื่น
	// read: เกิน limit อ่านแค่ต้นไฟล์แล้ว truncated=true / write: เกิน limit ปฏิเสธทันที
	maxFileSize = 1 << 20 // 1 MiB

	// uid/gid ที่ MC container รัน (User 1000:1000) — ไฟล์ที่ user เขียนผ่าน panel
	// ต้องเป็นของ user นี้ ไม่งั้น process ใน container แก้ต่อไม่ได้
	mcUID = 1000
	mcGID = 1000

	// maxImportSize จำกัดขนาดรวมของไฟล์ staged ที่เขียนแบบ chunk (เช่น import.zip)
	// per-chunk ยังคุมด้วย maxFileSize (1 MiB) — ตัวนี้กันไม่ให้ต่อ chunk ไปเรื่อย ๆ จน disk เต็ม
	maxImportSize = 2 << 30 // 2 GiB
)

// Manager ทำ file operation ต่อ server โดย jail = filepath.Join(mcDataDir, serverID)
// ทุก path จาก client ไม่เชื่อถือ — ต้องผ่าน SafeJoin ก่อนแตะ filesystem เสมอ
type Manager struct {
	mcDataDir string
}

func NewManager(mcDataDir string) *Manager {
	return &Manager{mcDataDir: mcDataDir}
}

// jail คืน root ของ server นี้ — ทุก operation ต้อง SafeJoin(jail, userPath) ก่อนใช้จริง
func (m *Manager) jail(serverID string) string {
	return filepath.Join(m.mcDataDir, serverID)
}

// FileInfo คือผลของ List ต่อ 1 entry — ตรงกับ field ของ agentv1.FileEntry
// (แยก type ไว้เพื่อไม่ให้ package นี้ผูกกับ proto — caller เป็นคนแปลง)
type FileInfo struct {
	Name        string
	IsDir       bool
	Size        int64
	ModTimeUnix int64
}

// List อ่าน dir คืนรายการ entry — dir มาก่อนไฟล์ แล้วเรียงชื่อ a-z ในแต่ละกลุ่ม
// path="" = root ของ server; ถ้า path ชี้ไปไฟล์ (ไม่ใช่ dir) → error
func (m *Manager) List(serverID, path string) ([]FileInfo, error) {
	full, err := SafeJoin(m.jail(serverID), path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, wrapStatErr(err)
	}
	if !info.IsDir() {
		return nil, errors.New("not a directory")
	}
	dirents, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	entries := make([]FileInfo, 0, len(dirents))
	for _, de := range dirents {
		fi, err := de.Info()
		if err != nil {
			// ไฟล์ถูกลบระหว่างอ่าน dir — ข้ามไป ไม่ทำทั้ง list พัง
			continue
		}
		entries = append(entries, FileInfo{
			Name:        de.Name(),
			IsDir:       de.IsDir(),
			Size:        fi.Size(),
			ModTimeUnix: fi.ModTime().Unix(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// Read อ่านไฟล์คืน content + truncated — เกิน maxFileSize อ่านแค่ต้นไฟล์แล้ว truncated=true
// ถ้า path เป็น dir → error
func (m *Manager) Read(serverID, path string) (content []byte, truncated bool, err error) {
	full, err := SafeJoin(m.jail(serverID), path)
	if err != nil {
		return nil, false, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, false, wrapStatErr(err)
	}
	if info.IsDir() {
		return nil, false, errors.New("is a directory")
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	// อ่าน maxFileSize+1 เพื่อตรวจว่ามีเกิน limit จริงไหม (ไม่พึ่ง stat size เผื่อไฟล์โตระหว่างอ่าน)
	buf, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return nil, false, err
	}
	if len(buf) > maxFileSize {
		return buf[:maxFileSize], true, nil
	}
	return buf, false, nil
}

// Write เขียนไฟล์ (สร้าง parent dir ถ้าจำเป็น) แล้ว chown ให้ user 1000 แก้ต่อได้
// content เกิน maxFileSize ปฏิเสธทันที
func (m *Manager) Write(serverID, path string, content []byte) error {
	if len(content) > maxFileSize {
		return fmt.Errorf("file too large: %d bytes exceeds limit of %d", len(content), maxFileSize)
	}
	full, err := SafeJoin(m.jail(serverID), path)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(full); statErr == nil && info.IsDir() {
		return errors.New("is a directory")
	}
	// parent dir ต้องผ่าน SafeJoin ด้วย — Dir ของ full อยู่ใต้ jail อยู่แล้วเพราะ full ผ่านมา
	parent := filepath.Dir(full)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	chownBestEffort(parent)
	if err := os.WriteFile(full, content, 0o644); err != nil {
		return err
	}
	chownBestEffort(full)
	return nil
}

// WriteChunk ต่อไฟล์ staged แบบทีละ chunk (ใช้ stream zip ของ import เข้ามาใน jail)
// first=สร้างใหม่ (truncate) / ไม่ first=append (ไฟล์ต้องมีอยู่แล้ว) / last=chown ปิดท้าย
// เขียนแบบ sequential เท่านั้น เปิด-ปิด handle ทุก call เพื่อความง่ายและปลอดภัย
func (m *Manager) WriteChunk(serverID, path string, content []byte, first, last bool) error {
	if len(content) > maxFileSize {
		return fmt.Errorf("chunk too large: %d bytes exceeds limit of %d", len(content), maxFileSize)
	}
	// import stage zip ก่อนที่ create/import job จะสร้าง server dir — jail root อาจยังไม่มี
	// SafeJoin EvalSymlinks jail root ก่อน จึงต้องสร้าง dir ให้มีก่อน (เฉพาะ chunk แรก)
	if first {
		if err := os.MkdirAll(m.jail(serverID), 0o755); err != nil {
			return err
		}
		chownBestEffort(m.jail(serverID))
	}
	full, err := SafeJoin(m.jail(serverID), path)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(full); statErr == nil && info.IsDir() {
		return errors.New("is a directory")
	}

	var f *os.File
	if first {
		parent := filepath.Dir(full)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
		chownBestEffort(parent)
		f, err = os.OpenFile(full, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
	} else {
		// ไฟล์ต้องถูกสร้างไว้แล้วจาก chunk แรก — append ต่อท้าย
		info, statErr := os.Stat(full)
		if statErr != nil {
			return wrapStatErr(statErr)
		}
		// กัน disk-fill: ขนาดปัจจุบัน + chunk นี้ต้องไม่เกิน maxImportSize
		if info.Size()+int64(len(content)) > maxImportSize {
			return errors.New("import too large")
		}
		f, err = os.OpenFile(full, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
	}

	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if last {
		chownBestEffort(full)
	}
	return nil
}

// Mkdir สร้างโฟลเดอร์ (MkdirAll) แล้ว chown ให้ user 1000
func (m *Manager) Mkdir(serverID, path string) error {
	full, err := SafeJoin(m.jail(serverID), path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(full, 0o755); err != nil {
		return err
	}
	chownBestEffort(full)
	return nil
}

// Delete ลบไฟล์/โฟลเดอร์ (RemoveAll) — ห้ามลบ root ของ server เอง
func (m *Manager) Delete(serverID, path string) error {
	// path ว่าง/"."/"/" = root ของ server — ปฏิเสธก่อน SafeJoin เพราะ SafeJoin คืน jail เอง
	// (SafeJoin ยอมให้ path=="" เพราะ rel=="." อยู่ในขอบเขต) การลบ root จะพัง bind mount
	if isRootPath(path) {
		return errors.New("cannot delete server root")
	}
	full, err := SafeJoin(m.jail(serverID), path)
	if err != nil {
		return err
	}
	// กันเหนียวชั้นสอง: ถ้า resolve แล้วเท่ากับ jail (เช่น symlink ชี้กลับ root) ปฏิเสธ
	jailResolved, err := SafeJoin(m.jail(serverID), "")
	if err != nil {
		return err
	}
	if full == jailResolved {
		return errors.New("cannot delete server root")
	}
	if _, err := os.Lstat(full); err != nil {
		return wrapStatErr(err)
	}
	return os.RemoveAll(full)
}

// Rename ย้าย/เปลี่ยนชื่อ — from และ to ต้องผ่าน SafeJoin ทั้งคู่ (ห้ามย้าย root)
func (m *Manager) Rename(serverID, from, to string) error {
	if isRootPath(from) || isRootPath(to) {
		return errors.New("cannot rename server root")
	}
	fromFull, err := SafeJoin(m.jail(serverID), from)
	if err != nil {
		return err
	}
	toFull, err := SafeJoin(m.jail(serverID), to)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(fromFull); err != nil {
		return wrapStatErr(err)
	}
	if err := os.MkdirAll(filepath.Dir(toFull), 0o755); err != nil {
		return err
	}
	if err := os.Rename(fromFull, toFull); err != nil {
		return err
	}
	chownBestEffort(toFull)
	return nil
}

// isRootPath เช็คว่า userPath ชี้ไป root ของ server (ว่าง/"."/"/") — Clean ให้เทียบได้
func isRootPath(p string) bool {
	c := filepath.Clean("/" + p)
	return c == "/"
}

// wrapStatErr แปลง os error เป็นข้อความชัดให้ control-plane map เป็น code ได้
func wrapStatErr(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("not found")
	}
	return err
}

// chownBestEffort โอน ownership ให้ user 1000 — fail ได้บน dev host ที่ไม่ใช่ root (mac)
// ซึ่งไม่เป็นไรเพราะ Docker Desktop จัดการ ownership ของ bind mount เอง (แค่ warn ไปต่อ)
func chownBestEffort(path string) {
	if err := os.Lchown(path, mcUID, mcGID); err != nil {
		log.Printf("chown %s to %d:%d failed: %v (continuing)", path, mcUID, mcGID, err)
	}
}
