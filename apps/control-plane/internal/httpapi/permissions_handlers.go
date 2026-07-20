package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

var validRoles = map[string]bool{"owner": true, "member": true}

func (a *API) handleListPermissions(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, perm, ok := a.loadServerAccess(w, r)
	if !ok {
		return
	}
	if !isOwner(user, perm) {
		writeError(w, http.StatusForbidden, "forbidden", "owner access required")
		return
	}

	perms, err := a.st.ListServerPermissions(r.Context(), srv.ID)
	if err != nil {
		a.log.Error("list permissions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	views := make([]permissionView, 0, len(perms))
	for _, p := range perms {
		views = append(views, toPermissionView(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": views})
}

func (a *API) handleUpsertPermission(w http.ResponseWriter, r *http.Request) {
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
		UserID       string   `json:"user_id"`
		Username     string   `json:"username"`
		Role         string   `json:"role"`
		Capabilities []string `json:"capabilities"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !validRoles[req.Role] {
		writeError(w, http.StatusBadRequest, "invalid_role", "role must be one of: owner, member")
		return
	}
	// owner ได้ทุก server-scoped cap โดยปริยาย — เก็บ capabilities ว่าง
	// member ต้องระบุ cap ที่ grant และทุก key ต้องเป็น server-scoped เท่านั้น
	caps := []string{}
	if req.Role == "member" {
		if !validateServerCapabilities(req.Capabilities) {
			writeError(w, http.StatusBadRequest, "invalid_capability",
				"capabilities must all be server-scoped keys")
			return
		}
		caps = dedupStrings(req.Capabilities)
	}

	// user_id มาก่อน username (picker ส่ง id ตรง ๆ, username ไว้พิมพ์มือ)
	var target *store.User
	var err error
	if strings.TrimSpace(req.UserID) != "" {
		id, perr := uuid.Parse(strings.TrimSpace(req.UserID))
		if perr != nil {
			writeError(w, http.StatusNotFound, "user_not_found", "no user with this id")
			return
		}
		target, err = a.st.GetUserByID(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user_not_found", "no user with this id")
			return
		}
	} else {
		target, err = a.st.GetUserByUsername(r.Context(), strings.TrimSpace(req.Username))
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user_not_found", "no user with this username")
			return
		}
	}
	if err != nil {
		a.log.Error("resolve permission target user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// การ demote owner คนสุดท้ายผ่าน upsert = ลบ owner คนสุดท้าย — ต้องกันเหมือนกัน
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

	a.audit(r, &user.ID, &srv.ID, "permission_updated", map[string]any{
		"target_user_id": target.ID.String(),
		"role":           updated.Role,
		"capabilities":   updated.Capabilities,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"permission": permissionView{
			UserID:       target.ID,
			Username:     target.Username,
			DisplayName:  target.DisplayName,
			AvatarURL:    avatarURL(target.ID, target.AvatarUpdatedAt),
			Role:         updated.Role,
			Capabilities: emptyIfNil(updated.Capabilities),
		},
	})
}

func (a *API) handleDeletePermission(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, perm, ok := a.loadServerAccess(w, r)
	if !ok {
		return
	}
	if !isOwner(user, perm) {
		writeError(w, http.StatusForbidden, "forbidden", "owner access required")
		return
	}

	targetID, err := uuidParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusNotFound, "permission_not_found", "permission not found")
		return
	}

	existing, err := a.st.GetPermission(r.Context(), targetID, srv.ID)
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

	if err := a.st.DeletePermission(r.Context(), srv.ID, targetID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "permission_not_found", "permission not found")
			return
		}
		a.log.Error("delete permission failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &user.ID, &srv.ID, "permission_removed",
		map[string]any{"target_user_id": targetID.String()})
	w.WriteHeader(http.StatusNoContent)
}
