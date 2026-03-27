//go:build linux

package mount

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/ulm0/argus/internal/logger"
)

func (m *Manager) mountLoopReadOnlyUserImpl(source, target, fsType string, uid, gid int) error {
	flags := uintptr(syscall.MS_RDONLY | syscall.MS_NOATIME)
	opts := fmt.Sprintf("uid=%d,gid=%d,umask=022", uid, gid)
	switch fsType {
	case "exfat":
		opts += ",iocharset=utf8"
	case "vfat":
		opts += ",iocharset=utf8,shortname=mixed"
	default:
		opts = "ro"
	}
	return inPID1Namespace(func() error {
		return syscall.Mount(source, target, fsType, flags, opts)
	})
}

func (m *Manager) mountImpl(source, target, fsType string, readOnly bool) error {
	flags := uintptr(syscall.MS_NOATIME)
	if readOnly {
		flags |= syscall.MS_RDONLY
	}

	options := ""
	switch fsType {
	case "exfat":
		options = "iocharset=utf8"
	case "vfat":
		options = "iocharset=utf8,shortname=mixed"
	}

	return inPID1Namespace(func() error {
		return syscall.Mount(source, target, fsType, flags, options)
	})
}

func (m *Manager) unmountImpl(target string, lazy bool) error {
	flags := 0
	if lazy {
		flags = syscall.MNT_DETACH
	}
	return inPID1Namespace(func() error {
		return syscall.Unmount(target, flags)
	})
}

func syncFS() {
	syscall.Sync()
}

func inPID1Namespace(fn func() error) error {
	// LockOSThread ensures the Go scheduler does not move this goroutine to a
	// different OS thread between the two setns calls, which would leave the
	// original thread permanently in PID 1's mount namespace.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	nsFd, err := os.Open("/proc/1/ns/mnt")
	if err != nil {
		return fn()
	}
	defer nsFd.Close()

	selfNsFd, err := os.Open("/proc/self/ns/mnt")
	if err != nil {
		return fn()
	}
	defer selfNsFd.Close()

	if err := setns(int(nsFd.Fd()), 0); err != nil {
		logger.L.WithError(err).Warn("setns failed, running in current namespace")
		return fn()
	}

	result := fn()

	if err := setns(int(selfNsFd.Fd()), 0); err != nil {
		logger.L.WithError(err).Error("failed to restore mount namespace")
	}

	return result
}

func setns(fd int, nstype int) error {
	_, _, errno := syscall.RawSyscall(308, uintptr(fd), uintptr(nstype), 0)
	if errno != 0 {
		return errno
	}
	return nil
}
