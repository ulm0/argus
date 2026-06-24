package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireLocalHost(t *testing.T) {
	mw := RequireLocalHost([]string{"argus.example.com"})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := mw(next)

	cases := []struct {
		name string
		path string
		host string
		want int
	}{
		{"ip literal", "/api/status", "192.168.4.1", http.StatusOK},
		{"ip with port", "/api/status", "192.168.4.1:80", http.StatusOK},
		{"localhost", "/api/status", "localhost:80", http.StatusOK},
		{"mdns .local", "/api/status", "argus.local", http.StatusOK},
		{"bare name", "/api/status", "argus", http.StatusOK},
		{"configured fqdn", "/api/status", "argus.example.com", http.StatusOK},
		{"rebinding public domain", "/api/status", "evil.com", http.StatusForbidden},
		{"unconfigured fqdn", "/api/status", "attacker.example.org", http.StatusForbidden},
		// Non-API paths (captive-portal probes, static assets) are never gated.
		{"captive probe public host", "/generate_204", "connectivitycheck.gstatic.com", http.StatusOK},
		{"static asset public host", "/index.html", "evil.com", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			req.Host = c.host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("host %q path %q: code = %d, want %d", c.host, c.path, rec.Code, c.want)
			}
		})
	}
}
