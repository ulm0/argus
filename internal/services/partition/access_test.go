package partition

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ulm0/argus/internal/config"
)

func TestAccessiblePath_PresentFallback(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		MountDir:  filepath.Join(dir, "mnt"),
		StateFile: filepath.Join(dir, "state.txt"),
	}
	if err := os.WriteFile(cfg.StateFile, []byte("present\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := AccessiblePath(cfg, "part1")
	want := cfg.MountPath("part1", true)
	if got != want {
		t.Errorf("AccessiblePath = %q, want %q", got, want)
	}
}

func TestAccessiblePath_EditFallback(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		MountDir:  filepath.Join(dir, "mnt"),
		StateFile: filepath.Join(dir, "state.txt"),
	}
	if err := os.WriteFile(cfg.StateFile, []byte("edit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := AccessiblePath(cfg, "part2")
	want := cfg.MountPath("part2", false)
	if got != want {
		t.Errorf("AccessiblePath = %q, want %q", got, want)
	}
}

func TestAccessiblePath_EmptyStateUsesRWFallback(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		MountDir:  filepath.Join(dir, "mnt"),
		StateFile: filepath.Join(dir, "missing-state.txt"),
	}
	got := AccessiblePath(cfg, "part3")
	want := cfg.MountPath("part3", false)
	if got != want {
		t.Errorf("AccessiblePath = %q, want %q", got, want)
	}
}
