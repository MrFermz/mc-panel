package jobs

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mc-panel/node-agent/internal/provision"
	"github.com/mc-panel/node-agent/internal/runner"
	jobv1 "github.com/mc-panel/proto/gen/go/mcpanel/job/v1"
)

// Handler แปลง JobEnvelope เป็นการเรียก runner/provisioner
// ทุก handler ต้อง idempotent — โดน redeliver ซ้ำต้องไม่พัง
type Handler struct {
	runner  *runner.DockerRunner
	prov    *provision.Provisioner
	dataDir string
}

func NewHandler(r *runner.DockerRunner, prov *provision.Provisioner, dataDir string) *Handler {
	return &Handler{runner: r, prov: prov, dataDir: dataDir}
}

func (h *Handler) Process(ctx context.Context, env *jobv1.JobEnvelope) (detail string, err error) {
	switch p := env.Payload.(type) {
	case *jobv1.JobEnvelope_CreateServer:
		return h.prov.CreateServer(ctx, env.ServerId, provision.Spec{
			ServerType: p.CreateServer.ServerType,
			MCVersion:  p.CreateServer.McVersion,
			AcceptEULA: p.CreateServer.AcceptEula,
		})
	case *jobv1.JobEnvelope_StartServer:
		return "", h.runner.Start(ctx, runner.ServerConfig{
			ID:       env.ServerId,
			MemoryMB: int(p.StartServer.MemoryMb),
			WorkDir:  filepath.Join(h.dataDir, env.ServerId),
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
