package mount

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/logger"
)

type Manager struct {
	mu sync.Mutex
}

func NewManager() *Manager {
	return &Manager{}
}

// Mount mounts a device at the target path in the PID 1 mount namespace.
func (m *Manager) Mount(source, target, fsType string, readOnly bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", target, err)
	}

	return m.mountImpl(source, target, fsType, readOnly)
}

// MountLoopReadOnlyUser mounts a loop device read-only with uid/gid for vfat/exfat (TeslaUSB-style local browsing).
func (m *Manager) MountLoopReadOnlyUser(source, target, fsType string, uid, gid int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", target, err)
	}

	return m.mountLoopReadOnlyUserImpl(source, target, fsType, uid, gid)
}

// Unmount unmounts a path in the PID 1 mount namespace with retries.
func (m *Manager) Unmount(target string, maxRetries int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := 0; i < maxRetries; i++ {
		if err := m.unmountImpl(target, false); err == nil {
			return nil
		}

		if i < maxRetries-1 {
			_ = exec.Command("fuser", "-km", target).Run()
			time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
		}
	}

	// Last resort: lazy unmount
	return m.unmountImpl(target, true)
}

// IsMounted checks if a path is a mount point.
func (m *Manager) IsMounted(target string) bool {
	cmd := exec.Command("mountpoint", "-q", target)
	return cmd.Run() == nil
}

// Sync flushes all filesystem buffers.
func (m *Manager) Sync() {
	syncFS()
}

// FlushBlockDevice flushes the block device buffer cache.
func (m *Manager) FlushBlockDevice(dev string) error {
	cmd := exec.Command("blockdev", "--flushbufs", dev)
	return cmd.Run()
}

// DropCaches writes to /proc/sys/vm/drop_caches to free page cache.
func (m *Manager) DropCaches() error {
	return os.WriteFile("/proc/sys/vm/drop_caches", []byte("3"), 0644)
}

// DetectFSType detects the filesystem type of a block device.
func (m *Manager) DetectFSType(dev string) (string, error) {
	out, err := exec.Command("blkid", "-o", "value", "-s", "TYPE", dev).Output()
	if err != nil {
		return "", fmt.Errorf("blkid %s: %w", dev, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// SafeUnmountDir unmounts a directory with escalating force.
func (m *Manager) SafeUnmountDir(target string) error {
	if !m.IsMounted(target) {
		return nil
	}

	err := m.Unmount(target, 3)
	if err == nil {
		return nil
	}

	logger.L.WithField("target", target).Warn("forced unmount after retry failures")
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unmountImpl(target, true)
}

// MountPath returns a standardized mount path for a partition.
func MountPath(baseDir, partition string, readOnly bool) string {
	suffix := ""
	if readOnly {
		suffix = "-ro"
	}
	return filepath.Join(baseDir, partition+suffix)
}
