package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	jobv1 "github.com/mc-panel/proto/gen/go/mcpanel/job/v1"

	"github.com/mc-panel/control-plane/internal/console"
	"github.com/mc-panel/control-plane/internal/events"
	"github.com/mc-panel/control-plane/internal/store"
)

const (
	// reapThreshold ต้องมากกว่า worst-case ของ delivery ปกติ (AckWait 5m × MaxDeliver 5
	// = 25m) เพื่อ reap เฉพาะ job ที่ค้างจริง ไม่ไปฆ่างานที่ agent ยัง retry อยู่
	reapThreshold = 30 * time.Minute
	reapInterval  = 5 * time.Minute
)

type ResultConsumer struct {
	st     *store.Store
	rings  *console.Registry
	ws     *console.Hub
	events *events.Hub
	log    *slog.Logger
	// disp ใช้ dispatch ขา start ของ restart เมื่อ stop สำเร็จ (set ตอน Start)
	disp *Dispatcher
}

func NewResultConsumer(st *store.Store, rings *console.Registry, ws *console.Hub, ev *events.Hub, log *slog.Logger) *ResultConsumer {
	return &ResultConsumer{st: st, rings: rings, ws: ws, events: ev, log: log}
}

func (rc *ResultConsumer) Start(ctx context.Context, js jetstream.JetStream) (jetstream.ConsumeContext, error) {
	// dispatcher แยกอินสแตนซ์ (stateless) สำหรับ chain start ของ restart
	rc.disp = NewDispatcher(rc.st, js, rc.log)

	cons, err := EnsureResultsConsumer(ctx, js)
	if err != nil {
		return nil, err
	}
	cc, err := cons.Consume(rc.handle)
	if err != nil {
		return nil, err
	}
	// reaper กู้ job ค้าง + สถานะ server ที่ค้าง transition (boot pass + periodic)
	go rc.runReaper(ctx)
	return cc, nil
}

func (rc *ResultConsumer) handle(msg jetstream.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var res jobv1.JobResult
	if err := proto.Unmarshal(msg.Data(), &res); err != nil {
		// message เพี้ยน retry ไปก็ไม่หาย — ack ทิ้งกัน redeliver วน
		rc.log.Error("drop malformed job result", "error", err)
		msg.Ack()
		return
	}

	jobID, err := uuid.Parse(res.JobId)
	if err != nil {
		rc.log.Error("drop job result with invalid job_id", "job_id", res.JobId)
		msg.Ack()
		return
	}

	job, err := rc.st.GetJobByID(ctx, jobID)
	if errors.Is(err, store.ErrNotFound) {
		rc.log.Warn("job result for unknown job", "job_id", jobID)
		msg.Ack()
		return
	}
	if err != nil {
		rc.log.Error("load job for result failed", "job_id", jobID, "error", err)
		msg.NakWithDelay(5 * time.Second)
		return
	}

	plan, restartStart := planTransition(job, res.Success)

	// delete สำเร็จลบ row ใน tx เดียว — โหลด name ไว้ก่อนเพื่อ audit หลัง commit
	var deleted *store.Server
	if job.Type == "delete_server" && res.Success && job.ServerID != nil {
		if srv, gerr := rc.st.GetServerByID(ctx, *job.ServerID); gerr == nil {
			deleted = srv
		}
	}

	// mark job complete + apply transition อะตอมมิกใน tx เดียว (กัน crash คั่นกลาง
	// ทำ transition หาย). applied=false = job จบไปแล้ว/redeliver ซ้ำ — ห้าม apply ซ้ำ
	applied, changed, err := rc.st.CompleteJobTx(ctx, jobID, job.ServerID, res.Success, res.Error, plan)
	if err != nil {
		rc.log.Error("complete job failed", "job_id", jobID, "error", err)
		msg.NakWithDelay(5 * time.Second)
		return
	}
	if !applied {
		msg.Ack()
		return
	}

	rc.log.Info("job result applied", "job_id", jobID, "type", job.Type,
		"success", res.Success, "error", res.Error)
	if job.Type == "import_server" && res.Success && job.ServerID != nil {
		rc.applyDetectedMCVersion(ctx, *job.ServerID, res.Detail)
	}
	rc.afterCommit(ctx, job, plan, changed, restartStart, deleted)
	msg.Ack()
}

// planTransition แปลงผลลัพธ์ของ job เป็น TransitionPlan ตาม state machine ใน docs
// คืน restartStart=true เมื่อ stop (ที่ติด restart intent) สำเร็จ → ต้อง dispatch start ต่อ
func planTransition(job *store.Job, success bool) (store.TransitionPlan, bool) {
	if job.ServerID == nil {
		return store.TransitionPlan{}, false
	}

	switch job.Type {
	case "create_server", "import_server":
		if success {
			return store.TransitionPlan{NewStatus: "stopped", GuardNotDeleting: true}, false
		}
		return store.TransitionPlan{NewStatus: "errored", GuardNotDeleting: true}, false

	case "start_server":
		if success {
			// container start แล้ว แต่สถานะ running จริงมาจาก ServerStatus — ถ้า agent เจอ
			// container รันอยู่แล้ว (ไม่มี running event ตามมา) reconcile จาก heartbeat
			// (#3) จะพา starting→running/errored เอง
			return store.TransitionPlan{}, false
		}
		return store.TransitionPlan{NewStatus: "errored", GuardNotDeleting: true}, false

	case "stop_server", "kill_server":
		if success {
			if hasRestartIntent(job.Payload) {
				// อย่า set stopped — จะ dispatch start ต่อทันที
				return store.TransitionPlan{}, true
			}
			// die event จาก agent เป็นแหล่ง stopped หลัก แต่เคส container ไม่มีอยู่แล้ว/no-op
			// จะไม่มี die event → fallback set stopped เฉพาะตอนยังค้าง 'stopping'
			// (conditional ไม่ override สถานะที่ถูกอยู่แล้ว)
			return store.TransitionPlan{NewStatus: "stopped", OnlyFromStatus: "stopping"}, false
		}
		// stop fail: container อาจยังรันอยู่ — ปล่อยให้ heartbeat reconcile (#3) แก้สถานะที่ค้าง
		return store.TransitionPlan{}, false

	case "delete_server":
		if success {
			return store.TransitionPlan{DeleteRow: true}, false
		}
		// สถานะปัจจุบันคือ deleting เอง จึง update ตรง ๆ ไม่ต้อง guard
		return store.TransitionPlan{NewStatus: "errored"}, false
	}
	return store.TransitionPlan{}, false
}

// afterCommit side effect ที่ทำหลัง tx commit แล้ว (broadcast/ring drop/audit/chain start)
// พวกนี้ไม่ durable โดยดีไซน์ — พลาดได้ แล้วรอ heartbeat/poll รอบถัดไป reconcile
func (rc *ResultConsumer) afterCommit(ctx context.Context, job *store.Job, plan store.TransitionPlan, changed, restartStart bool, deleted *store.Server) {
	if job.ServerID == nil {
		return
	}
	serverID := *job.ServerID

	// job result ถูก apply แล้ว → job list ของ server นี้เปลี่ยน (ไม่ว่าสถานะ server
	// จะเปลี่ยนหรือไม่) ให้ browser refetch jobs
	rc.events.ServerJobs(serverID)

	if plan.DeleteRow {
		rc.rings.Drop(serverID)
		detail := map[string]any{"job_id": job.ID.String()}
		if deleted != nil {
			detail["name"] = deleted.Name
		}
		if err := rc.st.InsertAudit(ctx, job.RequestedBy, &serverID, "server_deleted", detail, ""); err != nil {
			rc.log.Error("audit server_deleted failed", "server_id", serverID, "error", err)
		}
		rc.log.Info("server deleted", "server_id", serverID, "job_id", job.ID)
		return
	}

	if changed && plan.NewStatus != "" {
		rc.ws.BroadcastStatus(serverID, plan.NewStatus)
		rc.events.ServerStatus(serverID, plan.NewStatus)
	}

	if restartStart {
		rc.dispatchRestartStart(ctx, job, serverID)
	}
}

// dispatchRestartStart dispatch ขา start ของ restart หลัง stop สำเร็จ
func (rc *ResultConsumer) dispatchRestartStart(ctx context.Context, job *store.Job, serverID uuid.UUID) {
	if rc.disp == nil {
		return
	}
	srv, err := rc.st.GetServerByID(ctx, serverID)
	if err != nil {
		rc.log.Error("load server for restart start failed", "server_id", serverID, "error", err)
		return
	}
	requestedBy := uuid.Nil
	if job.RequestedBy != nil {
		requestedBy = *job.RequestedBy
	}
	if _, err := rc.disp.StartServer(ctx, srv, requestedBy); err != nil {
		// start ไม่ถูก dispatch — server จะค้าง 'stopping'/'stopped' ให้ reaper/reconcile กู้
		rc.log.Error("dispatch restart start failed", "server_id", serverID,
			"stop_job_id", job.ID, "error", err)
	}
}

// runReaper กู้ job ที่ค้าง pending/running นานเกิน threshold (agent ตายกลางคัน / MaxDeliver
// หมดโดยไม่มี JobResult) — mark job failed แล้ว reconcile สถานะ server ที่ค้าง transition
func (rc *ResultConsumer) runReaper(ctx context.Context) {
	rc.reapOnce(ctx)
	ticker := time.NewTicker(reapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rc.reapOnce(ctx)
		}
	}
}

func (rc *ResultConsumer) reapOnce(ctx context.Context) {
	staleJobs, err := rc.st.ListStaleJobs(ctx, time.Now().Add(-reapThreshold))
	if err != nil {
		rc.log.Error("list stale jobs failed", "error", err)
		return
	}
	for _, job := range staleJobs {
		plan := reapPlan(job)
		applied, changed, err := rc.st.CompleteJobTx(ctx, job.ID, job.ServerID, false,
			"job reaped: stuck beyond threshold with no result", plan)
		if err != nil {
			rc.log.Error("reap job failed", "job_id", job.ID, "error", err)
			continue
		}
		if !applied {
			continue
		}
		rc.log.Warn("stale job reaped", "job_id", job.ID, "type", job.Type,
			"created_at", job.CreatedAt)
		if job.ServerID != nil {
			rc.events.ServerJobs(*job.ServerID)
		}
		if changed && plan.NewStatus != "" && job.ServerID != nil {
			rc.ws.BroadcastStatus(*job.ServerID, plan.NewStatus)
			rc.events.ServerStatus(*job.ServerID, plan.NewStatus)
		}
	}
}

// reapPlan reconcile สถานะ server สำหรับ job ที่ค้าง — conditional เฉพาะสถานะ transition
// ที่ค้างจริง (ไม่ override server ที่ settle ไปแล้วผ่าน event อื่น). starting/stopping ที่
// reap ไปเป็น errored/stopped ถ้าจริง ๆ ยังรันอยู่ heartbeat reconcile (#3) จะพากลับ running
func reapPlan(job *store.Job) store.TransitionPlan {
	switch job.Type {
	case "create_server", "import_server":
		return store.TransitionPlan{NewStatus: "errored", OnlyFromStatus: "provisioning"}
	case "start_server":
		return store.TransitionPlan{NewStatus: "errored", OnlyFromStatus: "starting"}
	case "stop_server", "kill_server":
		return store.TransitionPlan{NewStatus: "stopped", OnlyFromStatus: "stopping"}
	case "delete_server":
		// delete ค้างกำกวม (container อาจลบไปแล้วบางส่วน) — ตั้ง errored ให้ retry เอง
		// ปลอดภัยกว่าลบ row ทิ้งทั้งที่ไม่รู้ว่า agent ลบจริงหรือยัง
		return store.TransitionPlan{NewStatus: "errored", OnlyFromStatus: "deleting"}
	}
	return store.TransitionPlan{}
}

// mcVersionRe กัน garbage/injection ก่อนเขียน mc_version — เผื่อ release (1.20.1),
// snapshot (23w13a), pre/rc (1.20-pre1) แต่ปฏิเสธค่าเพี้ยนยาว ๆ / มีอักขระแปลก
var mcVersionRe = regexp.MustCompile(`^[0-9][0-9A-Za-z._-]{0,31}$`)

// applyDetectedMCVersion อ่านเวอร์ชันที่ agent detect (JobResult.Detail เป็น JSON
// {"mc_version":"..."}) แล้ว update mc_version ของ server — ทำเฉพาะ import_server
// สำเร็จ. detail ว่าง/parse ไม่ได้/เวอร์ชันไม่ผ่าน regex → ข้ามเงียบ ๆ
func (rc *ResultConsumer) applyDetectedMCVersion(ctx context.Context, serverID uuid.UUID, detail string) {
	if detail == "" {
		return
	}
	var d struct {
		MCVersion string `json:"mc_version"`
	}
	if err := json.Unmarshal([]byte(detail), &d); err != nil {
		return
	}
	if d.MCVersion == "" || !mcVersionRe.MatchString(d.MCVersion) {
		return
	}
	if err := rc.st.UpdateServerMCVersion(ctx, serverID, d.MCVersion); err != nil {
		rc.log.Error("update server mc_version failed", "server_id", serverID,
			"mc_version", d.MCVersion, "error", err)
		return
	}
	rc.log.Info("server mc_version updated from import detection",
		"server_id", serverID, "mc_version", d.MCVersion)
}

// hasRestartIntent อ่าน marker ที่ dispatcher แทรกไว้ใน payload (_meta.restart)
func hasRestartIntent(payload []byte) bool {
	var m struct {
		Meta struct {
			Restart bool `json:"restart"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(payload, &m); err != nil {
		return false
	}
	return m.Meta.Restart
}
