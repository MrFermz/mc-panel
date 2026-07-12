// Package agenthub คุม gRPC bidirectional stream ระหว่าง control-plane กับ
// node-agent — เฉพาะข้อมูล realtime (heartbeat, console I/O, server status)
// ส่วนคำสั่งทั้งหมดต้องผ่าน NATS JetStream (ดู internal/jobs)
package agenthub

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"

	"github.com/mc-panel/control-plane/internal/console"
	"github.com/mc-panel/control-plane/internal/events"
	"github.com/mc-panel/control-plane/internal/serverstats"
	"github.com/mc-panel/control-plane/internal/store"
)

const (
	offlineCheckInterval = 15 * time.Second
	heartbeatStaleAfter  = 30 * time.Second
	// sendTimeout จำกัดเวลาที่ stream.Send บล็อกได้ — gRPC Send บล็อกเมื่อ client
	// อ่านช้า/ค้าง (flow control) ปล่อยไว้จะทำให้ console input ค้างทั้ง node
	sendTimeout = 5 * time.Second
	// fileRequestTimeout จำกัดเวลารอ FileResponse จาก agent — file op เป็น
	// synchronous ต่อ HTTP request จึงต้องมีเพดานไม่ให้ค้างถือ connection
	fileRequestTimeout = 15 * time.Second
)

var (
	ErrNodeNotConnected = errors.New("agenthub: node is not connected")
	ErrSendTimeout      = errors.New("agenthub: send to node timed out")
	ErrAgentTimeout     = errors.New("agenthub: agent did not respond in time")
)

type Hub struct {
	st     *store.Store
	rings  *console.Registry
	ws     *console.Hub
	stats  *serverstats.Cache
	events *events.Hub
	log    *slog.Logger

	mu    sync.Mutex
	conns map[uuid.UUID]*agentConn

	// fileMu คุม filePending: map จาก request_id → chan รอ FileResponse
	// (file op correlate ด้วย request_id ผ่าน stream เดิม ไม่ผ่าน NATS)
	fileMu      sync.Mutex
	filePending map[string]chan *agentv1.FileResponse
}

func NewHub(st *store.Store, rings *console.Registry, ws *console.Hub, stats *serverstats.Cache, ev *events.Hub, log *slog.Logger) *Hub {
	return &Hub{
		st:          st,
		rings:       rings,
		ws:          ws,
		stats:       stats,
		events:      ev,
		log:         log,
		conns:       make(map[uuid.UUID]*agentConn),
		filePending: make(map[string]chan *agentv1.FileResponse),
	}
}

type agentConn struct {
	nodeID uuid.UUID
	stream agentv1.AgentService_ConnectServer

	// gRPC stream ห้าม Send พร้อมกันหลาย goroutine — ConsoleInput มาจาก
	// WS connection หลายตัวได้. ใช้ semaphore (แทน Mutex) เพื่อ acquire แบบมี
	// timeout ได้ — ถ้า Send ก่อนหน้าค้าง caller ใหม่จะ timeout ไม่ค้างตาม
	sendSem chan struct{}

	done      chan struct{}
	closeOnce sync.Once
}

func newAgentConn(nodeID uuid.UUID, stream agentv1.AgentService_ConnectServer) *agentConn {
	return &agentConn{
		nodeID:  nodeID,
		stream:  stream,
		sendSem: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
}

func (c *agentConn) close() {
	c.closeOnce.Do(func() { close(c.done) })
}

// send ส่ง ControlMessage โดยจำกัดเวลาไม่ให้ค้างถือ send slot ไม่มีกำหนด:
// stream.Send บล็อกได้ถ้า client อ่านช้า (flow control) ถ้าปล่อยไว้ console input
// ของทั้ง node จะค้าง. goroutine ที่ Send ค้างจะถือ semaphore ไว้จนกว่า Send คืนค่า
// (กัน Send ซ้อน — gRPC ห้าม) และเมื่อ timeout จะสั่งปิด stream ให้ Send ถูกปลุก
// แล้วให้ agent reconnect.
func (c *agentConn) send(msg *agentv1.ControlMessage) error {
	select {
	case c.sendSem <- struct{}{}:
	case <-time.After(sendTimeout):
		return ErrSendTimeout
	case <-c.done:
		return ErrNodeNotConnected
	}

	result := make(chan error, 1)
	go func() {
		err := c.stream.Send(msg)
		<-c.sendSem // ปล่อย slot หลัง Send คืนค่าเสมอ แม้ caller เลิกรอไปแล้ว
		result <- err
	}()

	select {
	case err := <-result:
		return err
	case <-time.After(sendTimeout):
		c.close()
		return ErrSendTimeout
	case <-c.done:
		return ErrNodeNotConnected
	}
}

// register แทนที่ stream เก่าของ node เดิม (agent restart แล้ว reconnect
// ก่อน stream เก่า timeout) — ตัวเก่าถูกสั่งปิดทันที
func (h *Hub) register(nodeID uuid.UUID, stream agentv1.AgentService_ConnectServer) *agentConn {
	c := newAgentConn(nodeID, stream)
	h.mu.Lock()
	old := h.conns[nodeID]
	h.conns[nodeID] = c
	h.mu.Unlock()
	if old != nil {
		h.log.Info("replacing existing agent stream", "node_id", nodeID)
		old.close()
	}
	return c
}

func (h *Hub) unregister(c *agentConn) {
	h.mu.Lock()
	current := h.conns[c.nodeID] == c
	if current {
		delete(h.conns, c.nodeID)
	}
	h.mu.Unlock()
	c.close()

	// mark offline เฉพาะเมื่อยังเป็น stream ปัจจุบัน — กัน race ตอน agent
	// reconnect แล้ว stream ใหม่ set online ไปก่อนหน้านี้แล้ว
	if current {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.st.SetNodeStatus(ctx, c.nodeID, "offline"); err != nil {
			h.log.Error("mark node offline on disconnect failed", "node_id", c.nodeID, "error", err)
			return
		}
		h.emitNodeStats(ctx, c.nodeID)
	}
}

// emitNodeStats โหลด node row ล่าสุดแล้ว push node_stats ให้ browser (best-effort —
// พลาดได้ รอ heartbeat/refresh รอบถัดไป). caller ต้องมั่นใจว่าเพิ่งอัปเดต DB แล้ว
func (h *Hub) emitNodeStats(ctx context.Context, nodeID uuid.UUID) {
	node, err := h.st.GetNodeByID(ctx, nodeID)
	if err != nil {
		h.log.Error("load node for events failed", "node_id", nodeID, "error", err)
		return
	}
	h.events.NodeStats(events.NewNodePayload(node))
}

// SendConsoleInput ให้ ws hub เรียกส่งคำสั่ง (ผ่าน permission check มาแล้ว) ไป agent
func (h *Hub) SendConsoleInput(nodeID, serverID uuid.UUID, command string) error {
	h.mu.Lock()
	c := h.conns[nodeID]
	h.mu.Unlock()
	if c == nil {
		return ErrNodeNotConnected
	}
	return c.send(&agentv1.ControlMessage{
		Payload: &agentv1.ControlMessage_ConsoleInput{ConsoleInput: &agentv1.ConsoleInput{
			ServerId: serverID.String(),
			Command:  command,
		}},
	})
}

// SendFileRequest ส่ง FileRequest ไป agent ของ node แล้วรอ FileResponse ที่ correlate
// ด้วย request_id (สร้างที่นี่ ทับค่าเดิมใน req เสมอ). return ErrNodeNotConnected ถ้า node
// ไม่ออนไลน์, ErrAgentTimeout ถ้า agent ไม่ตอบใน fileRequestTimeout. cleanup map ทุก path.
func (h *Hub) SendFileRequest(ctx context.Context, nodeID uuid.UUID, req *agentv1.FileRequest) (*agentv1.FileResponse, error) {
	h.mu.Lock()
	c := h.conns[nodeID]
	h.mu.Unlock()
	if c == nil {
		return nil, ErrNodeNotConnected
	}

	reqID := uuid.NewString()
	req.RequestId = reqID
	// buffer 1 กัน deliverFileResponse บล็อกถ้าฝั่งนี้ timeout ไปแล้วก่อน response มาถึง
	ch := make(chan *agentv1.FileResponse, 1)

	h.fileMu.Lock()
	h.filePending[reqID] = ch
	h.fileMu.Unlock()
	defer func() {
		h.fileMu.Lock()
		delete(h.filePending, reqID)
		h.fileMu.Unlock()
	}()

	if err := c.send(&agentv1.ControlMessage{
		Payload: &agentv1.ControlMessage_FileRequest{FileRequest: req},
	}); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(fileRequestTimeout):
		return nil, ErrAgentTimeout
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, ErrNodeNotConnected
	}
}

// deliverFileResponse ส่ง response เข้า chan ที่รออยู่ตาม request_id — non-block:
// ถ้าไม่มี chan (timeout ไปแล้ว / request_id ไม่รู้จัก) ก็ทิ้ง
func (h *Hub) deliverFileResponse(resp *agentv1.FileResponse) {
	h.fileMu.Lock()
	ch := h.filePending[resp.RequestId]
	h.fileMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- resp:
	default:
	}
}

// RunOfflineChecker: node ที่ heartbeat ขาดเกิน 30 วินาที -> offline
func (h *Hub) RunOfflineChecker(ctx context.Context) {
	ticker := time.NewTicker(offlineCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ids, err := h.st.MarkStaleNodesOffline(ctx, time.Now().Add(-heartbeatStaleAfter))
			if err != nil {
				h.log.Error("mark stale nodes offline failed", "error", err)
				continue
			}
			if len(ids) > 0 {
				h.log.Warn("marked stale nodes offline", "count", len(ids))
			}
			for _, id := range ids {
				h.emitNodeStats(ctx, id)
			}
		}
	}
}
