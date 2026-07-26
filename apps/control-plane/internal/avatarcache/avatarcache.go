// Package avatarcache เก็บรูปประจำตัวผู้เล่น (PNG) ที่ดึงมาจาก identity service ของเกม
// ลง Postgres แล้วเสิร์ฟต่อ — ตัว package ไม่รู้จักเกมใดเลย: "วิธีได้รูปมา" ถูกส่งเข้ามาเป็น
// Fetcher จาก game definition (ดู internal/games)
//
// เหตุผลที่ cache ลง storage ไม่ใช่ in-memory: refresh เองเมื่อเกิน TTL, กัน rate-limit ของ
// upstream, รอดข้าม restart และ — จุดสำคัญ — **เสิร์ฟรูปเก่าที่เก็บไว้ได้ตอน upstream ล่ม**
package avatarcache

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/game-manager/control-plane/internal/games"
	"github.com/game-manager/control-plane/internal/store"
)

const (
	// TTL: hit เก็บนาน (รูปไม่ค่อยเปลี่ยน + ลด load upstream), miss เก็บสั้นให้ retry ได้เร็ว
	hitTTL  = 6 * time.Hour
	missTTL = 15 * time.Minute
)

// Fetcher = "วิธีได้รูปมา" ของเกมหนึ่ง — คืน games.ErrNoAvatar เมื่อรู้แน่ว่าผู้เล่นคนนี้
// ไม่มีรูป (จะถูก negative-cache), error อื่นถือว่า upstream ไม่พร้อมชั่วคราว
type Fetcher func(ctx context.Context, id uuid.UUID) ([]byte, error)

// Cache ห่อ Fetcher ด้วยชั้น cache ที่ใช้ Postgres เป็น storage
type Cache struct {
	st    *store.Store
	fetch Fetcher
}

func New(st *store.Store, fetch Fetcher) *Cache {
	return &Cache{st: st, fetch: fetch}
}

// Avatar คืน PNG ของผู้เล่น. อ่านจาก cache ก่อน ถ้ายังสด (ไม่เกิน TTL) คืนเลย;
// ถ้าเก่า/ยังไม่มี ลองดึงใหม่. upstream ล่มแต่มี cache เก่า → คืน cache เก่า
// (นี่คือเหตุผลหลักที่เก็บลง storage). คืน games.ErrNoAvatar เมื่อรู้ว่าไม่มีรูป
func (c *Cache) Avatar(ctx context.Context, id uuid.UUID) ([]byte, error) {
	cached, err := c.st.GetPlayerAvatar(ctx, id)
	hasCached := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		// DB error จริง (ไม่ใช่ "ไม่มีแถว") — ยังลองดึงจาก upstream ต่อ ถือว่าไม่มี cache
		hasCached = false
	}

	if hasCached && time.Now().Before(freshUntil(cached)) {
		return serveCached(cached)
	}

	png, ferr := c.fetch(ctx, id)
	switch {
	case ferr == nil:
		_ = c.st.SavePlayerAvatar(ctx, id, png) // best-effort — เสิร์ฟได้แม้เขียน cache พลาด
		return png, nil
	case errors.Is(ferr, games.ErrNoAvatar):
		_ = c.st.SavePlayerAvatar(ctx, id, nil) // negative cache
		return nil, games.ErrNoAvatar
	default:
		// upstream/network ล่ม — ตกกลับไปใช้ cache เก่าถ้ามี (แม้จะ stale) ไม่งั้นค่อยคืน error
		if hasCached {
			return serveCached(cached)
		}
		return nil, ferr
	}
}

// freshUntil: hit (มีรูป) สดนาน, miss (negative) สดสั้น ให้ retry upstream ไวขึ้น
func freshUntil(a *store.PlayerAvatar) time.Time {
	if a.PNG == nil {
		return a.FetchedAt.Add(missTTL)
	}
	return a.FetchedAt.Add(hitTTL)
}

func serveCached(a *store.PlayerAvatar) ([]byte, error) {
	if a.PNG == nil {
		return nil, games.ErrNoAvatar
	}
	return a.PNG, nil
}
