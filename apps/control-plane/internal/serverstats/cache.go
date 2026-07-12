// Package serverstats เก็บ resource usage ต่อ instance ที่ agent ส่งมาแบบ realtime
// ใน memory (ephemeral ไม่ลง DB — เป็น monitoring ชั่วคราว) ตาม docs/api.md
package serverstats

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type Stat struct {
	CPUPercent    float64
	MemoryUsedMB  int64
	MemoryLimitMB int64
	NetRxBps      float64
	NetTxBps      float64
	DiskReadBps   float64
	DiskWriteBps  float64
	UpdatedAt     time.Time
}

// Cache thread-safe map serverID -> Stat ล่าสุด. agent ส่ง stats ผ่าน gRPC
// (agenthub เขียน) และ HTTP handler อ่าน (คนละ goroutine) จึงต้องมี mutex
type Cache struct {
	mu   sync.RWMutex
	byID map[uuid.UUID]Stat
}

func NewCache() *Cache {
	return &Cache{byID: make(map[uuid.UUID]Stat)}
}

func (c *Cache) Set(serverID uuid.UUID, s Stat) {
	c.mu.Lock()
	c.byID[serverID] = s
	c.mu.Unlock()
}

func (c *Cache) Get(serverID uuid.UUID) (Stat, bool) {
	c.mu.RLock()
	s, ok := c.byID[serverID]
	c.mu.RUnlock()
	return s, ok
}
