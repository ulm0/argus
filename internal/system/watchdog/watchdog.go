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
func (m *Manager) Start(timeoutSec int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fd, err := os.OpenFile(watchdogDev, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open watchdog: %w", err)
	}
	m.fd = fd

	// Set timeout
	t := int32(timeoutSec)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd.Fd(), wdiocSetTimeout,
		uintptr(unsafe.Pointer(&t)))
	if errno != 0 {
		return fmt.Errorf("set watchdog timeout: %w", errno)
	}

	// Read back actual timeout
	var actual int32
	syscall.Syscall(syscall.SYS_IOCTL, fd.Fd(), wdiocGetTimeout,
		uintptr(unsafe.Pointer(&actual)))
	logger.L.WithField("timeout_sec", actual).Info("watchdog started")

	// Keepalive at half the timeout interval
	m.interval = time.Duration(timeoutSec/2) * time.Second
	m.stopCh = make(chan struct{})

	go m.keepAliveLoop()

	return nil
}

// Stop closes the watchdog device.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopCh != nil {
		close(m.stopCh)
		m.stopCh = nil
	}

	if m.fd != nil {
		// Write 'V' to disable watchdog on close (magic close character)
		m.fd.Write([]byte("V"))
		m.fd.Close()
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
