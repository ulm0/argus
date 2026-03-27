package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ulm0/argus/internal/config"
)

// LogsHandler streams systemd journal entries for the argus service.
type LogsHandler struct {
	cfg *config.Config
}

func NewLogsHandler(cfg *config.Config) *LogsHandler {
	return &LogsHandler{cfg: cfg}
}

// logLine is the JSON shape of each SSE data payload.
type logLine struct {
	Timestamp string `json:"timestamp"`
	Priority  string `json:"priority"`
	Message   string `json:"message"`
}

// Stream handles GET /api/logs as a Server-Sent Events endpoint.
// Query params:
//   - n     (int, default 200): number of historical lines to send on connect
//   - unit  (string, default "argus"): systemd unit name to follow
//   - follow (bool, default true): keep the connection open and tail new entries
func (h *LogsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()

	unit := q.Get("unit")
	if unit == "" {
		unit = "argus"
	}

	lines := 200
	if nStr := q.Get("n"); nStr != "" {
		if n, err := strconv.Atoi(nStr); err == nil && n > 0 && n <= 5000 {
			lines = n
		}
	}

	follow := true
	if f := q.Get("follow"); f == "false" || f == "0" {
		follow = false
	}

	args := []string{
		"-u", unit + ".service",
		"--output=short-iso",
		"--no-pager",
		fmt.Sprintf("-n%d", lines),
	}
	if follow {
		args = append(args, "-f")
	}

	cmd := exec.CommandContext(r.Context(), "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to open journalctl: " + err.Error()})
		return
	}
	if err := cmd.Start(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start journalctl: " + err.Error()})
		return
	}
	defer func() { _ = cmd.Wait() }()

	// Kill the process when the client disconnects so the goroutine unblocks
	// from scanner.Scan() promptly rather than waiting for the next log line.
	go func() {
		<-r.Context().Done()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if r.Context().Err() != nil {
			return
		}

		line := scanner.Text()
		parsed := parseJournalLine(line)
		data, err := json.Marshal(parsed)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

// parseJournalLine splits a short-iso journalctl line into its components.
// Format: "2024-01-15T12:34:56+0000 hostname unit[pid]: message"
func parseJournalLine(line string) logLine {
	// Timestamp is the first space-delimited field (ISO 8601 with timezone).
	ts := ""
	rest := line
	if idx := indexByte(line, ' '); idx > 0 {
		ts = line[:idx]
		rest = line[idx+1:]
	}

	// Strip "hostname unit[pid]: " prefix — the message starts after ": ".
	msg := rest
	if idx := strings.Index(rest, "]: "); idx >= 0 {
		msg = rest[idx+3:]
	} else if idx := strings.Index(rest, ": "); idx >= 0 {
		msg = rest[idx+2:]
	}

	return logLine{
		Timestamp: ts,
		Priority:  detectPriority(msg),
		Message:   msg,
	}
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// detectPriority infers a log level keyword from common patterns in the message.
func detectPriority(msg string) string {
	lower := toLower(msg)
	switch {
	case contains(lower, "error") || contains(lower, "failed") || contains(lower, "fatal"):
		return "error"
	case contains(lower, "warn") || contains(lower, "warning"):
		return "warn"
	case contains(lower, "debug"):
		return "debug"
	default:
		return "info"
	}
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}

func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
