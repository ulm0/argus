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
	SSID       string `json:"ssid"`
	Signal     int    `json:"signal"`
	Frequency  string `json:"frequency"`
	Security   string `json:"security"`
	InUse      bool   `json:"in_use"`
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
	out, err := exec.Command("nmcli", "-t", "-f", "SSID,SIGNAL,FREQ,SECURITY,IN-USE", "dev", "wifi", "list", "--rescan", "yes").CombinedOutput()
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
		ssid := fields[0]
		if ssid == "" || seen[ssid] {
			continue
		}
		seen[ssid] = true

		var signal int
		fmt.Sscanf(fields[1], "%d", &signal)

		networks = append(networks, Network{
			SSID:      ssid,
			Signal:    signal,
			Frequency: fields[2],
			Security:  fields[3],
			InUse:     fields[4] == "*",
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
