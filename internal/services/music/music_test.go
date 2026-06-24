package music

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulm0/argus/internal/config"
)

// newTestService returns a music Service rooted at a fresh temp mount.
func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	mount := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mount, MusicFolder), 0755); err != nil {
		t.Fatal(err)
	}
	return NewService(&config.Config{}), mount
}

// noFileOutside fails the test if any regular file was created outside musicDir.
func noFileOutside(t *testing.T, mount string) {
	t.Helper()
	musicDir := filepath.Join(mount, MusicFolder)
	_ = filepath.Walk(mount, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasPrefix(p, musicDir+string(filepath.Separator)) {
			t.Errorf("file written outside music dir: %s", p)
		}
		return nil
	})
}

func TestHandleChunkContainsTraversal(t *testing.T) {
	cases := []struct {
		name      string
		uploadID  string
		filename  string
		relPath   string
		wantError bool // true when the input cannot be safely sanitized
	}{
		// A filename with separators is sanitized to its base and written safely
		// inside the music dir — neutralized, not rejected.
		{"filename traversal", "abc", "../../../etc/evil", "", false},
		// relPath/uploadID/".." cannot be reduced to a safe in-dir target.
		{"relpath traversal", "abc", "song.mp3", "../../..", true},
		{"uploadID traversal", "../../../tmp/x", "song.mp3", "", true},
		{"dotdot filename", "abc", "..", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, mount := newTestService(t)
			_, err := svc.HandleChunk(c.uploadID, c.filename, 0, 1, strings.NewReader("data"), mount, c.relPath)
			if c.wantError && err == nil {
				t.Errorf("expected traversal to be rejected, got nil error")
			}
			// The invariant that matters: nothing is ever written outside musicDir.
			noFileOutside(t, mount)
		})
	}
}

func TestHandleChunkHappyPath(t *testing.T) {
	svc, mount := newTestService(t)
	complete, err := svc.HandleChunk("upload1", "song.mp3", 0, 1, strings.NewReader("hello"), mount, "sub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !complete {
		t.Fatalf("expected single-chunk upload to complete")
	}
	got, err := os.ReadFile(filepath.Join(mount, MusicFolder, "sub", "song.mp3"))
	if err != nil {
		t.Fatalf("assembled file missing: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("assembled content = %q, want %q", got, "hello")
	}
}

func TestSaveFileRejectsTraversal(t *testing.T) {
	svc, mount := newTestService(t)
	// relPath escapes the music dir.
	if err := svc.SaveFile(strings.NewReader("x"), "ok.mp3", mount, "../.."); err == nil {
		t.Errorf("expected SaveFile to reject relPath traversal")
	}
	noFileOutside(t, mount)
}
