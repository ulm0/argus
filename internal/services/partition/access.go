package partition

import (
	"os"
	"strings"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/system/mount"
)

// AccessiblePath returns the mount path for a partition (part1, part2, part3) matching
// TeslaUSB get_mount_path: prefer RO in present mode when mounted, RW in edit mode.
func AccessiblePath(cfg *config.Config, partition string) string {
	mnt := mount.NewManager()
	mode := readModeToken(cfg)
	ro := cfg.MountPath(partition, true)
	rw := cfg.MountPath(partition, false)

	if mode == "present" && mnt.IsMounted(ro) {
		return ro
	}
	if mode == "edit" && mnt.IsMounted(rw) {
		return rw
	}
	if mnt.IsMounted(ro) {
		return ro
	}
	if mnt.IsMounted(rw) {
		return rw
	}
	if mode == "present" {
		return ro
	}
	return rw
}

func readModeToken(cfg *config.Config) string {
	data, err := os.ReadFile(cfg.StateFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
