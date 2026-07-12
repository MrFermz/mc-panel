package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

// dummy bcrypt hash สำหรับเทียบตอน email ไม่มีในระบบ — ให้เวลาตอบใกล้เคียง
// กับเคส email ถูกแต่ password ผิด (กัน user enumeration ผ่าน timing)
const dummyPasswordHash = "$2a$12$R9h/cIPz0gi.URNNX3kh2OPST9/PgBkqquzi.Ss7KIUgO2t0jWMUW"

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !a.auth.AllowLogin(r.Context(), ip) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts, try again later")
		return
	}

	// รับได้ทั้ง `identifier` (email หรือ username) และ `email` แบบเดิม
	var req struct {
		Identifier string `json:"identifier"`
		Email      string `json:"email"`
		Password   string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	id := req.Identifier
	if id == "" {
		id = req.Email
	}
	id = strings.TrimSpace(id)

	user, err := a.st.GetUserByEmailOrUsername(r.Context(), id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		a.log.Error("login lookup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	hash := dummyPasswordHash
	if user != nil {
		hash = user.PasswordHash
	}
	passwordOK := auth.CheckPassword(hash, req.Password)

	if user == nil || !passwordOK || !user.IsActive {
		detail := map[string]any{"email": id}
		if user != nil {
			a.audit(r, &user.ID, nil, "login_failed", detail)
		} else {
			a.audit(r, nil, nil, "login_failed", detail)
		}
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}

	token, err := a.auth.IssueSession(user)
	if err != nil {
		a.log.Error("issue session failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	a.auth.SetCookie(w, token)

	if err := a.st.TouchLastLogin(r.Context(), user.ID); err != nil {
		a.log.Error("touch last login failed", "error", err)
	}
	a.audit(r, &user.ID, nil, "login_success", nil)

	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(user)})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.auth.ClearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(user)})
}

func (a *API) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		writeError(w, http.StatusBadRequest, "invalid_current_password", "current password is incorrect")
		return
	}
	if len(req.NewPassword) < auth.MinPasswordLength {
		writeError(w, http.StatusBadRequest, "password_too_short", "password must be at least 10 characters")
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		a.log.Error("hash password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	updated, err := a.st.SetUserPassword(r.Context(), user.ID, hash, false)
	if err != nil {
		a.log.Error("set password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// token_version ถูก bump — ต้องออก cookie ใหม่ ไม่งั้น session ปัจจุบันหลุดทันที
	token, err := a.auth.IssueSession(updated)
	if err != nil {
		a.log.Error("issue session failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	a.auth.SetCookie(w, token)
	a.audit(r, &user.ID, nil, "password_changed", nil)

	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(updated)})
}
