package httpapi

import (
	"errors"
	"net/http"

	"github.com/mc-panel/control-plane/internal/auth"
)

func (a *API) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := a.auth.Authenticate(r.Context(), r)
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		if err != nil {
			a.log.Error("authenticate failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}

		if user.MustChangePassword && !passwordChangeExempt(r) {
			writeError(w, http.StatusForbidden, "password_change_required",
				"you must change your password before continuing")
			return
		}

		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), user)))
	})
}

// endpoint ที่ยังใช้ได้ระหว่างถูกบังคับเปลี่ยน password ตาม docs/api.md
func passwordChangeExempt(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/auth/change-password", "/api/auth/me", "/api/auth/logout":
		return true
	}
	return false
}

func (a *API) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user := auth.UserFrom(r.Context()); user == nil || !user.IsAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireCap: ผ่านเมื่อ is_admin หรือ user มี capability key นั้น (ดู docs/api.md)
func (a *API) requireCap(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFrom(r.Context())
			if user == nil || !hasCapability(user, key) {
				writeError(w, http.StatusForbidden, "forbidden", "insufficient capability")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
