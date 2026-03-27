package mode

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ulm0/argus/internal/logger"
	"github.com/ulm0/argus/internal/system/samba"
)

const (
	quickEditLockTimeout = 30 * time.Second
	lockStaleAge         = 2 * time.Minute
)

// prepareSystemForPresent mirrors TeslaUSB present_usb.sh: wait for quick-edit locks,
// close Samba, stop smbd/nmbd, stop conflicting USB gadget services, remove g_mass_storage.
func (s *Service) prepareSystemForPresent() error {
	if err := s.waitQuickEditLocks(quickEditLockTimeout); err != nil {
		return err
	}

	sm := samba.NewManager(s.cfg)
	for _, part := range s.cfg.USBPartitions() {
		_ = sm.CloseSambaShare(samba.ShareNameForPartition(part))
	}
	_ = exec.Command("systemctl", "stop", "smbd").Run()
	_ = exec.Command("systemctl", "stop", "nmbd").Run()

	for _, svc := range []string{"rpi-usb-gadget.service", "usb-gadget.service"} {
		out, err := exec.Command("systemctl", "is-active", svc).Output()
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(out)) == "active" {
			logger.L.WithField("service", svc).Info("stopping conflicting USB gadget service")
			_ = exec.Command("systemctl", "stop", svc).Run()
			time.Sleep(300 * time.Millisecond)
		}
	}

	if data, err := os.ReadFile("/proc/modules"); err == nil && strings.Contains(string(data), "g_mass_storage") {
		_ = exec.Command("rmmod", "g_mass_storage").Run()
		time.Sleep(time.Second)
	}

	return nil
}

func (s *Service) waitQuickEditLocks(timeout time.Duration) error {
	locks := []string{
		filepath.Join(s.cfg.GadgetDir, ".quick_edit_part2.lock"),
		filepath.Join(s.cfg.GadgetDir, ".quick_edit_part3.lock"),
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		blocked := false
		for _, f := range locks {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			if time.Since(info.ModTime()) > lockStaleAge {
				logger.L.WithField("lock", f).Warn("removing stale quick_edit lock")
				_ = os.Remove(f)
				continue
			}
			blocked = true
		}
		if !blocked {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("%w: quick edit operation still in progress after %s", errors.New("timeout"), timeout)
}
