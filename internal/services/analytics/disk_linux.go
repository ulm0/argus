//go:build linux

package analytics

import "syscall"

// getDiskUsage uses syscall.Statfs to read real filesystem statistics in O(1),
// avoiding the expensive directory walk that would tax the Pi Zero 2 W.
func getDiskUsage(path string) PartitionUsage {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return PartitionUsage{}
	}

	total := int64(stat.Blocks) * stat.Bsize
	free := int64(stat.Bavail) * stat.Bsize
	used := total - int64(stat.Bfree)*stat.Bsize

	var percent float64
	if total > 0 {
		percent = float64(used) / float64(total) * 100
	}

	return PartitionUsage{
		TotalBytes: total,
		UsedBytes:  used,
		FreeBytes:  free,
		Percent:    percent,
	}
}
