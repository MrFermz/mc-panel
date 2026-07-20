package httpapi

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

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

// effectiveServerCap: user ทำ cap นี้กับ server นี้ได้ไหม — enforce 2 ชั้นแบบ AND
// (1) global capability = เพดานระดับ panel  (2) server_permissions ต่อ server นี้
// admin ครอบทุกอย่าง; owner ได้ทุก server-scoped cap; member ได้เฉพาะที่ grant ไว้
func effectiveServerCap(user *store.User, perm *store.Permission, cap string) bool {
	if user.IsAdmin {
		return true
	}
	if !hasCapability(user, cap) {
		return false
	}
	if perm == nil {
		return false
	}
	if perm.Role == "owner" {
		return true
	}
	return slices.Contains(perm.Capabilities, cap)
}

// loadServerCap = loadServerAccess + เช็ค effectiveServerCap สำหรับ cap ที่ handler ต้องการ
// เขียน 403 เองเมื่อไม่ผ่าน — ใช้แทน loadServerAccess ในทุก endpoint ระดับ server ที่ผูก cap
func (a *API) loadServerCap(w http.ResponseWriter, r *http.Request, cap string) (*store.Server, *store.Permission, bool) {
	srv, perm, ok := a.loadServerAccess(w, r)
	if !ok {
		return nil, nil, false
	}
	if !effectiveServerCap(auth.UserFrom(r.Context()), perm, cap) {
		writeError(w, http.StatusForbidden, "forbidden", "insufficient permission for this server")
		return nil, nil, false
	}
	return srv, perm, true
}

// loadDeletedServerCap = loadServerCap ฝั่งถังขยะ: โหลด server ที่ถูก soft delete ไว้
// (GetServerByID ปกติมองไม่เห็น) แล้วเช็คสิทธิ์ 2 ชั้นแบบเดียวกัน — server ที่ยัง active
// ตอบ 409 invalid_state เพื่อบังคับให้ผ่านขั้น delete ก่อน restore/purge เสมอ
func (a *API) loadDeletedServerCap(w http.ResponseWriter, r *http.Request, cap string) (*store.Server, *store.Permission, bool) {
	user := auth.UserFrom(r.Context())

	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "server not found")
		return nil, nil, false
	}

	srv, err := a.st.GetServerByIDAny(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "server_not_found", "server not found")
		return nil, nil, false
	}
	if err != nil {
		a.log.Error("load server failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return nil, nil, false
	}
	if srv.DeletedAt == nil {
		writeError(w, http.StatusConflict, "invalid_state", "server is not deleted")
		return nil, nil, false
	}

	var perm *store.Permission
	if !user.IsAdmin {
		perm, err = a.st.GetPermission(r.Context(), user.ID, srv.ID)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusForbidden, "forbidden", "no access to this server")
			return nil, nil, false
		}
		if err != nil {
			a.log.Error("load permission failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return nil, nil, false
		}
	}
	if !effectiveServerCap(user, perm, cap) {
		writeError(w, http.StatusForbidden, "forbidden", "insufficient permission for this server")
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
		StartedAt:     nilIfZero(st.StartedAt),
		OnlinePlayers: emptyIfNil(st.OnlinePlayers),
		MaxPlayers:    st.MaxPlayers,
		TPS:           st.TPS,
		UpdatedAt:     st.UpdatedAt,
	}
}

// nilIfZero แปลง zero time เป็น nil เพื่อให้ JSON เป็น null (agent ไม่รู้เวลาเริ่ม)
func nilIfZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
// emptyIfNil ทำให้ online_players เป็น [] ใน JSON เสมอ ไม่ใช่ null —
// contract ฝั่ง web คาดว่าเป็น array (ดู docs/api.md)
func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
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

// checkNodeMemory บังคับ RAM admission control: (ผลรวม memory_mb ที่จองบน node - excludeMB)
// + requestedMB ต้องไม่เกิน node.MemoryTotalMB. excludeMB ใช้กันนับ memory เดิมของ instance
// ที่กำลังขยายซ้ำ (create=0, update=memory ปัจจุบันของ server). คืน false + เขียน error แล้ว
// เมื่อเกิน/DB พลาด; คืน true (ปล่อยผ่าน) เมื่อผ่าน หรือ node ยังไม่รายงาน total (=0)
func (a *API) checkNodeMemory(w http.ResponseWriter, r *http.Request, node *store.Node, requestedMB, excludeMB int) bool {
	total := int(node.MemoryTotalMB)
	if total <= 0 {
		return true
	}
	sum, err := a.st.SumServerMemoryMBOnNode(r.Context(), node.ID)
	if err != nil {
		a.log.Error("sum node memory failed", "node_id", node.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return false
	}
	used := sum - excludeMB
	if used < 0 {
		used = 0
	}
	if used+requestedMB > total {
		writeInsufficientMemory(w, used, requestedMB, total)
		return false
	}
	return true
}

func (a *API) handleListServers(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())

	// scope=all คือ view ของหน้า admin เท่านั้น (ต้องมี servers.view_all) — รวม server
	// ที่อยู่ในถังขยะมาด้วยเพื่อให้ restore/purge ได้ ; default scope ("mine") คือรายการที่
	// user มีชื่อใน access เท่านั้น **แม้เป็น admin** ตามดีไซน์ของหน้า `/`
	scope := r.URL.Query().Get("scope")
	if scope != "" && scope != "mine" && scope != "all" {
		writeError(w, http.StatusBadRequest, "invalid_request", "scope must be 'mine' or 'all'")
		return
	}

	var (
		servers []*store.Server
		err     error
	)
	if scope == "all" {
		if !hasCapability(user, capServersViewAll) {
			writeError(w, http.StatusForbidden, "forbidden", "insufficient capability")
			return
		}
		servers, err = a.st.ListAllServersWithDeleted(r.Context())
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
	node, err := a.st.GetNodeByID(r.Context(), nodeID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "node_not_found", "node not found")
			return
		}
		a.log.Error("load node failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// admission control: กัน RAM overcommit — ผลรวม memory_mb ที่จองบน node + ตัวใหม่
	// ต้องไม่เกิน MemoryTotalMB (ข้ามเช็คเมื่อ node ยังไม่รายงาน total)
	if !a.checkNodeMemory(w, r, node, req.MemoryMB, 0) {
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

	// row เกิดแล้ว → แจ้ง browser refetch server list (dashboard อัปเดตทันที)
	a.events.ServerAdded(srv.ID)

	job, err := a.disp.CreateServer(r.Context(), srv, req.AcceptEula, user.ID)
	if err != nil {
		// job ถูก mark failed แล้วใน dispatcher — server ที่ provision ไม่ได้ให้จบที่ errored
		if serr := a.st.UpdateServerStatus(r.Context(), srv.ID, "errored"); serr != nil {
			a.log.Error("mark server errored failed", "server_id", srv.ID, "error", serr)
		}
		writeError(w, http.StatusBadGateway, "dispatch_failed", "failed to dispatch provisioning job")
		return
	}

	// job เพิ่งสร้าง ยังไม่ได้ join users — เติมชื่อคนสั่ง (ตัว requester เอง) ให้ response ครบ
	fillJobRequester(job, user)
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
	srv, _, ok := a.loadServerCap(w, r, capServersEdit)
	if !ok {
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

	// admission control ตอนขยาย RAM: กันไม่ให้ผลรวมเกิน node total (ไม่นับ memory เดิมของตัวเอง)
	if req.MemoryMB != nil && *req.MemoryMB > srv.MemoryMB {
		node, err := a.st.GetNodeByID(r.Context(), srv.NodeID)
		if err != nil {
			a.log.Error("load node failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		if !a.checkNodeMemory(w, r, node, *req.MemoryMB, srv.MemoryMB) {
			return
		}
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

// handleDeleteServer = soft delete: ย้าย server เข้าถังขยะโดยไม่แตะไฟล์/container บน node
// (กู้คืนได้ที่ /admin/servers) — การลบจริงอยู่ที่ handlePurgeServer
func (a *API) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, _, ok := a.loadServerCap(w, r, capServersDelete)
	if !ok {
		return
	}

	// ต้องหยุด instance ก่อนลบ — server ในถังขยะต้องไม่มี container ที่ยังรันค้างอยู่
	// (ไม่มีหน้าไหนคุม power ของมันได้อีกจนกว่าจะ restore)
	if srv.Status != "stopped" && srv.Status != "errored" {
		writeError(w, http.StatusConflict, "invalid_state",
			"stop the server before deleting it")
		return
	}

	deleted, err := a.st.SoftDeleteServer(r.Context(), srv.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "server_not_found", "server not found")
		return
	}
	if err != nil {
		a.log.Error("soft delete server failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &user.ID, &srv.ID, "server_soft_deleted", map[string]any{"name": srv.Name})
	a.log.Info("server soft deleted", "server_id", srv.ID, "user_id", user.ID)
	// หายจากทุก list ที่ user เห็น (เหมือนถูกลบจริง) — admin ยังเห็นผ่าน scope=all
	a.events.ServerRemoved(srv.ID)
	a.rings.Drop(srv.ID)

	writeJSON(w, http.StatusOK, map[string]any{"server": toServerView(deleted, nil)})
}

// handleRestoreServer กู้ server จากถังขยะกลับมาเป็นปกติ — ไฟล์ยังอยู่ครบ จึงพร้อม start ทันที
func (a *API) handleRestoreServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, _, ok := a.loadDeletedServerCap(w, r, capServersRestore)
	if !ok {
		return
	}

	restored, err := a.st.RestoreServer(r.Context(), srv.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "server_not_found", "server not found")
		return
	}
	if err != nil {
		a.log.Error("restore server failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &user.ID, &srv.ID, "server_restored", map[string]any{"name": srv.Name})
	a.log.Info("server restored", "server_id", srv.ID, "user_id", user.ID)
	// กลับเข้า list ของคนที่มีสิทธิ์ — เส้นเดียวกับตอน create/import
	a.events.ServerAdded(srv.ID)

	writeJSON(w, http.StatusOK, map[string]any{"server": toServerView(restored, nil)})
}

// handlePurgeServer ลบถาวร: dispatch job delete_server ให้ agent ลบ container + ไฟล์จริง
// แล้ว row จะถูกลบตอน JobResult สำเร็จ (ทางเดียวที่ข้อมูลหายจริง) — ทำได้เฉพาะกับ server
// ที่ถูก soft delete ไว้แล้ว เพื่อบังคับให้ผ่านขั้นตอน stop → delete → purge เสมอ
func (a *API) handlePurgeServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, _, ok := a.loadDeletedServerCap(w, r, capServersPurge)
	if !ok {
		return
	}

	if srv.Status != "stopped" && srv.Status != "errored" {
		writeError(w, http.StatusConflict, "invalid_state",
			"server is "+srv.Status+"; try again when it settles")
		return
	}

	job, err := a.disp.DeleteServer(r.Context(), srv, user.ID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "dispatch_failed", "failed to dispatch delete job")
		return
	}
	// audit `server_deleted` เกิดตอน job สำเร็จจริง (result consumer) เพราะจุดนี้ไฟล์ยังอยู่

	fillJobRequester(job, user)
	writeJSON(w, http.StatusOK, map[string]any{"job": toJobView(job)})
}

func (a *API) handleServerAction(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, _, ok := a.loadServerCap(w, r, capServersPower)
	if !ok {
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

	fillJobRequester(job, user)
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
	srv, _, ok := a.loadServerCap(w, r, capConsoleView)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": a.rings.Get(srv.ID).Snapshot()})
}
