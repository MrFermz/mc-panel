package httpapi

import (
	"errors"
	"net/http"
	"strings"

	agentv1 "github.com/game-manager/proto/gen/go/gamemanager/agent/v1"

	"github.com/game-manager/control-plane/internal/agenthub"
	"github.com/game-manager/control-plane/internal/auth"
	"github.com/game-manager/control-plane/internal/games"
	"github.com/game-manager/control-plane/internal/store"
)

// ไฟล์ config หลักของ server (ชื่อไฟล์มาจาก game definition) เป็นไฟล์ text ที่ root ของ
// instance — จัดการผ่าน file manager stream เดียวกัน (gate ด้วย cap settings.view/edit
// ต่อ server: admin/owner/grant มี cap). ชื่อไฟล์/catalog/ไวยากรณ์มาจาก game definition

// configFieldView = 1 key ใน catalog ในรูปที่ web ใช้ render form (shape เดิมของ API)
type configFieldView struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"` // enum | int | bool | string
	Options []string `json:"options"`
	Min     *int     `json:"min"`
	Max     *int     `json:"max"`
}

// catalogFields คืน catalog ในรูปที่พร้อม marshal (options เป็น [] เมื่อว่าง ไม่ใช่ null)
func catalogFields(spec games.ConfigSpec) []configFieldView {
	fields := make([]configFieldView, 0, len(spec.Fields))
	for _, f := range spec.Fields {
		opts := f.Options
		if opts == nil {
			opts = []string{}
		}
		fields = append(fields, configFieldView{
			Key: f.Key, Label: f.Label, Type: f.Type, Options: opts, Min: f.Min, Max: f.Max,
		})
	}
	return fields
}

// isFileNotFound: agent ไม่มี enum error — จับ substring แบบเดียวกับ writeFileOpError
func isFileNotFound(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "not found") || strings.Contains(m, "no such") || strings.Contains(m, "does not exist")
}

// readConfigFile อ่านไฟล์ config ของเกมผ่าน gRPC. คืน (content, true) เมื่อสำเร็จ
// (ไฟล์ไม่มี = content ว่าง + true, ไม่ถือเป็น error); เขียน error response เองแล้วคืน false เมื่อ fail จริง
func (a *API) readConfigFile(w http.ResponseWriter, r *http.Request, srv *store.Server, def *games.Definition) (string, bool) {
	resp, err := a.hub.SendFileRequest(r.Context(), srv.NodeID, &agentv1.FileRequest{
		ServerId: srv.ID.String(),
		Op:       &agentv1.FileRequest_Read{Read: &agentv1.FileRead{Path: def.Config.FileName}},
	})
	switch {
	case errors.Is(err, agenthub.ErrNodeNotConnected), errors.Is(err, agenthub.ErrSendTimeout):
		writeError(w, http.StatusServiceUnavailable, "node_offline", "node agent is offline")
		return "", false
	case errors.Is(err, agenthub.ErrAgentTimeout):
		writeError(w, http.StatusGatewayTimeout, "agent_timeout", "node agent did not respond in time")
		return "", false
	case err != nil:
		a.log.Error("properties read failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return "", false
	}
	if !resp.Success {
		if isFileNotFound(resp.Error) {
			return "", true
		}
		a.writeFileOpError(w, resp.Error)
		return "", false
	}
	return string(resp.Content), true
}

// handleMetaConfig คืน catalog + ค่า default โดยไม่ผูกกับ server ตัวไหน — wizard สร้าง server
// ใช้ render ฟอร์ม properties ตั้งแต่ก่อนมี instance จริง (ค่าที่กรอกถูก apply หลังสร้างเสร็จ)
// เลือกเกมด้วย `?game=` เหมือน meta endpoint อื่น
func (a *API) handleMetaConfig(w http.ResponseWriter, r *http.Request) {
	def, ok := a.gameFromQuery(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fields": catalogFields(def.Config),
		"values": def.Config.Defaults(),
	})
}

func (a *API) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	srv, _, ok := a.loadServerCap(w, r, capSettingsView)
	if !ok {
		return
	}
	def, ok := a.gameOf(w, srv)
	if !ok {
		return
	}

	text, ok := a.readConfigFile(w, r, srv, def)
	if !ok {
		return
	}
	parsed := def.Config.Format.Parse(text)

	// key นอก catalog ไม่ถูกคืนออกไป (UI ไม่มีที่แสดง) แต่ยังอยู่ในไฟล์ครบ — Merge
	// เขียนทับเฉพาะ key ที่ส่งมา ที่เหลือ byte-identical
	values := make(map[string]string, len(def.Config.Fields))
	for _, f := range def.Config.Fields {
		if v, ok := parsed[f.Key]; ok {
			values[f.Key] = v
		} else {
			values[f.Key] = f.Default
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"fields": catalogFields(def.Config),
		"values": values,
	})
}

func (a *API) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, _, ok := a.loadServerCap(w, r, capSettingsEdit)
	if !ok {
		return
	}
	def, ok := a.gameOf(w, srv)
	if !ok {
		return
	}

	// เกมที่เขียนทับไฟล์ config ตอน shutdown แก้ตอนรันอยู่จะถูก overwrite หายทันที
	if !def.Config.EditableWhileRunning && srv.Status != "stopped" && srv.Status != "errored" {
		writeError(w, http.StatusConflict, "invalid_state",
			"stop the server before editing "+def.Config.FileName+
				" ("+def.Label+" overwrites it on shutdown)")
		return
	}

	var req struct {
		Values map[string]string `json:"values"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	for key, val := range req.Values {
		f, ok := def.Config.Field(key)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_property", "unknown property: "+key)
			return
		}
		if !f.Valid(val) {
			writeError(w, http.StatusBadRequest, "invalid_property", "invalid value for property: "+key)
			return
		}
	}

	text, ok := a.readConfigFile(w, r, srv, def)
	if !ok {
		return
	}
	merged := def.Config.Format.Merge(text, req.Values, def.Config.Fields)

	resp, err := a.hub.SendFileRequest(r.Context(), srv.NodeID, &agentv1.FileRequest{
		ServerId: srv.ID.String(),
		Op:       &agentv1.FileRequest_Write{Write: &agentv1.FileWrite{Path: def.Config.FileName, Content: []byte(merged)}},
	})
	switch {
	case errors.Is(err, agenthub.ErrNodeNotConnected), errors.Is(err, agenthub.ErrSendTimeout):
		writeError(w, http.StatusServiceUnavailable, "node_offline", "node agent is offline")
		return
	case errors.Is(err, agenthub.ErrAgentTimeout):
		writeError(w, http.StatusGatewayTimeout, "agent_timeout", "node agent did not respond in time")
		return
	case err != nil:
		a.log.Error("properties write failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if !resp.Success {
		a.writeFileOpError(w, resp.Error)
		return
	}

	a.audit(r, &user.ID, &srv.ID, "config_update", map[string]any{"keys": keysOf(req.Values)})
	w.WriteHeader(http.StatusNoContent)
}

func keysOf(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
