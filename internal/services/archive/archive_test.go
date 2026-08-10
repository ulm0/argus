package archive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// age backdates a file past eventQuietPeriod so the sync treats it as an event
// the car has finished writing.
func age(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-2 * eventQuietPeriod)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestSyncFolder(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "SentryClips")

	event := filepath.Join(src, "2024-01-01_12-00-00")
	if err := os.MkdirAll(event, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(event, "front.mp4"), []byte("clip"), 0644); err != nil {
		t.Fatal(err)
	}
	// A loose file, as RecentClips stores them.
	if err := os.WriteFile(filepath.Join(src, "2024-01-01_13-00-00-front.mp4"), []byte("clip"), 0644); err != nil {
		t.Fatal(err)
	}
	age(t, filepath.Join(event, "front.mp4"))
	age(t, filepath.Join(src, "2024-01-01_13-00-00-front.mp4"))

	n, err := syncFolder(src, dst)
	if err != nil {
		t.Fatalf("syncFolder: %v", err)
	}
	if n != 2 {
		t.Errorf("copied %d events, want 2", n)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "2024-01-01_12-00-00", "front.mp4")); err != nil || string(data) != "clip" {
		t.Errorf("event clip not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "2024-01-01_12-00-00", archivedMarker)); err != nil {
		t.Errorf("marker not written: %v", err)
	}

	// Second pass must skip everything already archived.
	if n, err = syncFolder(src, dst); err != nil || n != 0 {
		t.Errorf("second pass copied %d (err %v), want 0", n, err)
	}

	// A target directory without the marker is an interrupted copy: retry it.
	if err := os.Remove(filepath.Join(dst, "2024-01-01_12-00-00", archivedMarker)); err != nil {
		t.Fatal(err)
	}
	if n, err = syncFolder(src, dst); err != nil || n != 1 {
		t.Errorf("retry copied %d (err %v), want 1", n, err)
	}
}

// An event the car is still writing must not be archived, or the marker would
// freeze it half-copied forever.
func TestSyncFolderSkipsFreshContent(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "SentryClips")

	event := filepath.Join(src, "2024-01-01_12-00-00")
	if err := os.MkdirAll(event, 0755); err != nil {
		t.Fatal(err)
	}
	// One finished clip and one the car just wrote.
	if err := os.WriteFile(filepath.Join(event, "front.mp4"), []byte("clip"), 0644); err != nil {
		t.Fatal(err)
	}
	age(t, filepath.Join(event, "front.mp4"))
	if err := os.WriteFile(filepath.Join(event, "back.mp4"), []byte("clip"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "2024-01-01_13-00-00-front.mp4"), []byte("clip"), 0644); err != nil {
		t.Fatal(err)
	}

	if n, err := syncFolder(src, dst); err != nil || n != 0 {
		t.Errorf("copied %d (err %v), want 0", n, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "2024-01-01_12-00-00", archivedMarker)); err == nil {
		t.Error("marked a half-written event complete")
	}

	// Once the car is done, the next pass takes it.
	age(t, filepath.Join(event, "back.mp4"))
	age(t, filepath.Join(src, "2024-01-01_13-00-00-front.mp4"))
	if n, err := syncFolder(src, dst); err != nil || n != 2 {
		t.Errorf("settled pass copied %d (err %v), want 2", n, err)
	}
}

func TestSyncFolderMissingSource(t *testing.T) {
	n, err := syncFolder(filepath.Join(t.TempDir(), "nope"), t.TempDir())
	if err != nil || n != 0 {
		t.Errorf("missing source: got (%d, %v), want (0, nil)", n, err)
	}
}
