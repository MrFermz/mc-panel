package jobs

import (
	"context"
	"encoding/json"
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
	case *jobv1.JobEnvelope_ImportServer:
		detectedVersion, ierr := h.prov.ImportServer(ctx, env.ServerId, provision.ImportSpec{
			ServerType:  p.ImportServer.ServerType,
			MCVersion:   p.ImportServer.McVersion,
			AcceptEULA:  p.ImportServer.AcceptEula,
			ArchivePath: p.ImportServer.ArchivePath,
		})
		if ierr != nil {
			return "", ierr
		}
		// control-plane อ่าน Detail ตอน job สำเร็จเพื่อ sync mc_version ของ server ให้ตรงจริง
		if detectedVersion != "" {
			b, merr := json.Marshal(struct {
				MCVersion string `json:"mc_version"`
			}{detectedVersion})
			if merr != nil {
				return "", merr
			}
			return string(b), nil
		}
		return "", nil
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
