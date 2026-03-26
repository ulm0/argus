package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"

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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			return
		default:
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
	if len(line) < 25 {
		return logLine{Message: line}
	}

	// Extract timestamp (first 25 chars cover ISO timestamp + timezone offset)
	ts := ""
	rest := line
	if len(line) > 25 {
		ts = line[:25]
		rest = line[26:]
	}

	// Strip the host + unit prefix to get just the message
	msg := rest
	if idx := indexByte(rest, ':'); idx >= 0 && idx+2 < len(rest) {
		msg = rest[idx+2:]
	}

	priority := detectPriority(msg)

	return logLine{
		Timestamp: ts,
		Priority:  priority,
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
