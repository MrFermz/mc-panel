package console

import (
	"sync"

	"github.com/google/uuid"
)

// Event คือ message ฝั่ง server -> client ตาม protocol ใน docs/api.md
type Event struct {
	Type    string   `json:"type"`
	Lines   []string `json:"lines,omitempty"`
	Status  string   `json:"status,omitempty"`
	Code    string   `json:"code,omitempty"`
	Message string   `json:"message,omitempty"`
}

type Hub struct {
	mu   sync.Mutex
	subs map[uuid.UUID]map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[uuid.UUID]map[chan Event]struct{})}
}

func (h *Hub) Subscribe(serverID uuid.UUID) chan Event {
	ch := make(chan Event, 128)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[serverID] == nil {
		h.subs[serverID] = make(map[chan Event]struct{})
	}
	h.subs[serverID][ch] = struct{}{}
	return ch
}

// SubscribeWithHistory อ่าน ring snapshot แล้ว register subscriber ภายใต้ mu
// เดียวกันแบบ atomic: broadcast (ซึ่งถือ mu เช่นกัน) จึงแทรกระหว่าง snapshot กับ
// register ไม่ได้ — บรรทัดหนึ่งจะอยู่ใน history หรือ event stream อย่างใดอย่างหนึ่ง
// ไม่ซ้ำ. (mu ล็อกก่อน ring.mu เสมอ — ทางอื่นไม่ถือ ring.mu แล้วขอ mu จึงไม่ deadlock)
func (h *Hub) SubscribeWithHistory(serverID uuid.UUID, ring *Ring) ([]string, chan Event) {
	ch := make(chan Event, 128)
	h.mu.Lock()
	defer h.mu.Unlock()
	history := ring.Snapshot()
	if h.subs[serverID] == nil {
		h.subs[serverID] = make(map[chan Event]struct{})
	}
	h.subs[serverID][ch] = struct{}{}
	return history, ch
}

func (h *Hub) Unsubscribe(serverID uuid.UUID, ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.subs[serverID]; m != nil {
		delete(m, ch)
		if len(m) == 0 {
			delete(h.subs, serverID)
		}
	}
}

func (h *Hub) BroadcastLines(serverID uuid.UUID, lines []string) {
	h.broadcast(serverID, Event{Type: "lines", Lines: lines})
}

func (h *Hub) BroadcastStatus(serverID uuid.UUID, status string) {
	h.broadcast(serverID, Event{Type: "status", Status: status})
}

func (h *Hub) broadcast(serverID uuid.UUID, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[serverID] {
		// non-blocking: client ที่อ่านช้าจน buffer เต็มยอมให้ event หล่นหาย
		// ดีกว่า block broadcast ของทุก client
		select {
		case ch <- ev:
		default:
		}
	}
}
