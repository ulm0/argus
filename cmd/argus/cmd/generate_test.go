package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildGenerateContent(t *testing.T) {
	t.Parallel()

	content := buildGenerateContent("alice")
	if !strings.Contains(content, "target_user: alice") {
		t.Fatalf("expected target_user to be replaced, got content missing %q", "target_user: alice")
	}
	if strings.Contains(content, "target_user: pi") {
		t.Fatalf("expected template placeholder to be replaced; found %q", "target_user: pi")
	}
	if !strings.Contains(content, "installation:") {
		t.Fatalf("expected generated config to contain %q", "installation:")
	}
}

func TestWriteGenerateConfig_NewFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	wrote, err := writeGenerateConfig(path, "foo", false)
	if err != nil {
		t.Fatalf("writeGenerateConfig returned error: %v", err)
	}
	if !wrote {
		t.Fatalf("expected wrote=true for a non-existing file")
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed reading written file: %v", err)
	}
	if string(b) != "foo" {
		t.Fatalf("unexpected file content: %q", string(b))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed stat of written file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0644 {
		t.Fatalf("unexpected file permissions: got=%#o want=%#o", got, 0644)
	}
}

func TestWriteGenerateConfig_ExistingNoForce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatalf("failed preparing test file: %v", err)
	}

	wrote, err := writeGenerateConfig(path, "new", false)
	if err != nil {
		t.Fatalf("writeGenerateConfig returned error: %v", err)
	}
	if wrote {
		t.Fatalf("expected wrote=false when file exists and force=false")
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed reading test file: %v", err)
	}
	if string(b) != "old" {
		t.Fatalf("expected file content to remain unchanged; got %q", string(b))
	}
}

func TestWriteGenerateConfig_ExistingForce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatalf("failed preparing test file: %v", err)
	}

	wrote, err := writeGenerateConfig(path, "new", true)
	if err != nil {
		t.Fatalf("writeGenerateConfig returned error: %v", err)
	}
	if !wrote {
		t.Fatalf("expected wrote=true when force=true")
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed reading test file: %v", err)
	}
	if string(b) != "new" {
		t.Fatalf("expected file content to be updated; got %q", string(b))
	}
}

func TestWriteGenerateConfig_AtomicWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	base := filepath.Base(path)

	pattern := filepath.Join(dir, "."+base+".tmp-*")

	wrote, err := writeGenerateConfig(path, "content", false)
	if err != nil {
		t.Fatalf("writeGenerateConfig returned error: %v", err)
	}
	if !wrote {
		t.Fatalf("expected wrote=true")
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("failed checking leftover temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no leftover temp files after success; found %d: %v", len(matches), matches)
	}
}

