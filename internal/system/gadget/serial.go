package gadget

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const serialFileName = "usb_gadget_serial.txt"

// LoadOrCreateSerial returns a stable 15-character serial for USB string descriptors,
// persisted under gadgetDir (same idea as TeslaUSB truncating a UUID).
func LoadOrCreateSerial(gadgetDir string) (string, error) {
	p := filepath.Join(gadgetDir, serialFileName)
	if data, err := os.ReadFile(p); err == nil {
		s := strings.TrimSpace(string(data))
		if len(s) >= 8 && len(s) <= 32 {
			return s, nil
		}
	}
	// 8 random bytes -> 16 hex chars, take first 15 to match TeslaUSB-style length
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate serial: %w", err)
	}
	s := hex.EncodeToString(b)[:15]
	if err := os.WriteFile(p, []byte(s+"\n"), 0644); err != nil {
		return "", fmt.Errorf("persist serial: %w", err)
	}
	return s, nil
}
