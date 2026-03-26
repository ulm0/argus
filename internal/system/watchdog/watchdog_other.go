//go:build !linux

package watchdog

import "fmt"

type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Start(timeoutSec int) error {
	return fmt.Errorf("watchdog not supported on this platform (Linux only)")
}

func (m *Manager) Stop() {}
