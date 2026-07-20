package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const jobCols = `id, server_id, node_id, type, status, payload, error,
	requested_by, created_at, started_at, completed_at`

func scanJob(row pgx.Row) (*Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.ServerID, &j.NodeID, &j.Type, &j.Status, &j.Payload,
		&j.Error, &j.RequestedBy, &j.CreatedAt, &j.StartedAt, &j.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// jobColsJoined ใช้กับ query ที่ LEFT JOIN users (alias j/u) เพื่อได้ชื่อคนสั่งงาน
// (display_name/username — web ประกอบเป็น userTitle เอง)
const jobColsJoined = `j.id, j.server_id, j.node_id, j.type, j.status, j.payload, j.error,
	j.requested_by, j.created_at, j.started_at, j.completed_at, u.display_name, u.username`

func scanJobJoined(row pgx.Row) (*Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.ServerID, &j.NodeID, &j.Type, &j.Status, &j.Payload,
		&j.Error, &j.RequestedBy, &j.CreatedAt, &j.StartedAt, &j.CompletedAt,
		&j.RequestedByName, &j.RequestedByUsername)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) CreateJob(ctx context.Context, id, serverID, nodeID uuid.UUID, jobType string, payload []byte, requestedBy *uuid.UUID) (*Job, error) {
	return scanJob(s.pool.QueryRow(ctx, `
		INSERT INTO jobs (id, server_id, node_id, type, payload, requested_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+jobCols,
		id, serverID, nodeID, jobType, payload, requestedBy))
}

func (s *Store) GetJobByID(ctx context.Context, id uuid.UUID) (*Job, error) {
	return scanJobJoined(s.pool.QueryRow(ctx,
		`SELECT `+jobColsJoined+` FROM jobs j
		 LEFT JOIN users u ON u.id = j.requested_by
		 WHERE j.id = $1`, id))
}

func (s *Store) ListJobsByServer(ctx context.Context, serverID uuid.UUID, limit int) ([]*Job, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+jobColsJoined+` FROM jobs j
		LEFT JOIN users u ON u.id = j.requested_by
		WHERE j.server_id = $1
		ORDER BY j.created_at DESC
		LIMIT $2`, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		j, err := scanJobJoined(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *Store) MarkJobRunning(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs SET status = 'running', started_at = now()
		WHERE id = $1 AND status = 'pending'`, id)
	return err
}

func (s *Store) MarkJobFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs SET status = 'failed', error = $2, completed_at = now()
		WHERE id = $1`, id, errMsg)
	return err
}
