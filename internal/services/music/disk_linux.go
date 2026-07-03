//go:build linux

package music

import "syscall"

// getDiskUsage returns used, free (available to unprivileged writers) and total
// bytes of the filesystem containing path. Uses syscall.Statfs for an O(1) query,
// avoiding an expensive directory walk.
func getDiskUsage(path string) (used, free, total int64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0
	}
	bs := int64(stat.Bsize)
	total = int64(stat.Blocks) * bs
	free = int64(stat.Bavail) * bs
	used = (int64(stat.Blocks) - int64(stat.Bfree)) * bs
	return used, free, total
}
