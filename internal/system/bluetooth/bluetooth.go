// Package bluetooth manages Bluetooth PAN (Personal Area Network) tethering so
// the Pi can pull internet from a paired phone — the same mechanism the CarlinKit
// T2C uses. The phone shares its data connection over Bluetooth (NAP profile) and
// the Pi consumes it as a PANU, getting a bnep0 interface with a default route.
//
// Pairing is expected to be done once out-of-band (the phone initiates pairing
// with the Pi); this package then connects/disconnects the already-paired device
// and reports status. It shells out to bluetoothctl/nmcli — no extra deps — to
// keep the on-device footprint small.
package bluetooth

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/ulm0/argus/internal/logger"
)

// panInterface is the kernel interface a Bluetooth PAN link shows up as.
const panInterface = "bnep0"

var macRe = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)

// Device is a paired Bluetooth device.
type Device struct {
	MAC       string `json:"mac"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
}

// Status describes the current Bluetooth tethering state.
type Status struct {
	Available  bool   `json:"available"` // bluetooth stack reachable
	Tethered   bool   `json:"tethered"`  // bnep0 is up (PAN active)
	Interface  string `json:"interface,omitempty"`
	DeviceMAC  string `json:"device_mac,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
}

type Manager struct{}

func NewManager() *Manager { return &Manager{} }

// isValidMAC guards every value passed to bluetoothctl/nmcli so it can't be a
// flag or carry shell metacharacters.
func isValidMAC(mac string) bool {
	return macRe.MatchString(mac)
}

// GetStatus reports whether a PAN link is up and which device backs it.
func (m *Manager) GetStatus() Status {
	s := Status{}

	// bluetoothctl present and the controller powered?
	if out, err := exec.Command("bluetoothctl", "show").CombinedOutput(); err == nil {
		s.Available = true
		if strings.Contains(string(out), "Powered: yes") {
			s.Available = true
		}
	}

	// bnep0 existing and up means PAN is active.
	if out, err := exec.Command("ip", "-o", "link", "show", panInterface).Output(); err == nil && len(out) > 0 {
		s.Tethered = strings.Contains(string(out), "state UP") || strings.Contains(string(out), "LOWER_UP")
		s.Interface = panInterface
	}

	if s.Tethered {
		for _, d := range m.listDevices() {
			if d.Connected {
				s.DeviceMAC = d.MAC
				s.DeviceName = d.Name
				break
			}
		}
	}
	return s
}

// ListDevices returns the paired Bluetooth devices.
func (m *Manager) ListDevices() []Device {
	return m.listDevices()
}

func (m *Manager) listDevices() []Device {
	// `bluetoothctl devices Paired` lists "Device <MAC> <Name>" lines.
	out, err := exec.Command("bluetoothctl", "devices", "Paired").Output()
	if err != nil {
		// Older bluetoothctl: fall back to `paired-devices`.
		out, err = exec.Command("bluetoothctl", "paired-devices").Output()
		if err != nil {
			return nil
		}
	}

	var devices []Device
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(fields) < 3 || fields[0] != "Device" {
			continue
		}
		mac := fields[1]
		if !isValidMAC(mac) {
			continue
		}
		devices = append(devices, Device{
			MAC:       mac,
			Name:      fields[2],
			Connected: deviceConnected(mac),
		})
	}
	return devices
}

// deviceConnected reports whether a paired device currently has an active link.
func deviceConnected(mac string) bool {
	if !isValidMAC(mac) {
		return false
	}
	out, err := exec.Command("bluetoothctl", "info", mac).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Connected: yes")
}

// Connect trusts and connects a paired device, bringing up its PAN profile so
// the Pi gains internet through it.
func (m *Manager) Connect(mac string) error {
	if !isValidMAC(mac) {
		return fmt.Errorf("invalid MAC address")
	}
	// Trust so the link auto-reconnects across reboots.
	exec.Command("bluetoothctl", "trust", mac).Run()

	if out, err := exec.Command("bluetoothctl", "connect", mac).CombinedOutput(); err != nil {
		return fmt.Errorf("bluetooth connect: %s", strings.TrimSpace(string(out)))
	}

	// Ask NetworkManager to bring up the PAN (NAP) connection if it manages one.
	// Best-effort: on many setups bluetoothctl connect already yields bnep0.
	if err := exec.Command("nmcli", "device", "connect", mac).Run(); err != nil {
		logger.L.WithField("mac", mac).Debug("nmcli device connect returned non-zero (PAN may already be up)")
	}
	return nil
}

// Disconnect tears down the PAN link for a device.
func (m *Manager) Disconnect(mac string) error {
	if !isValidMAC(mac) {
		return fmt.Errorf("invalid MAC address")
	}
	if out, err := exec.Command("bluetoothctl", "disconnect", mac).CombinedOutput(); err != nil {
		return fmt.Errorf("bluetooth disconnect: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
