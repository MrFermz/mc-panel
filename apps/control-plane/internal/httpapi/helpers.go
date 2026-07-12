package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mc-panel/control-plane/internal/store"
)

// รูปแบบ error ตาม docs/api.md: {"code": "...", "message": "..."}
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Code: code, Message: message})
}

const maxBodySize = 1 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("request body is empty")
		}
		return fmt.Errorf("invalid JSON body")
	}
	return nil
}

// trustedProxyCount จำนวน proxy hop ที่เชื่อถือได้หน้า control-plane.
// main.go set ครั้งเดียวตอน boot ก่อนรับ request (ไม่มี race) ผ่าน SetTrustedProxyCount
// — clientIP ถูกเรียกจากหลายไฟล์ที่ไม่รับ config โดยตรง จึงเก็บเป็น package var.
var trustedProxyCount = 1

// SetTrustedProxyCount ตั้งค่าจาก config ตอน boot — ต้องเรียกก่อน server เริ่มรับ request
func SetTrustedProxyCount(n int) {
	trustedProxyCount = n
}

func clientIP(r *http.Request) string {
	return clientIPTrusting(r, trustedProxyCount)
}

// clientIPTrusting เลือก client IP โดยเชื่อ X-Forwarded-For เฉพาะ trusted hop:
// client ปลอม entry ซ้ายมือได้ (Caddy append peer จริงต่อท้าย) จึงนับจากขวา
// index (len-trusted) = IP ที่ trusted proxy ตัวในสุดเห็น. ถ้า trusted<=0
// หรือ XFF สั้นกว่า trusted หรือ entry ไม่ใช่ IP ที่ valid → ใช้ RemoteAddr.
func clientIPTrusting(r *http.Request, trusted int) string {
	if trusted > 0 {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) >= trusted {
				candidate := strings.TrimSpace(parts[len(parts)-trusted])
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

func uuidParam(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, name))
}

// ---------- JSON views ตาม object shape ใน docs/api.md ----------

type userView struct {
	ID                 uuid.UUID `json:"id"`
	Email              string    `json:"email"`
	Username           *string   `json:"username"`
	DisplayName        string    `json:"display_name"`
	IsAdmin            bool      `json:"is_admin"`
	IsActive           bool      `json:"is_active"`
	MustChangePassword bool      `json:"must_change_password"`
	Capabilities       []string  `json:"capabilities"`
	CreatedAt          time.Time `json:"created_at"`
}

func toUserView(u *store.User) userView {
	// ส่ง [] ไม่ใช่ null เมื่อว่าง — web ผูก logic กับ array เสมอ
	caps := u.Capabilities
	if caps == nil {
		caps = []string{}
	}
	return userView{
		ID:                 u.ID,
		Email:              u.Email,
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		IsAdmin:            u.IsAdmin,
		IsActive:           u.IsActive,
		MustChangePassword: u.MustChangePassword,
		Capabilities:       caps,
		CreatedAt:          u.CreatedAt,
	}
}

type nodeView struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	AgentVersion    string     `json:"agent_version"`
	OS              string     `json:"os"`
	Arch            string     `json:"arch"`
	CPUPercent      float64    `json:"cpu_percent"`
	MemoryUsedMB    int64      `json:"memory_used_mb"`
	MemoryTotalMB   int64      `json:"memory_total_mb"`
	DiskUsedMB      int64      `json:"disk_used_mb"`
	DiskTotalMB     int64      `json:"disk_total_mb"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

func toNodeView(n *store.Node) nodeView {
	return nodeView{
		ID:              n.ID,
		Name:            n.Name,
		Status:          n.Status,
		AgentVersion:    n.AgentVersion,
		OS:              n.OS,
		Arch:            n.Arch,
		CPUPercent:      n.CPUPercent,
		MemoryUsedMB:    n.MemoryUsedMB,
		MemoryTotalMB:   n.MemoryTotalMB,
		DiskUsedMB:      n.DiskUsedMB,
		DiskTotalMB:     n.DiskTotalMB,
		LastHeartbeatAt: n.LastHeartbeatAt,
		CreatedAt:       n.CreatedAt,
	}
}

type serverStatsView struct {
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryUsedMB  int64     `json:"memory_used_mb"`
	MemoryLimitMB int64     `json:"memory_limit_mb"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type serverView struct {
	ID         uuid.UUID        `json:"id"`
	NodeID     uuid.UUID        `json:"node_id"`
	OwnerID    *uuid.UUID       `json:"owner_id"`
	Name       string           `json:"name"`
	ServerType string           `json:"server_type"`
	MCVersion  string           `json:"mc_version"`
	MemoryMB   int              `json:"memory_mb"`
	HostPort   *int             `json:"host_port"`
	Status     string           `json:"status"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	Stats      *serverStatsView `json:"stats"`
}

func toServerView(s *store.Server, stats *serverStatsView) serverView {
	return serverView{
		ID:         s.ID,
		NodeID:     s.NodeID,
		OwnerID:    s.OwnerID,
		Name:       s.Name,
		ServerType: s.ServerType,
		MCVersion:  s.MCVersion,
		MemoryMB:   s.MemoryMB,
		HostPort:   s.HostPort,
		Status:     s.Status,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
		Stats:      stats,
	}
}

type jobView struct {
	ID               uuid.UUID  `json:"id"`
	ServerID         *uuid.UUID `json:"server_id"`
	Type             string     `json:"type"`
	Status           string     `json:"status"`
	Error            string     `json:"error"`
	RequestedByEmail *string    `json:"requested_by_email"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at"`
}

func toJobView(j *store.Job) jobView {
	return jobView{
		ID:               j.ID,
		ServerID:         j.ServerID,
		Type:             j.Type,
		Status:           j.Status,
		Error:            j.Error,
		RequestedByEmail: j.RequestedByEmail,
		CreatedAt:        j.CreatedAt,
		StartedAt:        j.StartedAt,
		CompletedAt:      j.CompletedAt,
	}
}

type permissionView struct {
	UserID          uuid.UUID `json:"user_id"`
	Email           string    `json:"email"`
	DisplayName     string    `json:"display_name"`
	Role            string    `json:"role"`
	CanConsoleWrite bool      `json:"can_console_write"`
	CanManageFiles  bool      `json:"can_manage_files"`
}

func toPermissionView(p *store.PermissionWithUser) permissionView {
	return permissionView{
		UserID:          p.UserID,
		Email:           p.Email,
		DisplayName:     p.DisplayName,
		Role:            p.Role,
		CanConsoleWrite: p.CanConsoleWrite,
		CanManageFiles:  p.CanManageFiles,
	}
}
