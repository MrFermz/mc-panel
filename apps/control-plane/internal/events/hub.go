// Package events เป็น fan-out realtime ฝั่ง browser (WS /ws/events) — push
// server_status/server_stats/node_stats/server_jobs/job_update จาก hook ใน agenthub/jobs
// ไปหน้าเว็บ เพื่อเลิก poll REST. คนละ hub กับ console (console เป็น per-server
// stream + ring history; อันนี้เป็น panel-wide event ที่ filter ต่อ subscriber
// ตามสิทธิ์). ห้าม import httpapi เพื่อกัน import cycle — payload struct นิยามเอง
// ให้ตรง JSON shape ของ REST ทุกตัว
package events

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mc-panel/control-plane/internal/store"
)

// ServerStatsPayload มิเรอร์ httpapi.serverStatsView field-for-field
type ServerStatsPayload struct {
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryUsedMB  int64     `json:"memory_used_mb"`
	MemoryLimitMB int64     `json:"memory_limit_mb"`
	NetRxBps      float64   `json:"net_rx_bps"`
	NetTxBps      float64   `json:"net_tx_bps"`
	DiskReadBps   float64   `json:"disk_read_bps"`
	DiskWriteBps  float64    `json:"disk_write_bps"`
	StartedAt     *time.Time `json:"started_at"`
	OnlinePlayers []string   `json:"online_players"`
	MaxPlayers    int        `json:"max_players"`
	TPS           float64    `json:"tps"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// NodePayload มิเรอร์ httpapi.nodeView field-for-field (หนึ่ง item ของ GET /api/nodes)
type NodePayload struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	AgentVersion    string     `json:"agent_version"`
	OS              string     `json:"os"`
	Arch            string     `json:"arch"`
	CPUPercent      float64    `json:"cpu_percent"`
	MemoryUsedMB    int64      `json:"memory_used_mb"`
	MemoryTotalMB   int64      `json:"memory_total_mb"`
	DiskUsedMB      int64      `json:"disk_used_mb"`
	DiskTotalMB     int64      `json:"disk_total_mb"`
	NetRxBps        float64    `json:"net_rx_bps"`
	NetTxBps        float64    `json:"net_tx_bps"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

// NewNodePayload สร้าง payload จาก store.Node — จุดเดียวที่ map field เพื่อกัน drift
// กับ nodeView (ทั้งคู่ต้องตรง shape เดียวกันตาม docs/api.md)
func NewNodePayload(n *store.Node) NodePayload {
	return NodePayload{
		ID:              n.ID,
		Name:            n.Name,
		Status:          n.Status,
		AgentVersion:    n.AgentVersion,
		OS:              n.OS,
		Arch:            n.Arch,
		CPUPercent:      n.CPUPercent,
		MemoryUsedMB:    n.MemoryUsedMB,
		MemoryTotalMB:   n.MemoryTotalMB,
		DiskUsedMB:      n.DiskUsedMB,
		DiskTotalMB:     n.DiskTotalMB,
		NetRxBps:        n.NetRxBps,
		NetTxBps:        n.NetTxBps,
		LastHeartbeatAt: n.LastHeartbeatAt,
		CreatedAt:       n.CreatedAt,
	}
}

// message* struct — marshal ครั้งเดียวที่ broadcast แล้ว fan-out เป็น json.RawMessage
// (Stats ไม่มี omitempty เพราะ contract บังคับ null เมื่อ server ไม่ได้ running)
type serverStatsMsg struct {
	Type     string              `json:"type"`
	ServerID uuid.UUID           `json:"server_id"`
	Stats    *ServerStatsPayload `json:"stats"`
}

type serverStatusMsg struct {
	Type     string    `json:"type"`
	ServerID uuid.UUID `json:"server_id"`
	Status   string    `json:"status"`
}

type serverJobsMsg struct {
	Type     string    `json:"type"`
	ServerID uuid.UUID `json:"server_id"`
}

// jobUpdateMsg = ความคืบหน้าของ lifecycle job ตัวหนึ่ง (start/stop/kill/restart/create/
// import/delete) ตั้งแต่ dispatch จนจบ — ต่างจาก server_jobs ที่บอกแค่ "list เปลี่ยน
// ไป refetch เอง" อันนี้ carry ผลลัพธ์จริงรวม error ให้ UI แจ้ง user ได้ทันทีโดยไม่ต้อง
// refetch. มี error text อยู่ในนั้นจึงต้อง fan-out แบบ filter ตามสิทธิ์เสมอ
type jobUpdateMsg struct {
	Type     string    `json:"type"`
	ServerID uuid.UUID `json:"server_id"`
	JobID    uuid.UUID `json:"job_id"`
	JobType  string    `json:"job_type"`
	Status   string    `json:"status"`
	Error    string    `json:"error"`
	// Restart = job นี้เป็นขา stop ของ restart (ขา start ตามมาทีหลังเป็นคนละ job) —
	// web ใช้กันไม่ให้ขึ้น "stopped สำเร็จ" กลางคัน restart
	Restart bool `json:"restart"`
}

// serverListMsg = server_added/server_removed — แจ้ง browser ว่า list ของ server
// เปลี่ยน (create/import/delete) ให้ refetch ["servers"]. carry แค่ server_id จึง
// broadcast แบบ unfiltered ได้ (ไม่มีข้อมูลรั่ว — refetch ฝั่ง web เช็คสิทธิ์เอง)
type serverListMsg struct {
	Type     string    `json:"type"`
	ServerID uuid.UUID `json:"server_id"`
}

type nodeStatsMsg struct {
	Type string      `json:"type"`
	Node NodePayload `json:"node"`
}

// subscriber คือ browser connection หนึ่งตัว — write loop อ่านจาก ch
type subscriber struct {
	ch chan json.RawMessage
	// seeAllServers: is_admin หรือ servers.view_all → เห็น server event ทุกตัว
	seeAllServers bool
	// seeNodes: is_admin หรือ nodes.view → เห็น node_stats
	seeNodes bool

	// allowed คือ set ของ server ที่ subscriber นี้เข้าถึงได้ (owner/permission)
	// handler refresh ทุก ~15s ผ่าน setAllowed → broadcast อ่านภายใต้ mu เดียวกัน
	mu      sync.Mutex
	allowed map[uuid.UUID]bool
}

func (s *subscriber) canSeeServer(serverID uuid.UUID) bool {
	if s.seeAllServers {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allowed[serverID]
}

func (s *subscriber) setAllowed(allowed map[uuid.UUID]bool) {
	s.mu.Lock()
	s.allowed = allowed
	s.mu.Unlock()
}

// Hub fan-out ไป browser subscribers ทั้งหมด โดย filter ต่อ subscriber ตามสิทธิ์
type Hub struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[*subscriber]struct{})}
}

// subscribe register connection ใหม่ (buffer ลึกเท่า console hub = 128)
func (h *Hub) subscribe(seeAllServers, seeNodes bool, allowed map[uuid.UUID]bool) *subscriber {
	sub := &subscriber{
		ch:            make(chan json.RawMessage, 128),
		seeAllServers: seeAllServers,
		seeNodes:      seeNodes,
		allowed:       allowed,
	}
	h.mu.Lock()
	h.subs[sub] = struct{}{}
	h.mu.Unlock()
	return sub
}

func (h *Hub) unsubscribe(sub *subscriber) {
	h.mu.Lock()
	delete(h.subs, sub)
	h.mu.Unlock()
}

// ServerStats push stats ของ server หนึ่งตัว — stat=nil → JSON stats:null
// (mirror statsViewFor: emit ตัวเลขเฉพาะตอน running & มี cache)
func (h *Hub) ServerStats(serverID uuid.UUID, stat *ServerStatsPayload) {
	data, err := json.Marshal(serverStatsMsg{Type: "server_stats", ServerID: serverID, Stats: stat})
	if err != nil {
		return
	}
	h.fanoutServer(serverID, data)
}

func (h *Hub) ServerStatus(serverID uuid.UUID, status string) {
	data, err := json.Marshal(serverStatusMsg{Type: "server_status", ServerID: serverID, Status: status})
	if err != nil {
		return
	}
	h.fanoutServer(serverID, data)
}

// JobUpdate push สถานะล่าสุดของ job หนึ่งตัว (pending/running/succeeded/failed)
func (h *Hub) JobUpdate(serverID, jobID uuid.UUID, jobType, status, errMsg string, restart bool) {
	data, err := json.Marshal(jobUpdateMsg{
		Type:     "job_update",
		ServerID: serverID,
		JobID:    jobID,
		JobType:  jobType,
		Status:   status,
		Error:    errMsg,
		Restart:  restart,
	})
	if err != nil {
		return
	}
	h.fanoutServer(serverID, data)
}

func (h *Hub) ServerJobs(serverID uuid.UUID) {
	data, err := json.Marshal(serverJobsMsg{Type: "server_jobs", ServerID: serverID})
	if err != nil {
		return
	}
	h.fanoutServer(serverID, data)
}

// ServerAdded/ServerRemoved broadcast แบบ unfiltered ไปทุก subscriber — payload มีแค่
// server_id ไม่ใช่ข้อมูล server จริง จึงไม่ผ่าน per-subscriber auth filter (ต่างจาก
// server_stats/status): web รับแล้ว invalidate ["servers"] → refetch ที่เช็คสิทธิ์อีกที
func (h *Hub) ServerAdded(serverID uuid.UUID) {
	data, err := json.Marshal(serverListMsg{Type: "server_added", ServerID: serverID})
	if err != nil {
		return
	}
	h.fanoutAll(data)
}

func (h *Hub) ServerRemoved(serverID uuid.UUID) {
	data, err := json.Marshal(serverListMsg{Type: "server_removed", ServerID: serverID})
	if err != nil {
		return
	}
	h.fanoutAll(data)
}

func (h *Hub) NodeStats(node NodePayload) {
	data, err := json.Marshal(nodeStatsMsg{Type: "node_stats", Node: node})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		if sub.seeNodes {
			send(sub, data)
		}
	}
}

// fanoutServer ส่งไปเฉพาะ subscriber ที่เห็น server นี้ได้ (view_all/admin หรือมีสิทธิ์)
func (h *Hub) fanoutServer(serverID uuid.UUID, data json.RawMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		if sub.canSeeServer(serverID) {
			send(sub, data)
		}
	}
}

// fanoutAll ส่งไปทุก subscriber โดยไม่ filter (สำหรับ event ที่ปลอดภัยกับทุกคน เช่น
// server_added/server_removed ที่ carry แค่ server_id)
func (h *Hub) fanoutAll(data json.RawMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		send(sub, data)
	}
}

// send non-blocking: subscriber ที่อ่านช้าจน buffer เต็มยอมให้ event หล่นหาย
// (เหมือน console hub) — ดีกว่า block fan-out ของทุก connection
func send(sub *subscriber, data json.RawMessage) {
	select {
	case sub.ch <- data:
	default:
	}
}
