package handlers

import (
	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/system/mount"
)

// editMountPath returns the read-write mount for a partition, or "" when it is
// not currently mounted there.
//
// The check has to be "is something mounted here", not "does this directory
// exist": mount.Manager.Mount() MkdirAll's the target and nothing removes it on
// unmount, so in Present mode the read-write mountpoint is still a perfectly
// good empty directory on the SD card. Writing through it would silently land
// media on the root filesystem, report success, and leave the file invisible to
// every read path (which resolves through partition.AccessiblePath).
func editMountPath(cfg *config.Config, partition string) string {
	path := cfg.MountPath(partition, false)
	if mount.NewManager().IsMounted(path) {
		return path
	}
	return ""
}
