package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

// endpoint ชุดนี้คือ access list เดิม (server_permissions) มองจากฝั่ง user แทนฝั่ง server
// — admin เปิดหน้า /admin/users/{id}/servers แล้ว assign server ให้ user ได้ทีเดียว
// แทนที่จะต้องไล่เปิดแท็บ Access ทีละ server
//
// สิทธิ์: global cap (access.view/manage) **และ** ต้องเป็น owner ของ server ตัวนั้น
// เหมือน endpoint ฝั่ง server ทุกประการ (is_admin ข้ามได้) — ไม่งั้นใครที่มี access.manage
// จะ grant owner ให้ตัวเองบน server ที่ไม่เกี่ยวข้องได้

// serverPermissionView = grant หนึ่งแถวมองจากฝั่ง user
type serverPermissionView struct {
	ServerID     uuid.UUID `json:"server_id"`
	ServerName   string    `json:"server_name"`
	ServerStatus string    `json:"server_status"`
	NodeID       uuid.UUID `json:"node_id"`
	Role         string    `json:"role"`
	Capabilities []string  `json:"capabilities"`
}

// loadTargetUser: user เจ้าของ access list ที่กำลังจัดการ (คนที่ถูก assign ไม่ใช่ actor)
func (a *API) loadTargetUser(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return nil, false
	}
	user, err := a.st.GetUserByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return nil, false
	}
	if err != nil {
		a.log.Error("load target user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return nil, false
	}
	return user, true
}

// ownedServer: โหลด server แล้วยืนยันว่า actor เป็น owner ของมัน — ใช้แทน loadServerAccess
// เพราะ route ชุดนี้ {id} คือ user id ส่วน server id มาจาก body/path segment อื่น
func (a *API) ownedServer(w http.ResponseWriter, r *http.Request, serverID uuid.UUID) (*store.Server, bool) {
	actor := auth.UserFrom(r.Context())

	srv, err := a.st.GetServerByID(r.Context(), serverID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "server_not_found", "server not found")
		return nil, false
	}
	if err != nil {
		a.log.Error("load server failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return nil, false
	}
	if actor.IsAdmin {
		return srv, true
	}

	perm, err := a.st.GetPermission(r.Context(), actor.ID, srv.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusForbidden, "forbidden", "owner access required")
		return nil, false
	}
	if err != nil {
		a.log.Error("load permission failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return nil, false
	}
	if !isOwner(actor, perm) {
		writeError(w, http.StatusForbidden, "forbidden", "owner access required")
		return nil, false
	}
	return srv, true
}

// handleListUserServers คืนเฉพาะ server ที่ actor เป็น owner — คนที่มี access.view แต่ไม่ได้
// เป็น owner ของ server หนึ่ง ต้องไม่รู้ด้วยซ้ำว่า user คนนี้มีสิทธิ์บน server นั้นอยู่
func (a *API) handleListUserServers(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	target, ok := a.loadTargetUser(w, r)
	if !ok {
		return
	}

	perms, err := a.st.ListUserServerPermissions(r.Context(), target.ID)
	if err != nil {
		a.log.Error("list user server permissions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	var visible map[uuid.UUID]bool
	if !actor.IsAdmin {
		owned, err := a.st.ListUserServerPermissions(r.Context(), actor.ID)
		if err != nil {
			a.log.Error("list actor server permissions failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		visible = make(map[uuid.UUID]bool, len(owned))
		for _, p := range owned {
			if p.Role == "owner" {
				visible[p.ServerID] = true
			}
		}
	}

	views := make([]serverPermissionView, 0, len(perms))
	for _, p := range perms {
		if visible != nil && !visible[p.ServerID] {
			continue
		}
		views = append(views, serverPermissionView{
			ServerID:     p.ServerID,
			ServerName:   p.ServerName,
			ServerStatus: p.ServerStatus,
			NodeID:       p.NodeID,
			Role:         p.Role,
			Capabilities: emptyIfNil(p.Capabilities),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": views})
}

func (a *API) handleUpsertUserServer(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	target, ok := a.loadTargetUser(w, r)
	if !ok {
		return
	}

	var req struct {
		ServerID     string   `json:"server_id"`
		Role         string   `json:"role"`
		Capabilities []string `json:"capabilities"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	serverID, err := uuid.Parse(req.ServerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "server not found")
		return
	}
	if !validRoles[req.Role] {
		writeError(w, http.StatusBadRequest, "invalid_role", "role must be one of: owner, member")
		return
	}
	// owner ได้ทุก server-scoped cap โดยปริยาย — เก็บ capabilities ว่างเสมอ
	caps := []string{}
	if req.Role == "member" {
		if !validateServerCapabilities(req.Capabilities) {
			writeError(w, http.StatusBadRequest, "invalid_capability",
				"capabilities must all be server-scoped keys")
			return
		}
		caps = dedupStrings(req.Capabilities)
	}

	srv, ok := a.ownedServer(w, r, serverID)
	if !ok {
		return
	}

	// demote owner คนสุดท้าย = ทิ้ง server ไว้โดยไม่มีคนจัดการ access — กันเหมือนฝั่ง server
	if req.Role != "owner" {
		existing, err := a.st.GetPermission(r.Context(), target.ID, srv.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			a.log.Error("load existing permission failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		if existing != nil && existing.Role == "owner" {
			owners, err := a.st.CountServerOwners(r.Context(), srv.ID)
			if err != nil {
				a.log.Error("count owners failed", "error", err)
				writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
				return
			}
			if owners <= 1 {
				writeError(w, http.StatusConflict, "last_owner",
					"cannot demote the last owner of this server")
				return
			}
		}
	}

	updated, err := a.st.UpsertPermission(r.Context(), target.ID, srv.ID, req.Role, caps)
	if err != nil {
		a.log.Error("upsert permission failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &actor.ID, &srv.ID, "permission_updated", map[string]any{
		"target_user_id": target.ID.String(),
		"role":           updated.Role,
		"capabilities":   updated.Capabilities,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"permission": serverPermissionView{
			ServerID:     srv.ID,
			ServerName:   srv.Name,
			ServerStatus: srv.Status,
			NodeID:       srv.NodeID,
			Role:         updated.Role,
			Capabilities: emptyIfNil(updated.Capabilities),
		},
	})
}

func (a *API) handleDeleteUserServer(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	target, ok := a.loadTargetUser(w, r)
	if !ok {
		return
	}
	serverID, err := uuidParam(r, "server_id")
	if err != nil {
		writeError(w, http.StatusNotFound, "server_not_found", "server not found")
		return
	}
	srv, ok := a.ownedServer(w, r, serverID)
	if !ok {
		return
	}

	existing, err := a.st.GetPermission(r.Context(), target.ID, srv.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "permission_not_found", "permission not found")
		return
	}
	if err != nil {
		a.log.Error("load permission failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if existing.Role == "owner" {
		owners, err := a.st.CountServerOwners(r.Context(), srv.ID)
		if err != nil {
			a.log.Error("count owners failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		if owners <= 1 {
			writeError(w, http.StatusConflict, "last_owner",
				"cannot remove the last owner of this server")
			return
		}
	}

	if err := a.st.DeletePermission(r.Context(), srv.ID, target.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "permission_not_found", "permission not found")
			return
		}
		a.log.Error("delete permission failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &actor.ID, &srv.ID, "permission_removed",
		map[string]any{"target_user_id": target.ID.String()})
	w.WriteHeader(http.StatusNoContent)
}
