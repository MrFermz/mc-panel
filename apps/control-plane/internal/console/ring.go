// Package console คุม console history (ring buffer ต่อ server) และ WebSocket
// streaming ระหว่าง browser กับ control-plane
package console

import (
	"sync"

	"github.com/google/uuid"
)

// RingSize ตาม docs/api.md — history 500 บรรทัดล่าสุด
const RingSize = 500

type Ring struct {
	mu    sync.Mutex
	buf   []string
	start int
	count int
}

func NewRing(size int) *Ring {
	return &Ring{buf: make([]string, size)}
}

func (r *Ring) Append(lines []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, line := range lines {
		if r.count < len(r.buf) {
			r.buf[(r.start+r.count)%len(r.buf)] = line
			r.count++
		} else {
			r.buf[r.start] = line
			r.start = (r.start + 1) % len(r.buf)
		}
	}
}

func (r *Ring) Snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, r.count)
	for i := 0; i < r.count; i++ {
		out[i] = r.buf[(r.start+i)%len(r.buf)]
	}
	return out
}

type Registry struct {
	mu    sync.Mutex
	rings map[uuid.UUID]*Ring
}

func NewRegistry() *Registry {
	return &Registry{rings: make(map[uuid.UUID]*Ring)}
}

func (g *Registry) Get(serverID uuid.UUID) *Ring {
	g.mu.Lock()
	defer g.mu.Unlock()
	r, ok := g.rings[serverID]
	if !ok {
		r = NewRing(RingSize)
		g.rings[serverID] = r
	}
	return r
}

func (g *Registry) Drop(serverID uuid.UUID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.rings, serverID)
}
