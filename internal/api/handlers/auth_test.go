package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ulm0/argus/internal/api/middleware"
	"github.com/ulm0/argus/internal/config"
)

func authTestConfig() *config.Config {
	enabled := true
	return &config.Config{
		Web: config.WebConfig{
			SecretKey:    "test-secret-key",
			AuthEnabled:  &enabled,
			AuthUsername: config.DefaultAuthUsername,
			AuthPassword: config.DefaultAuthPassword,
		},
	}
}

func TestLoginAndGatedAccess(t *testing.T) {
	cfg := authTestConfig()
	h := NewAuthHandler(cfg)

	// Wrong password is rejected.
	bad := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"admin","password":"nope"}`))
	badRec := httptest.NewRecorder()
	h.Login(badRec, bad)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d, want 401", badRec.Code)
	}

	// Correct credentials issue a session cookie.
	ok := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"admin","password":"argus"}`))
	okRec := httptest.NewRecorder()
	h.Login(okRec, ok)
	if okRec.Code != http.StatusOK {
		t.Fatalf("good login status = %d, want 200", okRec.Code)
	}
	cookies := okRec.Result().Cookies()
	var session *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookie {
			session = c
		}
	}
	if session == nil || session.Value == "" {
		t.Fatal("login did not set a session cookie")
	}
	if !session.HttpOnly || session.SameSite != http.SameSiteStrictMode {
		t.Error("session cookie should be HttpOnly + SameSite=Strict")
	}

	// The middleware blocks a protected route without the cookie...
	gate := middleware.RequireAuth(cfg, cfg.Web.SecretKey, SessionCookie)
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; w.WriteHeader(200) })

	noCookie := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	gate(next).ServeHTTP(rec, noCookie)
	if rec.Code != http.StatusUnauthorized || reached {
		t.Fatalf("protected route without cookie: code=%d reached=%v, want 401/false", rec.Code, reached)
	}

	// ...and allows it with the cookie.
	reached = false
	withCookie := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	withCookie.AddCookie(session)
	rec = httptest.NewRecorder()
	gate(next).ServeHTTP(rec, withCookie)
	if rec.Code != http.StatusOK || !reached {
		t.Fatalf("protected route with cookie: code=%d reached=%v, want 200/true", rec.Code, reached)
	}

	// The login endpoint itself is always reachable.
	reached = false
	openReq := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	rec = httptest.NewRecorder()
	gate(next).ServeHTTP(rec, openReq)
	if !reached {
		t.Error("/api/login should bypass the auth gate")
	}
}

func TestAuthDisabledBypassesGate(t *testing.T) {
	disabled := false
	cfg := &config.Config{Web: config.WebConfig{SecretKey: "k", AuthEnabled: &disabled}}
	gate := middleware.RequireAuth(cfg, cfg.Web.SecretKey, SessionCookie)

	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true })
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	gate(next).ServeHTTP(httptest.NewRecorder(), req)
	if !reached {
		t.Error("auth disabled should pass all routes through")
	}
}
