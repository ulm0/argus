package wifi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

type ConnectionStatus struct {
	Connected bool   `json:"connected"`
	SSID      string `json:"ssid,omitempty"`
	Signal    int    `json:"signal,omitempty"`
	BSSID     string `json:"bssid,omitempty"`
	Frequency int    `json:"frequency,omitempty"`
}

type Network struct {
	SSID     string `json:"ssid"`
	Signal   int    `json:"signal"`
	Security string `json:"security"`
}

type Monitor struct {
	cfg    *config.Config
	mu     sync.RWMutex
	status ConnectionStatus
	stopCh chan struct{}
	onDisconnect func()
	onReconnect  func()
}

func NewMonitor(cfg *config.Config) *Monitor {
	return &Monitor{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
}

// SetCallbacks configures disconnect/reconnect handlers (e.g., for AP management).
func (m *Monitor) SetCallbacks(onDisconnect, onReconnect func()) {
	m.onDisconnect = onDisconnect
	m.onReconnect = onReconnect
}

// Start begins the WiFi monitoring goroutine.
func (m *Monitor) Start(ctx context.Context) {
	go m.monitorLoop(ctx)
}

func (m *Monitor) monitorLoop(ctx context.Context) {
	interval := time.Duration(m.cfg.OfflineAP.CheckInterval) * time.Second
	grace := time.Duration(m.cfg.OfflineAP.DisconnectGrace) * time.Second

	var disconnectedSince *time.Time
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			ok := m.checkWifi()

			m.mu.Lock()
			if ok {
				if disconnectedSince != nil && m.onReconnect != nil {
					logger.L.Info("WiFi reconnected")
					m.onReconnect()
				}
				disconnectedSince = nil
			} else {
				if disconnectedSince == nil {
					now := time.Now()
					disconnectedSince = &now
				} else if time.Since(*disconnectedSince) > grace {
					if m.onDisconnect != nil {
						logger.L.Warn("WiFi disconnected beyond grace period")
						m.onDisconnect()
					}
				}
			}
			m.mu.Unlock()
		}
	}
}

func (m *Monitor) checkWifi() bool {
	// Check link status
	if !m.linkUp() {
		m.updateStatus(false, "", 0)
		return false
	}

	// Check IP
	if !m.ipReady() {
		m.updateStatus(false, "", 0)
		return false
	}

	// Ping check
	if !m.pingOK() {
		m.updateStatus(false, "", 0)
		return false
	}

	// Get connection details
	status := m.getConnectionInfo()
	m.mu.Lock()
	m.status = status
	m.mu.Unlock()

	return status.Connected
}

func (m *Monitor) linkUp() bool {
	data, err := os.ReadFile("/sys/class/net/wlan0/operstate")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "up"
}

func (m *Monitor) ipReady() bool {
	iface, err := net.InterfaceByName("wlan0")
	if err != nil {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	return len(addrs) > 0
}

func (m *Monitor) pingOK() bool {
	cmd := exec.Command("ping", "-c", "1", "-W", "3", m.cfg.OfflineAP.PingTarget)
	return cmd.Run() == nil
}

func (m *Monitor) getConnectionInfo() ConnectionStatus {
	out, err := exec.Command("nmcli", "-t", "-f", "ACTIVE,SSID,SIGNAL", "dev", "wifi").Output()
	if err != nil {
		return ConnectionStatus{}
	}

	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 && parts[0] == "yes" {
			var signal int
			fmt.Sscanf(parts[2], "%d", &signal)
			return ConnectionStatus{
				Connected: true,
				SSID:      parts[1],
				Signal:    signal,
			}
		}
	}
	return ConnectionStatus{}
}

func (m *Monitor) updateStatus(connected bool, ssid string, signal int) {
	m.mu.Lock()
	m.status = ConnectionStatus{Connected: connected, SSID: ssid, Signal: signal}
	m.mu.Unlock()
}

// GetStatus returns the current WiFi connection status.
func (m *Monitor) GetStatus() ConnectionStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// GetCurrentConnection returns current WiFi info using nmcli and iw.
func (m *Monitor) GetCurrentConnection() ConnectionStatus {
	return m.getConnectionInfo()
}

// GetRSSI returns the current WiFi signal strength in dBm.
func (m *Monitor) GetRSSI() int {
	out, err := exec.Command("iw", "dev", "wlan0", "link").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "signal:") {
			var rssi int
			fmt.Sscanf(line, "signal: %d dBm", &rssi)
			return rssi
		}
	}
	return 0
}

// ScanNetworks scans for available WiFi networks.
func (m *Monitor) ScanNetworks(rescan bool) ([]Network, error) {
	if rescan {
		exec.Command("sudo", "-n", "nmcli", "dev", "wifi", "rescan").Run()
		time.Sleep(2 * time.Second)
	}

	out, err := exec.Command("sudo", "-n", "nmcli", "-t", "-f", "SSID,SIGNAL,SECURITY", "dev", "wifi", "list").Output()
	if err != nil {
		return nil, fmt.Errorf("scan networks: %w", err)
	}

	seen := make(map[string]bool)
	var networks []Network
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 || parts[0] == "" {
			continue
		}
		if seen[parts[0]] {
			continue
		}
		seen[parts[0]] = true

		var signal int
		fmt.Sscanf(parts[1], "%d", &signal)
		networks = append(networks, Network{
			SSID:     parts[0],
			Signal:   signal,
			Security: parts[2],
		})
	}
	return networks, nil
}

// UpdateCredentials changes the WiFi SSID/password using NetworkManager.
func (m *Monitor) UpdateCredentials(ssid, password string) error {
	// Check if connection exists
	out, err := exec.Command("nmcli", "connection", "show").Output()
	if err != nil {
		return fmt.Errorf("list connections: %w", err)
	}

	if strings.Contains(string(out), ssid) {
		// Modify existing
		cmd := exec.Command("sudo", "-n", "nmcli", "connection", "modify", ssid,
			"wifi-sec.key-mgmt", "wpa-psk",
			"wifi-sec.psk", password)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("modify connection: %w", err)
		}
		cmd = exec.Command("sudo", "-n", "nmcli", "connection", "up", ssid)
		return cmd.Run()
	}

	// Create new connection
	cmd := exec.Command("sudo", "-n", "nmcli", "device", "wifi", "connect", ssid,
		"password", password)
	return cmd.Run()
}

// GetWifiChangeStatus reads the persistent WiFi status file.
func (m *Monitor) GetWifiChangeStatus() map[string]any {
	data, err := os.ReadFile("/tmp/argus_wifi_status.json")
	if err != nil {
		return nil
	}
	var status map[string]any
	json.Unmarshal(data, &status)
	return status
}

// ClearWifiChangeStatus removes the WiFi change status file.
func (m *Monitor) ClearWifiChangeStatus() {
	os.Remove("/tmp/argus_wifi_status.json")
}

// Stop halts the monitor goroutine.
func (m *Monitor) Stop() {
	close(m.stopCh)
}
