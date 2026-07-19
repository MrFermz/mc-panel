// Package store คือ data access layer ทั้งหมดของ control-plane — SQL ตรง ๆ ผ่าน pgx
package store

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("store: not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type User struct {
	ID       uuid.UUID
	Email    string
	Username *string
	// DisplayName เจ้าของบัญชีตั้งเอง — ว่างได้ (ตกไปใช้ username/email ตอนแสดงผล)
	DisplayName string
	// AvatarUpdatedAt = nil แปลว่ายังไม่มีรูป (bytes ไม่เคยถูกโหลดมากับ User —
	// อ่านแยกผ่าน GetUserAvatar เพราะหนักเกินจะติดมาทุก query)
	AvatarUpdatedAt    *time.Time
	PasswordHash       string
	IsAdmin            bool
	IsActive           bool
	MustChangePassword bool
	TokenVersion       int
	Capabilities       []string
	LastLoginAt        *time.Time
	DeletedAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Node struct {
	ID              uuid.UUID
	Name            string
	AgentTokenHash  string
	Status          string
	AgentVersion    string
	OS              string
	Arch            string
	CPUPercent      float64
	MemoryUsedMB    int64
	MemoryTotalMB   int64
	DiskUsedMB      int64
	DiskTotalMB     int64
	NetRxBps        float64
	NetTxBps        float64
	LastHeartbeatAt *time.Time
	CreatedAt       time.Time
}

type Server struct {
	ID         uuid.UUID
	NodeID     uuid.UUID
	OwnerID    *uuid.UUID
	Name       string
	ServerType string
	MCVersion  string
	MemoryMB   int
	HostPort   *int
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Permission struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	ServerID        uuid.UUID
	Role            string
	CanConsoleWrite bool
	CanManageFiles  bool
	CreatedAt       time.Time
}

type PermissionWithUser struct {
	Permission
	Email           string
	Username        *string
	DisplayName     string
	AvatarUpdatedAt *time.Time
}

type Job struct {
	ID          uuid.UUID
	ServerID    *uuid.UUID
	NodeID      *uuid.UUID
	Type        string
	Status      string
	Payload     []byte
	Error       string
	RequestedBy *uuid.UUID
	// RequestedByEmail/Name/Username มาจาก LEFT JOIN users — null เมื่อ requested_by ถูก SET NULL
	// (user ถูกลบ) หรือ job เพิ่งสร้างจาก path ที่ไม่ได้ join
	RequestedByEmail    *string
	RequestedByName     *string
	RequestedByUsername *string
	CreatedAt           time.Time
	StartedAt           *time.Time
	CompletedAt         *time.Time
}
