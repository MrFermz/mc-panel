// migrate.go — ย้าย instance จาก layout เดิม ({GM_DATA_DIR}/{server_id})
// มาเป็น {GM_DATA_DIR}/{game}/{server_id}
//
// disk layout ไม่มีระบบ migration เหมือน DB — ทำเป็น step เดียวตอน agent boot
// แล้วจบ (dir ที่ย้ายไปแล้วไม่เข้าเงื่อนไขอีก จึง idempotent โดยตัวมันเอง)
// ต้องรันตอน boot เท่านั้น เพราะ container ที่กำลังรัน bind path เดิมค้างไว้
// จนกว่าจะถูกสร้างใหม่ — ย้ายระหว่างรันจะทำให้เกมเขียนลง dir ที่ไม่มีใครเห็นแล้ว
package games

import (
	"log"
	"os"
	"path/filepath"

	"github.com/game-manager/node-agent/internal/filemanager"
)

// MigrateLegacyLayout ย้าย instance ที่ยังอยู่ชั้นบนสุดของ data dir ลงชั้นเกม
// คัดว่า dir ไหนเป็น instance จาก .gamemanager/meta.json เท่านั้น — dir ชั้นเกม (ที่ย้ายแล้ว)
// ไม่มีไฟล์นี้ที่ระดับตัวเอง จึงถูกข้ามไปเองและทำให้รันซ้ำกี่ครั้งก็ได้
// dir ที่ย้ายไม่ได้จะถูกข้ามพร้อม log — instance ตัวเดียวที่มีปัญหาต้องไม่ทำให้ agent boot ไม่ขึ้น
func MigrateLegacyLayout(layout filemanager.Layout, reg *Registry) error {
	entries, err := os.ReadDir(layout.DataDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		legacy := filepath.Join(layout.DataDir, e.Name())
		// dir ที่ไม่มี meta.json ไม่ใช่ instance ของ panel — ห้ามแตะของคนอื่นใน data dir
		if _, err := os.Stat(filepath.Join(legacy, PanelDir, MetaFileName)); err != nil {
			continue
		}
		meta := ReadInstanceMeta(legacy)
		def, ok := reg.Resolve(meta.Game)
		if !ok {
			log.Printf("skip legacy instance with unknown game: server=%s game=%q", e.Name(), meta.Game)
			continue
		}
		target, err := layout.Dir(def.ID, e.Name())
		if err != nil {
			log.Printf("skip legacy instance with invalid id: server=%s err=%v", e.Name(), err)
			continue
		}
		if _, err := os.Stat(target); err == nil {
			log.Printf("skip legacy instance: target already exists: server=%s game=%s", e.Name(), def.ID)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			log.Printf("skip legacy instance: create game directory failed: game=%s err=%v", def.ID, err)
			continue
		}
		if err := os.Rename(legacy, target); err != nil {
			log.Printf("migrate legacy instance failed: server=%s err=%v", e.Name(), err)
			continue
		}
		log.Printf("migrated legacy instance directory: server=%s game=%s", e.Name(), def.ID)
	}
	return nil
}
