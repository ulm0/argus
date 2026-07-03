// Package bluetooth manages Bluetooth PAN (Personal Area Network) tethering so
// the Pi can pull internet from a paired phone — the same mechanism the CarlinKit
// T2C uses. The phone shares its data connection over Bluetooth (NAP profile) and
// the Pi consumes it as a PANU, getting a bnep0 interface with a default route.
//
// It also exposes the controller plumbing needed to get to that point on a
// headless device: power the adapter, scan for nearby devices, make the Pi
// discoverable/pairable (with an auto-accepting agent so a phone can pair without
// a screen), pair, and forget. Everything shells out to bluetoothctl/nmcli — no
// extra deps — to keep the on-device footprint small.
package bluetooth

import (
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/ulm0/argus/internal/logger"
)

// panInterface is the kernel interface a Bluetooth PAN link shows up as.
const panInterface = "bnep0"

// maxScanSeconds bounds a discovery scan so a caller can't pin the adapter.
const maxScanSeconds = 30

var macRe = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)

// Device is a Bluetooth device known to the adapter (paired or just discovered).
type Device struct {
	MAC       string `json:"mac"`
	Name      string `json:"name"`
	Paired    bool   `json:"paired"`
	Connected bool   `json:"connected"`
}

// Status describes the current Bluetooth controller and tethering state.
type Status struct {
	Available    bool   `json:"available"`    // bluetoothctl/controller reachable
	Powered      bool   `json:"powered"`      // adapter powered on
	Discoverable bool   `json:"discoverable"` // adapter visible + pairable
	Tethered     bool   `json:"tethered"`     // bnep0 is up (PAN active)
	Interface    string `json:"interface,omitempty"`
	DeviceMAC    string `json:"device_mac,omitempty"`
	DeviceName   string `json:"device_name,omitempty"`
}

type Manager struct {
	mu       sync.Mutex
	agentCmd *exec.Cmd      // persistent bluetoothctl holding the pairing agent
	agentIn  io.WriteCloser // its stdin, kept open to keep the agent registered
}

func NewManager() *Manager { return &Manager{} }

// isValidMAC guards every value passed to bluetoothctl/nmcli so it can't be a
// flag or carry shell metacharacters.
func isValidMAC(mac string) bool {
	return macRe.MatchString(mac)
}

// GetStatus reports controller power/discoverability and the PAN link state.
func (m *Manager) GetStatus() Status {
	s := Status{}

	if out, err := exec.Command("bluetoothctl", "show").CombinedOutput(); err == nil {
		text := string(out)
		s.Available = true
		s.Powered = strings.Contains(text, "Powered: yes")
		s.Discoverable = strings.Contains(text, "Discoverable: yes")
	}

	// bnep0 existing and up means PAN is active.
	if out, err := exec.Command("ip", "-o", "link", "show", panInterface).Output(); err == nil && len(out) > 0 {
		s.Tethered = strings.Contains(string(out), "state UP") || strings.Contains(string(out), "LOWER_UP")
		s.Interface = panInterface
	}

	if s.Tethered {
		for _, d := range m.listDevices("Paired") {
			if d.Connected {
				s.DeviceMAC = d.MAC
				s.DeviceName = d.Name
				break
			}
		}
	}
	return s
}

// SetPower powers the Bluetooth adapter on or off. Powering off also tears down
// the pairing agent.
func (m *Manager) SetPower(on bool) error {
	val := "off"
	if on {
		val = "on"
	}
	if out, err := exec.Command("bluetoothctl", "power", val).CombinedOutput(); err != nil {
		return fmt.Errorf("set power: %s", strings.TrimSpace(string(out)))
	}
	if !on {
		m.mu.Lock()
		m.stopAgentLocked()
		m.mu.Unlock()
	}
	return nil
}

// SetDiscoverable makes the Pi visible and pairable (so a phone can initiate
// pairing) and registers an auto-accepting agent for headless "just works"
// pairing. Turning it off hides the adapter and stops the agent.
func (m *Manager) SetDiscoverable(on bool) error {
	if on {
		exec.Command("bluetoothctl", "power", "on").Run()
		m.mu.Lock()
		err := m.startAgentLocked()
		m.mu.Unlock()
		if err != nil {
			logger.L.WithError(err).Warn("bluetooth: could not start pairing agent")
		}
		// Stay discoverable until explicitly turned off.
		exec.Command("bluetoothctl", "discoverable-timeout", "0").Run()
		if out, err := exec.Command("bluetoothctl", "discoverable", "on").CombinedOutput(); err != nil {
			return fmt.Errorf("discoverable on: %s", strings.TrimSpace(string(out)))
		}
		exec.Command("bluetoothctl", "pairable", "on").Run()
		return nil
	}

	exec.Command("bluetoothctl", "discoverable", "off").Run()
	exec.Command("bluetoothctl", "pairable", "off").Run()
	m.mu.Lock()
	m.stopAgentLocked()
	m.mu.Unlock()
	return nil
}

// Scan runs a timed discovery and returns the devices the adapter then knows
// about (paired and freshly discovered).
func (m *Manager) Scan(seconds int) ([]Device, error) {
	if seconds <= 0 || seconds > maxScanSeconds {
		seconds = 10
	}
	exec.Command("bluetoothctl", "power", "on").Run()
	if out, err := exec.Command("bluetoothctl", "--timeout", fmt.Sprintf("%d", seconds), "scan", "on").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("scan: %s", strings.TrimSpace(string(out)))
	}
	return m.listDevices(""), nil
}

// Pair pairs, trusts and connects a device — the full bring-up for a phone the
// Pi will tether through. Requires the device to be in range/discoverable.
func (m *Manager) Pair(mac string) error {
	if !isValidMAC(mac) {
		return fmt.Errorf("invalid MAC address")
	}
	if out, err := exec.Command("bluetoothctl", "pair", mac).CombinedOutput(); err != nil {
		return fmt.Errorf("pair: %s", strings.TrimSpace(string(out)))
	}
	exec.Command("bluetoothctl", "trust", mac).Run()
	exec.Command("bluetoothctl", "connect", mac).Run()
	return nil
}

// Remove unpairs and forgets a device.
func (m *Manager) Remove(mac string) error {
	if !isValidMAC(mac) {
		return fmt.Errorf("invalid MAC address")
	}
	if out, err := exec.Command("bluetoothctl", "remove", mac).CombinedOutput(); err != nil {
		return fmt.Errorf("remove: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// ListDevices returns the paired Bluetooth devices.
func (m *Manager) ListDevices() []Device {
	return m.listDevices("Paired")
}

// listDevices runs `bluetoothctl devices [filter]` ("" = all known) and resolves
// each device's paired/connected state.
func (m *Manager) listDevices(filter string) []Device {
	args := []string{"devices"}
	if filter != "" {
		args = append(args, filter)
	}
	out, err := exec.Command("bluetoothctl", args...).Output()
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
		paired, connected := deviceState(mac)
		devices = append(devices, Device{
			MAC:       mac,
			Name:      fields[2],
			Paired:    paired,
			Connected: connected,
		})
	}
	return devices
}

// deviceState reports a device's paired and connected flags from bluetoothctl.
func deviceState(mac string) (paired, connected bool) {
	if !isValidMAC(mac) {
		return false, false
	}
	out, err := exec.Command("bluetoothctl", "info", mac).Output()
	if err != nil {
		return false, false
	}
	text := string(out)
	return strings.Contains(text, "Paired: yes"), strings.Contains(text, "Connected: yes")
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

// startAgentLocked launches a persistent bluetoothctl that holds a
// NoInputNoOutput agent, so incoming "just works" pairing requests from a phone
// are auto-accepted while the Pi is discoverable. Caller holds m.mu.
func (m *Manager) startAgentLocked() error {
	if m.agentCmd != nil {
		return nil
	}
	cmd := exec.Command("bluetoothctl")
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	io.WriteString(in, "agent NoInputNoOutput\ndefault-agent\n")
	m.agentCmd = cmd
	m.agentIn = in
	go func() {
		cmd.Wait() //nolint:errcheck
		// If this agent exits on its own (bluetoothd restart, adapter reset,
		// OOM/crash), clear the stale handle so the next startAgentLocked
		// relaunches it instead of no-oping on a dead process.
		m.mu.Lock()
		if m.agentCmd == cmd {
			m.agentCmd = nil
			m.agentIn = nil
		}
		m.mu.Unlock()
	}()
	return nil
}

// stopAgentLocked tears down the persistent agent process. Caller holds m.mu.
func (m *Manager) stopAgentLocked() {
	if m.agentCmd == nil {
		return
	}
	if m.agentIn != nil {
		io.WriteString(m.agentIn, "quit\n")
		m.agentIn.Close()
	}
	if m.agentCmd.Process != nil {
		m.agentCmd.Process.Kill()
	}
	m.agentCmd = nil
	m.agentIn = nil
}
