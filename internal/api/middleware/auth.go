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
			// Non-API routes (static UI shell, captive-portal probes) are always
			// served so the login page can load.
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			// With auth disabled the API is open, but a SameSite session cookie no
			// longer guards it, so state-changing requests still need CSRF cover:
			// reject cross-site writes the browser has flagged via Sec-Fetch-Site.
			if !cfg.AuthEnabled() {
				if isCrossSiteWrite(r) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					json.NewEncoder(w).Encode(map[string]string{"error": "cross-site request blocked"})
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			// Auth enabled: the auth endpoints stay reachable; everything else
			// needs a valid session cookie.
			if open[r.URL.Path] {
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

// isCrossSiteWrite reports whether r is a state-changing request that the browser
// has flagged as coming from another site. Browsers set Sec-Fetch-Site on every
// request and forbid page scripts from overriding it, so a cross-site (or
// cross-origin same-site) value is a reliable CSRF signal. A missing header —
// non-browser clients, or browsers predating Fetch Metadata — is allowed, the
// known residual gap of this approach; the device UI always calls its own origin,
// so legitimate requests are same-origin and never blocked.
func isCrossSiteWrite(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site", "same-site":
		return true
	default:
		return false
	}
}
