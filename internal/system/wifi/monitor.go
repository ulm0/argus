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
	cfg          *config.Config
	mu           sync.RWMutex
	status       ConnectionStatus
	stopCh       chan struct{}
	stopOnce     sync.Once
	onDisconnect func()
	onReconnect  func()
	connInfoExp  time.Time // protected by mu; refreshes getConnectionInfo every 5 min
}

func NewMonitor(cfg *config.Config) *Monitor {
	return &Monitor{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
}

// SetCallbacks configures disconnect/reconnect handlers (e.g., for AP management).
func (m *Monitor) SetCallbacks(onDisconnect, onReconnect func()) {
	m.mu.Lock()
	m.onDisconnect = onDisconnect
	m.onReconnect = onReconnect
	m.mu.Unlock()
}

// fireDisconnect / fireReconnect snapshot the callback under the lock and invoke
// it *outside* the lock — the AP start/stop they trigger blocks for seconds, and
// holding m.mu across it would stall every status reader.
func (m *Monitor) fireDisconnect() {
	m.mu.RLock()
	cb := m.onDisconnect
	m.mu.RUnlock()
	if cb != nil {
		cb()
	}
}

func (m *Monitor) fireReconnect() {
	m.mu.RLock()
	cb := m.onReconnect
	m.mu.RUnlock()
	if cb != nil {
		cb()
	}
}

// Start begins the WiFi monitoring goroutine.
func (m *Monitor) Start(ctx context.Context) {
	go m.monitorLoop(ctx)
}

func (m *Monitor) monitorLoop(ctx context.Context) {
	interval := time.Duration(m.cfg.OfflineAP.CheckInterval) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	grace := time.Duration(m.cfg.OfflineAP.DisconnectGrace) * time.Second

	// How long the AP fallback is allowed to hold the radio before we free it to
	// retry the STA. Without this, the AP keeps wlan0 busy, the STA can never
	// rejoin another saved network, and the only way out is a reboot.
	retry := time.Duration(m.cfg.OfflineAP.RetrySeconds) * time.Second
	if retry <= 0 {
		retry = 2 * time.Minute
	}

	var disconnectedSince *time.Time
	var apUp bool
	var lastReconnect time.Time

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			if m.checkWifi() {
				if apUp {
					logger.L.Info("WiFi reconnected; stopping AP fallback")
					m.fireReconnect()
					apUp = false
				}
				disconnectedSince = nil
				continue
			}

			// Disconnected.
			if disconnectedSince == nil {
				now := time.Now()
				disconnectedSince = &now
			}

			if !apUp {
				// Actively nudge a reconnect to a saved network — NM's own
				// autoconnect backoff can be slow to retry on its own.
				if time.Since(lastReconnect) >= interval {
					m.attemptReconnect()
					lastReconnect = time.Now()
				}
				if time.Since(*disconnectedSince) > grace {
					logger.L.Warn("WiFi down beyond grace; starting AP fallback")
					m.fireDisconnect()
					apUp = true
					lastReconnect = time.Now()
				}
			} else if time.Since(lastReconnect) > retry {
				// Free the radio so the STA can scan and rejoin another saved
				// network, then give it a fresh grace window before the AP returns.
				logger.L.Info("AP fallback active; freeing radio to retry WiFi")
				m.fireReconnect()
				apUp = false
				m.attemptReconnect()
				lastReconnect = time.Now()
				disconnectedSince = nil
			}
		}
	}
}

// attemptReconnect forces a rescan and asks NetworkManager to activate the best
// available saved profile on the STA interface. Bounded with -w so a failing
// activation can't stall the monitor loop.
func (m *Monitor) attemptReconnect() {
	exec.Command("nmcli", "radio", "wifi", "on").Run()
	exec.Command("nmcli", "device", "wifi", "rescan").Run()
	if iface := m.cfg.OfflineAP.Interface; iface != "" {
		exec.Command("nmcli", "-w", "20", "device", "connect", iface).Run()
	}
}

func (m *Monitor) checkWifi() bool {
	if !m.linkUp() {
		m.updateStatus(false, "", 0)
		return false
	}

	if !m.ipReady() {
		m.updateStatus(false, "", 0)
		return false
	}

	if !m.pingOK() {
		m.updateStatus(false, "", 0)
		return false
	}

	// Refresh SSID/signal details at most once every 5 minutes; the connectivity
	// checks above already confirmed we are online.
	m.mu.Lock()
	needsRefresh := time.Now().After(m.connInfoExp)
	m.mu.Unlock()

	if needsRefresh {
		status := m.getConnectionInfo()
		m.mu.Lock()
		m.status = status
		m.connInfoExp = time.Now().Add(5 * time.Minute)
		m.mu.Unlock()
		return status.Connected
	}

	return true
}

func (m *Monitor) linkUp() bool {
	data, err := os.ReadFile("/sys/class/net/" + m.cfg.OfflineAP.Interface + "/operstate")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "up"
}

func (m *Monitor) ipReady() bool {
	iface, err := net.InterfaceByName(m.cfg.OfflineAP.Interface)
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
	target := m.cfg.OfflineAP.PingTarget
	// Refuse a target that ping would parse as an option flag (e.g. "-f" flood,
	// "-s <big>"); it reaches ping as a positional arg, so a leading '-' is the
	// only injection vector and an empty target is meaningless.
	if target == "" || strings.HasPrefix(target, "-") {
		return false
	}
	cmd := exec.Command("ping", "-c", "1", "-W", "3", target)
	return cmd.Run() == nil
}

// splitTerse splits one line of `nmcli -t` terse output into fields. nmcli
// escapes the field separator ':' as '\:' and a literal '\' as '\\' inside
// values, so a naive strings.Split on ':' corrupts any field (e.g. an SSID)
// that contains a colon. This walks the line honoring those escapes and
// returns the unescaped fields.
func splitTerse(line string) []string {
	var fields []string
	var b strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) {
			b.WriteByte(line[i+1])
			i++
			continue
		}
		if line[i] == ':' {
			fields = append(fields, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(line[i])
	}
	fields = append(fields, b.String())
	return fields
}

func (m *Monitor) getConnectionInfo() ConnectionStatus {
	out, err := exec.Command("nmcli", "-t", "-f", "ACTIVE,SSID,SIGNAL", "dev", "wifi").Output()
	if err != nil {
		return ConnectionStatus{}
	}

	for _, line := range strings.Split(string(out), "\n") {
		parts := splitTerse(line)
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
	out, err := exec.Command("iw", "dev", m.cfg.OfflineAP.Interface, "link").Output()
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
func (m *Monitor) ScanNetworks(ctx context.Context, rescan bool) ([]Network, error) {
	if rescan {
		exec.Command("sudo", "-n", "nmcli", "dev", "wifi", "rescan").Run()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	out, err := exec.Command("sudo", "-n", "nmcli", "-t", "-f", "SSID,SIGNAL,SECURITY", "dev", "wifi", "list").Output()
	if err != nil {
		return nil, fmt.Errorf("scan networks: %w", err)
	}

	seen := make(map[string]bool)
	var networks []Network
	for _, line := range strings.Split(string(out), "\n") {
		parts := splitTerse(line)
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
	// Reject leading '-' so nmcli can't parse the value as an option flag.
	if strings.HasPrefix(ssid, "-") || strings.HasPrefix(password, "-") {
		return fmt.Errorf("SSID and password must not start with '-'")
	}

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

// Stop halts the monitor goroutine. Safe to call more than once.
func (m *Monitor) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}
