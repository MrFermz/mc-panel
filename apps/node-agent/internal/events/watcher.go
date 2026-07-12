// Package events แปลง docker container events (start/die) ของ container
// ที่ agent จัดการ เป็น ServerStatus รายงานขึ้น control plane
package events

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	dockerevents "github.com/docker/docker/api/types/events"
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
	Detach(serverID string)
}

// StopTracker บอกว่า die event มาจากคำสั่ง stop/kill ของเราเอง
// — ใช้แยก STOPPED (ตั้งใจ) กับ ERRORED (crash)
type StopTracker interface {
	ConsumeStopRequested(id string) bool
}

// Cleaner ลบ container ที่ crash แล้วค้าง (data คงไว้) — เรียกเฉพาะเคส ERRORED
type Cleaner interface {
	Cleanup(id string) error
}

// Notifier ดัน console line จากระบบให้ user เห็น — ใช้แจ้งตอน crash
type Notifier interface {
	PushSystemLine(serverID, text string)
}

// Watch ค้างจน ctx ถูกยกเลิก — event stream หลุดจะต่อใหม่เอง
func Watch(ctx context.Context, cli *client.Client, tracker StopTracker, sender StatusSender, consoles ConsoleManager, cleaner Cleaner, notifier Notifier) {
	f := filters.NewArgs(
		filters.Arg("type", "container"),
		filters.Arg("label", runner.LabelManagedBy+"="+runner.LabelManagedByValue),
		filters.Arg("event", "start"),
		filters.Arg("event", "die"),
	)
	for ctx.Err() == nil {
		msgCh, errCh := cli.Events(ctx, dockerevents.ListOptions{Filters: f})
	stream:
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-msgCh:
				handle(msg, tracker, sender, consoles, cleaner, notifier)
			case err := <-errCh:
				if ctx.Err() != nil {
					return
				}
				log.Printf("docker events stream error: %v (reconnecting in 2s)", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
				break stream
			}
		}
	}
}

func handle(msg dockerevents.Message, tracker StopTracker, sender StatusSender, consoles ConsoleManager, cleaner Cleaner, notifier Notifier) {
	serverID := msg.Actor.Attributes[runner.LabelServerID]
	if serverID == "" {
		return
	}
	switch msg.Action {
	case dockerevents.ActionStart:
		if err := sender.SendServerStatus(serverID, agentv1.ServerState_SERVER_STATE_RUNNING, 0); err != nil {
			log.Printf("send server status failed: server=%s err=%v", serverID, err)
		}
		if err := consoles.Attach(serverID); err != nil {
			log.Printf("console attach on start failed: server=%s err=%v", serverID, err)
		}
	case dockerevents.ActionDie:
		exitCode, _ := strconv.Atoi(msg.Actor.Attributes["exitCode"])
		requested := tracker.ConsumeStopRequested(serverID)
		state := agentv1.ServerState_SERVER_STATE_STOPPED
		if exitCode != 0 && !requested {
			state = agentv1.ServerState_SERVER_STATE_ERRORED
		}
		log.Printf("container died: server=%s exit_code=%d stop_requested=%t", serverID, exitCode, requested)
		if err := sender.SendServerStatus(serverID, state, int32(exitCode)); err != nil {
			log.Printf("send server status failed: server=%s err=%v", serverID, err)
		}
		if state == agentv1.ServerState_SERVER_STATE_ERRORED {
			// crash เท่านั้น: container ค้างเป็น Exited ต้องลบเอง (Stop()/Kill() ลบให้อยู่แล้ว
			// ในเคสสั่งหยุด — ที่นี่จึงห้ามลบซ้ำหรือ push line ในเคสนั้น)
			if err := cleaner.Cleanup(serverID); err != nil {
				log.Printf("cleanup crashed container failed: server=%s err=%v", serverID, err)
			}
			// exit 137 = OOM kill พบบ่อยสุด — ข้อความ user-facing ภาษาอังกฤษ
			notifier.PushSystemLine(serverID, fmt.Sprintf("[mc-panel] instance crashed (exit code %d) — removed the leftover container, your data is preserved. Press Start to run again.", exitCode))
		}
		consoles.Detach(serverID)
	}
}
