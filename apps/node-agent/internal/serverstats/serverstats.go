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

// Run ค้างจน ctx ถูกยกเลิก
func Run(ctx context.Context, cli *client.Client, statter Statter, sender Sender) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report(ctx, cli, statter, sender)
		}
	}
}

func report(ctx context.Context, cli *client.Client, statter Statter, sender Sender) {
	// ContainerList (ไม่ตั้ง All) คืนเฉพาะ container ที่รันอยู่ — ตรงกับที่ heartbeat ใช้ collect
	f := filters.NewArgs(filters.Arg("label", runner.LabelManagedBy+"="+runner.LabelManagedByValue))
	containers, err := cli.ContainerList(ctx, container.ListOptions{Filters: f})
	if err != nil {
		log.Printf("server stats container list failed: %v", err)
		return
	}
	for _, c := range containers {
		id := c.Labels[runner.LabelServerID]
		if id == "" {
			continue
		}
		st, err := statter.Stats(id)
		if err != nil {
			// container อาจเพิ่งตายระหว่าง list กับ stats — ข้ามตัวนี้ ไปตัวถัดไป
			log.Printf("server stats collect failed: server=%s err=%v", id, err)
			continue
		}
		if err := sender.SendServerStats(&agentv1.ServerStats{
			ServerId:      id,
			CpuPercent:    st.CPUPercent,
			MemoryUsedMb:  int64(st.MemoryMB),
			MemoryLimitMb: int64(st.MemoryLimitMB),
		}); err != nil {
			// stream หลุด — ข้ามทั้งรอบ รอบถัดไปมาใหม่
			return
		}
	}
}
