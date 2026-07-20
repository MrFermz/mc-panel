package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const permCols = `id, user_id, server_id, role, capabilities, created_at`

func scanPermission(row pgx.Row) (*Permission, error) {
	var p Permission
	err := row.Scan(&p.ID, &p.UserID, &p.ServerID, &p.Role,
		&p.Capabilities, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetPermission(ctx context.Context, userID, serverID uuid.UUID) (*Permission, error) {
	return scanPermission(s.pool.QueryRow(ctx,
		`SELECT `+permCols+` FROM server_permissions WHERE user_id = $1 AND server_id = $2`,
		userID, serverID))
}

func (s *Store) ListServerPermissions(ctx context.Context, serverID uuid.UUID) ([]*PermissionWithUser, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.user_id, p.server_id, p.role, p.capabilities,
		       p.created_at, u.username,
		       u.display_name, u.avatar_updated_at
		FROM server_permissions p
		JOIN users u ON u.id = p.user_id
		WHERE p.server_id = $1
		ORDER BY p.created_at`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []*PermissionWithUser
	for rows.Next() {
		var p PermissionWithUser
		if err := rows.Scan(&p.ID, &p.UserID, &p.ServerID, &p.Role,
			&p.Capabilities, &p.CreatedAt,
			&p.Username, &p.DisplayName, &p.AvatarUpdatedAt); err != nil {
			return nil, err
		}
		perms = append(perms, &p)
	}
	return perms, rows.Err()
}

func (s *Store) UpsertPermission(ctx context.Context, userID, serverID uuid.UUID, role string, capabilities []string) (*Permission, error) {
	// owner ได้ทุก server-scoped cap โดยปริยาย จึงเก็บ capabilities ว่างเสมอ
	if capabilities == nil {
		capabilities = []string{}
	}
	return scanPermission(s.pool.QueryRow(ctx, `
		INSERT INTO server_permissions (user_id, server_id, role, capabilities)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, server_id) DO UPDATE SET
			role         = EXCLUDED.role,
			capabilities = EXCLUDED.capabilities
		RETURNING `+permCols,
		userID, serverID, role, capabilities))
}

func (s *Store) DeletePermission(ctx context.Context, serverID, userID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM server_permissions WHERE server_id = $1 AND user_id = $2`,
		serverID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountServerOwners(ctx context.Context, serverID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM server_permissions WHERE server_id = $1 AND role = 'owner'`,
		serverID).Scan(&n)
	return n, err
}
