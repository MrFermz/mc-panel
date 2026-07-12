package store

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// deleted_at ไม่อยู่ใน userCols — ทุก query filter `deleted_at IS NULL` อยู่แล้ว
// จึงไม่มีทาง surface แถวที่ถูกลบ (User.DeletedAt คงเป็น nil เสมอในเส้นทางปกติ)
const userCols = `id, email, username, password_hash, display_name, is_admin, is_active,
	must_change_password, token_version, capabilities, last_login_at, created_at, updated_at`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.DisplayName, &u.IsAdmin,
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

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userCols+` FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL`, email))
}

// GetUserByEmailOrUsername ใช้ตอน login — identifier เป็น email หรือ username ก็ได้
func (s *Store) GetUserByEmailOrUsername(ctx context.Context, identifier string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userCols+` FROM users
		 WHERE (lower(email) = lower($1) OR lower(username) = lower($1)) AND deleted_at IS NULL`, identifier))
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
			"(email ILIKE '%'||"+p+"||'%' OR coalesce(username,'') ILIKE '%'||"+p+"||'%' OR display_name ILIKE '%'||"+p+"||'%')")
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

func (s *Store) CreateUser(ctx context.Context, email string, username *string, passwordHash, displayName string, isAdmin bool, capabilities []string) (*User, error) {
	// pgx encode nil slice เป็น SQL NULL ไม่ใช่ '{}' — ชน NOT NULL ของ capabilities
	// (โผล่ตอน seed admin บน DB เปล่า) จึงต้อง coalesce เป็น empty slice ก่อนเสมอ
	if capabilities == nil {
		capabilities = []string{}
	}
	// username nil -> NULL (partial unique index มองข้ามแถว username IS NULL)
	return scanUser(s.pool.QueryRow(ctx, `
		INSERT INTO users (email, username, password_hash, display_name, is_admin, capabilities, must_change_password)
		VALUES ($1, $2, $3, $4, $5, $6, TRUE)
		RETURNING `+userCols,
		email, username, passwordHash, displayName, isAdmin, capabilities))
}

// UpdateUser: capabilities = nil หมายถึงไม่เปลี่ยน (ต่างจาก []string{} = ล้างทิ้ง)
// ส่งเป็น NULL ให้ COALESCE คงค่าเดิม ; empty array ไม่ใช่ NULL จึงล้างได้จริง
func (s *Store) UpdateUser(ctx context.Context, id uuid.UUID, displayName *string, isAdmin, isActive *bool, capabilities *[]string) (*User, error) {
	var capsArg any
	if capabilities != nil {
		capsArg = *capabilities
	}
	return scanUser(s.pool.QueryRow(ctx, `
		UPDATE users SET
			display_name = COALESCE($2, display_name),
			is_admin     = COALESCE($3, is_admin),
			is_active    = COALESCE($4, is_active),
			capabilities = COALESCE($5, capabilities),
			updated_at   = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userCols,
		id, displayName, isAdmin, isActive, capsArg))
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
// และล้าง server_permissions ทิ้ง (ไม่อยาก grant สิทธิ์ค้างถ้ามี user email ซ้ำในอนาคต)
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
