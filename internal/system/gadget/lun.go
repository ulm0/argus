package gadget

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LUNState represents the current state of a LUN.
type LUNState struct {
	Number   int    `json:"number"`
	File     string `json:"file"`
	ReadOnly bool   `json:"read_only"`
	HasFile  bool   `json:"has_file"`
}

// GetLUNStates reads the current state of all configured LUNs.
func (m *Manager) GetLUNStates() ([]LUNState, error) {
	funcDir := filepath.Join(m.gadgetDir, "functions", "mass_storage.usb0")
	entries, err := os.ReadDir(funcDir)
	if err != nil {
		return nil, fmt.Errorf("read function dir: %w", err)
	}

	var states []LUNState
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "lun.") {
			continue
		}

		var num int
		fmt.Sscanf(e.Name(), "lun.%d", &num)
		lunDir := filepath.Join(funcDir, e.Name())

		state := LUNState{Number: num}

		if data, err := os.ReadFile(filepath.Join(lunDir, "file")); err == nil {
			state.File = strings.TrimSpace(string(data))
			state.HasFile = state.File != ""
		}
		if data, err := os.ReadFile(filepath.Join(lunDir, "ro")); err == nil {
			state.ReadOnly = strings.TrimSpace(string(data)) == "1"
		}

		states = append(states, state)
	}

	return states, nil
}

// ClearLUN removes the backing file from a LUN (sets it to empty).
func (m *Manager) ClearLUN(lunNumber int) error {
	return m.SetLUNFile(lunNumber, "")
}

// RestoreLUN sets the backing file for a LUN with retries.
func (m *Manager) RestoreLUN(lunNumber int, filePath string, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := m.SetLUNFile(lunNumber, filePath); err != nil {
			lastErr = err
			time.Sleep(time.Duration(100*(i+1)) * time.Millisecond)
			continue
		}

		// Verify it took effect
		actual, err := m.GetLUNFile(lunNumber)
		if err != nil {
			lastErr = err
			continue
		}
		if actual == filePath {
			return nil
		}
		lastErr = fmt.Errorf("LUN %d file mismatch: want %q, got %q", lunNumber, filePath, actual)
		time.Sleep(time.Duration(100*(i+1)) * time.Millisecond)
	}
	return fmt.Errorf("restore LUN %d after %d retries: %w", lunNumber, maxRetries, lastErr)
}
