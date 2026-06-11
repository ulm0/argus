package handlers

import (
	"path/filepath"
	"testing"
)

func TestWithinBase(t *testing.T) {
	base := filepath.FromSlash("/mnt/cam/TeslaCam")

	cases := []struct {
		name string
		p    string
		want bool
	}{
		{"base itself", base, true},
		{"nested file", filepath.Join(base, "SentryClips", "x.mp4"), true},
		{"sibling escape", filepath.FromSlash("/mnt/cam/TeslaCam-evil/x"), false},
		{"prefix sibling", base + "-evil", false},
		{"parent", filepath.FromSlash("/mnt/cam"), false},
		{"unrelated", filepath.FromSlash("/etc/passwd"), false},
	}
	for _, c := range cases {
		if got := withinBase(c.p, base); got != c.want {
			t.Errorf("%s: withinBase(%q, %q) = %v, want %v", c.name, c.p, base, got, c.want)
		}
	}
}
