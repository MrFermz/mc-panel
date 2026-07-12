package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const serverCols = `id, node_id, owner_id, name, server_type, mc_version,
	memory_mb, host_port, status, created_at, updated_at`

func scanServer(row pgx.Row) (*Server, error) {
	var v Server
	err := row.Scan(&v.ID, &v.NodeID, &v.OwnerID, &v.Name, &v.ServerType,
		&v.MCVersion, &v.MemoryMB, &v.HostPort, &v.Status, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func collectServers(rows pgx.Rows) ([]*Server, error) {
	defer rows.Close()
	var servers []*Server
	for rows.Next() {
		v, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		servers = append(servers, v)
	}
	return servers, rows.Err()
}

// CreateServerWithOwner ทำใน tx เดียว: server row + permission owner ของคนสร้าง
// เพื่อไม่ให้เกิด server ที่ไม่มี owner ถ้า insert permission ล้ม
func (s *Store) CreateServerWithOwner(ctx context.Context, nodeID, ownerID uuid.UUID, name, serverType, mcVersion string, memoryMB int, hostPort *int) (*Server, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	srv, err := scanServer(tx.QueryRow(ctx, `
		INSERT INTO servers (node_id, owner_id, name, server_type, mc_version, memory_mb, host_port)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+serverCols,
		nodeID, ownerID, name, serverType, mcVersion, memoryMB, hostPort))
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO server_permissions (user_id, server_id, role, can_console_write, can_manage_files)
		VALUES ($1, $2, 'owner', TRUE, TRUE)`,
		ownerID, srv.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return srv, nil
}

func (s *Store) GetServerByID(ctx context.Context, id uuid.UUID) (*Server, error) {
	return scanServer(s.pool.QueryRow(ctx,
		`SELECT `+serverCols+` FROM servers WHERE id = $1`, id))
}

func (s *Store) ListAllServers(ctx context.Context) ([]*Server, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+serverCols+` FROM servers ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return collectServers(rows)
}

func (s *Store) ListServersForUser(ctx context.Context, userID uuid.UUID) ([]*Server, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+serverCols+` FROM servers s
		WHERE EXISTS (
			SELECT 1 FROM server_permissions p
			WHERE p.server_id = s.id AND p.user_id = $1
		)
		ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	return collectServers(rows)
}

// ListAccessibleServerIDs คืน id ของ server ที่ user เข้าถึงได้ — filter เดียวกับ
// ListServersForUser (owner ก็มี server_permissions row) ใช้โดย events WS คำนวณ scope
func (s *Store) ListAccessibleServerIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.id FROM servers s
		WHERE EXISTS (
			SELECT 1 FROM server_permissions p
			WHERE p.server_id = s.id AND p.user_id = $1
		)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) ListServersByNode(ctx context.Context, nodeID uuid.UUID) ([]*Server, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+serverCols+` FROM servers WHERE node_id = $1 ORDER BY created_at`, nodeID)
	if err != nil {
		return nil, err
	}
	return collectServers(rows)
}

// UpdateServerConfig: clearHostPort=true -> set NULL (ไม่ expose host port)
func (s *Store) UpdateServerConfig(ctx context.Context, id uuid.UUID, name *string, memoryMB, hostPort *int, clearHostPort bool) (*Server, error) {
	return scanServer(s.pool.QueryRow(ctx, `
		UPDATE servers SET
			name       = COALESCE($2, name),
			memory_mb  = COALESCE($3, memory_mb),
			host_port  = CASE WHEN $5 THEN NULL ELSE COALESCE($4, host_port) END,
			updated_at = now()
		WHERE id = $1
		RETURNING `+serverCols,
		id, name, memoryMB, hostPort, clearHostPort))
}

func (s *Store) UpdateServerStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE servers SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	return err
}

// UpdateServerStatusIfNotDeleting กันไม่ให้ ServerStatus จาก agent (เช่น stopped
// ระหว่าง delete กำลัง stop container) ไปทับสถานะ deleting
func (s *Store) UpdateServerStatusIfNotDeleting(ctx context.Context, id uuid.UUID, status string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE servers SET status = $2, updated_at = now()
		WHERE id = $1 AND status <> 'deleting'`, id, status)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) DeleteServerRow(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM servers WHERE id = $1`, id)
	return err
}
