package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

var validRoles = map[string]bool{"owner": true, "operator": true, "viewer": true}

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
		Email           string `json:"email"`
		Role            string `json:"role"`
		CanConsoleWrite bool   `json:"can_console_write"`
		CanManageFiles  bool   `json:"can_manage_files"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !validRoles[req.Role] {
		writeError(w, http.StatusBadRequest, "invalid_role", "role must be one of: owner, operator, viewer")
		return
	}

	target, err := a.st.GetUserByEmail(r.Context(), strings.TrimSpace(req.Email))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user_not_found", "no user with this email")
		return
	}
	if err != nil {
		a.log.Error("resolve user by email failed", "error", err)
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

	updated, err := a.st.UpsertPermission(r.Context(), target.ID, srv.ID,
		req.Role, req.CanConsoleWrite, req.CanManageFiles)
	if err != nil {
		a.log.Error("upsert permission failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &user.ID, &srv.ID, "permission_updated", map[string]any{
		"target_user_id":    target.ID.String(),
		"role":              updated.Role,
		"can_console_write": updated.CanConsoleWrite,
		"can_manage_files":  updated.CanManageFiles,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"permission": permissionView{
			UserID:          target.ID,
			Email:           target.Email,
			DisplayName:     target.DisplayName,
			Role:            updated.Role,
			CanConsoleWrite: updated.CanConsoleWrite,
			CanManageFiles:  updated.CanManageFiles,
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
