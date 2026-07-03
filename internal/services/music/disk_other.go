//go:build !linux

package music

import (
	"os"
	"path/filepath"
)

// getDiskUsage returns used bytes (summed via directory walk) plus free/total,
// which are unavailable without a Statfs syscall. This fallback is used on
// non-Linux platforms (development/testing only), where the storage bar stays
// hidden (the client gates it on total > 0).
func getDiskUsage(path string) (used, free, total int64) {
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			used += info.Size()
		}
		return nil
	})
	return used, 0, 0
}
