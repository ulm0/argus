package gadget

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/logger"
)

const (
	configfsMount = "/sys/kernel/config"
	gadgetBase    = "/sys/kernel/config/usb_gadget"
	gadgetName    = "argus"
	usbVendor     = "0x1d6b" // Linux Foundation
	usbProduct    = "0x0104" // Multifunction Composite Gadget
	manufacturer  = "Argus"
	productName   = "Argus Mass Storage"
	maxPowerMA    = "500" // Same as TeslaUSB (mA)
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
// serial is written to the USB string descriptor (e.g. from LoadOrCreateSerial).
func (m *Manager) Create(luns []LUNConfig, serial string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ensureConfigFS(); err != nil {
		return fmt.Errorf("configfs: %w", err)
	}

	if strings.TrimSpace(serial) == "" {
		serial = "0000000000000"
	}

	// usb_gadget is created by kernel/libcomposite, not by userspace.
	// Create only our gadget node under that kernel-managed base path.
	if _, err := os.Stat(m.gadgetDir); os.IsNotExist(err) {
		if err := os.Mkdir(m.gadgetDir, 0755); err != nil {
			return fmt.Errorf("create gadget dir: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("stat gadget dir: %w", err)
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
		"serialnumber": serial,
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
	if err := writeFilePath(filepath.Join(cfgDir, "MaxPower"), maxPowerMA); err != nil {
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
		"cdrom":     "0",
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

// Remove tears down the entire gadget configuration (TeslaUSB-style ordering).
func (m *Manager) Remove() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(m.gadgetDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	// Unbind UDC first
	_ = writeFilePath(filepath.Join(m.gadgetDir, "UDC"), "")
	time.Sleep(300 * time.Millisecond)

	funcDir := filepath.Join(m.gadgetDir, "functions", "mass_storage.usb0")
	entries, _ := filepath.Glob(filepath.Join(funcDir, "lun.*"))
	for _, lunDir := range entries {
		fi, err := os.Stat(lunDir)
		if err != nil || !fi.IsDir() {
			continue
		}
		filePath := filepath.Join(lunDir, "file")
		if _, err := os.Stat(filePath); err == nil {
			_ = writeFilePath(filePath, "")
		}
	}
	time.Sleep(100 * time.Millisecond)

	cfgDir := filepath.Join(m.gadgetDir, "configs", "c.1")
	cfgEntries, _ := os.ReadDir(cfgDir)
	for _, e := range cfgEntries {
		if e.Type()&os.ModeSymlink != 0 {
			_ = os.Remove(filepath.Join(cfgDir, e.Name()))
		}
	}

	_ = os.Remove(filepath.Join(cfgDir, "strings", "0x409", "configuration"))
	_ = os.Remove(filepath.Join(cfgDir, "strings", "0x409"))
	_ = os.Remove(filepath.Join(cfgDir, "strings"))
	_ = os.Remove(filepath.Join(cfgDir, "MaxPower"))
	_ = os.Remove(cfgDir)

	for _, lunDir := range entries {
		fi, err := os.Stat(lunDir)
		if err != nil || !fi.IsDir() {
			continue
		}
		_ = os.Remove(filepath.Join(lunDir, "cdrom"))
		_ = os.Remove(filepath.Join(lunDir, "file"))
		_ = os.Remove(filepath.Join(lunDir, "nofua"))
		_ = os.Remove(filepath.Join(lunDir, "removable"))
		_ = os.Remove(filepath.Join(lunDir, "ro"))
		_ = os.Remove(lunDir)
	}
	_ = os.Remove(filepath.Join(funcDir, "stall"))
	_ = os.Remove(funcDir)

	strDir := filepath.Join(m.gadgetDir, "strings", "0x409")
	_ = os.Remove(filepath.Join(strDir, "manufacturer"))
	_ = os.Remove(filepath.Join(strDir, "product"))
	_ = os.Remove(filepath.Join(strDir, "serialnumber"))
	_ = os.Remove(strDir)
	_ = os.Remove(filepath.Join(m.gadgetDir, "strings"))

	_ = os.Remove(filepath.Join(m.gadgetDir, "bcdDevice"))
	_ = os.Remove(filepath.Join(m.gadgetDir, "bcdUSB"))
	_ = os.Remove(filepath.Join(m.gadgetDir, "idProduct"))
	_ = os.Remove(filepath.Join(m.gadgetDir, "idVendor"))
	_ = os.Remove(filepath.Join(m.gadgetDir, "UDC"))
	_ = os.Remove(m.gadgetDir)

	return nil
}

// GadgetDir returns the configfs path for this gadget.
func (m *Manager) GadgetDir() string { return m.gadgetDir }

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

// ensureConfigFS checks if configfs is mounted and mounts it if necessary.
func ensureConfigFS() error {
	// Load modules explicitly (same family of behavior TeslaUSB expects on Pi).
	if out, err := exec.Command("modprobe", "configfs").CombinedOutput(); err != nil {
		return fmt.Errorf("modprobe configfs: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("modprobe", "libcomposite").CombinedOutput(); err != nil {
		return fmt.Errorf("modprobe libcomposite: %s: %w", strings.TrimSpace(string(out)), err)
	}

	if !isConfigFSMounted() {
		logger.L.Info("configfs not mounted, mounting at " + configfsMount)
		if err := os.MkdirAll(configfsMount, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", configfsMount, err)
		}
		cmd := exec.Command("mount", "-t", "configfs", "none", configfsMount)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mount configfs: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	// libcomposite should expose /sys/kernel/config/usb_gadget.
	if st, err := os.Stat(gadgetBase); err != nil || !st.IsDir() {
		return fmt.Errorf("%s unavailable (libcomposite not active or kernel missing USB gadget support)", gadgetBase)
	}
	return nil
}

func isConfigFSMounted() bool {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[1] == configfsMount && fields[2] == "configfs" {
			return true
		}
	}
	return false
}
