package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrPlayerExists คืนจาก AddServerPlayer เมื่อ (server_id, uuid) ซ้ำ — handler map เป็น 409
var ErrPlayerExists = errors.New("store: player already exists")

type ServerPlayer struct {
	UUID      uuid.UUID
	Username  string
	CreatedAt time.Time
}

func (s *Store) ListServerPlayers(ctx context.Context, serverID uuid.UUID) ([]ServerPlayer, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT uuid, username, created_at
		FROM server_players
		WHERE server_id = $1
		ORDER BY created_at`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	players := make([]ServerPlayer, 0)
	for rows.Next() {
		var p ServerPlayer
		if err := rows.Scan(&p.UUID, &p.Username, &p.CreatedAt); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

// AddServerPlayer insert แถวใหม่ — คืน ErrPlayerExists เมื่อ (server_id, uuid) ชนกับที่มีอยู่
func (s *Store) AddServerPlayer(ctx context.Context, serverID, playerUUID uuid.UUID, username string, addedBy *uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO server_players (server_id, uuid, username, added_by)
		VALUES ($1, $2, $3, $4)`,
		serverID, playerUUID, username, addedBy)
	if IsUniqueViolation(err) {
		return ErrPlayerExists
	}
	return err
}

// RemoveServerPlayer ลบแถว — คืน ErrNotFound เมื่อไม่มี player นั้นใน server
func (s *Store) RemoveServerPlayer(ctx context.Context, serverID, playerUUID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM server_players WHERE server_id = $1 AND uuid = $2`,
		serverID, playerUUID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// HostPortUsage = host_port ที่ถูกใช้อยู่บน node หนึ่ง + เกมของ server ตัวนั้น
// (เกมเป็นตัวบอกว่า instance นั้นกิน port ต่อจากนี้อีกกี่ตัว — store ไม่รู้เรื่องนั้นเอง)
type HostPortUsage struct {
	Port int
	Game string
}

// ListHostPortUsage คืน host_port ทั้งหมดที่ถูกจองไว้บน node (รวมของที่อยู่ในถังขยะ —
// ไฟล์ยังอยู่และ restore แล้วต้อง start ได้ จึงยังนับว่ากิน port อยู่)
func (s *Store) ListHostPortUsage(ctx context.Context, nodeID uuid.UUID) ([]HostPortUsage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT host_port, game FROM servers
		WHERE node_id = $1 AND host_port IS NOT NULL`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HostPortUsage
	for rows.Next() {
		var u HostPortUsage
		if err := rows.Scan(&u.Port, &u.Game); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
