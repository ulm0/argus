package loop

import (
	"sync"
)

type Manager struct {
	mu sync.Mutex
}

func NewManager() *Manager {
	return &Manager{}
}

// FindByFile returns all loop devices backed by the given image file.
func (m *Manager) FindByFile(imagePath string) ([]string, error) {
	return m.findByFileImpl(imagePath)
}

// DetachAllForFile detaches all loop devices associated with an image file.
func (m *Manager) DetachAllForFile(imagePath string) error {
	devs, err := m.FindByFile(imagePath)
	if err != nil {
		return err
	}
	for _, dev := range devs {
		if err := m.Detach(dev); err != nil {
			return err
		}
	}
	return nil
}
