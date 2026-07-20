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

// ListServerPermissions ซ่อน grant ของ user ที่อยู่ในถังขยะ — soft delete ไม่ลบ
// server_permissions ทิ้งแล้ว (restore ต้องได้สิทธิ์คืนครบ) แถวพวกนั้นจึงยังอยู่ใน DB
// แต่ต้องไม่โผล่ในลิสต์ access ราวกับคนนั้นยังเข้าถึง server ได้อยู่
func (s *Store) ListServerPermissions(ctx context.Context, serverID uuid.UUID) ([]*PermissionWithUser, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.user_id, p.server_id, p.role, p.capabilities,
		       p.created_at, u.username,
		       u.display_name, u.avatar_updated_at
		FROM server_permissions p
		JOIN users u ON u.id = p.user_id
		WHERE p.server_id = $1 AND u.deleted_at IS NULL
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

// PermissionWithServer = grant หนึ่งแถวมองจากฝั่ง user (หน้า /admin/users/{id}/servers)
// — สลับกับ PermissionWithUser ที่มองจากฝั่ง server
type PermissionWithServer struct {
	Permission
	ServerName   string
	ServerStatus string
	NodeID       uuid.UUID
}

// ListUserServerPermissions คืน server ทุกตัวที่ user คนนี้มีสิทธิ์ (ข้าม server ที่อยู่
// ในถังขยะ — จัดการ access ของ server ที่ถูกลบไม่มีความหมาย)
func (s *Store) ListUserServerPermissions(ctx context.Context, userID uuid.UUID) ([]*PermissionWithServer, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.user_id, p.server_id, p.role, p.capabilities, p.created_at,
		       sv.name, sv.status, sv.node_id
		FROM server_permissions p
		JOIN servers sv ON sv.id = p.server_id
		WHERE p.user_id = $1 AND sv.deleted_at IS NULL
		ORDER BY sv.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []*PermissionWithServer
	for rows.Next() {
		var p PermissionWithServer
		if err := rows.Scan(&p.ID, &p.UserID, &p.ServerID, &p.Role,
			&p.Capabilities, &p.CreatedAt,
			&p.ServerName, &p.ServerStatus, &p.NodeID); err != nil {
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

// CountServerOwners นับเฉพาะ owner ที่ยังไม่ถูกลบ — guard "ห้ามถอด owner คนสุดท้าย"
// ต้องสะท้อนคนที่ login เข้ามาจัดการได้จริง ไม่ใช่แถวที่ค้างอยู่ของบัญชีในถังขยะ
func (s *Store) CountServerOwners(ctx context.Context, serverID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM server_permissions p
		JOIN users u ON u.id = p.user_id
		WHERE p.server_id = $1 AND p.role = 'owner' AND u.deleted_at IS NULL`,
		serverID).Scan(&n)
	return n, err
}
