package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ulm0/argus/internal/auth"
	"github.com/ulm0/argus/internal/config"
)

// requestIsHTTPS reports whether the request reached us over TLS, either
// directly or via a TLS-terminating reverse proxy. Argus serves plain HTTP on
// the local network/AP by default, so the session cookie's Secure attribute is
// set conditionally: forcing Secure unconditionally would stop the browser from
// ever sending the cookie over HTTP and break login on the common deployment.
func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// SessionCookie is the name of the cookie carrying the signed session token.
const SessionCookie = "argus_session"

type AuthHandler struct {
	cfg *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

// Status reports whether auth is enabled, whether the caller is authenticated,
// and whether the default credentials are still in use. Always reachable so the
// UI can decide whether to show the login screen.
func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       h.cfg.AuthEnabled(),
		"authenticated": !h.cfg.AuthEnabled() || h.isAuthenticated(r),
		"using_default": h.cfg.UsingDefaultAuth(),
	})
}

// Login validates credentials and, on success, sets a signed session cookie.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Constant-time comparison of both fields to avoid leaking which one is wrong.
	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(h.cfg.Web.AuthUsername)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.cfg.Web.AuthPassword)) == 1
	if !userOK || !passOK {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	token := auth.SignSession(h.cfg.Web.SecretKey, h.cfg.Web.AuthUsername, time.Now())
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(auth.SessionTTL / time.Second),
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "using_default": h.cfg.UsingDefaultAuth()})
}

// Logout clears the session cookie.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) isAuthenticated(r *http.Request) bool {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return false
	}
	_, ok := auth.VerifySession(h.cfg.Web.SecretKey, c.Value, time.Now())
	return ok
}
