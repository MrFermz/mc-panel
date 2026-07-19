package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PlayerFace = แถว cache ของรูปหน้าผู้เล่น. PNG = nil แปลว่า negative cache
// (uuid นี้ไม่มี skin — offline-mode/ไม่มี texture) ไม่ใช่ "ยังไม่เคยดึง"
type PlayerFace struct {
	PNG       []byte
	FetchedAt time.Time
}

// GetPlayerFace อ่าน cache. ErrNotFound = ยังไม่เคยดึง uuid นี้ (แยกจาก "ดึงแล้วไม่มี skin"
// ซึ่งคืน row ที่ PNG=nil) — ผู้เรียกต้องแยกสองเคสนี้เพื่อ fallback ตอน Mojang ล่ม
func (s *Store) GetPlayerFace(ctx context.Context, id uuid.UUID) (*PlayerFace, error) {
	var f PlayerFace
	err := s.pool.QueryRow(ctx, `
		SELECT png, fetched_at FROM player_faces WHERE uuid = $1`, id).
		Scan(&f.PNG, &f.FetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// SavePlayerFace upsert ผล crop ล่าสุด + reset fetched_at (png=nil เก็บเป็น negative cache)
func (s *Store) SavePlayerFace(ctx context.Context, id uuid.UUID, png []byte) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO player_faces (uuid, png, fetched_at) VALUES ($1, $2, now())
		ON CONFLICT (uuid) DO UPDATE SET png = EXCLUDED.png, fetched_at = now()`, id, png)
	return err
}
