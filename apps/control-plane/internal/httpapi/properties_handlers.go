package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"

	"github.com/mc-panel/control-plane/internal/agenthub"
	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

// server.properties เป็นไฟล์ text ที่ root ของ server instance — จัดการผ่าน file manager
// stream เดียวกัน (gate ด้วย cap settings.view/edit ต่อ server: admin/owner/grant มี cap)
const propertiesFileName = "server.properties"

// propField อธิบาย 1 key ใน catalog สำหรับให้ web render form + validate ฝั่ง server
type propField struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"` // enum | int | bool | string
	Options []string `json:"options"`
	Min     *int     `json:"min"`
	Max     *int     `json:"max"`
	Default string   `json:"-"`
}

func intPtr(n int) *int { return &n }

// propCatalog: curated set ของ key ที่ให้แก้ผ่าน UI (key อื่นใน server.properties เก็บไว้ verbatim ใน extra)
var propCatalog = []propField{
	{Key: "gamemode", Label: "Game Mode", Type: "enum", Options: []string{"survival", "creative", "adventure", "spectator"}, Default: "survival"},
	{Key: "difficulty", Label: "Difficulty", Type: "enum", Options: []string{"peaceful", "easy", "normal", "hard"}, Default: "easy"},
	{Key: "hardcore", Label: "Hardcore", Type: "bool", Default: "false"},
	{Key: "pvp", Label: "PvP", Type: "bool", Default: "true"},
	{Key: "max-players", Label: "Max Players", Type: "int", Min: intPtr(1), Max: intPtr(2147483647), Default: "20"},
	{Key: "motd", Label: "MOTD", Type: "string", Default: "A Minecraft Server"},
	{Key: "online-mode", Label: "Online Mode", Type: "bool", Default: "true"},
	{Key: "white-list", Label: "Whitelist", Type: "bool", Default: "false"},
	{Key: "enforce-whitelist", Label: "Enforce Whitelist", Type: "bool", Default: "false"},
	{Key: "spawn-protection", Label: "Spawn Protection", Type: "int", Min: intPtr(0), Default: "16"},
	{Key: "view-distance", Label: "View Distance", Type: "int", Min: intPtr(3), Max: intPtr(32), Default: "10"},
	{Key: "simulation-distance", Label: "Simulation Distance", Type: "int", Min: intPtr(3), Max: intPtr(32), Default: "10"},
	{Key: "level-name", Label: "Level Name", Type: "string", Default: "world"},
	{Key: "level-seed", Label: "Level Seed", Type: "string", Default: ""},
	{Key: "level-type", Label: "Level Type", Type: "enum", Options: []string{"minecraft:normal", "minecraft:flat", "minecraft:large_biomes", "minecraft:amplified"}, Default: "minecraft:normal"},
	{Key: "allow-nether", Label: "Allow Nether", Type: "bool", Default: "true"},
	{Key: "allow-flight", Label: "Allow Flight", Type: "bool", Default: "false"},
	{Key: "enable-command-block", Label: "Enable Command Block", Type: "bool", Default: "false"},
	{Key: "spawn-monsters", Label: "Spawn Monsters", Type: "bool", Default: "true"},
	{Key: "spawn-animals", Label: "Spawn Animals", Type: "bool", Default: "true"},
	{Key: "spawn-npcs", Label: "Spawn NPCs", Type: "bool", Default: "true"},
	{Key: "generate-structures", Label: "Generate Structures", Type: "bool", Default: "true"},
	{Key: "force-gamemode", Label: "Force Game Mode", Type: "bool", Default: "false"},
	{Key: "player-idle-timeout", Label: "Player Idle Timeout", Type: "int", Min: intPtr(0), Default: "0"},
	{Key: "max-world-size", Label: "Max World Size", Type: "int", Min: intPtr(1), Max: intPtr(29999984), Default: "29999984"},
}

// catalogFields คืน catalog ในรูปที่พร้อม marshal (options เป็น [] เมื่อว่าง ไม่ใช่ null)
func catalogFields() []propField {
	fields := make([]propField, len(propCatalog))
	for i, f := range propCatalog {
		if f.Options == nil {
			f.Options = []string{}
		}
		fields[i] = f
	}
	return fields
}

func propByKey(key string) (propField, bool) {
	for _, f := range propCatalog {
		if f.Key == key {
			return f, true
		}
	}
	return propField{}, false
}

// isFileNotFound: agent ไม่มี enum error — จับ substring แบบเดียวกับ writeFileOpError
func isFileNotFound(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "not found") || strings.Contains(m, "no such") || strings.Contains(m, "does not exist")
}

// readPropertiesFile อ่าน server.properties ผ่าน gRPC. คืน (content, true) เมื่อสำเร็จ
// (ไฟล์ไม่มี = content ว่าง + true, ไม่ถือเป็น error); เขียน error response เองแล้วคืน false เมื่อ fail จริง
func (a *API) readPropertiesFile(w http.ResponseWriter, r *http.Request, srv *store.Server) (string, bool) {
	resp, err := a.hub.SendFileRequest(r.Context(), srv.NodeID, &agentv1.FileRequest{
		ServerId: srv.ID.String(),
		Op:       &agentv1.FileRequest_Read{Read: &agentv1.FileRead{Path: propertiesFileName}},
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

// parseProperties แยก key=value จาก text (ข้าม comment/บรรทัดว่าง). split ตัว `=` แรกเท่านั้น
func parseProperties(text string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(line[i+1:])
	}
	return out
}

// mergeProperties รวมค่าที่จะเปลี่ยนกลับเข้าไฟล์ โดยรักษา comment/บรรทัดว่าง/ลำดับ key เดิม
// (key ที่มีอยู่แล้ว → แทนค่าในบรรทัดนั้น; catalog key ที่ยังไม่มี → append ต่อท้าย) ส่วนที่เหลือ byte-identical
func mergeProperties(text string, values map[string]string) string {
	lines := strings.Split(text, "\n")
	seen := make(map[string]bool)

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		if v, ok := values[key]; ok {
			lines[idx] = key + "=" + v
			seen[key] = true
		}
	}

	// append catalog key ที่ยังไม่มีในไฟล์ ตามลำดับ catalog (deterministic)
	var appended []string
	for _, f := range propCatalog {
		if v, ok := values[f.Key]; ok && !seen[f.Key] {
			appended = append(appended, f.Key+"="+v)
		}
	}

	if len(appended) > 0 {
		// ตัด trailing empty line ที่เกิดจากไฟล์ลงท้ายด้วย "\n" ก่อน append เพื่อไม่ให้เกิดบรรทัดว่างคั่น
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, appended...)
	}

	return strings.Join(lines, "\n")
}

// handleMetaProperties คืน catalog + ค่า default โดยไม่ผูกกับ server ตัวไหน — wizard สร้าง server
// ใช้ render ฟอร์ม properties ตั้งแต่ก่อนมี instance จริง (ค่าที่กรอกถูก apply หลังสร้างเสร็จ)
func (a *API) handleMetaProperties(w http.ResponseWriter, r *http.Request) {
	values := make(map[string]string, len(propCatalog))
	for _, f := range propCatalog {
		values[f.Key] = f.Default
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fields": catalogFields(),
		"values": values,
	})
}

func (a *API) handleGetProperties(w http.ResponseWriter, r *http.Request) {
	srv, _, ok := a.loadServerCap(w, r, capSettingsView)
	if !ok {
		return
	}

	text, ok := a.readPropertiesFile(w, r, srv)
	if !ok {
		return
	}
	parsed := parseProperties(text)

	// key นอก catalog ไม่ถูกคืนออกไป (UI ไม่มีที่แสดง) แต่ยังอยู่ในไฟล์ครบ — mergeProperties
	// เขียนทับเฉพาะ key ที่ส่งมา ที่เหลือ byte-identical
	values := make(map[string]string, len(propCatalog))
	for _, f := range propCatalog {
		if v, ok := parsed[f.Key]; ok {
			values[f.Key] = v
		} else {
			values[f.Key] = f.Default
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"fields": catalogFields(),
		"values": values,
	})
}

func (a *API) handleUpdateProperties(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, _, ok := a.loadServerCap(w, r, capSettingsEdit)
	if !ok {
		return
	}

	// MC เขียนทับ server.properties ตอน shutdown — แก้ตอนรันอยู่จะถูก overwrite หายทันที
	if srv.Status != "stopped" && srv.Status != "errored" {
		writeError(w, http.StatusConflict, "invalid_state",
			"stop the server before editing server.properties (Minecraft overwrites it on shutdown)")
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
		f, ok := propByKey(key)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_property", "unknown property: "+key)
			return
		}
		if !validatePropValue(f, val) {
			writeError(w, http.StatusBadRequest, "invalid_property", "invalid value for property: "+key)
			return
		}
	}

	text, ok := a.readPropertiesFile(w, r, srv)
	if !ok {
		return
	}
	merged := mergeProperties(text, req.Values)

	resp, err := a.hub.SendFileRequest(r.Context(), srv.NodeID, &agentv1.FileRequest{
		ServerId: srv.ID.String(),
		Op:       &agentv1.FileRequest_Write{Write: &agentv1.FileWrite{Path: propertiesFileName, Content: []byte(merged)}},
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

	a.audit(r, &user.ID, &srv.ID, "properties_update", map[string]any{"keys": keysOf(req.Values)})
	w.WriteHeader(http.StatusNoContent)
}

func validatePropValue(f propField, val string) bool {
	switch f.Type {
	case "enum":
		for _, opt := range f.Options {
			if val == opt {
				return true
			}
		}
		return false
	case "int":
		n, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		if f.Min != nil && n < *f.Min {
			return false
		}
		if f.Max != nil && n > *f.Max {
			return false
		}
		return true
	case "bool":
		return val == "true" || val == "false"
	case "string":
		return true
	default:
		return false
	}
}

func keysOf(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
