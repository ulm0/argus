package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
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

// Brute-force throttling for Login: after loginFailThreshold consecutive
// failures from the same source IP, responses are delayed with exponential
// backoff. State is in-memory and per source; it clears on a successful login
// or once loginFailWindow elapses without further failures.
const (
	loginFailThreshold = 5
	loginFailWindow    = 15 * time.Minute
	loginMaxBackoff    = 30 * time.Second
)

type loginFailure struct {
	count int
	last  time.Time
}

type AuthHandler struct {
	cfg *config.Config

	mu       sync.Mutex
	failures map[string]*loginFailure
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg, failures: make(map[string]*loginFailure)}
}

// Status reports whether auth is enabled, whether the caller is authenticated,
// and whether the default credentials are still in use. Always reachable so the
// UI can decide whether to show the login screen.
func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	authenticated := !h.cfg.AuthEnabled() || h.isAuthenticated(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       h.cfg.AuthEnabled(),
		"authenticated": authenticated,
		// Only reveal whether the shipped defaults are live to authenticated
		// callers; an open status endpoint must not advertise them.
		"using_default": authenticated && h.cfg.UsingDefaultAuth(),
	})
}

// Login validates credentials and, on success, sets a signed session cookie.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if d := h.loginBackoff(ip); d > 0 {
		time.Sleep(d)
	}

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
		h.recordLoginFailure(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	h.clearLoginFailures(ip)

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

// clientIP returns the source host of the request, stripping any port.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// loginBackoff returns how long to delay a login attempt from ip given its
// recent failure count, or 0 if no throttling applies.
func (h *AuthHandler) loginBackoff(ip string) time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.failures[ip]
	if f == nil || time.Since(f.last) > loginFailWindow || f.count < loginFailThreshold {
		return 0
	}
	shift := f.count - loginFailThreshold
	if shift > 5 {
		shift = 5
	}
	d := time.Second << shift
	if d > loginMaxBackoff {
		d = loginMaxBackoff
	}
	return d
}

// recordLoginFailure increments the failure counter for ip and prunes entries
// that have aged out so the map stays bounded.
func (h *AuthHandler) recordLoginFailure(ip string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.failures[ip]
	if f == nil || time.Since(f.last) > loginFailWindow {
		f = &loginFailure{}
		h.failures[ip] = f
	}
	f.count++
	f.last = time.Now()
	for k, v := range h.failures {
		if time.Since(v.last) > loginFailWindow {
			delete(h.failures, k)
		}
	}
}

// clearLoginFailures resets throttling state after a successful login.
func (h *AuthHandler) clearLoginFailures(ip string) {
	h.mu.Lock()
	delete(h.failures, ip)
	h.mu.Unlock()
}
