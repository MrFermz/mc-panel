package events

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

const (
	writeTimeout    = 10 * time.Second
	pongTimeout     = 90 * time.Second
	pingInterval    = 30 * time.Second
	refreshInterval = 15 * time.Second
	// read-only stream: จำกัด read size เล็ก ๆ (ดูดทิ้งเพื่อจับ close/pong เท่านั้น)
	maxReadSize = 1024
)

// capability key ที่กำหนดสิทธิ์การเห็น event — สะกดให้ตรง httpapi.capabilities
// (นิยามซ้ำที่นี่แทน import httpapi เพื่อกัน import cycle)
const (
	capViewAllServers = "servers.view_all"
	capNodesView      = "nodes.view"
)

type WSHandler struct {
	Auth           *auth.Manager
	Store          *store.Store
	Hub            *Hub
	AllowedOrigins []string
	Log            *slog.Logger
}

// HandleEvents รับ browser WS /ws/events — mirror console handshake:
// Origin (ทุก handshake) → Authenticate (cookie mc_session, ตอบ JSON ก่อน upgrade)
// → คำนวณ scope จาก capability → subscribe → push realtime อย่างเดียว (ไม่รับ input)
func (h *WSHandler) HandleEvents(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		writeErr(w, http.StatusForbidden, "forbidden", "origin not allowed")
		return
	}

	user, err := h.Auth.Authenticate(r.Context(), r)
	if errors.Is(err, auth.ErrUnauthorized) {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if err != nil {
		h.Log.Error("events ws authenticate failed", "error", err)
		writeErr(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if user.MustChangePassword {
		writeErr(w, http.StatusForbidden, "password_change_required", "password change required")
		return
	}

	// scope ต่อ connection: admin/servers.view_all เห็นทุก server, admin/nodes.view เห็น node
	seeAllServers := user.IsAdmin || slices.Contains(user.Capabilities, capViewAllServers)
	seeNodes := user.IsAdmin || slices.Contains(user.Capabilities, capNodesView)

	allowed, err := h.accessibleSet(r.Context(), user, seeAllServers)
	if err != nil {
		h.Log.Error("events ws load accessible servers failed", "error", err, "user_id", user.ID)
		writeErr(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		// Origin ถูกเช็คเองด้านบนแล้ว (ตอบ JSON ได้) — ปล่อย gorilla ผ่าน
		CheckOrigin: func(*http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sub := h.Hub.subscribe(seeAllServers, seeNodes, allowed)
	defer h.Hub.unsubscribe(sub)

	done := make(chan struct{})
	go h.readDrain(conn, done)
	h.writeLoop(conn, sub, user, seeAllServers, done)
}

// accessibleSet คำนวณ set ของ server ที่ user เห็นได้ — seeAll ไม่ต้องคำนวณ (nil,
// filter ปล่อยผ่านหมด). ที่เหลือ = server ที่มี server_permissions row (owner ก็มี
// row นี้) ตรงกับ filter ของ handleListServers/ListServersForUser
func (h *WSHandler) accessibleSet(ctx context.Context, user *store.User, seeAll bool) (map[uuid.UUID]bool, error) {
	if seeAll {
		return nil, nil
	}
	ids, err := h.Store.ListAccessibleServerIDs(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	allowed := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		allowed[id] = true
	}
	return allowed, nil
}

func (h *WSHandler) writeLoop(conn *websocket.Conn, sub *subscriber, user *store.User, seeAll bool, done chan struct{}) {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	// refresh allowed set เป็นระยะ เพื่อให้ server ที่เพิ่งถูก grant/สร้างโผล่โดยไม่ต้อง
	// reconnect (seeAll ไม่ต้อง refresh — เห็นหมดอยู่แล้ว)
	refresh := time.NewTicker(refreshInterval)
	defer refresh.Stop()

	for {
		select {
		case data := <-sub.ch:
			conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if conn.WriteMessage(websocket.TextMessage, data) != nil {
				return
			}
		case <-ping.C:
			conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		case <-refresh.C:
			if seeAll {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			ids, err := h.Store.ListAccessibleServerIDs(ctx, user.ID)
			cancel()
			if err != nil {
				h.Log.Warn("events ws refresh accessible servers failed", "error", err, "user_id", user.ID)
				continue
			}
			allowed := make(map[uuid.UUID]bool, len(ids))
			for _, id := range ids {
				allowed[id] = true
			}
			sub.setAllowed(allowed)
		case <-done:
			conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
	}
}

// readDrain อ่านทิ้งทุก message (stream นี้ read-only) — ที่ต้องอ่านเพราะ pong handler
// ต่ออายุ read deadline + ตรวจจับ client ปิด connection แล้วสั่ง writeLoop จบผ่าน done
func (h *WSHandler) readDrain(conn *websocket.Conn, done chan struct{}) {
	defer close(done)
	conn.SetReadLimit(maxReadSize)
	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *WSHandler) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	// ไม่มี Origin = non-browser client — CSRF ผ่าน browser เท่านั้นที่ต้องกัน
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	trimmed := strings.TrimRight(origin, "/")
	for _, allowed := range h.AllowedOrigins {
		if strings.EqualFold(allowed, trimmed) {
			return true
		}
	}
	return false
}

func writeErr(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
