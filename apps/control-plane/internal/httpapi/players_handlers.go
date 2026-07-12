package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/mojang"
	"github.com/mc-panel/control-plane/internal/store"
)

// whitelist.json อยู่ที่ root ของ server dir — DB คือ source of truth, ไฟล์ rebuild ทุกครั้ง
const whitelistFileName = "whitelist.json"

type playerView struct {
	UUID     uuid.UUID `json:"uuid"`
	Username string    `json:"username"`
	AddedAt  time.Time `json:"added_at"`
}

func toPlayerView(p store.ServerPlayer) playerView {
	return playerView{UUID: p.UUID, Username: p.Username, AddedAt: p.CreatedAt}
}

// whitelistEntry คือ shape ที่ Minecraft อ่านจาก whitelist.json
type whitelistEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// isValidUsername: 3-16 ตัว [A-Za-z0-9_] ตามกติกา Minecraft (เช็คก่อนยิง Mojang)
func isValidUsername(s string) bool {
	if len(s) < 3 || len(s) > 16 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}

func (a *API) handleListPlayers(w http.ResponseWriter, r *http.Request) {
	srv, ok := a.loadServerForFiles(w, r)
	if !ok {
		return
	}

	players, err := a.st.ListServerPlayers(r.Context(), srv.ID)
	if err != nil {
		a.log.Error("list players failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	views := make([]playerView, 0, len(players))
	for _, p := range players {
		views = append(views, toPlayerView(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"players": views})
}

func (a *API) handleAddPlayer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, ok := a.loadServerForFiles(w, r)
	if !ok {
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	name := strings.TrimSpace(req.Username)
	if !isValidUsername(name) {
		writeError(w, http.StatusBadRequest, "invalid_username",
			"username must be 3-16 characters of A-Z, a-z, 0-9, or underscore")
		return
	}

	profile, err := mojang.Lookup(r.Context(), name)
	if errors.Is(err, mojang.ErrNotFound) {
		writeError(w, http.StatusNotFound, "player_not_found", "no Minecraft account with that username")
		return
	}
	if err != nil {
		a.log.Error("mojang lookup failed", "username", name, "error", err)
		writeError(w, http.StatusBadGateway, "mojang_unavailable", "could not reach Mojang to verify the username")
		return
	}

	if err := a.st.AddServerPlayer(r.Context(), srv.ID, profile.UUID, profile.Username, &user.ID); err != nil {
		if errors.Is(err, store.ErrPlayerExists) {
			writeError(w, http.StatusConflict, "player_exists", "player is already on the whitelist")
			return
		}
		a.log.Error("add player failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if !a.writeWhitelist(w, r, srv) {
		return
	}
	a.reloadWhitelistIfRunning(srv)

	a.audit(r, &user.ID, &srv.ID, "player_add", map[string]any{
		"uuid": profile.UUID.String(), "username": profile.Username,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"player": playerView{
		UUID: profile.UUID, Username: profile.Username, AddedAt: time.Now().UTC(),
	}})
}

func (a *API) handleRemovePlayer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, ok := a.loadServerForFiles(w, r)
	if !ok {
		return
	}

	playerUUID, err := uuidParam(r, "uuid")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "uuid must be a valid UUID")
		return
	}

	if err := a.st.RemoveServerPlayer(r.Context(), srv.ID, playerUUID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "player not found on this server")
			return
		}
		a.log.Error("remove player failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if !a.writeWhitelist(w, r, srv) {
		return
	}
	a.reloadWhitelistIfRunning(srv)

	a.audit(r, &user.ID, &srv.ID, "player_remove", map[string]any{"uuid": playerUUID.String()})
	w.WriteHeader(http.StatusNoContent)
}

// writeWhitelist rebuild whitelist.json จาก DB rows แล้วเขียนผ่าน agent FileWrite (SafeJoin ที่ agent)
// map transport error เหมือน file manager. คืน false + เขียน error response แล้วเมื่อ fail
func (a *API) writeWhitelist(w http.ResponseWriter, r *http.Request, srv *store.Server) bool {
	players, err := a.st.ListServerPlayers(r.Context(), srv.ID)
	if err != nil {
		a.log.Error("list players for whitelist failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return false
	}

	entries := make([]whitelistEntry, 0, len(players))
	for _, p := range players {
		entries = append(entries, whitelistEntry{UUID: p.UUID.String(), Name: p.Username})
	}
	content, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		a.log.Error("marshal whitelist failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return false
	}

	_, ok := a.sendFileRequest(w, r, srv, &agentv1.FileRequest{
		Op: &agentv1.FileRequest_Write{Write: &agentv1.FileWrite{Path: whitelistFileName, Content: content}},
	})
	return ok
}

// reloadWhitelistIfRunning best-effort: ถ้า server running ส่ง `whitelist reload` เข้า stdin
// ให้ผลทันทีโดยไม่ restart. ถ้าไม่ running หรือ node offline ข้ามเงียบ ๆ (ไฟล์ apply ตอน start ครั้งหน้า)
func (a *API) reloadWhitelistIfRunning(srv *store.Server) {
	if srv.Status != "running" {
		return
	}
	if err := a.hub.SendConsoleInput(srv.NodeID, srv.ID, "whitelist reload"); err != nil {
		a.log.Warn("whitelist reload skipped", "server_id", srv.ID, "error", err)
	}
}
