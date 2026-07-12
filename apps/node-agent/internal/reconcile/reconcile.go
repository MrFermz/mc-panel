// Package reconcile ปรับ state ให้ตรงความจริงตอน agent boot —
// container อาจเกิด/ตายไประหว่างที่ agent ไม่อยู่
package reconcile

import (
	"context"
	"fmt"
	"log"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/mc-panel/node-agent/internal/runner"
	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"
)

type StatusSender interface {
	SendServerStatus(serverID string, state agentv1.ServerState, exitCode int32) error
}

type ConsoleManager interface {
	Attach(serverID string) error
}

func Run(ctx context.Context, cli *client.Client, sender StatusSender, consoles ConsoleManager) error {
	f := filters.NewArgs(filters.Arg("label", runner.LabelManagedBy+"="+runner.LabelManagedByValue))
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return fmt.Errorf("list managed containers: %w", err)
	}

	for _, c := range containers {
		serverID := c.Labels[runner.LabelServerID]
		if serverID == "" {
			continue
		}
		if c.State == container.StateRunning {
			if err := sender.SendServerStatus(serverID, agentv1.ServerState_SERVER_STATE_RUNNING, 0); err != nil {
				log.Printf("reconcile status send failed: server=%s err=%v", serverID, err)
			}
			if err := consoles.Attach(serverID); err != nil {
				log.Printf("reconcile console attach failed: server=%s err=%v", serverID, err)
			}
			continue
		}
		// container หยุดไปแล้วระหว่าง agent ไม่อยู่ — รายงาน exit code จริงจาก inspect
		exitCode := 0
		if insp, err := cli.ContainerInspect(ctx, c.ID); err == nil && insp.State != nil {
			exitCode = insp.State.ExitCode
		}
		state := agentv1.ServerState_SERVER_STATE_STOPPED
		if exitCode != 0 {
			state = agentv1.ServerState_SERVER_STATE_ERRORED
		}
		if err := sender.SendServerStatus(serverID, state, int32(exitCode)); err != nil {
			log.Printf("reconcile status send failed: server=%s err=%v", serverID, err)
		}
	}
	log.Printf("reconcile complete: managed_containers=%d", len(containers))
	return nil
}
