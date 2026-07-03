package wifi

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
)

type Connection struct {
	Connected bool   `json:"connected"`
	SSID      string `json:"ssid,omitempty"`
	Signal    int    `json:"signal,omitempty"`
	Frequency string `json:"frequency,omitempty"`
	IP        string `json:"ip,omitempty"`
}

type Network struct {
	SSID      string `json:"ssid"`
	Signal    int    `json:"signal"`
	Frequency string `json:"frequency"`
	Security  string `json:"security"`
	InUse     bool   `json:"in_use"`
}

type ChangeStatus struct {
	Pending   bool      `json:"pending"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

type Monitor struct {
	cfg          *config.Config
	mu           sync.Mutex
	changeStatus *ChangeStatus
}

func NewMonitor(cfg *config.Config) *Monitor {
	return &Monitor{
		cfg:          cfg,
		changeStatus: &ChangeStatus{},
	}
}

// GetCurrentConnection returns the current WiFi connection info.
func (m *Monitor) GetCurrentConnection() Connection {
	conn := Connection{}

	out, err := exec.Command("iwgetid", "-r").CombinedOutput()
	if err == nil {
		ssid := strings.TrimSpace(string(out))
		if ssid != "" {
			conn.Connected = true
			conn.SSID = ssid
		}
	}

	if conn.Connected {
		if out, err := exec.Command("iwconfig", "wlan0").CombinedOutput(); err == nil {
			output := string(out)
			if idx := strings.Index(output, "Signal level="); idx >= 0 {
				var sig int
				fmt.Sscanf(output[idx:], "Signal level=%d", &sig)
				conn.Signal = sig
			}
			if idx := strings.Index(output, "Frequency:"); idx >= 0 {
				var freq string
				fmt.Sscanf(output[idx:], "Frequency:%s", &freq)
				conn.Frequency = freq
			}
		}

		if out, err := exec.Command("hostname", "-I").CombinedOutput(); err == nil {
			ips := strings.Fields(string(out))
			if len(ips) > 0 {
				conn.IP = ips[0]
			}
		}
	}

	return conn
}

// ScanNetworks returns available WiFi networks.
func (m *Monitor) ScanNetworks() ([]Network, error) {
	// SSID is last because it may contain escaped colons in terse output; the
	// preceding fields never do, so SplitN with the field count is safe.
	out, err := exec.Command("nmcli", "-t", "-f", "SIGNAL,FREQ,SECURITY,IN-USE,SSID", "dev", "wifi", "list", "--rescan", "yes").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("wifi scan failed: %w", err)
	}

	var networks []Network
	seen := make(map[string]bool)

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, ":", 5)
		if len(fields) < 5 {
			continue
		}
		ssid := unescapeNmcli(fields[4])
		if ssid == "" || seen[ssid] {
			continue
		}
		seen[ssid] = true

		var signal int
		fmt.Sscanf(fields[0], "%d", &signal)

		networks = append(networks, Network{
			SSID:      ssid,
			Signal:    signal,
			Frequency: fields[1],
			Security:  fields[2],
			InUse:     fields[3] == "*",
		})
	}

	return networks, nil
}

// UpdateCredentials sets new WiFi credentials and reconnects.
func (m *Monitor) UpdateCredentials(ssid, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ssid == "" {
		return fmt.Errorf("SSID is required")
	}
	// Reject leading '-' so nmcli can't parse the value as an option flag.
	if strings.HasPrefix(ssid, "-") || strings.HasPrefix(password, "-") {
		return fmt.Errorf("SSID and password must not start with '-'")
	}

	var cmd *exec.Cmd
	if password != "" {
		cmd = exec.Command("nmcli", "dev", "wifi", "connect", ssid, "password", password)
	} else {
		cmd = exec.Command("nmcli", "dev", "wifi", "connect", ssid)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		m.changeStatus = &ChangeStatus{
			Pending:   true,
			Message:   fmt.Sprintf("Failed to connect to %s: %s", ssid, strings.TrimSpace(string(out))),
			Timestamp: time.Now(),
		}
		return fmt.Errorf("connect failed: %s", strings.TrimSpace(string(out)))
	}

	m.changeStatus = &ChangeStatus{
		Pending:   true,
		Message:   fmt.Sprintf("Successfully connected to %s", ssid),
		Timestamp: time.Now(),
	}

	return nil
}

// SavedNetwork is a stored NetworkManager WiFi profile (a network the device
// has joined before and may auto-join again).
type SavedNetwork struct {
	Name        string `json:"name"`
	UUID        string `json:"uuid"`
	AutoConnect bool   `json:"autoconnect"`
	Priority    int    `json:"priority"`
	Active      bool   `json:"active"`
}

// ListSavedNetworks returns the stored WiFi connection profiles, newest-priority
// first, so the UI can manage past and future auto-join networks.
func (m *Monitor) ListSavedNetworks() ([]SavedNetwork, error) {
	// NAME is last because it may contain escaped colons in terse output; the
	// preceding fields never do, so SplitN with the field count is safe.
	out, err := exec.Command("nmcli", "-t",
		"-f", "UUID,TYPE,AUTOCONNECT,AUTOCONNECT-PRIORITY,ACTIVE,NAME",
		"connection", "show").Output()
	if err != nil {
		return nil, fmt.Errorf("list saved networks: %w", err)
	}

	var nets []SavedNetwork
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, ":", 6)
		if len(f) < 6 || f[1] != "802-11-wireless" {
			continue
		}
		var prio int
		fmt.Sscanf(f[3], "%d", &prio)
		nets = append(nets, SavedNetwork{
			UUID:        f[0],
			AutoConnect: f[2] == "yes",
			Priority:    prio,
			Active:      f[4] == "yes",
			Name:        unescapeNmcli(f[5]),
		})
	}
	return nets, nil
}

// ForgetNetwork deletes a saved WiFi profile by UUID.
func (m *Monitor) ForgetNetwork(uuid string) error {
	if !isValidUUID(uuid) {
		return fmt.Errorf("invalid connection UUID")
	}
	out, err := exec.Command("nmcli", "connection", "delete", "uuid", uuid).CombinedOutput()
	if err != nil {
		return fmt.Errorf("forget network: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// SetAutoConnect enables or disables auto-join for a saved WiFi profile.
func (m *Monitor) SetAutoConnect(uuid string, enabled bool) error {
	if !isValidUUID(uuid) {
		return fmt.Errorf("invalid connection UUID")
	}
	val := "no"
	if enabled {
		val = "yes"
	}
	out, err := exec.Command("nmcli", "connection", "modify", "uuid", uuid, "connection.autoconnect", val).CombinedOutput()
	if err != nil {
		return fmt.Errorf("set autoconnect: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// isValidUUID checks that s looks like a NetworkManager UUID (hex and dashes),
// which also prevents it from being parsed by nmcli as an option flag.
func isValidUUID(s string) bool {
	if len(s) < 8 || len(s) > 64 {
		return false
	}
	if s[0] == '-' {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '-') {
			return false
		}
	}
	return true
}

// unescapeNmcli reverses nmcli terse-mode escaping for ':' and '\'.
func unescapeNmcli(s string) string {
	s = strings.ReplaceAll(s, `\:`, ":")
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

// GetWifiChangeStatus returns the status of the last WiFi change operation.
func (m *Monitor) GetWifiChangeStatus() ChangeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.changeStatus == nil {
		return ChangeStatus{}
	}
	return *m.changeStatus
}

// ClearWifiChangeStatus clears the pending WiFi change notification.
func (m *Monitor) ClearWifiChangeStatus() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changeStatus = &ChangeStatus{}
}
