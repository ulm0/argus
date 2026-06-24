package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

// RequireLocalHost mitigates DNS-rebinding: a malicious public site can rebind
// its domain to the device's LAN IP and drive the API from a victim's browser.
// Such requests arrive with a public-domain Host header (always dotted), so we
// only let /api/ requests through when the Host is a local identity — an IP
// literal, localhost, a bare single-label name, an mDNS *.local / *.lan name —
// or an operator-configured host. Root paths (captive-portal probes, which the
// OS hits with public Host headers on purpose) and static assets are not gated.
func RequireLocalHost(allowed []string) func(http.Handler) http.Handler {
	allowSet := make(map[string]bool, len(allowed))
	for _, h := range allowed {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			allowSet[h] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") || hostAllowed(r.Host, allowSet) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "host not allowed"})
		})
	}
}

func hostAllowed(hostport string, allowSet map[string]bool) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(strings.TrimSuffix(host, "."), "[]"))

	switch {
	case host == "" || host == "localhost":
		return true
	case net.ParseIP(host) != nil: // any IPv4/IPv6 literal
		return true
	case !strings.Contains(host, "."): // bare single-label local name
		return true
	case strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan"):
		return true
	default:
		return allowSet[host]
	}
}
