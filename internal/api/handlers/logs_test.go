package handlers

import "testing"

func TestParseJournalEntry(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantPrio string
		wantMsg  string
		wantOK   bool
	}{
		{
			name:     "priority wins over keywords",
			line:     `{"__REALTIME_TIMESTAMP":"1700000000000000","PRIORITY":"6","MESSAGE":"retrying after transient failure (recovered)"}`,
			wantPrio: "info",
			wantMsg:  "retrying after transient failure (recovered)",
			wantOK:   true,
		},
		{
			name:     "err priority without keywords",
			line:     `{"__REALTIME_TIMESTAMP":"1700000000000000","PRIORITY":"3","MESSAGE":"gadget disconnected"}`,
			wantPrio: "error",
			wantMsg:  "gadget disconnected",
			wantOK:   true,
		},
		{
			name:     "warn",
			line:     `{"__REALTIME_TIMESTAMP":"1700000000000000","PRIORITY":"4","MESSAGE":"low space"}`,
			wantPrio: "warn",
			wantMsg:  "low space",
			wantOK:   true,
		},
		{
			name:     "debug",
			line:     `{"__REALTIME_TIMESTAMP":"1700000000000000","PRIORITY":"7","MESSAGE":"tick"}`,
			wantPrio: "debug",
			wantMsg:  "tick",
			wantOK:   true,
		},
		{
			name:     "missing priority falls back to the logrus level token",
			line:     `{"__REALTIME_TIMESTAMP":"1700000000000000","MESSAGE":"time=\"2024-01-01T00:00:00Z\" level=error msg=\"mount failed\""}`,
			wantPrio: "error",
			wantMsg:  `time="2024-01-01T00:00:00Z" level=error msg="mount failed"`,
			wantOK:   true,
		},
		{
			// A word in the text is not a level: this used to land in the Error filter.
			name:     "keyword in an info message does not promote",
			line:     `{"__REALTIME_TIMESTAMP":"1700000000000000","PRIORITY":"6","MESSAGE":"time=\"2024-01-01T00:00:00Z\" level=info msg=\"cleanup finished, 0 failed\""}`,
			wantPrio: "info",
			wantMsg:  `time="2024-01-01T00:00:00Z" level=info msg="cleanup finished, 0 failed"`,
			wantOK:   true,
		},
		{
			// PRIORITY is uniform for argus's own output, so the level token is the
			// only thing that can surface an application error in the Error filter.
			name:     "level token promotes over a uniform journald priority",
			line:     `{"__REALTIME_TIMESTAMP":"1700000000000000","PRIORITY":"6","MESSAGE":"time=\"2024-01-01T00:00:00Z\" level=error msg=\"clip export failed\""}`,
			wantPrio: "error",
			wantMsg:  `time="2024-01-01T00:00:00Z" level=error msg="clip export failed"`,
			wantOK:   true,
		},
		{
			name:   "non-string MESSAGE is dropped",
			line:   `{"__REALTIME_TIMESTAMP":"1700000000000000","MESSAGE":[104,105]}`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseJournalEntry([]byte(tt.line))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Priority != tt.wantPrio {
				t.Errorf("priority = %q, want %q", got.Priority, tt.wantPrio)
			}
			if got.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", got.Message, tt.wantMsg)
			}
			if got.Timestamp == "" {
				t.Error("timestamp is empty")
			}
		})
	}
}

func TestParseJournalEntryBadTimestamp(t *testing.T) {
	got, ok := parseJournalEntry([]byte(`{"PRIORITY":"6","MESSAGE":"no timestamp"}`))
	if !ok {
		t.Fatal("entry was dropped")
	}
	if got.Timestamp != "" {
		t.Errorf("timestamp = %q, want empty", got.Timestamp)
	}
}
