package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

// unitNameRe matches valid systemd unit names (without the ".service" suffix
// this handler appends). Rejects spaces, control characters, and anything that
// could be parsed as a separate journalctl argument.
var unitNameRe = regexp.MustCompile(`^[a-zA-Z0-9_.@:-]{1,128}$`)

func validUnitName(s string) bool {
	return unitNameRe.MatchString(s)
}

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
	// Constrain to the systemd unit-name charset so a stray value can't turn into
	// extra journalctl arguments or anything other than a single unit selector.
	if !validUnitName(unit) {
		writeJSONError(w, http.StatusBadRequest, "invalid unit name")
		return
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

	// "kernel" is not a unit: kernel messages (USB gadget, filesystem errors)
	// only come out of the journal via -k.
	selector := []string{"-u", unit + ".service"}
	if unit == "kernel" {
		selector = []string{"-k"}
	}

	args := append(selector,
		"--output=json",
		"--no-pager",
		fmt.Sprintf("-n%d", lines),
	)
	if follow {
		args = append(args, "-f")
	}

	cmd := exec.CommandContext(r.Context(), "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to open journalctl: "+err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to start journalctl: "+err.Error())
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
	// A single journal entry (long stack trace, embedded blob) can exceed the
	// default 64 KiB limit; without a bigger bound Scan stops and the UI just
	// freezes with no indication why.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if r.Context().Err() != nil {
			return
		}

		parsed, ok := parseJournalEntry(scanner.Bytes())
		if !ok {
			continue
		}
		data, err := json.Marshal(parsed)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// A read error ends the stream while the connection stays open, which is
	// indistinguishable from an idle journal in the UI. Report it as a normal
	// log line so the user sees why the tail stopped.
	if err := scanner.Err(); err != nil && r.Context().Err() == nil {
		logger.L.WithError(err).Warn("logs: journal stream read failed")
		if data, mErr := json.Marshal(logLine{
			Timestamp: time.Now().Format("2006-01-02T15:04:05-0700"),
			Priority:  "error",
			Message:   "log stream ended: " + err.Error(),
		}); mErr == nil {
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// journalEntry is the subset of journalctl --output=json we care about. Fields
// journald recorded as non-UTF-8 come back as byte arrays rather than strings;
// those entries fail to decode and are dropped.
type journalEntry struct {
	Timestamp string `json:"__REALTIME_TIMESTAMP"`
	Priority  string `json:"PRIORITY"`
	Message   string `json:"MESSAGE"`
}

// parseJournalEntry converts one journalctl JSON entry into the SSE payload.
// Reports false for entries that cannot be decoded.
func parseJournalEntry(b []byte) (logLine, bool) {
	var e journalEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return logLine{}, false
	}

	ts := ""
	if us, err := strconv.ParseInt(e.Timestamp, 10, 64); err == nil {
		ts = time.UnixMicro(us).Format("2006-01-02T15:04:05-0700")
	}

	prio := priorityName(e.Priority)
	if prio == "" {
		prio = detectPriority(e.Message)
	} else if d := detectPriority(e.Message); d != "info" && prioSeverity[d] > prioSeverity[prio] {
		// Argus logs every level at the same journal PRIORITY and encodes the real
		// level in the message text, so PRIORITY alone hides application errors
		// from the Error/Warn filter. "info" is detectPriority's fallback rather
		// than a match, so it must not promote genuine debug lines.
		prio = d
	}

	return logLine{
		Timestamp: ts,
		Priority:  prio,
		Message:   e.Message,
	}, true
}

// priorityName maps a syslog severity onto the four levels the UI filters on.
// Returns "" when the field is missing or unparsable.
func priorityName(p string) string {
	n, err := strconv.Atoi(p)
	if err != nil {
		return ""
	}
	switch {
	case n <= 3:
		return "error"
	case n == 4:
		return "warn"
	case n <= 6:
		return "info"
	default:
		return "debug"
	}
}

// prioSeverity ranks the four UI levels so the more severe of the journald
// PRIORITY and the message-text heuristic can win.
var prioSeverity = map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}

// detectPriority reads the level argus encodes in the message text. Its logrus
// TextFormatter emits `time="..." level=error msg="..."`, so the level is a
// real token, not a word to hunt for: scanning the whole message promoted any
// info line that happened to contain "failed" or "error" (a filename,
// "cleanup finished, 0 failed") into the Error filter people open to triage.
// Returns "info" when no level token is present, which is also the value that
// must not promote — see parseJournalEntry.
func detectPriority(msg string) string {
	const tok = "level="
	i := strings.Index(msg, tok)
	if i < 0 || (i > 0 && msg[i-1] != ' ') {
		return "info"
	}
	val := msg[i+len(tok):]
	if j := strings.IndexByte(val, ' '); j >= 0 {
		val = val[:j]
	}
	switch strings.ToLower(val) {
	case "error", "fatal", "panic":
		return "error"
	case "warn", "warning":
		return "warn"
	case "debug", "trace":
		return "debug"
	default:
		return "info"
	}
}
