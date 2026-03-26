package gadget

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/logger"
)

const (
	gadgetBase  = "/sys/kernel/config/usb_gadget"
	gadgetName  = "argus"
	usbVendor   = "0x1d6b" // Linux Foundation
	usbProduct  = "0x0104" // Multifunction Composite Gadget
	manufacturer = "Argus"
	productName  = "Argus Mass Storage"
	serialNumber = "000000000001"
)

type Manager struct {
	mu        sync.Mutex
	gadgetDir string
}

type LUNConfig struct {
	Number   int
	File     string
	ReadOnly bool
	Removable bool
}

func NewManager() *Manager {
	return &Manager{
		gadgetDir: filepath.Join(gadgetBase, gadgetName),
	}
}

// Create sets up the USB gadget configfs structure without binding to UDC.
func (m *Manager) Create(luns []LUNConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.gadgetDir, 0755); err != nil {
		return fmt.Errorf("create gadget dir: %w", err)
	}

	writes := map[string]string{
		"idVendor":  usbVendor,
		"idProduct": usbProduct,
		"bcdUSB":    "0x0200",
		"bcdDevice": "0x0100",
	}
	for name, val := range writes {
		if err := m.writeFile(name, val); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	// Strings
	strDir := filepath.Join(m.gadgetDir, "strings", "0x409")
	if err := os.MkdirAll(strDir, 0755); err != nil {
		return fmt.Errorf("create strings dir: %w", err)
	}
	strWrites := map[string]string{
		"manufacturer": manufacturer,
		"product":      productName,
		"serialnumber": serialNumber,
	}
	for name, val := range strWrites {
		if err := writeFilePath(filepath.Join(strDir, name), val); err != nil {
			return fmt.Errorf("write string %s: %w", name, err)
		}
	}

	// Function: mass_storage
	funcDir := filepath.Join(m.gadgetDir, "functions", "mass_storage.usb0")
	if err := os.MkdirAll(funcDir, 0755); err != nil {
		return fmt.Errorf("create function dir: %w", err)
	}
	if err := writeFilePath(filepath.Join(funcDir, "stall"), "1"); err != nil {
		return fmt.Errorf("write stall: %w", err)
	}

	// Configure LUNs
	for _, lun := range luns {
		if err := m.configureLUN(funcDir, lun); err != nil {
			return fmt.Errorf("configure lun %d: %w", lun.Number, err)
		}
	}

	// Configuration
	cfgDir := filepath.Join(m.gadgetDir, "configs", "c.1")
	if err := os.MkdirAll(filepath.Join(cfgDir, "strings", "0x409"), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := writeFilePath(filepath.Join(cfgDir, "strings", "0x409", "configuration"), "Mass Storage Config"); err != nil {
		return fmt.Errorf("write config string: %w", err)
	}
	if err := writeFilePath(filepath.Join(cfgDir, "MaxPower"), "250"); err != nil {
		return fmt.Errorf("write MaxPower: %w", err)
	}

	// Link function to config
	linkPath := filepath.Join(cfgDir, "mass_storage.usb0")
	if _, err := os.Lstat(linkPath); os.IsNotExist(err) {
		if err := os.Symlink(funcDir, linkPath); err != nil {
			return fmt.Errorf("symlink function: %w", err)
		}
	}

	return nil
}

func (m *Manager) configureLUN(funcDir string, lun LUNConfig) error {
	lunDir := filepath.Join(funcDir, fmt.Sprintf("lun.%d", lun.Number))
	if err := os.MkdirAll(lunDir, 0755); err != nil {
		return err
	}

	ro := "0"
	if lun.ReadOnly {
		ro = "1"
	}
	removable := "0"
	if lun.Removable {
		removable = "1"
	}

	writes := map[string]string{
		"ro":        ro,
		"removable": removable,
		"nofua":     "1",
	}
	for name, val := range writes {
		if err := writeFilePath(filepath.Join(lunDir, name), val); err != nil {
			return err
		}
	}

	if lun.File != "" {
		if err := writeFilePath(filepath.Join(lunDir, "file"), lun.File); err != nil {
			return err
		}
	}

	return nil
}

// Bind attaches the gadget to the first available UDC.
func (m *Manager) Bind() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	udc, err := m.findUDC()
	if err != nil {
		return err
	}

	return m.writeFile("UDC", udc)
}

// Unbind detaches the gadget from UDC.
func (m *Manager) Unbind() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.writeFile("UDC", "")
}

// Rebind performs an unbind/bind cycle to force Tesla to re-enumerate the USB device.
func (m *Manager) Rebind(delay time.Duration) error {
	if err := m.Unbind(); err != nil {
		return fmt.Errorf("unbind: %w", err)
	}
	time.Sleep(delay)
	return m.Bind()
}

// Remove tears down the entire gadget configuration.
func (m *Manager) Remove() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Unbind first
	_ = writeFilePath(filepath.Join(m.gadgetDir, "UDC"), "")
	time.Sleep(100 * time.Millisecond)

	// Remove config symlinks
	cfgDir := filepath.Join(m.gadgetDir, "configs", "c.1")
	entries, _ := os.ReadDir(cfgDir)
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			os.Remove(filepath.Join(cfgDir, e.Name()))
		}
	}

	// Remove dirs in reverse order
	dirsToRemove := []string{
		filepath.Join(cfgDir, "strings", "0x409"),
		cfgDir,
		filepath.Join(m.gadgetDir, "functions", "mass_storage.usb0"),
		filepath.Join(m.gadgetDir, "strings", "0x409"),
		m.gadgetDir,
	}
	for _, d := range dirsToRemove {
		os.Remove(d)
	}

	return nil
}

// SetLUNFile updates the backing file for a LUN (e.g., to clear or restore it).
func (m *Manager) SetLUNFile(lunNumber int, filePath string) error {
	lunFile := filepath.Join(m.gadgetDir, "functions", "mass_storage.usb0",
		fmt.Sprintf("lun.%d", lunNumber), "file")
	return writeFilePath(lunFile, filePath)
}

// GetLUNFile reads the current backing file for a LUN.
func (m *Manager) GetLUNFile(lunNumber int) (string, error) {
	lunFile := filepath.Join(m.gadgetDir, "functions", "mass_storage.usb0",
		fmt.Sprintf("lun.%d", lunNumber), "file")
	data, err := os.ReadFile(lunFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// IsPresent checks if the gadget is currently bound to a UDC.
func (m *Manager) IsPresent() bool {
	data, err := os.ReadFile(filepath.Join(m.gadgetDir, "UDC"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) != ""
}

func (m *Manager) findUDC() (string, error) {
	entries, err := os.ReadDir("/sys/class/udc")
	if err != nil {
		return "", fmt.Errorf("read UDC list: %w", err)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("no UDC controllers available")
	}
	udc := entries[0].Name()
	logger.L.WithField("udc", udc).Info("found UDC controller")
	return udc, nil
}

func (m *Manager) writeFile(name, value string) error {
	return writeFilePath(filepath.Join(m.gadgetDir, name), value)
}

func writeFilePath(path, value string) error {
	return os.WriteFile(path, []byte(value), 0644)
}
