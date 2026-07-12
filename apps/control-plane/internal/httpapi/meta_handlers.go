package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
	"github.com/mc-panel/control-plane/internal/versions"
)

func (a *API) handleGetJob(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())

	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "job_not_found", "job not found")
		return
	}

	job, err := a.st.GetJobByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "job_not_found", "job not found")
		return
	}
	if err != nil {
		a.log.Error("load job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if !a.canSeeJob(r, user, job) {
		writeError(w, http.StatusForbidden, "forbidden", "no access to this job")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"job": toJobView(job)})
}

// canSeeJob: สิทธิ์ตาม server ของ job — ถ้า server โดนลบไปแล้ว (server_id NULL)
// เหลือ admin กับคนสั่ง job เองที่ยังดูได้
func (a *API) canSeeJob(r *http.Request, user *store.User, job *store.Job) bool {
	if user.IsAdmin {
		return true
	}
	if job.ServerID == nil {
		return job.RequestedBy != nil && *job.RequestedBy == user.ID
	}
	_, err := a.st.GetPermission(r.Context(), user.ID, *job.ServerID)
	if err == nil {
		return true
	}
	if !errors.Is(err, store.ErrNotFound) {
		a.log.Error("load permission for job failed", "error", err)
	}
	return job.RequestedBy != nil && *job.RequestedBy == user.ID
}

type serverTypeMeta struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	NeedsEula bool   `json:"needs_eula"`
}

func (a *API) handleServerTypes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"types": []serverTypeMeta{
		{ID: "vanilla", Label: "Vanilla", NeedsEula: true},
		{ID: "paper", Label: "Paper", NeedsEula: true},
		{ID: "fabric", Label: "Fabric", NeedsEula: true},
		{ID: "forge", Label: "Forge", NeedsEula: true},
		{ID: "velocity", Label: "Velocity (proxy)", NeedsEula: false},
	}})
}

func (a *API) handleVersions(w http.ResponseWriter, r *http.Request) {
	serverType := r.URL.Query().Get("type")
	list, err := a.versions.Versions(r.Context(), serverType)
	if errors.Is(err, versions.ErrUnknownType) {
		writeError(w, http.StatusBadRequest, "invalid_server_type",
			"type must be one of: vanilla, paper, fabric, forge, velocity")
		return
	}
	if err != nil {
		a.log.Error("fetch upstream versions failed", "type", serverType, "error", err)
		writeError(w, http.StatusBadGateway, "upstream_unavailable",
			"failed to fetch versions from upstream")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": list})
}

// handleCapabilities: catalog global capability คงที่สำหรับหน้า admin (login required)
func (a *API) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": capabilityCatalog})
}

type metaNodeView struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Status string    `json:"status"`
}

// handleMetaNodes: ข้อมูลขั้นต่ำสำหรับ dropdown ตอนสร้าง server
// (ตัวเต็มดูได้เฉพาะ admin ที่ /api/nodes)
func (a *API) handleMetaNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := a.st.ListNodes(r.Context())
	if err != nil {
		a.log.Error("list nodes failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	views := make([]metaNodeView, 0, len(nodes))
	for _, n := range nodes {
		views = append(views, metaNodeView{ID: n.ID, Name: n.Name, Status: n.Status})
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": views})
}

// handleMetaNextPort: suggestion เท่านั้น — host_port ว่างต่ำสุดบน node (เริ่ม 25565)
// สำหรับ prefill ฟอร์มสร้าง server. ไม่ reserve จริง (create เป็นคน enforce UNIQUE)
func (a *API) handleMetaNextPort(w http.ResponseWriter, r *http.Request) {
	nodeID, err := uuid.Parse(r.URL.Query().Get("node_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "node_id must be a valid UUID")
		return
	}
	if _, err := a.st.GetNodeByID(r.Context(), nodeID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "node_not_found", "node not found")
			return
		}
		a.log.Error("load node failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	port, err := a.st.NextFreeHostPort(r.Context(), nodeID)
	if err != nil {
		a.log.Error("next free host port failed", "node_id", nodeID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"port": port})
}
