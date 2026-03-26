//go:build !linux

package analytics

import (
	"os"
	"path/filepath"
)

// getDiskUsage falls back to a directory walk on non-Linux platforms (dev/test only).
func getDiskUsage(path string) PartitionUsage {
	var used int64
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			used += info.Size()
		}
		return nil
	})
	return PartitionUsage{
		UsedBytes: used,
	}
}
