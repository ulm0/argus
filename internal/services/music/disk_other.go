//go:build !linux

package music

import (
	"os"
	"path/filepath"
)

// getDiskUsed returns the total size of all files under path via directory walk.
// This fallback is used on non-Linux platforms (development/testing only).
func getDiskUsed(path string) int64 {
	var used int64
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			used += info.Size()
		}
		return nil
	})
	return used
}
