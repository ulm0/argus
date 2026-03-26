//go:build linux

package music

import "syscall"

// getDiskUsed returns the number of used bytes on the filesystem containing path.
// Uses syscall.Statfs for an O(1) query, avoiding an expensive directory walk.
func getDiskUsed(path string) int64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0
	}
	return (int64(stat.Blocks) - int64(stat.Bfree)) * stat.Bsize
}
