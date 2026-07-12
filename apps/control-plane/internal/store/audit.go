package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

func (s *Store) InsertAudit(ctx context.Context, userID, serverID *uuid.UUID, action string, detail map[string]any, ip string) error {
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO audit_logs (user_id, server_id, action, detail, ip)
		VALUES ($1, $2, $3, $4, $5)`,
		userID, serverID, action, raw, ip)
	return err
}
