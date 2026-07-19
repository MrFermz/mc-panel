package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

const (
	// maxDisplayName ต้องไม่เกินความยาวคอลัมน์ users.display_name (VARCHAR(64))
	maxDisplayName = 64
	// maxAvatarBytes เพดานรูป avatar — เก็บลง Postgres ตรง ๆ จึงตั้งไว้เล็ก
	maxAvatarBytes = 512 * 1024
	// avatarFormField ชื่อ part ใน multipart ที่รับรูป
	avatarFormField = "avatar"
)

// avatarMimes = ชนิดรูปที่ยอมให้เก็บ/เสิร์ฟ (คีย์เทียบกับผลของ http.DetectContentType
// ไม่ใช่ Content-Type ที่ client ส่งมา — client ปลอมได้) ไม่รับ SVG เพราะเป็น XML ที่รัน
// script ได้ ถ้าเปิดตรง ๆ ในแท็บเดียวกับ panel
var avatarMimes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// sanitizeDisplayName ตัดช่องว่างหัวท้าย + ทิ้ง control char (รวม \n ที่ทำให้ชื่อกินหลายบรรทัดใน UI)
func sanitizeDisplayName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// handleUpdateProfile: user แก้ข้อมูลของตัวเองเท่านั้น (ไม่มี capability — เหมือน
// change-password ที่เจ้าของบัญชีทำได้เสมอ) ไม่แตะ email/username/สิทธิ์ ซึ่งเป็นงานของ admin
func (a *API) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())

	var req struct {
		DisplayName *string `json:"display_name"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.DisplayName == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "display_name is required")
		return
	}

	name := sanitizeDisplayName(*req.DisplayName)
	if len([]rune(name)) > maxDisplayName {
		writeError(w, http.StatusBadRequest, "invalid_display_name",
			fmt.Sprintf("display name must be at most %d characters", maxDisplayName))
		return
	}

	updated, err := a.st.UpdateUserProfile(r.Context(), user.ID, name)
	if err != nil {
		a.log.Error("update profile failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	a.audit(r, &user.ID, nil, "profile_updated", map[string]any{"display_name": name})

	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(updated)})
}

func writeAvatarTooLarge(w http.ResponseWriter) {
	writeError(w, http.StatusRequestEntityTooLarge, "avatar_too_large",
		fmt.Sprintf("avatar must be at most %d KB", maxAvatarBytes/1024))
}

// handleUploadAvatar รับ multipart field `avatar` — ชนิดไฟล์ตัดสินจาก content sniffing
// ของ bytes จริง ไม่เชื่อ Content-Type/นามสกุลที่ client ส่งมา
func (a *API) handleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())

	// +1KB เผื่อ multipart overhead — เกินเพดานจริงถูกจับตอนวัดขนาด data ด้านล่าง
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+1024)
	file, _, err := r.FormFile(avatarFormField)
	if err != nil {
		// body ใหญ่เกินถูก MaxBytesReader ตัดตั้งแต่ตอน parse — ต้องแยกจาก "ฟอร์มผิดรูป"
		// ไม่งั้น user ที่อัปรูปใหญ่เกินจะเห็นข้อความว่าฟอร์มไม่ถูกต้อง
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeAvatarTooLarge(w)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request",
			"multipart form with an `"+avatarFormField+"` file part is required")
		return
	}
	defer file.Close()

	// อ่าน maxAvatarBytes+1 ไบต์: ได้ครบ = ไฟล์ใหญ่เกินเพดาน
	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to read uploaded file")
		return
	}
	if len(data) > maxAvatarBytes {
		writeAvatarTooLarge(w)
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "uploaded file is empty")
		return
	}

	mime := http.DetectContentType(data)
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	if !avatarMimes[mime] {
		writeError(w, http.StatusBadRequest, "invalid_image_type",
			"avatar must be a PNG, JPEG, GIF or WebP image")
		return
	}

	updated, err := a.st.SetUserAvatar(r.Context(), user.ID, data, mime)
	if err != nil {
		a.log.Error("set avatar failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	a.audit(r, &user.ID, nil, "avatar_updated", map[string]any{"bytes": len(data), "mime": mime})

	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(updated)})
}

func (a *API) handleDeleteAvatar(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())

	updated, err := a.st.ClearUserAvatar(r.Context(), user.ID)
	if err != nil {
		a.log.Error("clear avatar failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	a.audit(r, &user.ID, nil, "avatar_removed", nil)

	writeJSON(w, http.StatusOK, map[string]any{"user": toUserView(updated)})
}

// handleGetAvatar เสิร์ฟรูปของ user คนไหนก็ได้ให้ user ที่ login แล้ว (avatar โผล่ในลิสต์
// สมาชิก/access อยู่แล้ว จึงไม่ผูก capability เหมือน /users/directory)
func (a *API) handleGetAvatar(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid user id")
		return
	}

	data, mime, updatedAt, err := a.st.GetUserAvatar(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "avatar not found")
		return
	}
	if err != nil {
		a.log.Error("get avatar failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// URL มี ?v=<unix> เป็น cache-buster อยู่แล้ว — cache ยาวได้ แต่ต้อง private
	// (รูปนี้เสิร์ฟหลัง auth cookie ห้ามให้ proxie ตัวกลางเก็บแชร์)
	etag := `"` + strconv.FormatInt(updatedAt.UnixNano(), 36) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// กันไม่ให้ bytes ที่ user อัปโหลดถูก render เป็นหน้าเว็บใน origin เดียวกับ panel
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")

	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(w, r, "", updatedAt, bytes.NewReader(data))
}
