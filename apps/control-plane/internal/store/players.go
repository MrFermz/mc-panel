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

// NextFreeHostPort คืน host_port ว่างต่ำสุดบน node เริ่มจาก 25565 (cap 65535)
// สำหรับ prefill ฝั่ง web เท่านั้น — ไม่ได้ reserve จริง (create เป็นคน enforce UNIQUE)
func (s *Store) NextFreeHostPort(ctx context.Context, nodeID uuid.UUID) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT host_port FROM servers
		WHERE node_id = $1 AND host_port IS NOT NULL`, nodeID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	taken := make(map[int]bool)
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return 0, err
		}
		taken[p] = true
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	const minPort, maxPort = 25565, 65535
	for p := minPort; p <= maxPort; p++ {
		if !taken[p] {
			return p, nil
		}
	}
	return maxPort, nil
}
