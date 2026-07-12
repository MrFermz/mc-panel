package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"

	"github.com/mc-panel/control-plane/internal/agenthub"
	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

// canManageFiles: owner/admin หรือ permission ที่ตั้ง can_manage_files ไว้ (ดู docs/api.md)
func canManageFiles(user *store.User, perm *store.Permission) bool {
	if user.IsAdmin {
		return true
	}
	return perm != nil && (perm.Role == "owner" || perm.CanManageFiles)
}

// loadServerForFiles โหลด server + เช็คสิทธิ์จัดการไฟล์ เขียน error response เองเมื่อไม่ผ่าน
func (a *API) loadServerForFiles(w http.ResponseWriter, r *http.Request) (*store.Server, bool) {
	user := auth.UserFrom(r.Context())
	srv, perm, ok := a.loadServerAccess(w, r)
	if !ok {
		return nil, false
	}
	if !canManageFiles(user, perm) {
		writeError(w, http.StatusForbidden, "forbidden", "file management access required")
		return nil, false
	}
	return srv, true
}

type fileEntryView struct {
	Name    string    `json:"name"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// sendFileRequest ส่ง request ไป agent แล้ว map error transport + FileResponse.error
// เป็น HTTP code ตาม docs/api.md. คืน (resp, true) เฉพาะเมื่อ success — ไม่งั้นเขียน error response แล้ว
func (a *API) sendFileRequest(w http.ResponseWriter, r *http.Request, srv *store.Server, req *agentv1.FileRequest) (*agentv1.FileResponse, bool) {
	req.ServerId = srv.ID.String()
	resp, err := a.hub.SendFileRequest(r.Context(), srv.NodeID, req)
	switch {
	case errors.Is(err, agenthub.ErrNodeNotConnected), errors.Is(err, agenthub.ErrSendTimeout):
		writeError(w, http.StatusServiceUnavailable, "node_offline", "node agent is offline")
		return nil, false
	case errors.Is(err, agenthub.ErrAgentTimeout):
		writeError(w, http.StatusGatewayTimeout, "agent_timeout", "node agent did not respond in time")
		return nil, false
	case err != nil:
		a.log.Error("file request failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return nil, false
	}
	if !resp.Success {
		a.writeFileOpError(w, resp.Error)
		return nil, false
	}
	return resp, true
}

// writeFileOpError map error string จาก agent เป็น code/สถานะ — agent ตรวจ SafeJoin/ขนาด
// ฝั่งมันเอง ที่นี่จับคู่ด้วย substring (ไม่มี enum ใน proto) แล้ว fallback เป็น 400 file_error
func (a *API) writeFileOpError(w http.ResponseWriter, msg string) {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "not found"), strings.Contains(m, "no such"), strings.Contains(m, "does not exist"):
		writeError(w, http.StatusNotFound, "file_not_found", "file not found")
	case strings.Contains(m, "too large"), strings.Contains(m, "too big"), strings.Contains(m, "exceeds"):
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "file is too large")
	case strings.Contains(m, "traversal"), strings.Contains(m, "invalid path"),
		strings.Contains(m, "outside"), strings.Contains(m, "escapes"), strings.Contains(m, "jail"):
		writeError(w, http.StatusBadRequest, "invalid_path", "invalid path")
	default:
		writeError(w, http.StatusBadRequest, "file_error", msg)
	}
}

func (a *API) handleListFiles(w http.ResponseWriter, r *http.Request) {
	srv, ok := a.loadServerForFiles(w, r)
	if !ok {
		return
	}
	path := r.URL.Query().Get("path")

	resp, ok := a.sendFileRequest(w, r, srv, &agentv1.FileRequest{
		Op: &agentv1.FileRequest_List{List: &agentv1.FileList{Path: path}},
	})
	if !ok {
		return
	}

	entries := make([]fileEntryView, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		entries = append(entries, fileEntryView{
			Name:    e.Name,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: time.Unix(e.ModTimeUnix, 0).UTC(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "entries": entries})
}

func (a *API) handleReadFile(w http.ResponseWriter, r *http.Request) {
	srv, ok := a.loadServerForFiles(w, r)
	if !ok {
		return
	}
	path := r.URL.Query().Get("path")

	resp, ok := a.sendFileRequest(w, r, srv, &agentv1.FileRequest{
		Op: &agentv1.FileRequest_Read{Read: &agentv1.FileRead{Path: path}},
	})
	if !ok {
		return
	}

	// content ตอบเป็น text utf-8 เท่านั้น — binary จะพัง JSON/editor ฝั่ง web
	if !utf8.Valid(resp.Content) {
		writeError(w, http.StatusUnsupportedMediaType, "binary_file", "file is not valid UTF-8 text")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":      path,
		"content":   string(resp.Content),
		"truncated": resp.Truncated,
	})
}

func (a *API) handleWriteFile(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, ok := a.loadServerForFiles(w, r)
	if !ok {
		return
	}

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if _, ok := a.sendFileRequest(w, r, srv, &agentv1.FileRequest{
		Op: &agentv1.FileRequest_Write{Write: &agentv1.FileWrite{Path: req.Path, Content: []byte(req.Content)}},
	}); !ok {
		return
	}

	a.audit(r, &user.ID, &srv.ID, "file_write", map[string]any{"path": req.Path})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleMakeDir(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, ok := a.loadServerForFiles(w, r)
	if !ok {
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if _, ok := a.sendFileRequest(w, r, srv, &agentv1.FileRequest{
		Op: &agentv1.FileRequest_Mkdir{Mkdir: &agentv1.FileMkdir{Path: req.Path}},
	}); !ok {
		return
	}

	a.audit(r, &user.ID, &srv.ID, "file_mkdir", map[string]any{"path": req.Path})
	w.WriteHeader(http.StatusCreated)
}

func (a *API) handleRenameFile(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, ok := a.loadServerForFiles(w, r)
	if !ok {
		return
	}

	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if _, ok := a.sendFileRequest(w, r, srv, &agentv1.FileRequest{
		Op: &agentv1.FileRequest_Rename{Rename: &agentv1.FileRename{From: req.From, To: req.To}},
	}); !ok {
		return
	}

	a.audit(r, &user.ID, &srv.ID, "file_rename", map[string]any{"from": req.From, "to": req.To})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, ok := a.loadServerForFiles(w, r)
	if !ok {
		return
	}
	path := r.URL.Query().Get("path")

	if _, ok := a.sendFileRequest(w, r, srv, &agentv1.FileRequest{
		Op: &agentv1.FileRequest_Delete{Delete: &agentv1.FileDelete{Path: path}},
	}); !ok {
		return
	}

	a.audit(r, &user.ID, &srv.ID, "file_delete", map[string]any{"path": path})
	w.WriteHeader(http.StatusNoContent)
}
