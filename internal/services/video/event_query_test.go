package video

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeTriggerOffset(t *testing.T) {
	clips := []string{"2024-01-02_15-00-00", "2024-01-02_15-01-00"}

	tests := []struct {
		name     string
		clips    []string
		datetime string
		want     float64
	}{
		{"offset into first clip", clips, "2024-01-02T15:00:42", 42},
		{"offset into later clip", clips, "2024-01-02T15:01:30", 90},
		{"trigger before first clip", clips, "2024-01-02T14:59:00", 0},
		{"unparseable datetime", clips, "whenever", 0},
		{"no clips", nil, "2024-01-02T15:00:42", 0},
	}

	for _, tt := range tests {
		if got := computeTriggerOffset(tt.clips, tt.datetime); got != tt.want {
			t.Errorf("%s: computeTriggerOffset() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestGetEventsBefore(t *testing.T) {
	svc := NewService(testConfig(t))
	dir := t.TempDir()

	for _, name := range []string{
		"2024-01-01_00-00-00",
		"2024-01-15_12-00-00",
		"2024-02-01_08-00-00",
	} {
		if err := os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}

	events, hasNext := svc.GetEvents(dir, 0, 10, "2024-01-15")
	if hasNext {
		t.Error("expected hasNext = false")
	}
	// The cutoff day itself must be included, newest first.
	if len(events) != 2 {
		t.Fatalf("len = %d, want 2", len(events))
	}
	if events[0].Name != "2024-01-15_12-00-00" {
		t.Errorf("first event = %q, want %q", events[0].Name, "2024-01-15_12-00-00")
	}

	if all, _ := svc.GetEvents(dir, 0, 10, ""); len(all) != 3 {
		t.Errorf("unfiltered len = %d, want 3", len(all))
	}
}

func TestToggleKeep(t *testing.T) {
	svc := NewService(testConfig(t))
	dir := t.TempDir()
	eventName := "2024-01-01_00-00-00"
	if err := os.Mkdir(filepath.Join(dir, eventName), 0755); err != nil {
		t.Fatal(err)
	}

	kept, err := svc.ToggleKeep(dir, eventName)
	if err != nil || !kept {
		t.Fatalf("ToggleKeep() = %v, %v; want true, nil", kept, err)
	}
	if events, _ := svc.GetEvents(dir, 0, 10, ""); len(events) != 1 || !events[0].Kept {
		t.Error("expected the event to report kept = true")
	}

	kept, err = svc.ToggleKeep(dir, eventName)
	if err != nil || kept {
		t.Fatalf("ToggleKeep() = %v, %v; want false, nil", kept, err)
	}
	if fileExists(filepath.Join(dir, eventName, KeepMarker)) {
		t.Error("marker still present after untoggle")
	}

	if _, err := svc.ToggleKeep(dir, "../escape"); err == nil {
		t.Error("expected path traversal to be rejected")
	}
}
