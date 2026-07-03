package middleware

import (
	"net"
	"net/http"
	"strings"
)

// RealIP sets r.RemoteAddr from X-Real-IP or X-Forwarded-For headers
// when the request arrives through a reverse proxy.
func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rip := r.Header.Get("X-Real-IP"); rip != "" && net.ParseIP(rip) != nil {
			r.RemoteAddr = rip
		} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				xff = strings.TrimSpace(xff[:i])
			}
			// Only override with a valid client IP; net.ParseIP rejects empty
			// values (leading-comma XFF) and non-IP strings that would
			// otherwise be written verbatim to the access log.
			if net.ParseIP(xff) != nil {
				r.RemoteAddr = xff
			}
		}

		if _, _, err := net.SplitHostPort(r.RemoteAddr); err != nil {
			r.RemoteAddr = net.JoinHostPort(r.RemoteAddr, "0")
		}

		next.ServeHTTP(w, r)
	})
}
