package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"

	"github.com/mc-panel/control-plane/internal/agenthub"
	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/mojang"
	"github.com/mc-panel/control-plane/internal/playerface"
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

// ไฟล์ state ของ Minecraft ที่ root ของ server dir ที่ merge เข้ารายชื่อผู้เล่น
const (
	usercacheFileName = "usercache.json"
	opsFileName       = "ops.json"
	bannedFileName    = "banned-players.json"
)

// mcPlayerEntry = subset ที่พอสำหรับ usercache/ops/banned (ทุกไฟล์มี name+uuid)
type mcPlayerEntry struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

// mergedPlayerView = 1 ผู้เล่นหลัง merge ทุก source (ดู docs/api.md)
type mergedPlayerView struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	Whitelisted bool   `json:"whitelisted"`
	Seen        bool   `json:"seen"`
	Op          bool   `json:"op"`
	Banned      bool   `json:"banned"`
	// Online มาจาก serverstats cache (agent อ่านจาก console) ไม่ใช่ไฟล์ — ไม่มี I/O เพิ่ม
	Online bool `json:"online"`
	// PlaytimeSeconds จาก world stats ของ MC — 0 = ไม่รู้ (ยังไม่เคยเล่น/อ่านไม่ได้/เกิน cap)
	PlaytimeSeconds int64 `json:"playtime_seconds"`
}

// normUUID ทำ key รวม: ตัด dash + lowercase (ไฟล์ MC ใช้ dashed, DB ก็ dashed แต่กันเคสไม่ตรง)
func normUUID(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "-", ""))
}

// displayUUID normalize เป็น canonical dashed lowercase เมื่อ parse ได้ ไม่งั้นคืนค่าดิบ
func displayUUID(s string) string {
	if u, err := uuid.Parse(strings.TrimSpace(s)); err == nil {
		return u.String()
	}
	return strings.TrimSpace(s)
}

// handleListPlayers รวมรายชื่อผู้เล่นจากหลาย source: DB whitelist + usercache (seen) +
// ops + banned-players (อ่านไฟล์ผ่าน agent). node offline = degrade เหลือ DB whitelist
// (แท็บยังใช้ได้ตอน server หยุด/offline). ไฟล์ไม่มี = ถือว่าว่าง ไม่ error
func (a *API) handleListPlayers(w http.ResponseWriter, r *http.Request) {
	srv, _, ok := a.loadServerCap(w, r, capPlayersView)
	if !ok {
		return
	}

	dbPlayers, err := a.st.ListServerPlayers(r.Context(), srv.ID)
	if err != nil {
		a.log.Error("list players failed", "server_id", srv.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// สร้าง accumulator จาก DB ก่อน (whitelisted=true) — key ด้วย normUUID
	acc := make(map[string]*mergedPlayerView)
	upsert := func(rawUUID, username string) *mergedPlayerView {
		key := normUUID(rawUUID)
		p := acc[key]
		if p == nil {
			p = &mergedPlayerView{UUID: displayUUID(rawUUID)}
			acc[key] = p
		}
		// username จากไฟล์ (usercache/ops/banned) สะท้อนชื่อปัจจุบัน จึงให้ override DB
		if strings.TrimSpace(username) != "" {
			p.Username = username
		}
		return p
	}

	for _, dp := range dbPlayers {
		p := upsert(dp.UUID.String(), "")
		if p.Username == "" {
			p.Username = dp.Username
		}
		p.Whitelisted = true
	}

	whitelistEnabled := false
	offline := false
	levelName := defaultLevelName

	// server.properties → white-list flag + level-name (ใช้หา world stats ต่อ) best-effort
	if content, found, off, ferr := a.readServerFile(r.Context(), srv, propertiesFileName); off {
		offline = true
	} else if ferr != nil {
		a.log.Warn("players: read properties failed", "server_id", srv.ID, "error", ferr)
	} else if found {
		props := parseProperties(string(content))
		whitelistEnabled = props["white-list"] == "true"
		if lv := strings.TrimSpace(props["level-name"]); lv != "" && isSafeLevelName(lv) {
			levelName = lv
		}
	}

	// อ่านไฟล์ผู้เล่น 3 ตัวเฉพาะเมื่อ node ยัง online — ไม่งั้น degrade เหลือ DB whitelist
	if !offline {
		merge := func(name string, apply func(*mergedPlayerView)) {
			entries, off := a.readMCPlayerFile(r.Context(), srv, name)
			if off {
				offline = true
				return
			}
			for _, e := range entries {
				if normUUID(e.UUID) == "" {
					continue
				}
				apply(upsert(e.UUID, e.Name))
			}
		}
		merge(usercacheFileName, func(p *mergedPlayerView) { p.Seen = true })
		if !offline {
			merge(opsFileName, func(p *mergedPlayerView) { p.Op = true })
		}
		if !offline {
			merge(bannedFileName, func(p *mergedPlayerView) { p.Banned = true })
		}
	}

	// ผู้เล่นที่ออนไลน์ — match ด้วยชื่อ (console บอกแค่ username ไม่มี uuid)
	if st, ok := a.stats.Get(srv.ID); ok && srv.Status == "running" {
		byName := make(map[string]*mergedPlayerView, len(acc))
		for _, p := range acc {
			byName[strings.ToLower(p.Username)] = p
		}
		for _, name := range st.OnlinePlayers {
			if p, found := byName[strings.ToLower(name)]; found {
				p.Online = true
				continue
			}
			// อยู่ในเกมแต่ไม่โผล่ในไฟล์ไหนเลย — usercache.json flush ช้ากว่าคนเพิ่ง join
			// ถ้าไม่ใส่เข้าไป คนที่กำลังเล่นอยู่จะหายจากรายชื่อทั้งที่เห็นใน dashboard
			// (ไม่มี uuid ให้ — UI key ด้วย uuid ก่อนแล้ว fallback username)
			acc["online:"+strings.ToLower(name)] = &mergedPlayerView{
				Username: name,
				Online:   true,
			}
		}
	}

	players := make([]mergedPlayerView, 0, len(acc))
	for _, p := range acc {
		players = append(players, *p)
	}
	if !offline {
		a.fillPlaytimes(r.Context(), srv, levelName, players)
	}
	sort.Slice(players, func(i, j int) bool {
		return strings.ToLower(players[i].Username) < strings.ToLower(players[j].Username)
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"whitelist_enabled": whitelistEnabled,
		"players":           players,
	})
}

// handlePlayerFace เสิร์ฟรูปหน้าผู้เล่น (crop จาก skin ของ Mojang) เป็น PNG — control-plane เป็น
// ตัวดึงเอง ไม่ให้ browser ยิง third-party host (leak IP + เพิ่ม host ที่ต้องเชื่อใจ)
// uuid ที่ไม่มี skin (offline-mode/ไม่มี texture) ตอบ 404 → web fallback ไปตัวอักษรย่อ
func (a *API) handlePlayerFace(w http.ResponseWriter, r *http.Request) {
	// สิทธิ์เท่ากับดูรายชื่อผู้เล่น (รูปโผล่ในลิสต์เดียวกัน) — ยึด access ต่อ server ไม่ให้เป็น open proxy
	if _, _, ok := a.loadServerCap(w, r, capPlayersView); !ok {
		return
	}

	playerUUID, err := uuidParam(r, "uuid")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "uuid must be a valid UUID")
		return
	}

	facePNG, err := a.faces.Face(r.Context(), playerUUID)
	if errors.Is(err, playerface.ErrNoSkin) {
		writeError(w, http.StatusNotFound, "not_found", "no skin for this player")
		return
	}
	if err != nil {
		a.log.Warn("player face fetch failed", "uuid", playerUUID, "error", err)
		writeError(w, http.StatusBadGateway, "mojang_unavailable", "could not reach Mojang for the player skin")
		return
	}

	// skin เปลี่ยนถูก refresh ฝั่ง control-plane ด้วย TTL — browser cache สั้น ๆ พอ ให้เห็นรูปใหม่ไว
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(facePNG)
}

// readServerFile อ่านไฟล์ text/JSON ผ่าน agent. คืน (content, found, offline, err):
// offline=true เมื่อ node ติดต่อไม่ได้ (ไม่ hard-fail), found=false เมื่อไฟล์ไม่มี (ไม่ใช่ error)
func (a *API) readServerFile(ctx context.Context, srv *store.Server, path string) ([]byte, bool, bool, error) {
	resp, err := a.hub.SendFileRequest(ctx, srv.NodeID, &agentv1.FileRequest{
		ServerId: srv.ID.String(),
		Op:       &agentv1.FileRequest_Read{Read: &agentv1.FileRead{Path: path}},
	})
	switch {
	case errors.Is(err, agenthub.ErrNodeNotConnected), errors.Is(err, agenthub.ErrSendTimeout),
		errors.Is(err, agenthub.ErrAgentTimeout):
		return nil, false, true, nil
	case err != nil:
		return nil, false, false, err
	}
	if !resp.Success {
		if isFileNotFound(resp.Error) {
			return nil, false, false, nil
		}
		return nil, false, false, fmt.Errorf("%s", resp.Error)
	}
	return resp.Content, true, false, nil
}

// readMCPlayerFile อ่าน + parse ไฟล์ JSON array ของ MC (usercache/ops/banned) ผ่าน agent.
// คืน (entries, offline). ไฟล์ไม่มี/parse ไม่ได้/agent error (ที่ไม่ใช่ offline) = entries ว่าง
// (best-effort, ไม่ทำให้แท็บล่ม); offline=true เมื่อ node ติดต่อไม่ได้
func (a *API) readMCPlayerFile(ctx context.Context, srv *store.Server, path string) ([]mcPlayerEntry, bool) {
	content, found, offline, err := a.readServerFile(ctx, srv, path)
	if offline {
		return nil, true
	}
	if err != nil {
		a.log.Warn("players: read file failed", "server_id", srv.ID, "path", path, "error", err)
		return nil, false
	}
	if !found || len(content) == 0 {
		return nil, false
	}
	var entries []mcPlayerEntry
	if err := json.Unmarshal(content, &entries); err != nil {
		a.log.Warn("players: parse file failed", "server_id", srv.ID, "path", path, "error", err)
		return nil, false
	}
	return entries, false
}

func (a *API) handleAddPlayer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, _, ok := a.loadServerCap(w, r, capPlayersManage)
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
	srv, _, ok := a.loadServerCap(w, r, capPlayersManage)
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

// ---------- playtime (world stats ของ MC) ----------

const (
	defaultLevelName = "world"
	// playtimeMaxPlayers จำกัดจำนวนไฟล์ที่อ่านต่อ 1 request — stats เป็นไฟล์ละคน
	// server ที่มีผู้เล่นเยอะจะกลายเป็น N round-trip ต่อการเปิดหน้า จึงตัดที่เพดานนี้
	// (เกินเพดาน = playtime_seconds 0 → UI โชว์ "—" ไม่ใช่ค่าผิด)
	playtimeMaxPlayers = 50
	playtimeWorkers    = 8
	// ticks ต่อวินาทีของ MC — stat เก็บเป็น tick
	ticksPerSecond = 20
)

// isSafeLevelName กัน level-name จาก server.properties พาไปนอก jail (ค่ามาจากไฟล์ที่ user แก้ได้)
// agent มี SafeJoin อยู่แล้ว แต่ปฏิเสธตั้งแต่ต้นทางชัดกว่า
func isSafeLevelName(s string) bool {
	if s == "" || strings.Contains(s, "/") || strings.Contains(s, `\`) || strings.Contains(s, "..") {
		return false
	}
	return true
}

// mcStatsFile = shape ของ world/stats/{uuid}.json เท่าที่ต้องใช้
// play_time (1.17+) กับ play_one_minute (เวอร์ชันเก่า) เก็บ tick เหมือนกัน คนละ key
type mcStatsFile struct {
	Stats struct {
		Custom map[string]int64 `json:"minecraft:custom"`
	} `json:"stats"`
}

// fillPlaytimes อ่าน world/stats/{uuid}.json ของแต่ละคนแบบขนาน (best-effort)
// อ่านไม่ได้/ไม่มีไฟล์ = ปล่อย 0 ไม่ทำให้ทั้ง request ล่ม
func (a *API) fillPlaytimes(ctx context.Context, srv *store.Server, levelName string, players []mergedPlayerView) {
	targets := make([]int, 0, len(players))
	for i := range players {
		// ไม่เคยเข้าเซิร์ฟเวอร์ = ไม่มีไฟล์ stats แน่นอน ไม่ต้องยิงถาม
		if players[i].Seen && players[i].UUID != "" {
			targets = append(targets, i)
		}
	}
	if len(targets) > playtimeMaxPlayers {
		a.log.Info("players: playtime lookup capped", "server_id", srv.ID,
			"players", len(targets), "cap", playtimeMaxPlayers)
		targets = targets[:playtimeMaxPlayers]
	}
	if len(targets) == 0 {
		return
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, playtimeWorkers)
	for _, idx := range targets {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			players[i].PlaytimeSeconds = a.readPlaytime(ctx, srv, levelName, players[i].UUID)
		}(idx)
	}
	wg.Wait()
}

func (a *API) readPlaytime(ctx context.Context, srv *store.Server, levelName, playerUUID string) int64 {
	// uuid มาจากไฟล์ที่ user ที่มีสิทธิ์ file manager เขียนเองได้ (usercache/ops/banned) —
	// displayUUID คืนค่าดิบเมื่อ parse ไม่ผ่าน จึงต้องบังคับให้เป็น uuid จริงก่อนเอาไปต่อเป็น path
	// (agent มี SafeJoin กันอยู่แล้ว แต่ห้ามพึ่งชั้นเดียว — เหมือนที่ทำกับ level-name)
	parsed, err := uuid.Parse(strings.TrimSpace(playerUUID))
	if err != nil {
		return 0
	}
	path := levelName + "/stats/" + parsed.String() + ".json"
	content, found, offline, err := a.readServerFile(ctx, srv, path)
	if offline || err != nil || !found || len(content) == 0 {
		return 0
	}
	var f mcStatsFile
	if err := json.Unmarshal(content, &f); err != nil {
		return 0
	}
	ticks := f.Stats.Custom["minecraft:play_time"]
	if ticks == 0 {
		ticks = f.Stats.Custom["minecraft:play_one_minute"]
	}
	return ticks / ticksPerSecond
}

// ---------- player action (op/deop/kick/ban/pardon) ----------

// playerCommands map action -> คำสั่ง console ของ MC (allow-list — ห้ามรับคำสั่งดิบจาก client)
var playerCommands = map[string]string{
	"op":     "op",
	"deop":   "deop",
	"kick":   "kick",
	"ban":    "ban",
	"pardon": "pardon",
}

// safeUsernameRe — ชื่อที่จะถูกต่อเข้าไปในคำสั่ง console ต้องไม่มี whitespace/newline
// ไม่งั้นเป็น command injection เข้า server console ได้ตรง ๆ (WriteInput ต่อ "\n" ท้ายคำสั่ง)
var safeUsernameRe = regexp.MustCompile(`^[A-Za-z0-9_.*-]{1,32}$`)

// handlePlayerAction ส่งคำสั่งจัดการผู้เล่นเข้า console ของ server
// ต้องมี cap players.moderate ต่อ server (op/deop/kick/ban) — running เพราะสั่งผ่าน stdin
func (a *API) handlePlayerAction(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())
	srv, _, ok := a.loadServerCap(w, r, capPlayersModerate)
	if !ok {
		return
	}

	var req struct {
		Action   string `json:"action"`
		Username string `json:"username"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	cmd, known := playerCommands[req.Action]
	if !known {
		writeError(w, http.StatusBadRequest, "invalid_action", "unsupported player action")
		return
	}
	username := strings.TrimSpace(req.Username)
	if !safeUsernameRe.MatchString(username) {
		writeError(w, http.StatusBadRequest, "invalid_username", "invalid minecraft username")
		return
	}
	if srv.Status != "running" {
		writeError(w, http.StatusConflict, "invalid_state", "server must be running")
		return
	}

	if err := a.hub.SendConsoleInput(srv.NodeID, srv.ID, cmd+" "+username); err != nil {
		a.log.Warn("player action failed", "server_id", srv.ID, "action", req.Action, "error", err)
		writeError(w, http.StatusServiceUnavailable, "node_offline", "the node is not reachable")
		return
	}

	a.audit(r, &user.ID, &srv.ID, "player_action", map[string]any{
		"action": req.Action, "username": username,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
