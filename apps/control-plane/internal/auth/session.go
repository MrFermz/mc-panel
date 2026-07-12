package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/mc-panel/control-plane/internal/store"
)

const (
	CookieName = "mc_session"
	sessionTTL = 24 * time.Hour
)

var ErrUnauthorized = errors.New("auth: unauthorized")

type Manager struct {
	st           *store.Store
	rdb          *redis.Client
	secret       []byte
	cookieSecure bool
	log          *slog.Logger
}

func NewManager(st *store.Store, rdb *redis.Client, jwtSecret string, cookieSecure bool, log *slog.Logger) *Manager {
	return &Manager{
		st:           st,
		rdb:          rdb,
		secret:       []byte(jwtSecret),
		cookieSecure: cookieSecure,
		log:          log,
	}
}

type sessionClaims struct {
	Ver int `json:"ver"`
	jwt.RegisteredClaims
}

func (m *Manager) IssueSession(u *store.User) (string, error) {
	now := time.Now()
	claims := sessionClaims{
		Ver: u.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(sessionTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *Manager) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL / time.Second),
		HttpOnly: true,
		Secure:   m.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// Authenticate โหลด user จาก DB ทุก request ตาม docs/api.md — เพื่อให้ is_active
// และ token_version มีผลทันที ไม่ต้องรอ JWT หมดอายุ
func (m *Manager) Authenticate(ctx context.Context, r *http.Request) (*store.User, error) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return nil, ErrUnauthorized
	}

	var claims sessionClaims
	_, err = jwt.ParseWithClaims(c.Value, &claims, func(t *jwt.Token) (any, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, ErrUnauthorized
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, ErrUnauthorized
	}

	u, err := m.st.GetUserByID(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("load session user: %w", err)
	}
	if !u.IsActive || u.TokenVersion != claims.Ver {
		return nil, ErrUnauthorized
	}
	return u, nil
}

const (
	loginRateLimitMax    = 10
	loginRateLimitWindow = time.Minute
)

// AllowLogin จำกัด 10 ครั้ง/นาที/IP — redis ล่มให้ fail-open (เลือก availability
// มากกว่า strict rate limit) พร้อม warn log
func (m *Manager) AllowLogin(ctx context.Context, ip string) bool {
	key := "mcpanel:login_rl:" + ip
	n, err := m.rdb.Incr(ctx, key).Result()
	if err != nil {
		m.log.Warn("login rate limit check failed, failing open", "error", err, "ip", ip)
		return true
	}
	if n == 1 {
		if err := m.rdb.Expire(ctx, key, loginRateLimitWindow).Err(); err != nil {
			m.log.Warn("login rate limit expire failed", "error", err, "ip", ip)
		}
	}
	return n <= loginRateLimitMax
}
