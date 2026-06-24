package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ulm0/argus/internal/auth"
)

// authConfig is the minimal config surface RequireAuth needs, kept as an
// interface so the middleware package doesn't import the config package.
type authConfig interface {
	AuthEnabled() bool
}

// RequireAuth gates every /api/ route behind a valid session cookie, except the
// auth endpoints themselves (login/logout/status) which must stay reachable so
// a logged-out client can authenticate. Non-API routes (the static UI shell and
// captive-portal probes) are always served so the login page can load.
func RequireAuth(cfg authConfig, secretKey, cookieName string) func(http.Handler) http.Handler {
	open := map[string]bool{
		"/api/login":       true,
		"/api/logout":      true,
		"/api/auth/status": true,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.AuthEnabled() || !strings.HasPrefix(r.URL.Path, "/api/") || open[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			if c, err := r.Cookie(cookieName); err == nil {
				if _, ok := auth.VerifySession(secretKey, c.Value, time.Now()); ok {
					next.ServeHTTP(w, r)
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
		})
	}
}
