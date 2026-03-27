package middleware

import (
	"net/http"
	"time"

	"github.com/ulm0/argus/internal/logger"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher by delegating to the underlying ResponseWriter
// when it supports flushing. This is required for SSE streaming endpoints.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Logging is HTTP middleware that logs each request's method, path, status and duration.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.L.WithField("method", r.Method).
			WithField("path", r.URL.Path).
			WithField("status", rec.status).
			WithField("duration", time.Since(start).String()).
			WithField("remote", r.RemoteAddr).
			Info("http")
	})
}
