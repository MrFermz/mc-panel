package console

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

// capability key ที่กำหนดสิทธิ์คอนโซล — สะกดให้ตรง httpapi.capabilities
// (นิยามซ้ำที่นี่แทน import httpapi เพื่อกัน import cycle)
const (
	capConsoleView  = "console.view"
	capConsoleWrite = "console.write"
)

// InputSender ตัดวงจร import กับ agenthub — agenthub เป็นคน implement
type InputSender interface {
	SendConsoleInput(nodeID, serverID uuid.UUID, command string) error
}

type WSHandler struct {
	Auth           *auth.Manager
	Store          *store.Store
	Rings          *Registry
	Hub            *Hub
	Sender         InputSender
	AllowedOrigins []string
	// TrustedProxyCount จำนวน proxy hop ที่เชื่อถือได้หน้า control-plane —
	// เลือก entry ที่ถูกต้องจาก X-Forwarded-For (นับจากขวา) กัน client ปลอม IP
	TrustedProxyCount int
	Log               *slog.Logger
}

const (
	writeTimeout = 10 * time.Second
	pongTimeout  = 90 * time.Second
	pingInterval = 30 * time.Second
	maxInputSize = 4096
)

type inboundMessage struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func (h *WSHandler) HandleConsole(w http.ResponseWriter, r *http.Request) {
	// เช็ค Origin เองก่อน upgrade เพื่อให้ตอบ error เป็น JSON ตาม docs/api.md
	// (CheckOrigin ของ gorilla ตอบ plain text)
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
		h.Log.Error("ws authenticate failed", "error", err)
		writeErr(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if user.MustChangePassword {
		writeErr(w, http.StatusForbidden, "password_change_required", "password change required")
		return
	}

	serverID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "server_not_found", "server not found")
		return
	}
	srv, err := h.Store.GetServerByID(r.Context(), serverID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "server_not_found", "server not found")
		return
	}
	if err != nil {
		h.Log.Error("ws load server failed", "error", err, "server_id", serverID)
		writeErr(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if !user.IsAdmin {
		// global capability ก่อน แล้วค่อยสิทธิ์ต่อ server (ต้องผ่านทั้งคู่)
		if !slices.Contains(user.Capabilities, capConsoleView) {
			writeErr(w, http.StatusForbidden, "forbidden", "insufficient capability")
			return
		}
		perm, err := h.Store.GetPermission(r.Context(), user.ID, srv.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusForbidden, "forbidden", "no access to this server")
				return
			}
			h.Log.Error("ws load permission failed", "error", err)
			writeErr(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		// grant ต่อ server ต้องมี console.view ด้วย (owner ได้ทุก server-scoped cap โดยปริยาย)
		if perm.Role != "owner" && !slices.Contains(perm.Capabilities, capConsoleView) {
			writeErr(w, http.StatusForbidden, "forbidden", "no access to this server")
			return
		}
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin:     func(*http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// subscribe + snapshot ต้อง atomic ภายใต้ lock เดียวกัน — ไม่งั้นบรรทัดที่
	// append ในช่องว่างระหว่างสองจังหวะจะโผล่ทั้งใน history และ event stream (ซ้ำ)
	history, events := h.Hub.SubscribeWithHistory(srv.ID, h.Rings.Get(srv.ID))
	defer h.Hub.Unsubscribe(srv.ID, events)

	done := make(chan struct{})
	go h.writeLoop(conn, history, events, done)

	h.readLoop(r.Context(), conn, user, srv, events, h.clientIP(r))
	close(done)
}

func (h *WSHandler) writeLoop(conn *websocket.Conn, history []string, events chan Event, done chan struct{}) {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()

	write := func(v any) bool {
		conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		return conn.WriteJSON(v) == nil
	}

	// ส่ง history เป็นก้อนแรกเสมอ แล้วค่อยตาม realtime ตาม docs/api.md
	if !write(Event{Type: "lines", Lines: history}) {
		return
	}
	for {
		select {
		case ev := <-events:
			if !write(ev) {
				return
			}
		case <-ping.C:
			conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		case <-done:
			conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
	}
}

func (h *WSHandler) readLoop(ctx context.Context, conn *websocket.Conn, user *store.User, srv *store.Server, events chan Event, ip string) {
	conn.SetReadLimit(maxInputSize)
	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg inboundMessage
		if err := json.Unmarshal(data, &msg); err != nil || msg.Type != "input" {
			continue
		}
		command := strings.TrimSpace(msg.Command)
		if command == "" {
			continue
		}
		h.handleInput(ctx, user, srv, command, events, ip)
	}
}

func (h *WSHandler) handleInput(ctx context.Context, user *store.User, srv *store.Server, command string, events chan Event, ip string) {
	sendErr := func(code, message string) {
		select {
		case events <- Event{Type: "error", Code: code, Message: message}:
		default:
		}
	}

	// เช็คสิทธิ์เขียนใหม่จาก DB ทุก message — สิทธิ์ที่ถูกถอดระหว่าง session
	// ต้องมีผลทันที ไม่ใช่รอเปิด connection ใหม่
	if !user.IsAdmin {
		// โหลด capability ใหม่ด้วย — ค่าใน user เป็น snapshot ตอนเปิด connection
		fresh, err := h.Store.GetUserByID(ctx, user.ID)
		if err != nil {
			h.Log.Error("ws reload user failed", "error", err)
			sendErr("internal_error", "internal error")
			return
		}
		if !fresh.IsAdmin && !slices.Contains(fresh.Capabilities, capConsoleWrite) {
			sendErr("forbidden", "insufficient capability")
			return
		}
		perm, err := h.Store.GetPermission(ctx, user.ID, srv.ID)
		if errors.Is(err, store.ErrNotFound) {
			sendErr("forbidden", "no access to this server")
			return
		}
		if err != nil {
			h.Log.Error("ws reload permission failed", "error", err)
			sendErr("internal_error", "internal error")
			return
		}
		if perm.Role != "owner" && !slices.Contains(perm.Capabilities, capConsoleWrite) {
			sendErr("forbidden", "console write not allowed")
			return
		}
	}

	if err := h.Store.InsertAudit(ctx, &user.ID, &srv.ID, "console_command",
		map[string]any{"command": command}, ip); err != nil {
		h.Log.Error("audit console_command failed", "error", err)
	}

	if err := h.Sender.SendConsoleInput(srv.NodeID, srv.ID, command); err != nil {
		sendErr("node_offline", "node agent is not connected")
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

// clientIP เลือก client IP โดยเชื่อ X-Forwarded-For เฉพาะ trusted hop:
// client ปลอม entry ซ้ายมือได้ (Caddy append peer จริงต่อท้าย) จึงนับจากขวา
// index (len-trusted) = IP ที่ trusted proxy ตัวในสุดเห็น. ถ้า trusted<=0
// หรือ XFF สั้นกว่า trusted หรือ entry ไม่ใช่ IP ที่ valid → ใช้ RemoteAddr.
func (h *WSHandler) clientIP(r *http.Request) string {
	if h.TrustedProxyCount > 0 {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) >= h.TrustedProxyCount {
				candidate := strings.TrimSpace(parts[len(parts)-h.TrustedProxyCount])
				if net.ParseIP(candidate) != nil {
					return candidate
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
