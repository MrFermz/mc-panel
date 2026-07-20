package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// deleted_at ไม่อยู่ใน userCols — ทุก query filter `deleted_at IS NULL` อยู่แล้ว
// จึงไม่มีทาง surface แถวที่ถูกลบ (User.DeletedAt คงเป็น nil เสมอในเส้นทางปกติ)
// คอลัมน์ avatar (bytes) จงใจไม่อยู่ในนี้ — หนักเกินจะติดมากับทุก query, อ่านผ่าน GetUserAvatar
const userCols = `id, username, display_name, avatar_updated_at, password_hash, is_admin, is_active,
	must_change_password, token_version, capabilities, last_login_at, created_at, updated_at`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarUpdatedAt,
		&u.PasswordHash, &u.IsAdmin,
		&u.IsActive, &u.MustChangePassword, &u.TokenVersion, &u.Capabilities, &u.LastLoginAt,
		&u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userCols+` FROM users WHERE id = $1 AND deleted_at IS NULL`, id))
}

// GetUserByUsername ใช้ตอน login — username เป็น identifier เดียวของระบบ (case-insensitive)
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userCols+` FROM users
		 WHERE lower(username) = lower($1) AND deleted_at IS NULL`, username))
}

// UserFilter สำหรับ ListUsers — ค่าว่างของแต่ละ field = ไม่ filter ด้วย field นั้น
type UserFilter struct {
	Search string
	Role   string // "admin" | "user"
	Status string // "active" | "inactive"
}

func (s *Store) ListUsers(ctx context.Context, f UserFilter) ([]*User, error) {
	where := []string{"deleted_at IS NULL"}
	var args []any

	if search := strings.TrimSpace(f.Search); search != "" {
		args = append(args, search)
		p := "$" + strconv.Itoa(len(args))
		where = append(where,
			"(username ILIKE '%'||"+p+"||'%' OR display_name ILIKE '%'||"+p+"||'%')")
	}
	switch f.Role {
	case "admin":
		where = append(where, "is_admin = true")
	case "user":
		where = append(where, "is_admin = false")
	}
	switch f.Status {
	case "active":
		where = append(where, "is_active = true")
	case "inactive":
		where = append(where, "is_active = false")
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+userCols+` FROM users WHERE `+strings.Join(where, " AND ")+` ORDER BY created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// DirectoryUser = ชุด field เบาสำหรับ access picker (ไม่ leak hash/สิทธิ์/สถานะ)
type DirectoryUser struct {
	ID              uuid.UUID
	Username        string
	DisplayName     string
	AvatarUpdatedAt *time.Time
}

// ListUserDirectory คืน user ที่ active + ยังไม่ถูกลบ สำหรับให้ owner เลือก collaborator
// (ไม่ต้องมี users.view — เป็นข้อมูลเบา ๆ ที่ทุกคนใน panel เห็นได้)
func (s *Store) ListUserDirectory(ctx context.Context) ([]DirectoryUser, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, username, display_name, avatar_updated_at
		FROM users
		WHERE deleted_at IS NULL AND is_active = true
		ORDER BY coalesce(nullif(display_name, ''), username), created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]DirectoryUser, 0)
	for rows.Next() {
		var d DirectoryUser
		if err := rows.Scan(&d.ID, &d.Username, &d.DisplayName, &d.AvatarUpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string, isAdmin bool, capabilities []string) (*User, error) {
	// pgx encode nil slice เป็น SQL NULL ไม่ใช่ '{}' — ชน NOT NULL ของ capabilities
	// (โผล่ตอน seed admin บน DB เปล่า) จึงต้อง coalesce เป็น empty slice ก่อนเสมอ
	if capabilities == nil {
		capabilities = []string{}
	}
	return scanUser(s.pool.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, is_admin, capabilities, must_change_password)
		VALUES ($1, $2, $3, $4, TRUE)
		RETURNING `+userCols,
		username, passwordHash, isAdmin, capabilities))
}

// UpdateUser: capabilities = nil หมายถึงไม่เปลี่ยน (ต่างจาก []string{} = ล้างทิ้ง)
// ส่งเป็น NULL ให้ COALESCE คงค่าเดิม ; empty array ไม่ใช่ NULL จึงล้างได้จริง
func (s *Store) UpdateUser(ctx context.Context, id uuid.UUID, isAdmin, isActive *bool, capabilities *[]string) (*User, error) {
	var capsArg any
	if capabilities != nil {
		capsArg = *capabilities
	}
	return scanUser(s.pool.QueryRow(ctx, `
		UPDATE users SET
			is_admin     = COALESCE($2, is_admin),
			is_active    = COALESCE($3, is_active),
			capabilities = COALESCE($4, capabilities),
			updated_at   = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userCols,
		id, isAdmin, isActive, capsArg))
}

// UpdateUserProfile แก้เฉพาะข้อมูลที่เจ้าของบัญชีตั้งเองได้ (ไม่แตะสิทธิ์/สถานะ)
func (s *Store) UpdateUserProfile(ctx context.Context, id uuid.UUID, displayName string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		UPDATE users SET display_name = $2, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userCols, id, displayName))
}

// SetUserAvatar เขียนรูปใหม่ทับของเดิม — avatar_updated_at เป็นทั้ง cache-buster และ ETag
func (s *Store) SetUserAvatar(ctx context.Context, id uuid.UUID, data []byte, mime string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		UPDATE users SET avatar = $2, avatar_mime = $3, avatar_updated_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userCols, id, data, mime))
}

func (s *Store) ClearUserAvatar(ctx context.Context, id uuid.UUID) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		UPDATE users SET avatar = NULL, avatar_mime = '', avatar_updated_at = NULL, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userCols, id))
}

// GetUserAvatar อ่าน bytes ของรูป — ErrNotFound เมื่อ user ไม่มีอยู่ *หรือ* ยังไม่ตั้งรูป
// (ผู้เรียกไม่ต้องแยกสองเคสนี้ ทั้งคู่ตอบ 404 เหมือนกัน)
func (s *Store) GetUserAvatar(ctx context.Context, id uuid.UUID) (data []byte, mime string, updatedAt time.Time, err error) {
	var at *time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT avatar, avatar_mime, avatar_updated_at FROM users
		WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&data, &mime, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, "", time.Time{}, err
	}
	if len(data) == 0 || at == nil {
		return nil, "", time.Time{}, ErrNotFound
	}
	return data, mime, *at, nil
}

// SetUserPassword bump token_version เสมอ -> JWT เก่าทุกใบใช้ไม่ได้ทันที
func (s *Store) SetUserPassword(ctx context.Context, id uuid.UUID, passwordHash string, mustChange bool) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		UPDATE users SET
			password_hash        = $2,
			must_change_password = $3,
			token_version        = token_version + 1,
			updated_at           = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userCols,
		id, passwordHash, mustChange))
}

func (s *Store) TouchLastLogin(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET last_login_at = now() WHERE id = $1`, id)
	return err
}

// SoftDeleteUser: mark ลบ + ปิด active + bump token_version (session เก่าตายหมด)
// และล้าง server_permissions ทิ้ง (ไม่อยาก grant สิทธิ์ค้างถ้ามี user username ซ้ำในอนาคต)
// ทำใน transaction เดียว
func (s *Store) SoftDeleteUser(ctx context.Context, id uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE users SET
			deleted_at    = now(),
			is_active     = false,
			token_version = token_version + 1,
			updated_at    = now()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	if _, err := tx.Exec(ctx, `DELETE FROM server_permissions WHERE user_id = $1`, id); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
