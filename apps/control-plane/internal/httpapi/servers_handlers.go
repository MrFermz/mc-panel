package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

var validServerTypes = map[string]bool{
	"vanilla": true, "paper": true, "fabric": true, "forge": true, "velocity": true,
}

// loadServerAccess โหลด server + permission ของ user ปัจจุบัน
// เขียน error response เองเมื่อไม่เจอ/ไม่มีสิทธิ์เห็น (viewer ขึ้นไป)
// perm เป็น nil สำหรับ admin (ทำได้ทุกอย่างโดยไม่มีแถว permission)
func (a *API) loadServerAccess(w http.ResponseWriter, r *http.Request) (*store.Server, *store.Permission, bool) {
	user := auth.UserFrom(r.Context())

	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "server not found")
		return nil, nil, false
	}

	srv, err := a.st.GetServerByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "server_not_found", "server not found")
		return nil, nil, false
	}
	if err != nil {
		a.log.Error("load server failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return nil, nil, false
	}

	if user.IsAdmin {
		return srv, nil, true
	}

	perm, err := a.st.GetPermission(r.Context(), user.ID, srv.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusForbidden, "forbidden", "no access to this server")
		return nil, nil, false
	}
	if err != nil {
		a.log.Error("load permission failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return nil, nil, false
	}
	return srv, perm, true
}

// statsViewFor คืน stats จาก cache เฉพาะ server ที่ running และมีข้อมูลแล้ว
// ไม่งั้น nil (JSON null) ตาม docs/api.md
func (a *API) statsViewFor(s *store.Server) *serverStatsView {
	if s.Status != "running" {
		return nil
	}
	st, ok := a.stats.Get(s.ID)
	if !ok {
		return nil
	}
	return &serverStatsView{
		CPUPercent:    st.CPUPercent,
		MemoryUsedMB:  st.MemoryUsedMB,
		MemoryLimitMB: st.MemoryLimitMB,
		NetRxBps:      st.NetRxBps,
		NetTxBps:      st.NetTxBps,
		DiskReadBps:   st.DiskReadBps,
		DiskWriteBps:  st.DiskWriteBps,
		UpdatedAt:     st.UpdatedAt,
	}
}

func canOperate(user *store.User, perm *store.Permission) bool {
	if user.IsAdmin {
		return true
	}
	return perm != nil && (perm.Role == "owner" || perm.Role == "operator")
}

func isOwner(user *store.User, perm *store.Permission) bool {
	if user.IsAdmin {
		return true
	}
	return perm != nil && perm.Role == "owner"
}

func validateHostPort(p *int) bool {
	return p == nil || (*p >= 1024 && *p <= 65535)
}

func (a *API) handleListServers(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())

	var (
		servers []*store.Server
		err     error
	)
	// servers.view_all (และ is_admin) เห็นทุก server; ที่เหลือเห็นเฉพาะที่มี server_permission
	if hasCapability(user, capViewAllServers) {
		servers, err = a.st.ListAllServers(r.Context())
	} else {
		servers, err = a.st.ListServersForUser(r.Context(), user.ID)
	}
	if err != nil {
		a.log.Error("list servers failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	views := make([]serverView, 0, len(servers))
	for _, s := range servers {
		views = append(views, toServerView(s, a.statsViewFor(s)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": views})
}

func (a *API) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())

	var req struct {
		Name       string `json:"name"`
		NodeID     string `json:"node_id"`
		ServerType string `json:"server_type"`
		MCVersion  string `json:"mc_version"`
		MemoryMB   int    `json:"memory_mb"`
		HostPort   *int   `json:"host_port"`
		AcceptEula bool   `json:"accept_eula"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.MCVersion = strings.TrimSpace(req.MCVersion)

	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "name is required (max 100 characters)")
		return
	}
	if !validServerTypes[req.ServerType] {
		writeError(w, http.StatusBadRequest, "invalid_server_type",
			"server_type must be one of: vanilla, paper, fabric, forge, velocity")
		return
	}
	if req.MCVersion == "" || len(req.MCVersion) > 50 {
		writeError(w, http.StatusBadRequest, "invalid_mc_version", "mc_version is required (max 50 characters)")
		return
	}
	if req.MemoryMB < 256 {
		writeError(w, http.StatusBadRequest, "invalid_memory", "memory_mb must be at least 256")
		return
	}
	if !validateHostPort(req.HostPort) {
		writeError(w, http.StatusBadRequest, "invalid_host_port", "host_port must be between 1024 and 65535")
		return
	}
	// velocity เป็น proxy ไม่รัน Mojang server jar — ไม่มี EULA ให้ยอมรับ
	if !req.AcceptEula && req.ServerType != "velocity" {
		writeError(w, http.StatusBadRequest, "eula_required",
			"you must accept the Minecraft EULA to create this server")
		return
	}

	nodeID, err := uuid.Parse(req.NodeID)
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

	srv, err := a.st.CreateServerWithOwner(r.Context(), nodeID, user.ID,
		req.Name, req.ServerType, req.MCVersion, req.MemoryMB, req.HostPort)
	if store.IsUniqueViolation(err) {
		writeError(w, http.StatusConflict, "host_port_taken", "host_port is already used on this node")
		return
	}
	if err != nil {
		a.log.Error("create server failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &user.ID, &srv.ID, "server_created", map[string]any{
		"name": srv.Name, "server_type": srv.ServerType, "mc_version": srv.MCVersion,
		"node_id": srv.NodeID.String(),
	})

	job, err := a.disp.CreateServer(r.Context(), srv, req.AcceptEula, user.ID)
	if err != nil {
		// job ถูก mark failed แล้วใน dispatcher — server ที่ provision ไม่ได้ให้จบที่ errored
		if serr := a.st.UpdateServerStatus(r.Context(), srv.ID, "errored"); serr != nil {
			a.log.Error("mark server errored failed", "server_id", srv.ID, "error", serr)
		}
		writeError(w, http.StatusBadGateway, "dispatch_failed", "failed to dispatch provisioning job")
		return
	}

	// job เพิ่งสร้าง ยังไม่ได้ join users — เติม email ของคนสั่ง (ตัว requester เอง) ให้ response ครบ
	job.RequestedByEmail = &user.Email
	writeJSON(w, http.StatusCreated, map[string]any{
		"server": toServerView(srv, a.statsViewFor(srv)),
		"job":    toJobView(job),
	})
}

func (a *API) handleGetServer(w http.ResponseWriter, r *http.Request) {
	srv, _, ok := a.loadServerAccess(w, r)
	if !ok {
		return
	}

	perms, err := a.st.ListServerPermissions(r.Context(), srv.ID)
	if err != nil {
		a.log.Error("list server permissions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	views := make([]permissionView, 0, len(perms))
	for _, p := range perms {
		views = append(views, toPermissionView(p))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"server":      toServerView(srv, a.statsViewFor(srv)),
		"permissions": views,
	})
}

func (a *API) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, perm, ok := a.loadServerAccess(w, r)
	if !ok {
		return
	}
	if !isOwner(user, perm) {
		writeError(w, http.StatusForbidden, "forbidden", "owner access required")
		return
	}

	var req struct {
		Name     *string `json:"name"`
		MemoryMB *int    `json:"memory_mb"`
		HostPort *int    `json:"host_port"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" || len(trimmed) > 100 {
			writeError(w, http.StatusBadRequest, "invalid_name", "name is required (max 100 characters)")
			return
		}
		req.Name = &trimmed
	}
	if req.MemoryMB != nil && *req.MemoryMB < 256 {
		writeError(w, http.StatusBadRequest, "invalid_memory", "memory_mb must be at least 256")
		return
	}
	// host_port = 0 หมายถึงเลิก expose host port (set NULL)
	clearHostPort := req.HostPort != nil && *req.HostPort == 0
	if !clearHostPort && !validateHostPort(req.HostPort) {
		writeError(w, http.StatusBadRequest, "invalid_host_port",
			"host_port must be between 1024 and 65535, or 0 to unset")
		return
	}

	// host_port ผูก UNIQUE slot ต่อ node และ memory_mb คือ resource ของ container
	// ที่รันอยู่ — แก้ตอน server ยังไม่หยุดจะทำ slot ชนกัน/ค่าไม่ตรง container จริง
	// (ชื่อ name ไม่กระทบ runtime จึงแก้ได้ทุกสถานะ)
	if (req.MemoryMB != nil || req.HostPort != nil) && srv.Status != "stopped" && srv.Status != "errored" {
		writeError(w, http.StatusConflict, "invalid_state",
			"host_port and memory_mb can only be changed while the server is stopped or errored")
		return
	}

	updated, err := a.st.UpdateServerConfig(r.Context(), srv.ID, req.Name, req.MemoryMB, req.HostPort, clearHostPort)
	if store.IsUniqueViolation(err) {
		writeError(w, http.StatusConflict, "host_port_taken", "host_port is already used on this node")
		return
	}
	if err != nil {
		a.log.Error("update server failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	detail := map[string]any{}
	if req.Name != nil {
		detail["name"] = *req.Name
	}
	if req.MemoryMB != nil {
		detail["memory_mb"] = *req.MemoryMB
	}
	if req.HostPort != nil {
		detail["host_port"] = *req.HostPort
	}
	a.audit(r, &user.ID, &srv.ID, "server_updated", detail)

	writeJSON(w, http.StatusOK, map[string]any{"server": toServerView(updated, a.statsViewFor(updated))})
}

func (a *API) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, perm, ok := a.loadServerAccess(w, r)
	if !ok {
		return
	}
	if !isOwner(user, perm) {
		writeError(w, http.StatusForbidden, "forbidden", "owner access required")
		return
	}

	job, err := a.disp.DeleteServer(r.Context(), srv, user.ID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "dispatch_failed", "failed to dispatch delete job")
		return
	}
	// audit `server_deleted` เกิดตอน job สำเร็จจริง (result consumer)
	// เพราะจุดนี้ข้อมูลยังไม่ถูกลบ

	job.RequestedByEmail = &user.Email
	writeJSON(w, http.StatusOK, map[string]any{"job": toJobView(job)})
}

func (a *API) handleServerAction(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, perm, ok := a.loadServerAccess(w, r)
	if !ok {
		return
	}
	if !canOperate(user, perm) {
		writeError(w, http.StatusForbidden, "forbidden", "operator access required")
		return
	}

	var req struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	switch req.Action {
	case "start", "stop", "restart", "kill":
	default:
		writeError(w, http.StatusBadRequest, "invalid_action",
			"action must be one of: start, stop, restart, kill")
		return
	}

	// provisioning ยังไม่มีไฟล์ให้ start / deleting กำลังถูกลบ — action ไหนก็ไม่ make sense
	if srv.Status == "provisioning" || srv.Status == "deleting" {
		writeError(w, http.StatusConflict, "invalid_state",
			"server is "+srv.Status+"; try again when it settles")
		return
	}

	a.audit(r, &user.ID, &srv.ID, "server_action", map[string]any{"action": req.Action})

	var (
		job *store.Job
		err error
	)
	switch req.Action {
	case "start":
		job, err = a.disp.StartServer(r.Context(), srv, user.ID)
	case "stop":
		job, err = a.disp.StopServer(r.Context(), srv, user.ID)
	case "restart":
		job, err = a.disp.RestartServer(r.Context(), srv, user.ID)
	case "kill":
		job, err = a.disp.KillServer(r.Context(), srv, user.ID)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "dispatch_failed", "failed to dispatch job")
		return
	}

	job.RequestedByEmail = &user.Email
	writeJSON(w, http.StatusOK, map[string]any{"job": toJobView(job)})
}

func (a *API) handleListServerJobs(w http.ResponseWriter, r *http.Request) {
	srv, _, ok := a.loadServerAccess(w, r)
	if !ok {
		return
	}

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
			return
		}
		limit = n
	}

	jobList, err := a.st.ListJobsByServer(r.Context(), srv.ID, limit)
	if err != nil {
		a.log.Error("list jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	views := make([]jobView, 0, len(jobList))
	for _, j := range jobList {
		views = append(views, toJobView(j))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": views})
}

func (a *API) handleConsoleHistory(w http.ResponseWriter, r *http.Request) {
	srv, _, ok := a.loadServerAccess(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": a.rings.Get(srv.ID).Snapshot()})
}
