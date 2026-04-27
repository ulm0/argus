//go:build linux

package watchdog

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ulm0/argus/internal/logger"
)

const (
	watchdogDev     = "/dev/watchdog"
	wdiocSetTimeout = 0xC0045706
	wdiocKeepAlive  = 0x80045705
	wdiocGetTimeout = 0x80045707
)

// Manager handles the hardware watchdog timer on the Raspberry Pi.
type Manager struct {
	mu       sync.Mutex
	fd       *os.File
	interval time.Duration
	stopCh   chan struct{}
}

func NewManager() *Manager {
	return &Manager{}
}

// Start opens the watchdog device and begins periodic keepalive pings.
//
// The keepalive interval is computed from the timeout the kernel actually
// programmed (via WDIOC_GETTIMEOUT) and not from the requested value, because
// most SoC watchdog drivers clamp the timeout to a hardware maximum. On the
// Raspberry Pi, for example, the BCM2835 watchdog caps at ~15s; if we trusted
// the requested value here a `watchdog_timeout_sec: 60` config would silently
// reboot the box every ~15s.
func (m *Manager) Start(timeoutSec int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fd, err := os.OpenFile(watchdogDev, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open watchdog: %w", err)
	}

	t := int32(timeoutSec)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd.Fd(), wdiocSetTimeout,
		uintptr(unsafe.Pointer(&t)))
	if errno != 0 {
		closeWatchdog(fd)
		return fmt.Errorf("set watchdog timeout: %w", errno)
	}

	var actual int32
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd.Fd(), wdiocGetTimeout,
		uintptr(unsafe.Pointer(&actual)))
	if errno != 0 {
		logger.L.WithField("errno", errno).Warn("watchdog get timeout failed; using requested value")
		actual = t
	}
	if actual != t {
		logger.L.WithFields(map[string]any{
			"requested": t,
			"actual":    actual,
		}).Warn("watchdog: kernel clamped timeout (likely hardware limit)")
	}

	// Ping at half the actual timeout, with a 1-second floor so a small
	// timeoutSec (e.g. 1) doesn't yield a zero-duration ticker that panics.
	interval := time.Duration(actual/2) * time.Second
	if interval < time.Second {
		interval = time.Second
	}
	m.fd = fd
	m.interval = interval
	m.stopCh = make(chan struct{})

	logger.L.WithFields(map[string]any{
		"timeout_sec":     actual,
		"keepalive_every": interval,
	}).Info("watchdog started")

	go m.keepAliveLoop()

	return nil
}

// closeWatchdog disables the device and closes the fd. The "magic close"
// character 'V' tells the kernel that we shut down on purpose, otherwise the
// driver may keep counting down and reboot the box even after we've gone away.
func closeWatchdog(fd *os.File) {
	_, _ = fd.Write([]byte("V"))
	_ = fd.Close()
}

// Stop closes the watchdog device. Safe to call multiple times.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopCh != nil {
		close(m.stopCh)
		m.stopCh = nil
	}

	if m.fd != nil {
		closeWatchdog(m.fd)
		m.fd = nil
	}
}

func (m *Manager) keepAliveLoop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.ping()
		}
	}
}

func (m *Manager) ping() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fd == nil {
		return
	}

	var dummy int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, m.fd.Fd(), wdiocKeepAlive,
		uintptr(unsafe.Pointer(&dummy)))
	if errno != 0 {
		logger.L.WithField("errno", errno).Warn("watchdog keepalive failed")
	}
}
