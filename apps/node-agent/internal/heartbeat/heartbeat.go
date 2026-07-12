// Package heartbeat รายงาน resource ของเครื่อง + server ที่รันอยู่จริง ทุก 10 วินาที
package heartbeat

import (
	"context"
	"log"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/mc-panel/node-agent/internal/runner"
	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

const interval = 10 * time.Second

type Sender interface {
	SendHeartbeat(hb *agentv1.Heartbeat) error
}

// Run ค้างจน ctx ถูกยกเลิก — ส่งไม่ได้ (stream หลุด) ให้ข้ามรอบนั้นไปเลย
// heartbeat เป็นข้อมูล realtime ที่รอบถัดไปมาแทนได้เสมอ
func Run(ctx context.Context, cli *client.Client, sender Sender, dataDir string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := sender.SendHeartbeat(collect(ctx, cli, dataDir)); err != nil {
				log.Printf("heartbeat skipped: %v", err)
			}
		}
	}
}

func collect(ctx context.Context, cli *client.Client, dataDir string) *agentv1.Heartbeat {
	hb := &agentv1.Heartbeat{}

	// interval=0 คือวัด delta จากการเรียกครั้งก่อน — ไม่ block รอ sample
	if percents, err := cpu.Percent(0, false); err == nil && len(percents) > 0 {
		hb.CpuPercent = percents[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		hb.MemoryUsedMb = int64(vm.Used / (1024 * 1024))
		hb.MemoryTotalMb = int64(vm.Total / (1024 * 1024))
	}
	if du, err := disk.Usage(dataDir); err == nil {
		hb.DiskUsedMb = int64(du.Used / (1024 * 1024))
		hb.DiskTotalMb = int64(du.Total / (1024 * 1024))
	}

	f := filters.NewArgs(filters.Arg("label", runner.LabelManagedBy+"="+runner.LabelManagedByValue))
	if containers, err := cli.ContainerList(ctx, container.ListOptions{Filters: f}); err == nil {
		for _, c := range containers {
			if id := c.Labels[runner.LabelServerID]; id != "" {
				hb.RunningServerIds = append(hb.RunningServerIds, id)
			}
		}
	} else {
		log.Printf("heartbeat container list failed: %v", err)
	}
	return hb
}
