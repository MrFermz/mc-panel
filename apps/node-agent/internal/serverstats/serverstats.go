// Package serverstats รายงาน resource usage ต่อ instance ทุก ~5 วินาที
// เป็นข้อมูล realtime/ephemeral (ไม่เก็บลง DB) — ส่งไม่ได้ (stream หลุด) ข้ามรอบไป
package serverstats

import (
	"context"
	"log"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/mc-panel/node-agent/internal/mcstate"
	"github.com/mc-panel/node-agent/internal/runner"
	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"
)

const interval = 5 * time.Second

type Sender interface {
	SendServerStats(st *agentv1.ServerStats) error
}

// Statter คือส่วนของ Runner ที่ reporter ต้องใช้ (แยก interface ตามสไตล์ package อื่น)
type Statter interface {
	Stats(id string) (runner.ResourceStats, error)
}

// Snapshotter คือส่วนของ mcstate.Tracker ที่ reporter ต้องใช้ — สถานะในเกมที่อ่านจาก console
// (nil ได้: ไม่มี tracker = ส่ง stats เฉพาะ resource เหมือนเดิม)
type Snapshotter interface {
	Snapshot(serverID string) mcstate.Snapshot
}

// ioSample เก็บ counter สะสมของรอบก่อน เพื่อคำนวณ rate/sec จาก delta ต่อ container
type ioSample struct {
	rx, tx, read, write uint64
	at                  time.Time
}

// Run ค้างจน ctx ถูกยกเลิก
func Run(ctx context.Context, cli *client.Client, statter Statter, sender Sender, state Snapshotter) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// prev per-container — จำ counter สะสมรอบก่อนไว้แปลงเป็น rate (state อยู่ที่ agent เท่านั้น)
	prev := make(map[string]ioSample)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report(ctx, cli, statter, sender, state, prev)
		}
	}
}

func report(ctx context.Context, cli *client.Client, statter Statter, sender Sender, state Snapshotter, prev map[string]ioSample) {
	// ContainerList (ไม่ตั้ง All) คืนเฉพาะ container ที่รันอยู่ — ตรงกับที่ heartbeat ใช้ collect
	f := filters.NewArgs(filters.Arg("label", runner.LabelManagedBy+"="+runner.LabelManagedByValue))
	containers, err := cli.ContainerList(ctx, container.ListOptions{Filters: f})
	if err != nil {
		log.Printf("server stats container list failed: %v", err)
		return
	}
	// เก็บ id ที่ยังรันอยู่ เพื่อ prune sample ของ container ที่หายไป (กัน map โต + counter reset ตอน restart)
	live := make(map[string]struct{}, len(containers))
	for _, c := range containers {
		id := c.Labels[runner.LabelServerID]
		if id == "" {
			continue
		}
		live[id] = struct{}{}
		st, err := statter.Stats(id)
		if err != nil {
			// container อาจเพิ่งตายระหว่าง list กับ stats — ข้ามตัวนี้ ไปตัวถัดไป
			log.Printf("server stats collect failed: server=%s err=%v", id, err)
			continue
		}
		now := time.Now()
		cur := ioSample{rx: st.NetRxBytes, tx: st.NetTxBytes, read: st.DiskReadBytes, write: st.DiskWrBytes, at: now}
		var netRx, netTx, dRead, dWrite float64
		if p, ok := prev[id]; ok {
			if secs := now.Sub(p.at).Seconds(); secs > 0 {
				netRx = rate(cur.rx, p.rx, secs)
				netTx = rate(cur.tx, p.tx, secs)
				dRead = rate(cur.read, p.read, secs)
				dWrite = rate(cur.write, p.write, secs)
			}
		}
		prev[id] = cur
		// สถานะในเกมมาจาก console (คนละแหล่งกับ container stats) — ไม่มี tracker ก็ส่งเป็นค่าว่าง
		var snap mcstate.Snapshot
		if state != nil {
			snap = state.Snapshot(id)
		}
		if err := sender.SendServerStats(&agentv1.ServerStats{
			ServerId:      id,
			CpuPercent:    st.CPUPercent,
			MemoryUsedMb:  int64(st.MemoryMB),
			MemoryLimitMb: int64(st.MemoryLimitMB),
			NetRxBps:      netRx,
			NetTxBps:      netTx,
			DiskReadBps:   dRead,
			DiskWriteBps:  dWrite,
			// agent สร้าง container ใหม่ทุกครั้งที่ start (ลบทิ้งตอน stop/crash)
			// Created จึงเท่ากับเวลาที่เริ่มรันรอบนี้ — ไม่ต้อง inspect เพิ่มทุก 5 วิ
			StartedAtUnix: c.Created,
			OnlinePlayers: snap.Online,
			MaxPlayers:    int32(snap.MaxPlayers),
			Tps:           snap.TPS,
		}); err != nil {
			// stream หลุด — ข้ามทั้งรอบ รอบถัดไปมาใหม่
			return
		}
	}
	for id := range prev {
		if _, ok := live[id]; !ok {
			delete(prev, id)
		}
	}
}

// rate คืน bytes/sec จาก delta ของ counter สะสม — counter ถอยหลัง (restart/reset) คืน 0
func rate(cur, prev uint64, secs float64) float64 {
	if cur < prev {
		return 0
	}
	return float64(cur-prev) / secs
}
