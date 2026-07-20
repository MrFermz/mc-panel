package httpapi

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

// username: ตัวพิมพ์เล็ก/ตัวเลข/`_.-` ยาว 3-64 (ต้องตรงกับ regex ฝั่ง web)
// ไม่รับตัวพิมพ์ใหญ่ — canonical เป็น lowercase ตั้งแต่ชั้น DB (migration 00018 มี CHECK คุมอยู่)
var usernameRe = regexp.MustCompile(`^[a-z0-9_.-]{3,64}$`)

// canonicalUsername: ทุกทางเข้าที่รับ username จากภายนอกต้องผ่านตัวนี้ก่อน validate/เทียบ/บันทึก
// (สร้าง user, เช็คชื่อซ้ำ, login, grant permission ด้วย username) — พิมพ์ `Alice` มาก็ได้ `alice`
// ไม่ใช่โดน reject ให้ไปแก้เอง
func canonicalUsername(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

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
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url"`
}

// handleUserDirectory เปิดให้ทุก user ที่ login แล้ว (ไม่ต้องมี users.view) —
// owner ใช้เลือก collaborator ตอน grant server_permission ผ่าน username/id
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
			Username:    u.Username,
			DisplayName: u.DisplayName,
			AvatarURL:   avatarURL(u.ID, u.AvatarUpdatedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": views})
}

// handleCheckUsername: ให้ฟอร์มสร้าง user บอกได้ทันทีว่าชื่อนี้ใช้ได้ไหม ไม่ต้องกดสร้างก่อนถึงรู้
// ผูก cap `users.create` (ไม่ใช่เปิดให้ทุกคน) เพราะคำตอบ "ชื่อนี้ถูกใช้แล้ว" =
// บอกว่ามีบัญชีนั้นอยู่จริง — เป็น user enumeration ถ้าปล่อยให้ใครก็เรียกได้
//
// เกณฑ์ตรงกับ handleCreateUser เป๊ะ (regex → reserved → มีอยู่แล้ว) ไม่งั้นจะมีเคสที่
// บอกว่าว่างแต่สร้างไม่ผ่าน
func (a *API) handleCheckUsername(w http.ResponseWriter, r *http.Request) {
	username := canonicalUsername(r.URL.Query().Get("username"))

	reason := ""
	switch {
	case !usernameRe.MatchString(username):
		reason = "invalid"
	case isReservedUsername(username):
		reason = "reserved"
	default:
		exists, err := a.st.UsernameExists(r.Context(), username)
		if err != nil {
			a.log.Error("check username failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		if exists {
			reason = "taken"
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"username":  username,
		"available": reason == "",
		"reason":    reason,
	})
}

func (a *API) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	var req struct {
		Username     string   `json:"username"`
		IsAdmin      bool     `json:"is_admin"`
		Capabilities []string `json:"capabilities"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Username = canonicalUsername(req.Username)

	if !usernameRe.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "invalid_username",
			"username must be 3-64 characters of letters, digits, or _.-")
		return
	}
	// ชื่อที่ระบบจองไว้ = ถือว่าถูกใช้แล้ว (409 เหมือนชื่อซ้ำ) — ดู reserved_usernames.go
	if isReservedUsername(req.Username) {
		writeError(w, http.StatusConflict, "username_reserved",
			"this username is reserved by the system and cannot be used")
		return
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

	user, err := a.st.CreateUser(r.Context(), req.Username, hash, req.IsAdmin, req.Capabilities)
	// username unique ทั้งตาราง — บัญชีที่อยู่ในถังขยะก็ยังจองชื่อไว้ (ดู migration 00017)
	if store.IsUniqueViolation(err) {
		writeError(w, http.StatusConflict, "username_exists",
			"this username is taken — it may belong to a deleted account, which still reserves its username")
		return
	}
	if err != nil {
		a.log.Error("create user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &actor.ID, nil, "user_created",
		map[string]any{"user_id": user.ID.String(), "username": user.Username, "is_admin": user.IsAdmin})

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

// handleRestoreUser: เอาบัญชีออกจากถังขยะ — server_permissions ไม่เคยถูกลบตอน soft delete
// จึงกลับมาครบเอง (เจ้าตัวยังต้อง login ใหม่: token_version ถูก bump ตอนลบไปแล้ว)
func (a *API) handleRestoreUser(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}

	user, err := a.st.RestoreUser(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user_not_found", "no deleted user with this id")
		return
	}
	// safety net: username unique ทั้งตารางตั้งแต่ 00017 ชื่อจึงถูกจองไว้ตลอดที่อยู่ในถังขยะ
	// เคสนี้เกิดไม่ได้กับ constraint ปัจจุบัน — ดักไว้เผื่อ index กลับไปเป็น partial อีก
	if store.IsUniqueViolation(err) {
		writeError(w, http.StatusConflict, "username_exists",
			"another account now uses this username — rename it before restoring")
		return
	}
	if err != nil {
		a.log.Error("restore user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a.audit(r, &actor.ID, nil, "user_restored",
		map[string]any{"user_id": user.ID.String(), "username": user.Username})

	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(user)})
}
