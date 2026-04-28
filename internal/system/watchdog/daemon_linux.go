//go:build linux

package watchdog

import (
	"fmt"
	"os"
	"os/exec"
)

const confPath = "/etc/watchdog.conf"

// ApplyDaemon configures the Debian watchdog(8) daemon (TeslaUSB-style) to match
// Argus system.watchdog_* settings. Only one userspace process should open
// /dev/watchdog; we use this package instead of opening the device from Argus.
//
// When enabled is false, watchdog.service is stopped, disabled, and masked.
func ApplyDaemon(enabled bool, timeoutSec int) error {
	if timeoutSec < 10 {
		timeoutSec = 10
	}

	if enabled {
		if err := writeWatchdogConf(timeoutSec); err != nil {
			return err
		}
		// wd_keepalive can claim /dev/watchdog on some images; only run the main daemon.
		_ = exec.Command("systemctl", "stop", "wd_keepalive.service").Run()
		_ = exec.Command("systemctl", "disable", "wd_keepalive.service").Run()
		_ = exec.Command("systemctl", "mask", "wd_keepalive.service").Run()
		_ = exec.Command("systemctl", "unmask", "watchdog.service").Run()
		if out, err := exec.Command("systemctl", "enable", "watchdog.service").CombinedOutput(); err != nil {
			return fmt.Errorf("enable watchdog.service: %w\n%s", err, out)
		}
		if out, err := exec.Command("systemctl", "restart", "watchdog.service").CombinedOutput(); err != nil {
			return fmt.Errorf("restart watchdog.service: %w\n%s", err, out)
		}
		return nil
	}

	_ = exec.Command("systemctl", "stop", "watchdog.service").Run()
	_ = exec.Command("systemctl", "disable", "watchdog.service").Run()
	_ = exec.Command("systemctl", "mask", "watchdog.service").Run()
	_ = exec.Command("systemctl", "stop", "wd_keepalive.service").Run()
	_ = exec.Command("systemctl", "disable", "wd_keepalive.service").Run()
	_ = exec.Command("systemctl", "mask", "wd_keepalive.service").Run()
	return nil
}

func writeWatchdogConf(timeoutSec int) error {
	// Conservative defaults per TeslaUSB / mphacker readme: avoid min-memory,
	// repair-binary, and very low intervals on Pi Zero 2 W (512MB RAM).
	content := fmt.Sprintf(`# Managed by Argus — mirrors system.watchdog_* in config.yaml
# TeslaUSB-style defaults (https://github.com/mphacker/TeslaUSB)
watchdog-device = /dev/watchdog
watchdog-timeout = %d
max-load-1 = 24
realtime = yes
priority = 1
`, timeoutSec)
	return os.WriteFile(confPath, []byte(content), 0644)
}
