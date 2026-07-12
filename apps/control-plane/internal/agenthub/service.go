package agenthub

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/events"
	"github.com/mc-panel/control-plane/internal/serverstats"
	"github.com/mc-panel/control-plane/internal/store"
)

type nodeCtxKey struct{}

type Service struct {
	agentv1.UnimplementedAgentServiceServer
	hub *Hub
}

func NewService(hub *Hub) *Service {
	return &Service{hub: hub}
}

// StreamAuthInterceptor resolve node จาก token เสมอ — ห้ามเชื่อ node_id
// ที่ client ส่งมา (ดู agent.proto)
func (s *Service) StreamAuthInterceptor(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	md, ok := metadata.FromIncomingContext(ss.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	var token string
	for _, v := range md.Get("authorization") {
		if t, found := strings.CutPrefix(v, "Bearer "); found {
			token = t
			break
		}
	}
	if token == "" {
		return status.Error(codes.Unauthenticated, "missing bearer token")
	}

	node, err := s.hub.st.GetNodeByTokenHash(ss.Context(), auth.HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return status.Error(codes.Unauthenticated, "invalid node token")
	}
	if err != nil {
		s.hub.log.Error("node token lookup failed", "error", err)
		return status.Error(codes.Internal, "internal error")
	}

	return handler(srv, &authedStream{
		ServerStream: ss,
		ctx:          context.WithValue(ss.Context(), nodeCtxKey{}, node),
	})
}

type authedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authedStream) Context() context.Context { return s.ctx }

func nodeFrom(ctx context.Context) *store.Node {
	n, _ := ctx.Value(nodeCtxKey{}).(*store.Node)
	return n
}

func (s *Service) Connect(stream agentv1.AgentService_ConnectServer) error {
	node := nodeFrom(stream.Context())
	if node == nil {
		return status.Error(codes.Unauthenticated, "unauthenticated stream")
	}
	h := s.hub
	ctx := stream.Context()

	first, err := stream.Recv()
	if err != nil {
		return err
	}
	hello := first.GetHello()
	if hello == nil {
		return status.Error(codes.InvalidArgument, "first message must be hello")
	}

	if err := h.st.UpdateNodeHello(ctx, node.ID, hello.AgentVersion, hello.Os,
		hello.Arch, hello.TotalRamMb, hello.TotalDiskMb); err != nil {
		h.log.Error("update node hello failed", "node_id", node.ID, "error", err)
		return status.Error(codes.Internal, "internal error")
	}

	// Welcome เป็นทางเดียวที่ agent รู้ identity ตัวเอง (config ฝั่ง agent มีแค่ token)
	if err := stream.Send(&agentv1.ControlMessage{
		Payload: &agentv1.ControlMessage_Welcome{Welcome: &agentv1.Welcome{
			NodeId: node.ID.String(),
		}},
	}); err != nil {
		return err
	}

	conn := h.register(node.ID, stream)
	defer h.unregister(conn)
	h.log.Info("agent connected", "node_id", node.ID, "agent_version", hello.AgentVersion,
		"os", hello.Os, "arch", hello.Arch)

	type recvResult struct {
		msg *agentv1.AgentMessage
		err error
	}
	recvCh := make(chan recvResult)
	go func() {
		for {
			msg, err := stream.Recv()
			select {
			case recvCh <- recvResult{msg, err}:
			case <-conn.done:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-conn.done:
			// โดน stream ใหม่ของ node เดียวกันแทนที่
			return nil
		case r := <-recvCh:
			if r.err != nil {
				if errors.Is(r.err, io.EOF) {
					h.log.Info("agent disconnected", "node_id", node.ID)
					return nil
				}
				h.log.Info("agent stream closed", "node_id", node.ID, "error", r.err)
				return r.err
			}
			h.handleMessage(ctx, node.ID, r.msg)
		}
	}
}

func (h *Hub) handleMessage(ctx context.Context, nodeID uuid.UUID, msg *agentv1.AgentMessage) {
	switch p := msg.Payload.(type) {
	case *agentv1.AgentMessage_Heartbeat:
		h.handleHeartbeat(ctx, nodeID, p.Heartbeat)
	case *agentv1.AgentMessage_ConsoleOutput:
		h.handleConsoleOutput(ctx, nodeID, p.ConsoleOutput)
	case *agentv1.AgentMessage_ServerStatus:
		h.handleServerStatus(ctx, nodeID, p.ServerStatus)
	case *agentv1.AgentMessage_ServerStats:
		h.handleServerStats(ctx, nodeID, p.ServerStats)
	case *agentv1.AgentMessage_FileResponse:
		h.deliverFileResponse(p.FileResponse)
	case *agentv1.AgentMessage_Hello:
		// hello ซ้ำหลัง handshake — ไม่มีความหมาย
	}
}

func (h *Hub) handleHeartbeat(ctx context.Context, nodeID uuid.UUID, hb *agentv1.Heartbeat) {
	if err := h.st.UpdateNodeHeartbeat(ctx, nodeID, hb.CpuPercent, hb.MemoryUsedMb,
		hb.MemoryTotalMb, hb.DiskUsedMb, hb.DiskTotalMb, hb.NetRxBps, hb.NetTxBps); err != nil {
		h.log.Error("update node heartbeat failed", "node_id", nodeID, "error", err)
		return
	}
	// push node_stats ทุก heartbeat (1 DB read/heartbeat) — browser เห็น cpu/mem/disk
	// ของ node แบบ realtime โดยไม่ poll REST
	h.emitNodeStats(ctx, nodeID)
	h.reconcileServers(ctx, nodeID, hb.RunningServerIds)
}

// reconcileServers แก้ drift ระหว่างสถานะใน DB กับ container ที่รันจริงตาม heartbeat.
// stable state (running/stopped/errored) แก้ทันที ; สถานะ transition (starting/stopping)
// ที่ค้างเกิน grace period (job result / ServerStatus ไม่มา — เช่น agent เจอ container
// รันอยู่แล้วคืน nil, หรือ container ไม่มีอยู่แล้วตอน stop) reconcile ตามความจริงจาก heartbeat.
// provisioning/deleting ปล่อยให้ job result / reaper จัดการ (heartbeat ไม่มีข้อมูลพอ)
func (h *Hub) reconcileServers(ctx context.Context, nodeID uuid.UUID, runningIDs []string) {
	// grace period กัน race: transition ที่เพิ่งเริ่ม (job result / ServerStatus ยังเดินทางอยู่)
	// ต้องไม่โดน reconcile ก่อนเวลา
	const transitionGrace = 60 * time.Second

	servers, err := h.st.ListServersByNode(ctx, nodeID)
	if err != nil {
		h.log.Error("list servers for reconcile failed", "node_id", nodeID, "error", err)
		return
	}
	running := make(map[string]bool, len(runningIDs))
	for _, id := range runningIDs {
		running[id] = true
	}

	for _, srv := range servers {
		isRunning := running[srv.ID.String()]
		stuck := time.Since(srv.UpdatedAt) > transitionGrace
		var newStatus string
		switch {
		case isRunning && (srv.Status == "stopped" || srv.Status == "errored"):
			newStatus = "running"
		case !isRunning && srv.Status == "running":
			newStatus = "stopped"
		case srv.Status == "starting" && stuck:
			if isRunning {
				newStatus = "running"
			} else {
				newStatus = "errored"
			}
		case srv.Status == "stopping" && stuck:
			if isRunning {
				newStatus = "running"
			} else {
				newStatus = "stopped"
			}
		default:
			continue
		}
		h.log.Warn("server status drift corrected", "server_id", srv.ID,
			"db_status", srv.Status, "actual_running", isRunning)
		h.setServerStatus(ctx, srv.ID, newStatus)
	}
}

func (h *Hub) handleConsoleOutput(ctx context.Context, nodeID uuid.UUID, out *agentv1.ConsoleOutput) {
	serverID, srv, ok := h.resolveServer(ctx, nodeID, out.ServerId)
	if !ok || srv == nil {
		return
	}
	if len(out.Lines) == 0 {
		return
	}
	h.rings.Get(serverID).Append(out.Lines)
	h.ws.BroadcastLines(serverID, out.Lines)
}

func (h *Hub) handleServerStatus(ctx context.Context, nodeID uuid.UUID, st *agentv1.ServerStatus) {
	serverID, srv, ok := h.resolveServer(ctx, nodeID, st.ServerId)
	if !ok || srv == nil {
		return
	}
	status := serverStateToStatus(st.State)
	if status == "" {
		return
	}
	h.log.Info("server status reported", "server_id", serverID, "status", status,
		"exit_code", st.ExitCode)
	h.setServerStatus(ctx, serverID, status)
}

func (h *Hub) handleServerStats(ctx context.Context, nodeID uuid.UUID, st *agentv1.ServerStats) {
	serverID, srv, ok := h.resolveServer(ctx, nodeID, st.ServerId)
	if !ok || srv == nil {
		return
	}
	stat := serverstats.Stat{
		CPUPercent:    st.CpuPercent,
		MemoryUsedMB:  st.MemoryUsedMb,
		MemoryLimitMB: st.MemoryLimitMb,
		NetRxBps:      st.NetRxBps,
		NetTxBps:      st.NetTxBps,
		DiskReadBps:   st.DiskReadBps,
		DiskWriteBps:  st.DiskWriteBps,
		UpdatedAt:     time.Now(),
	}
	h.stats.Set(serverID, stat)
	// mirror statsViewFor: push ตัวเลขเฉพาะตอน server running จริง (ไม่งั้น stats:null)
	if srv.Status == "running" {
		h.events.ServerStats(serverID, &events.ServerStatsPayload{
			CPUPercent:    stat.CPUPercent,
			MemoryUsedMB:  stat.MemoryUsedMB,
			MemoryLimitMB: stat.MemoryLimitMB,
			NetRxBps:      stat.NetRxBps,
			NetTxBps:      stat.NetTxBps,
			DiskReadBps:   stat.DiskReadBps,
			DiskWriteBps:  stat.DiskWriteBps,
			UpdatedAt:     stat.UpdatedAt,
		})
	} else {
		h.events.ServerStats(serverID, nil)
	}
}

// resolveServer ตรวจว่า server เป็นของ node นี้จริง — agent ที่ถูก compromise
// ห้าม inject console/status ของ server บน node อื่น
func (h *Hub) resolveServer(ctx context.Context, nodeID uuid.UUID, rawID string) (uuid.UUID, *store.Server, bool) {
	serverID, err := uuid.Parse(rawID)
	if err != nil {
		h.log.Warn("agent sent invalid server id", "node_id", nodeID, "server_id", rawID)
		return uuid.Nil, nil, false
	}
	srv, err := h.st.GetServerByID(ctx, serverID)
	if errors.Is(err, store.ErrNotFound) {
		// เกิดได้ปกติช่วง delete server เพิ่งลบแถวไป
		return serverID, nil, true
	}
	if err != nil {
		h.log.Error("load server failed", "server_id", serverID, "error", err)
		return serverID, nil, false
	}
	if srv.NodeID != nodeID {
		h.log.Warn("agent reported server owned by another node",
			"node_id", nodeID, "server_id", serverID, "owner_node_id", srv.NodeID)
		return serverID, nil, false
	}
	return serverID, srv, true
}

func (h *Hub) setServerStatus(ctx context.Context, serverID uuid.UUID, status string) {
	changed, err := h.st.UpdateServerStatusIfNotDeleting(ctx, serverID, status)
	if err != nil {
		h.log.Error("update server status failed", "server_id", serverID,
			"status", status, "error", err)
		return
	}
	if changed {
		h.ws.BroadcastStatus(serverID, status)
		h.events.ServerStatus(serverID, status)
	}
}

func serverStateToStatus(s agentv1.ServerState) string {
	switch s {
	case agentv1.ServerState_SERVER_STATE_STARTING:
		return "starting"
	case agentv1.ServerState_SERVER_STATE_RUNNING:
		return "running"
	case agentv1.ServerState_SERVER_STATE_STOPPING:
		return "stopping"
	case agentv1.ServerState_SERVER_STATE_STOPPED:
		return "stopped"
	case agentv1.ServerState_SERVER_STATE_ERRORED:
		return "errored"
	default:
		return ""
	}
}
