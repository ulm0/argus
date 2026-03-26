package ap

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/ulm0/argus/internal/config"
)

type APStatus struct {
	Enabled    bool   `json:"enabled"`
	Active     bool   `json:"active"`
	SSID       string `json:"ssid"`
	Interface  string `json:"interface"`
	ForceMode  string `json:"force_mode"`
	Channel    int    `json:"channel"`
	ClientCount int   `json:"client_count"`
}

type APConfig struct {
	SSID            string `json:"ssid"`
	Passphrase      string `json:"passphrase"`
	Channel         int    `json:"channel"`
	Interface       string `json:"interface"`
	IPv4CIDR        string `json:"ipv4_cidr"`
	DHCPStart       string `json:"dhcp_start"`
	DHCPEnd         string `json:"dhcp_end"`
	CheckInterval   int    `json:"check_interval"`
	DisconnectGrace int    `json:"disconnect_grace"`
}

type Manager struct {
	cfg *config.Config
	mu  sync.Mutex
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

// GetStatus returns the current access point status.
func (m *Manager) GetStatus() APStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := APStatus{
		Enabled:   m.cfg.OfflineAP.Enabled,
		SSID:      m.cfg.OfflineAP.SSID,
		Interface: m.cfg.OfflineAP.Interface,
		ForceMode: m.cfg.OfflineAP.ForceMode,
		Channel:   m.cfg.OfflineAP.Channel,
	}

	if m.cfg.OfflineAP.Enabled {
		status.Active = m.isAPActive()
		status.ClientCount = m.getClientCount()
	}

	return status
}

// SetForceMode sets the AP force mode (auto, on, off).
func (m *Manager) SetForceMode(mode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch mode {
	case "auto", "on", "off":
		m.cfg.OfflineAP.ForceMode = mode
		return m.cfg.Save()
	default:
		return fmt.Errorf("invalid force mode: %s (must be auto, on, or off)", mode)
	}
}

// UpdateAPConfig updates the AP configuration and persists it.
func (m *Manager) UpdateAPConfig(apCfg APConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if apCfg.SSID != "" {
		m.cfg.OfflineAP.SSID = apCfg.SSID
	}
	if apCfg.Passphrase != "" {
		m.cfg.OfflineAP.Passphrase = apCfg.Passphrase
	}
	if apCfg.Channel > 0 {
		m.cfg.OfflineAP.Channel = apCfg.Channel
	}
	if apCfg.Interface != "" {
		m.cfg.OfflineAP.Interface = apCfg.Interface
	}
	if apCfg.IPv4CIDR != "" {
		m.cfg.OfflineAP.IPv4CIDR = apCfg.IPv4CIDR
	}
	if apCfg.DHCPStart != "" {
		m.cfg.OfflineAP.DHCPStart = apCfg.DHCPStart
	}
	if apCfg.DHCPEnd != "" {
		m.cfg.OfflineAP.DHCPEnd = apCfg.DHCPEnd
	}
	if apCfg.CheckInterval > 0 {
		m.cfg.OfflineAP.CheckInterval = apCfg.CheckInterval
	}
	if apCfg.DisconnectGrace > 0 {
		m.cfg.OfflineAP.DisconnectGrace = apCfg.DisconnectGrace
	}

	return m.cfg.Save()
}

// GetAPConfig returns the current AP configuration.
func (m *Manager) GetAPConfig() APConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	return APConfig{
		SSID:            m.cfg.OfflineAP.SSID,
		Passphrase:      m.cfg.OfflineAP.Passphrase,
		Channel:         m.cfg.OfflineAP.Channel,
		Interface:       m.cfg.OfflineAP.Interface,
		IPv4CIDR:        m.cfg.OfflineAP.IPv4CIDR,
		DHCPStart:       m.cfg.OfflineAP.DHCPStart,
		DHCPEnd:         m.cfg.OfflineAP.DHCPEnd,
		CheckInterval:   m.cfg.OfflineAP.CheckInterval,
		DisconnectGrace: m.cfg.OfflineAP.DisconnectGrace,
	}
}

func (m *Manager) isAPActive() bool {
	iface := m.cfg.OfflineAP.VirtualInterface
	if iface == "" {
		iface = "uap0"
	}
	out, err := exec.Command("ip", "link", "show", iface).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "state UP")
}

func (m *Manager) getClientCount() int {
	iface := m.cfg.OfflineAP.VirtualInterface
	if iface == "" {
		iface = "uap0"
	}
	out, err := exec.Command("iw", "dev", iface, "station", "dump").CombinedOutput()
	if err != nil {
		return 0
	}
	return strings.Count(string(out), "Station")
}
