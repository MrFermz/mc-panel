package httpapi

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

// username: ตัวอักษร/ตัวเลข/`_.-` ยาว 3-64 (ต้องตรงกับ regex ฝั่ง web)
var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,64}$`)

func (a *API) handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	users, err := a.st.ListUsers(r.Context(), store.UserFilter{
		Search: q.Get("search"),
		Role:   q.Get("role"),
		Status: q.Get("status"),
	})
	if err != nil {
		a.log.Error("list users failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	views := make([]userView, 0, len(users))
	for _, u := range users {
		views = append(views, toUserView(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": views})
}

// handleGetUser: โหลด user คนเดียว — หน้า permission ต่อ user เปิดตรงจาก URL ได้
// โดยไม่ต้องโหลดทั้ง list
func (a *API) handleGetUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	user, err := a.st.GetUserByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if err != nil {
		a.log.Error("get user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(user)})
}

// directoryUserView = shape เบาสำหรับ access picker (ดู docs/api.md)
type directoryUserView struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	Username    *string   `json:"username"`
	DisplayName string    `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url"`
}

// handleUserDirectory เปิดให้ทุก user ที่ login แล้ว (ไม่ต้องมี users.view) —
// owner ใช้เลือก collaborator ตอน grant server_permission ผ่าน email/id
func (a *API) handleUserDirectory(w http.ResponseWriter, r *http.Request) {
	users, err := a.st.ListUserDirectory(r.Context())
	if err != nil {
		a.log.Error("list user directory failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	views := make([]directoryUserView, 0, len(users))
	for _, u := range users {
		views = append(views, directoryUserView{
			ID:          u.ID,
			Email:       u.Email,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			AvatarURL:   avatarURL(u.ID, u.AvatarUpdatedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": views})
}

func (a *API) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	var req struct {
		Email        string   `json:"email"`
		Username     string   `json:"username"`
		IsAdmin      bool     `json:"is_admin"`
		Capabilities []string `json:"capabilities"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Username = strings.TrimSpace(req.Username)

	// ต้องมี email หรือ username อย่างน้อยหนึ่งอย่าง (username-only account ได้)
	if req.Email == "" && req.Username == "" {
		writeError(w, http.StatusBadRequest, "identifier_required", "provide an email or a username")
		return
	}
	if req.Email != "" && (len(req.Email) > 255 || !strings.Contains(req.Email, "@")) {
		writeError(w, http.StatusBadRequest, "invalid_email", "a valid email is required")
		return
	}
	var username *string
	if req.Username != "" {
		if !usernameRe.MatchString(req.Username) {
			writeError(w, http.StatusBadRequest, "invalid_request",
				"username must be 3-64 characters of letters, digits, or _.-")
			return
		}
		username = &req.Username
	}
	if !validateCapabilities(req.Capabilities) {
		writeError(w, http.StatusBadRequest, "invalid_capability", "capabilities must be keys from the catalog")
		return
	}

	initialPassword, err := auth.GeneratePassword()
	if err != nil {
		a.log.Error("generate password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	hash, err := auth.HashPassword(initialPassword)
	if err != nil {
		a.log.Error("hash password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	user, err := a.st.CreateUser(r.Context(), req.Email, username, hash, req.IsAdmin, req.Capabilities)
	if store.IsUniqueViolation(err) {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "idx_users_username_lower" {
			writeError(w, http.StatusConflict, "username_exists", "a user with this username already exists")
			return
		}
		writeError(w, http.StatusConflict, "email_exists", "a user with this email already exists")
		return
	}
	if err != nil {
		a.log.Error("create user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &actor.ID, nil, "user_created",
		map[string]any{"user_id": user.ID.String(), "email": user.Email, "is_admin": user.IsAdmin})

	// initial_password แสดงครั้งเดียว — เก็บแค่ hash ใน DB
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":             toUserView(user),
		"initial_password": initialPassword,
	})
}

func (a *API) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}

	var req struct {
		IsAdmin      *bool     `json:"is_admin"`
		IsActive     *bool     `json:"is_active"`
		Capabilities *[]string `json:"capabilities"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.Capabilities != nil && !validateCapabilities(*req.Capabilities) {
		writeError(w, http.StatusBadRequest, "invalid_capability", "capabilities must be keys from the catalog")
		return
	}

	// กัน admin ล็อกตัวเองออกจากระบบ: ห้ามถอดสิทธิ์ admin หรือปิด active ตัวเอง
	if id == actor.ID {
		if (req.IsAdmin != nil && !*req.IsAdmin) || (req.IsActive != nil && !*req.IsActive) {
			writeError(w, http.StatusBadRequest, "cannot_modify_self",
				"cannot remove your own admin role or deactivate yourself")
			return
		}
	}

	user, err := a.st.UpdateUser(r.Context(), id, req.IsAdmin, req.IsActive, req.Capabilities)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if err != nil {
		a.log.Error("update user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	detail := map[string]any{"user_id": user.ID.String()}
	if req.IsAdmin != nil {
		detail["is_admin"] = *req.IsAdmin
	}
	if req.IsActive != nil {
		detail["is_active"] = *req.IsActive
	}
	if req.Capabilities != nil {
		detail["capabilities"] = *req.Capabilities
	}
	a.audit(r, &actor.ID, nil, "user_updated", detail)

	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(user)})
}

func (a *API) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}

	initialPassword, err := auth.GeneratePassword()
	if err != nil {
		a.log.Error("generate password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	hash, err := auth.HashPassword(initialPassword)
	if err != nil {
		a.log.Error("hash password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// must_change_password=true + bump token_version -> session เก่าหลุดทุกใบ
	user, err := a.st.SetUserPassword(r.Context(), id, hash, true)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if err != nil {
		a.log.Error("reset password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &actor.ID, nil, "password_reset", map[string]any{"user_id": user.ID.String()})

	writeJSON(w, http.StatusOK, map[string]any{"initial_password": initialPassword})
}

func (a *API) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}

	// กัน admin ลบตัวเอง (จะเหลือระบบไม่มีคนดูแล/ตัดตัวเองออก)
	if id == actor.ID {
		writeError(w, http.StatusBadRequest, "cannot_delete_self", "cannot delete yourself")
		return
	}

	if err := a.st.SoftDeleteUser(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	} else if err != nil {
		a.log.Error("delete user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &actor.ID, nil, "user_delete", map[string]any{"user_id": id.String()})

	w.WriteHeader(http.StatusNoContent)
}
