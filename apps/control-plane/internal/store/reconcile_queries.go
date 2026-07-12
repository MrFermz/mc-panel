package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TransitionPlan บอกว่าให้ทำอะไรกับ server row หลัง mark job complete (ใน tx เดียวกับ job)
type TransitionPlan struct {
	// NewStatus = สถานะ server ที่จะ set ("" = ไม่แตะ server row)
	NewStatus string
	// OnlyFromStatus != "" → set เฉพาะเมื่อ status ปัจจุบันตรงค่านี้ (conditional fallback
	// เช่น stop สำเร็จ set stopped เฉพาะตอนยังค้าง 'stopping' — ไม่ override สถานะที่ถูกแล้ว)
	OnlyFromStatus string
	// GuardNotDeleting = ใช้ WHERE status <> 'deleting' (กัน result เก่าไป override ตอนกำลังลบ)
	// มีผลเฉพาะตอน OnlyFromStatus == ""
	GuardNotDeleting bool
	// DeleteRow = ลบ server row แทน set status (สำหรับ delete_server สำเร็จ)
	DeleteRow bool
}

// CompleteJobTx mark job เป็น succeeded/failed แล้ว apply server transition ใน tx เดียว
// เพื่อกัน crash คั่นกลางทำ transition หาย (server ค้าง provisioning/deleting).
// guard job status IN (pending,running) ทำให้ idempotent ต่อ NATS redeliver: ถ้า job จบ
// ไปแล้วคืน applied=false และไม่แตะ server (กัน result เก่า revert สถานะที่ progress ไปแล้ว)
func (s *Store) CompleteJobTx(ctx context.Context, jobID uuid.UUID, serverID *uuid.UUID, success bool, errMsg string, plan TransitionPlan) (applied bool, changed bool, err error) {
	status := "succeeded"
	if !success {
		status = "failed"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, false, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE jobs SET status = $2, error = $3, completed_at = now()
		WHERE id = $1 AND status IN ('pending', 'running')`,
		jobID, status, errMsg)
	if err != nil {
		return false, false, err
	}
	if tag.RowsAffected() == 0 {
		// job จบไปแล้ว (redeliver ซ้ำ) — ห้าม apply transition ซ้ำ
		return false, false, nil
	}

	if serverID != nil {
		switch {
		case plan.DeleteRow:
			if _, err := tx.Exec(ctx, `DELETE FROM servers WHERE id = $1`, *serverID); err != nil {
				return false, false, err
			}
			changed = true
		case plan.NewStatus != "":
			q := `UPDATE servers SET status = $2, updated_at = now() WHERE id = $1`
			args := []any{*serverID, plan.NewStatus}
			switch {
			case plan.OnlyFromStatus != "":
				q += ` AND status = $3`
				args = append(args, plan.OnlyFromStatus)
			case plan.GuardNotDeleting:
				q += ` AND status <> 'deleting'`
			}
			stag, err := tx.Exec(ctx, q, args...)
			if err != nil {
				return false, false, err
			}
			changed = stag.RowsAffected() > 0
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, false, err
	}
	return true, changed, nil
}

// ListStaleJobs คืน job ที่ยังค้าง pending/running นานเกิน cutoff — ใช้ใน reaper
// กู้ job ที่ agent ตายกลางคัน / MaxDeliver หมด แล้วไม่มี JobResult ตามมา
func (s *Store) ListStaleJobs(ctx context.Context, olderThan time.Time) ([]*Job, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+jobCols+` FROM jobs
		WHERE status IN ('pending', 'running') AND created_at < $1
		ORDER BY created_at`, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
