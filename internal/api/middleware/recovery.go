package middleware

import (
	"encoding/json"
	"net/http"
	"runtime/debug"

	"github.com/ulm0/argus/internal/logger"
)

// PanicRecovery catches panics in HTTP handlers and returns a 500 JSON response.
func PanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.L.WithField("panic", rec).
					WithField("stack", string(debug.Stack())).
					Error("http handler panic")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "internal server error",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
