//go:build !linux

package loop

import "fmt"

func (m *Manager) Create(imagePath string, readOnly bool) (string, error) {
	return "", fmt.Errorf("loop devices not supported on this platform (Linux only)")
}

func (m *Manager) Detach(devPath string) error {
	return fmt.Errorf("loop devices not supported on this platform (Linux only)")
}

func (m *Manager) findByFileImpl(imagePath string) ([]string, error) {
	return nil, nil
}
