package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	jobv1 "github.com/mc-panel/proto/gen/go/mcpanel/job/v1"

	"github.com/mc-panel/control-plane/internal/store"
)

type Dispatcher struct {
	st  *store.Store
	js  jetstream.JetStream
	log *slog.Logger
}

func NewDispatcher(st *store.Store, js jetstream.JetStream, log *slog.Logger) *Dispatcher {
	return &Dispatcher{st: st, js: js, log: log}
}

func (d *Dispatcher) CreateServer(ctx context.Context, srv *store.Server, acceptEula bool, requestedBy uuid.UUID) (*store.Job, error) {
	env := &jobv1.JobEnvelope{
		Payload: &jobv1.JobEnvelope_CreateServer{CreateServer: &jobv1.CreateServer{
			ServerType: srv.ServerType,
			McVersion:  srv.MCVersion,
			AcceptEula: acceptEula,
		}},
	}
	return d.dispatch(ctx, srv, requestedBy, "create_server", "", env, false)
}

// ImportServer เหมือน CreateServer แต่ไม่โหลด jar — agent แตก zip ที่ staged ไว้ที่ archivePath
// (relative ต่อ jail เช่น ".mcpanel/import.zip") ด้วย SafeJoin/zip-slip guard. success → stopped
// เหมือน create ทุกประการ (statusAfter "" ให้ค้าง provisioning จน JobResult พา stopped)
func (d *Dispatcher) ImportServer(ctx context.Context, srv *store.Server, acceptEula bool, archivePath string, requestedBy uuid.UUID) (*store.Job, error) {
	env := &jobv1.JobEnvelope{
		Payload: &jobv1.JobEnvelope_ImportServer{ImportServer: &jobv1.ImportServer{
			ServerType:  srv.ServerType,
			McVersion:   srv.MCVersion,
			AcceptEula:  acceptEula,
			ArchivePath: archivePath,
		}},
	}
	return d.dispatch(ctx, srv, requestedBy, "import_server", "", env, false)
}

func (d *Dispatcher) StartServer(ctx context.Context, srv *store.Server, requestedBy uuid.UUID) (*store.Job, error) {
	hostPort := 0
	if srv.HostPort != nil {
		hostPort = *srv.HostPort
	}
	env := &jobv1.JobEnvelope{
		Payload: &jobv1.JobEnvelope_StartServer{StartServer: &jobv1.StartServer{
			MemoryMb:    int32(srv.MemoryMB),
			HostPort:    int32(hostPort),
			DockerImage: DockerImage(srv.ServerType, srv.MCVersion),
		}},
	}
	// สถานะ running จริงมาจาก ServerStatus ผ่าน gRPC — dispatch แค่ set starting
	return d.dispatch(ctx, srv, requestedBy, "start_server", "starting", env, false)
}

func (d *Dispatcher) StopServer(ctx context.Context, srv *store.Server, requestedBy uuid.UUID) (*store.Job, error) {
	env := &jobv1.JobEnvelope{
		Payload: &jobv1.JobEnvelope_StopServer{StopServer: &jobv1.StopServer{Graceful: true}},
	}
	return d.dispatch(ctx, srv, requestedBy, "stop_server", "stopping", env, false)
}

func (d *Dispatcher) KillServer(ctx context.Context, srv *store.Server, requestedBy uuid.UUID) (*store.Job, error) {
	env := &jobv1.JobEnvelope{
		Payload: &jobv1.JobEnvelope_KillServer{KillServer: &jobv1.KillServer{}},
	}
	return d.dispatch(ctx, srv, requestedBy, "kill_server", "stopping", env, false)
}

func (d *Dispatcher) DeleteServer(ctx context.Context, srv *store.Server, requestedBy uuid.UUID) (*store.Job, error) {
	env := &jobv1.JobEnvelope{
		Payload: &jobv1.JobEnvelope_DeleteServer{DeleteServer: &jobv1.DeleteServer{}},
	}
	return d.dispatch(ctx, srv, requestedBy, "delete_server", "deleting", env, false)
}

// RestartServer dispatch เฉพาะ stop (ติด restart intent) — ขา start จะถูก dispatch
// โดย ResultConsumer เมื่อ stop สำเร็จเท่านั้น (ผูก start กับผลของ stop: ถ้า stop fail
// ต้องไม่ start ต่อ ไม่งั้นจะรายงาน restart สำเร็จทั้งที่ server ยังไม่ได้ restart)
// คืน stop job ให้ web poll — stop fail = restart fail
func (d *Dispatcher) RestartServer(ctx context.Context, srv *store.Server, requestedBy uuid.UUID) (*store.Job, error) {
	env := &jobv1.JobEnvelope{
		Payload: &jobv1.JobEnvelope_StopServer{StopServer: &jobv1.StopServer{Graceful: true}},
	}
	return d.dispatch(ctx, srv, requestedBy, "stop_server", "stopping", env, true)
}

// dispatch: insert แถว jobs (payload = protojson ของ JobEnvelope เพื่อ debug/replay)
// แล้ว publish protobuf binary ไป mcpanel.jobs.{node_id}
// statusAfter = สถานะ server หลัง publish ("" = ไม่เปลี่ยน)
// restartIntent = ติด marker ให้ ResultConsumer รู้ว่า stop นี้เป็นขาแรกของ restart
func (d *Dispatcher) dispatch(ctx context.Context, srv *store.Server, requestedBy uuid.UUID, jobType, statusAfter string, env *jobv1.JobEnvelope, restartIntent bool) (*store.Job, error) {
	jobID := uuid.New()
	env.JobId = jobID.String()
	env.ServerId = srv.ID.String()

	payloadJSON, err := protojson.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal job payload: %w", err)
	}
	if restartIntent {
		payloadJSON = injectRestartMeta(payloadJSON)
	}

	job, err := d.st.CreateJob(ctx, jobID, srv.ID, srv.NodeID, jobType, payloadJSON, &requestedBy)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	data, err := proto.Marshal(env)
	if err != nil {
		d.markFailed(ctx, jobID, "marshal envelope: "+err.Error())
		return nil, fmt.Errorf("marshal job envelope: %w", err)
	}

	if err := d.publishJob(ctx, srv.NodeID.String(), data, jobID); err != nil {
		// publish กำกวม: message อาจถึง agent แล้ว (delete/stop วิ่งจริง) — ห้าม markFailed
		// เพราะจะทำให้ JobResult ที่ตามมาโดน guard ปัดทิ้งแล้ว server row ค้าง. ปล่อย job
		// เป็น 'pending' ให้ result consumer (ถ้า agent ทำจริง) หรือ reaper (#18) reconcile.
		// ไม่ตั้ง statusAfter: เคส publish ล้มจริง server จะได้ไม่ค้างสถานะ transition ยาว —
		// เคสที่ agent ทำจริงจะถูก reconcile ผ่าน JobResult / heartbeat (#3) เอง
		d.log.Error("job publish failed after retries", "job_id", jobID, "type", jobType,
			"node_id", srv.NodeID, "error", err)
		return nil, fmt.Errorf("publish job: %w", err)
	}

	if err := d.st.MarkJobRunning(ctx, jobID); err != nil {
		d.log.Error("mark job running failed", "job_id", jobID, "error", err)
	} else {
		now := time.Now()
		job.Status = "running"
		job.StartedAt = &now
	}

	if statusAfter != "" {
		if err := d.st.UpdateServerStatus(ctx, srv.ID, statusAfter); err != nil {
			d.log.Error("update server status after dispatch failed",
				"server_id", srv.ID, "status", statusAfter, "error", err)
		}
	}

	d.log.Info("job dispatched", "job_id", jobID, "type", jobType,
		"server_id", srv.ID, "node_id", srv.NodeID)
	return job, nil
}

// publishJob publish พร้อม Nats-Msg-Id = job_id เพื่อให้ retry ปลอดภัย (JetStream dedup
// ที่ stream JOBS จะทิ้ง copy ซ้ำภายใน window) — กัน publish timeout กำกวมยิงงานซ้ำเข้า agent
func (d *Dispatcher) publishJob(ctx context.Context, nodeID string, data []byte, jobID uuid.UUID) error {
	const maxAttempts = 3
	subject := JobSubject(nodeID)
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, err := d.js.Publish(ctx, subject, data, jetstream.WithMsgID(jobID.String()))
		if err == nil {
			return nil
		}
		lastErr = err
		d.log.Warn("job publish attempt failed", "job_id", jobID, "attempt", attempt, "error", err)
		if attempt == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
		}
	}
	return lastErr
}

// injectRestartMeta แทรก _meta.restart ลง payload JSONB (นอก field ของ protojson)
// เพื่อให้ ResultConsumer แยกออกว่า stop นี้เป็นขาแรกของ restart แล้ว dispatch start ต่อ
func injectRestartMeta(payload []byte) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(payload, &m); err != nil {
		return payload
	}
	m["_meta"] = json.RawMessage(`{"restart":true}`)
	out, err := json.Marshal(m)
	if err != nil {
		return payload
	}
	return out
}

func (d *Dispatcher) markFailed(ctx context.Context, jobID uuid.UUID, msg string) {
	if err := d.st.MarkJobFailed(ctx, jobID, msg); err != nil {
		d.log.Error("mark job failed errored", "job_id", jobID, "error", err)
	}
}
