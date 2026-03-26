package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
	"github.com/ulm0/argus/internal/system/gadget"
	"github.com/ulm0/argus/internal/system/loop"
)

const (
	lockStaleTimeout = 120 * time.Second
)

// QuickEditor provides temporary RW access to a partition while in present mode.
// It safely clears the LUN, unmounts RO, mounts RW, runs the callback, then restores.
type QuickEditor struct {
	cfg       *config.Config
	gadgetMgr *gadget.Manager
	loopMgr   *loop.Manager
	mountMgr  *Manager
	mu        sync.Mutex
}

func NewQuickEditor(cfg *config.Config, g *gadget.Manager, l *loop.Manager, m *Manager) *QuickEditor {
	return &QuickEditor{
		cfg:       cfg,
		gadgetMgr: g,
		loopMgr:   l,
		mountMgr:  m,
	}
}

// QuickEditPart2 provides temporary RW access to partition 2 (LightShow).
func (q *QuickEditor) QuickEditPart2(callback func(rwPath string) error, timeout time.Duration) error {
	return q.quickEdit("part2", 1, q.cfg.ImgLightshow, callback, timeout)
}

// QuickEditPart3 provides temporary RW access to partition 3 (Music).
func (q *QuickEditor) QuickEditPart3(callback func(rwPath string) error, timeout time.Duration) error {
	return q.quickEdit("part3", 2, q.cfg.ImgMusicPath, callback, timeout)
}

func (q *QuickEditor) quickEdit(partition string, lunNumber int, imgPath string, callback func(rwPath string) error, timeout time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	lockFile := filepath.Join(q.cfg.GadgetDir, fmt.Sprintf(".quick_edit_%s.lock", partition))

	// Check for stale lock
	if info, err := os.Stat(lockFile); err == nil {
		if time.Since(info.ModTime()) > lockStaleTimeout {
			logger.L.WithField("lock", lockFile).Warn("removing stale quick_edit lock")
			os.Remove(lockFile)
		} else {
			return fmt.Errorf("quick edit already in progress for %s", partition)
		}
	}

	// Acquire lock
	if err := os.WriteFile(lockFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		return fmt.Errorf("create lock: %w", err)
	}
	defer os.Remove(lockFile)

	roPath := q.cfg.MountPath(partition, true)
	rwPath := q.cfg.MountPath(partition, false)

	// 1. Clear LUN backing file
	logger.L.WithField("lun", lunNumber).Debug("quick_edit: clearing LUN")
	if err := q.gadgetMgr.ClearLUN(lunNumber); err != nil {
		return fmt.Errorf("clear LUN: %w", err)
	}

	// 2. Unmount RO
	logger.L.WithField("path", roPath).Debug("quick_edit: unmounting RO")
	if q.mountMgr.IsMounted(roPath) {
		if err := q.mountMgr.SafeUnmountDir(roPath); err != nil {
			q.restoreLUN(lunNumber, imgPath)
			return fmt.Errorf("unmount RO: %w", err)
		}
	}

	// 3. Detach old loop devices
	if err := q.loopMgr.DetachAllForFile(imgPath); err != nil {
		logger.L.WithError(err).WithField("image", imgPath).Warn("failed to detach loops")
	}

	// 4. Create RW loop device and mount
	loopDev, err := q.loopMgr.Create(imgPath, false)
	if err != nil {
		q.restoreLUN(lunNumber, imgPath)
		return fmt.Errorf("create RW loop: %w", err)
	}

	fsType, err := q.mountMgr.DetectFSType(loopDev)
	if err != nil {
		q.loopMgr.Detach(loopDev)
		q.restoreLUN(lunNumber, imgPath)
		return fmt.Errorf("detect fs type: %w", err)
	}

	logger.L.WithField("loop", loopDev).WithField("path", rwPath).WithField("fs", fsType).Debug("quick_edit: mounting RW")
	if err := q.mountMgr.Mount(loopDev, rwPath, fsType, false); err != nil {
		q.loopMgr.Detach(loopDev)
		q.restoreLUN(lunNumber, imgPath)
		return fmt.Errorf("mount RW: %w", err)
	}

	// 5. Execute callback with timeout
	done := make(chan error, 1)
	go func() {
		done <- callback(rwPath)
	}()

	var callbackErr error
	select {
	case callbackErr = <-done:
	case <-time.After(timeout):
		callbackErr = fmt.Errorf("quick edit timed out after %s", timeout)
	}

	// 6. Cleanup: sync, unmount, detach, restore
	q.mountMgr.Sync()

	logger.L.WithField("path", rwPath).Debug("quick_edit: unmounting RW")
	if err := q.mountMgr.SafeUnmountDir(rwPath); err != nil {
		logger.L.WithError(err).WithField("path", rwPath).Warn("failed to unmount RW")
	}

	if err := q.loopMgr.Detach(loopDev); err != nil {
		logger.L.WithError(err).WithField("loop", loopDev).Warn("failed to detach loop device")
	}

	// 7. Restore RO loop + mount
	roLoop, err := q.loopMgr.Create(imgPath, true)
	if err != nil {
		logger.L.WithError(err).WithField("image", imgPath).Warn("failed to create RO loop")
	} else {
		if err := q.mountMgr.Mount(roLoop, roPath, fsType, true); err != nil {
			logger.L.WithError(err).WithField("path", roPath).Warn("failed to remount RO")
		}
	}

	// 8. Restore LUN
	q.restoreLUN(lunNumber, imgPath)

	q.mountMgr.Sync()
	q.mountMgr.DropCaches()

	return callbackErr
}

func (q *QuickEditor) restoreLUN(lunNumber int, imgPath string) {
	logger.L.WithField("lun", lunNumber).WithField("image", imgPath).Debug("quick_edit: restoring LUN")
	if err := q.gadgetMgr.RestoreLUN(lunNumber, imgPath, 5); err != nil {
		logger.L.WithError(err).WithField("lun", lunNumber).Error("failed to restore LUN")
	}
}

// IsOperationInProgress checks if any quick edit lock is active.
func (q *QuickEditor) IsOperationInProgress() (bool, float64) {
	for _, part := range []string{"part2", "part3"} {
		lockFile := filepath.Join(q.cfg.GadgetDir, fmt.Sprintf(".quick_edit_%s.lock", part))
		if info, err := os.Stat(lockFile); err == nil {
			age := time.Since(info.ModTime()).Seconds()
			return true, age
		}
	}
	return false, 0
}
