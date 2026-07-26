package filemanager

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
)

// ErrInstanceNotFound = ไม่มี directory ของ server นี้บน node — ข้อความมีคำว่า
// "not found" เพราะ control-plane map error ของ file op เป็น 404 จาก substring นี้
var ErrInstanceNotFound = errors.New("instance directory not found")

// Layout = ที่อยู่ของ instance บน disk: {GM_DATA_DIR}/{game}/{server_id}
// ชั้นเกมทำให้ข้อมูลของแต่ละเกมไม่ปนกัน แต่ layer อื่นส่วนใหญ่รู้แค่ server id
// (start/stop/file op) จึงต้องหา dir ด้วย Find ที่สแกนชั้นเกมให้
//
// game/server id มาจาก job หรือ label ของ container — ห้ามเชื่อว่าปลอดภัย
// ทุก path ที่คืนออกไปผ่าน SafeJoin แล้วเสมอ
type Layout struct {
	DataDir string
}

// Dir คืน path ของ instance ที่รู้เกมแน่นอนแล้ว (provision) — dir ยังไม่ต้องมีอยู่จริง
func (l Layout) Dir(game, serverID string) (string, error) {
	if !validSegment(game) {
		return "", fmt.Errorf("invalid game id %q", game)
	}
	if !validSegment(serverID) {
		return "", fmt.Errorf("invalid server id %q", serverID)
	}
	dir, err := SafeJoin(l.DataDir, path.Join(game, serverID))
	if err != nil {
		return "", fmt.Errorf("server path validation failed: %w", err)
	}
	return dir, nil
}

// Find หา dir ของ instance จาก server id อย่างเดียว โดยไล่ดูทุกชั้นเกมใต้ DataDir
// ไม่พบ = ErrInstanceNotFound (caller ที่ต้อง idempotent ให้เช็คด้วย errors.Is)
func (l Layout) Find(serverID string) (string, error) {
	if !validSegment(serverID) {
		return "", fmt.Errorf("invalid server id %q", serverID)
	}
	entries, err := os.ReadDir(l.DataDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrInstanceNotFound
		}
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir, err := l.Dir(e.Name(), serverID)
		if err != nil {
			continue
		}
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}
	return "", ErrInstanceNotFound
}

// validSegment กัน path traversal ตั้งแต่ก่อนประกอบ path — 1 segment เท่านั้น
func validSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	return !strings.ContainsAny(s, `/\`) && !strings.ContainsRune(s, 0)
}
