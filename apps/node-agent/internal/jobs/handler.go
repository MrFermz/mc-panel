package jobs

import (
	"context"
	"fmt"

	"github.com/game-manager/node-agent/internal/filemanager"
	"github.com/game-manager/node-agent/internal/provision"
	"github.com/game-manager/node-agent/internal/runner"
	jobv1 "github.com/game-manager/proto/gen/go/gamemanager/job/v1"
)

// Handler แปลง JobEnvelope เป็นการเรียก runner/provisioner
// ทุก handler ต้อง idempotent — โดน redeliver ซ้ำต้องไม่พัง
type Handler struct {
	runner *runner.DockerRunner
	prov   *provision.Provisioner
	layout filemanager.Layout
}

func NewHandler(r *runner.DockerRunner, prov *provision.Provisioner, layout filemanager.Layout) *Handler {
	return &Handler{runner: r, prov: prov, layout: layout}
}

func (h *Handler) Process(ctx context.Context, env *jobv1.JobEnvelope) (detail string, err error) {
	switch p := env.Payload.(type) {
	case *jobv1.JobEnvelope_CreateServer:
		return h.prov.CreateServer(ctx, env.ServerId, provision.Spec{
			Game:          p.CreateServer.Game,
			Variant:       p.CreateServer.Variant,
			GameVersion:   p.CreateServer.GameVersion,
			AcceptLicense: p.CreateServer.AcceptLicense,
		})
	case *jobv1.JobEnvelope_StartServer:
		// job start ไม่พกเกมมาด้วย — dir ของ instance มาจากการสแกนชั้นเกมบน disk
		workDir, err := h.layout.Find(env.ServerId)
		if err != nil {
			return "", err
		}
		return "", h.runner.Start(ctx, runner.ServerConfig{
			ID:       env.ServerId,
			MemoryMB: int(p.StartServer.MemoryMb),
			WorkDir:  workDir,
			Port:     int(p.StartServer.HostPort),
			Image:    p.StartServer.DockerImage,
		})
	case *jobv1.JobEnvelope_StopServer:
		return "", h.runner.Stop(env.ServerId, p.StopServer.Graceful)
	case *jobv1.JobEnvelope_KillServer:
		return "", h.runner.Kill(env.ServerId)
	case *jobv1.JobEnvelope_DeleteServer:
		// ลบ container ก่อน (kill = force) แล้วค่อยลบ directory
		if err := h.runner.Kill(env.ServerId); err != nil {
			return "", err
		}
		return "", h.prov.DeleteServer(env.ServerId)
	default:
		return "", fmt.Errorf("job %s has unknown payload type", env.JobId)
	}
}
