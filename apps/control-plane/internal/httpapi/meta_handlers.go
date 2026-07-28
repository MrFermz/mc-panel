package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/game-manager/control-plane/internal/auth"
	"github.com/game-manager/control-plane/internal/games"
	"github.com/game-manager/control-plane/internal/store"
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

type gameMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// MinMemoryMB ให้ฟอร์มสร้าง server เตือนก่อนยิง (backend ยังเป็นคนปฏิเสธจริงอยู่ดี) —
	// ค่านี้ต่างกันต่อเกม จึงเป็นค่าคงที่ของ web ไม่ได้
	MinMemoryMB int `json:"min_memory_mb"`
	// LicenseName = ข้อตกลงที่ต้องยอมรับก่อนสร้าง ("" = เกมนี้ไม่มี)
	LicenseName string `json:"license_name"`
	// CanCreate = user คนนี้สร้าง server ของเกมนี้ได้ไหม (`servers.create` AND `games.{id}`)
	// — ให้หน้าเลือกเกมล็อกการ์ดที่กดไม่ได้ไว้แต่แรก แทนที่จะไปเจอ 403 ตอนกดสร้าง
	CanCreate bool `json:"can_create"`
}

// handleGames: เกมทั้งหมดที่ instance นี้รองรับ (มาจาก registry) — endpoint meta
// อื่น ๆ รับ `?game=` ที่หยิบมาจากรายการนี้ได้ทุกเส้น ว่างไว้ = เกม default
// รายการไม่ถูกกรองตามสิทธิ์ (เห็นได้ว่ามีเกมอะไรเสมอ) — สิทธิ์อยู่ที่ can_create ต่อแถว
func (a *API) handleGames(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	canCreateAny := hasCapability(user, capServersCreate)

	defs := a.games.All()
	views := make([]gameMeta, 0, len(defs))
	for _, d := range defs {
		views = append(views, gameMeta{
			ID:          d.ID,
			Label:       d.Label,
			MinMemoryMB: d.MinMemoryMB,
			LicenseName: d.LicenseName,
			CanCreate:   canCreateAny && hasCapability(user, gameCapKey(d.ID)),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": views})
}

type variantMeta struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	RequiresLicense bool   `json:"requires_license"`
}

// handleVariants คืน variant ของเกมที่ระบุ (`?game=`, ว่าง = default) —
// รายการนี้เป็นของ game definition ไม่ใช่ค่าคงที่ของ handler
func (a *API) handleVariants(w http.ResponseWriter, r *http.Request) {
	def, ok := a.gameFromQuery(w, r)
	if !ok {
		return
	}
	views := make([]variantMeta, 0, len(def.Variants))
	for _, v := range def.Variants {
		views = append(views, variantMeta{ID: v.ID, Label: v.Label, RequiresLicense: v.RequiresLicense})
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": views})
}

func (a *API) handleVersions(w http.ResponseWriter, r *http.Request) {
	def, ok := a.gameFromQuery(w, r)
	if !ok {
		return
	}
	variant := r.URL.Query().Get("type")
	if !def.HasVariant(variant) || def.Version.List == nil {
		writeError(w, http.StatusBadRequest, "invalid_variant",
			"type must be one of: "+def.VariantList())
		return
	}

	list, err := def.Version.List(r.Context(), variant)
	if errors.Is(err, games.ErrUnknownVariant) {
		writeError(w, http.StatusBadRequest, "invalid_variant",
			"type must be one of: "+def.VariantList())
		return
	}
	if err != nil {
		a.log.Error("fetch upstream versions failed", "game", def.ID, "type", variant, "error", err)
		writeError(w, http.StatusBadGateway, "upstream_unavailable",
			"failed to fetch versions from upstream")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": list})
}

// nextFreeHostPort หาช่วง host port ว่างต่ำสุดบน node ให้เกมนั้น
// เกมที่กินหลาย port ติดกัน (Definition.HostPortSpan) ต้องได้ช่วงที่ว่างทั้งช่วง และต้องไม่ไป
// ทับ port รองของ instance เกมอื่นที่จองไว้แล้ว — จึงต้องขยาย port ที่ถูกใช้ตาม span
// ของเกมของ server ตัวนั้น (DB เก็บแค่ port หลัก) ก่อนไล่หา
func (a *API) nextFreeHostPort(def *games.Definition, usage []store.HostPortUsage) int {
	taken := make(map[int]bool, len(usage))
	for _, u := range usage {
		span := 1
		if d, ok := a.games.Resolve(u.Game); ok && d.HostPortSpan > span {
			span = d.HostPortSpan
		}
		for i := range span {
			taken[u.Port+i] = true
		}
	}

	span := max(def.HostPortSpan, 1)

	const maxPort = 65535
	for p := def.DefaultHostPort; p+span-1 <= maxPort; p++ {
		free := true
		for i := range span {
			if taken[p+i] {
				free = false
				break
			}
		}
		if free {
			return p
		}
	}
	return maxPort
}

// handleCapabilities: catalog global capability สำหรับหน้า admin (login required) —
// กลุ่ม `games` ต่อท้ายตาม registry ของ instance นี้ จึงไม่ใช่ค่าคงที่ของ web
func (a *API) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": a.capabilityCatalog()})
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

// handleMetaNextPort: suggestion เท่านั้น — host_port ว่างต่ำสุดบน node นับจาก port เริ่มต้น
// ของเกมนั้น (Definition.DefaultHostPort) สำหรับ prefill ฟอร์มสร้าง server
// ไม่ reserve จริง (create เป็นคน enforce UNIQUE)
func (a *API) handleMetaNextPort(w http.ResponseWriter, r *http.Request) {
	def, ok := a.gameFromQuery(w, r)
	if !ok {
		return
	}

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

	usage, err := a.st.ListHostPortUsage(r.Context(), nodeID)
	if err != nil {
		a.log.Error("list host port usage failed", "node_id", nodeID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"port": a.nextFreeHostPort(def, usage)})
}
