package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const nodeCols = `id, name, agent_token_hash, status, agent_version, os, arch,
	cpu_percent, memory_used_mb, memory_total_mb, disk_used_mb, disk_total_mb,
	net_rx_bps, net_tx_bps, last_heartbeat_at, created_at`

func scanNode(row pgx.Row) (*Node, error) {
	var n Node
	err := row.Scan(&n.ID, &n.Name, &n.AgentTokenHash, &n.Status, &n.AgentVersion,
		&n.OS, &n.Arch, &n.CPUPercent, &n.MemoryUsedMB, &n.MemoryTotalMB,
		&n.DiskUsedMB, &n.DiskTotalMB, &n.NetRxBps, &n.NetTxBps,
		&n.LastHeartbeatAt, &n.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Store) CountNodes(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM nodes`).Scan(&n)
	return n, err
}

func (s *Store) ListNodes(ctx context.Context) ([]*Node, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+nodeCols+` FROM nodes ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// CreateNode รับ id จาก caller เพราะ token format คือ <node_id>.<secret>
// ต้องรู้ id ก่อนถึงจะ hash token ทั้งเส้นลง DB ได้
func (s *Store) CreateNode(ctx context.Context, id uuid.UUID, name, tokenHash string) (*Node, error) {
	return scanNode(s.pool.QueryRow(ctx, `
		INSERT INTO nodes (id, name, agent_token_hash)
		VALUES ($1, $2, $3)
		RETURNING `+nodeCols,
		id, name, tokenHash))
}

func (s *Store) GetNodeByID(ctx context.Context, id uuid.UUID) (*Node, error) {
	return scanNode(s.pool.QueryRow(ctx,
		`SELECT `+nodeCols+` FROM nodes WHERE id = $1`, id))
}

func (s *Store) GetNodeByTokenHash(ctx context.Context, tokenHash string) (*Node, error) {
	return scanNode(s.pool.QueryRow(ctx,
		`SELECT `+nodeCols+` FROM nodes WHERE agent_token_hash = $1`, tokenHash))
}

func (s *Store) DeleteNode(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountServersByNode(ctx context.Context, nodeID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM servers WHERE node_id = $1`, nodeID).Scan(&n)
	return n, err
}

func (s *Store) UpdateNodeHello(ctx context.Context, id uuid.UUID, agentVersion, osName, arch string, memTotalMB, diskTotalMB int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE nodes SET
			agent_version     = $2,
			os                = $3,
			arch              = $4,
			memory_total_mb   = $5,
			disk_total_mb     = $6,
			status            = 'online',
			last_heartbeat_at = now()
		WHERE id = $1`,
		id, agentVersion, osName, arch, memTotalMB, diskTotalMB)
	return err
}

func (s *Store) UpdateNodeHeartbeat(ctx context.Context, id uuid.UUID, cpuPercent float64, memUsedMB, memTotalMB, diskUsedMB, diskTotalMB int64, netRxBps, netTxBps float64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE nodes SET
			cpu_percent       = $2,
			memory_used_mb    = $3,
			memory_total_mb   = $4,
			disk_used_mb      = $5,
			disk_total_mb     = $6,
			net_rx_bps        = $7,
			net_tx_bps        = $8,
			status            = 'online',
			last_heartbeat_at = now()
		WHERE id = $1`,
		id, cpuPercent, memUsedMB, memTotalMB, diskUsedMB, diskTotalMB, netRxBps, netTxBps)
	return err
}

func (s *Store) SetNodeStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE nodes SET status = $2 WHERE id = $1`, id, status)
	return err
}

// MarkStaleNodesOffline คืน id ของ node ที่เพิ่งถูก mark offline (RETURNING) เพื่อให้
// caller push node_stats แจ้ง transition ได้ — ไม่ใช่แค่นับจำนวน
func (s *Store) MarkStaleNodesOffline(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE nodes SET status = 'offline'
		WHERE status = 'online'
		  AND (last_heartbeat_at IS NULL OR last_heartbeat_at < $1)
		RETURNING id`,
		cutoff)
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
