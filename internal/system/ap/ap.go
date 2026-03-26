package ap

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

type ForceMode string

const (
	ForceModeAuto    ForceMode = "auto"
	ForceModeOn      ForceMode = "force_on"
	ForceModeOff     ForceMode = "force_off"

	runtimeDir = "/run/argus-ap"
)

type Status struct {
	APActive    bool      `json:"ap_active"`
	ForceMode   ForceMode `json:"force_mode"`
	SSID        string    `json:"ssid"`
	StaticIP    string    `json:"static_ip"`
	DHCPStart   string    `json:"dhcp_range_start"`
	DHCPEnd     string    `json:"dhcp_range_end"`
	Error       string    `json:"error,omitempty"`
}

type APConfig struct {
	SSID       string `json:"ssid"`
	Passphrase string `json:"passphrase"`
}

type Manager struct {
	cfg       *config.Config
	mu        sync.Mutex
	active    bool
	forceMode ForceMode
	hostapdCmd *exec.Cmd
	dnsmasqCmd *exec.Cmd
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		cfg: cfg,
	}
	m.forceMode = ForceMode(cfg.OfflineAP.ForceMode)
	m.ensureRuntimeDir()
	return m
}

func (m *Manager) ensureRuntimeDir() {
	os.MkdirAll(runtimeDir, 0755)
}

// GetStatus returns the current AP status.
func (m *Manager) GetStatus() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	return Status{
		APActive:  m.active,
		ForceMode: m.forceMode,
		SSID:      m.cfg.OfflineAP.SSID,
		StaticIP:  m.cfg.OfflineAP.IPv4CIDR,
		DHCPStart: m.cfg.OfflineAP.DHCPStart,
		DHCPEnd:   m.cfg.OfflineAP.DHCPEnd,
	}
}

// SetForceMode sets the AP force mode and persists it.
func (m *Manager) SetForceMode(mode ForceMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch mode {
	case ForceModeAuto, ForceModeOn, ForceModeOff:
	default:
		return fmt.Errorf("invalid force mode: %s", mode)
	}

	m.forceMode = mode

	// Persist to runtime file
	runtimeFile := filepath.Join(runtimeDir, "force.mode")
	if err := os.WriteFile(runtimeFile, []byte(string(mode)), 0644); err != nil {
		return fmt.Errorf("write runtime force mode: %w", err)
	}

	switch mode {
	case ForceModeOn:
		return m.startAPLocked()
	case ForceModeOff:
		return m.stopAPLocked()
	case ForceModeAuto:
		// AP will be managed by WiFi monitor
	}

	return nil
}

// GetForceMode returns the current force mode.
func (m *Manager) GetForceMode() ForceMode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.forceMode
}

// StartAP starts the access point on the virtual interface.
func (m *Manager) StartAP() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startAPLocked()
}

// StopAP stops the access point.
func (m *Manager) StopAP() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopAPLocked()
}

func (m *Manager) startAPLocked() error {
	if m.active {
		return nil
	}

	vif := m.cfg.OfflineAP.VirtualInterface
	iface := m.cfg.OfflineAP.Interface

	// Create virtual interface
	logger.L.WithField("interface", vif).Info("creating virtual AP interface")
	exec.Command("iw", "dev", vif, "del").Run() // clean up stale
	if err := exec.Command("iw", "dev", iface, "interface", "add", vif, "type", "__ap").Run(); err != nil {
		return fmt.Errorf("create virtual interface: %w", err)
	}

	// Configure IP address
	ip := strings.Split(m.cfg.OfflineAP.IPv4CIDR, "/")[0]
	if err := exec.Command("ip", "addr", "add", m.cfg.OfflineAP.IPv4CIDR, "dev", vif).Run(); err != nil {
		logger.L.WithError(err).Warn("ip addr add may have failed")
	}
	if err := exec.Command("ip", "link", "set", vif, "up").Run(); err != nil {
		return fmt.Errorf("bring up %s: %w", vif, err)
	}

	// Write and start hostapd
	hostapdConf := filepath.Join(runtimeDir, "hostapd.conf")
	if err := m.writeHostapdConf(hostapdConf); err != nil {
		return fmt.Errorf("write hostapd config: %w", err)
	}

	hostapdCmd := exec.Command("hostapd", "-B", hostapdConf)
	if err := hostapdCmd.Start(); err != nil {
		return fmt.Errorf("start hostapd: %w", err)
	}
	m.hostapdCmd = hostapdCmd
	// hostapd is daemonized (-B), reap the launcher process in a goroutine.
	go hostapdCmd.Wait() //nolint:errcheck

	// Write and start dnsmasq
	dnsmasqConf := filepath.Join(runtimeDir, "dnsmasq.conf")
	if err := m.writeDnsmasqConf(dnsmasqConf, ip); err != nil {
		return fmt.Errorf("write dnsmasq config: %w", err)
	}

	dnsmasqCmd := exec.Command("dnsmasq", "-C", dnsmasqConf, "--keep-in-foreground")
	if err := dnsmasqCmd.Start(); err != nil {
		return fmt.Errorf("start dnsmasq: %w", err)
	}
	m.dnsmasqCmd = dnsmasqCmd
	// dnsmasq runs in foreground; reap it in a goroutine so resources are freed.
	go func() {
		if err := dnsmasqCmd.Wait(); err != nil {
			logger.L.WithError(err).Debug("dnsmasq exited")
		}
	}()

	m.active = true
	m.recordAPStart()
	logger.L.WithField("ssid", m.cfg.OfflineAP.SSID).WithField("interface", vif).WithField("ip", ip).Info("access point started")
	return nil
}

func (m *Manager) stopAPLocked() error {
	if !m.active {
		return nil
	}

	// Kill hostapd and dnsmasq via stored process handles when available,
	// falling back to pkill for externally-spawned processes.
	if m.hostapdCmd != nil && m.hostapdCmd.Process != nil {
		m.hostapdCmd.Process.Kill()
		m.hostapdCmd = nil
	} else {
		exec.Command("pkill", "-f", "hostapd.*argus").Run()
	}
	if m.dnsmasqCmd != nil && m.dnsmasqCmd.Process != nil {
		m.dnsmasqCmd.Process.Kill()
		m.dnsmasqCmd = nil
	} else {
		exec.Command("pkill", "-f", "dnsmasq.*argus").Run()
	}
	time.Sleep(500 * time.Millisecond)

	// Remove virtual interface
	vif := m.cfg.OfflineAP.VirtualInterface
	exec.Command("ip", "link", "set", vif, "down").Run()
	exec.Command("iw", "dev", vif, "del").Run()

	m.active = false
	m.clearAPState()
	logger.L.Info("access point stopped")
	return nil
}

// IsActive returns whether the AP is currently running.
func (m *Manager) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

// GetAPConfig reads the AP configuration from config.yaml.
func (m *Manager) GetAPConfig() APConfig {
	return APConfig{
		SSID:       m.cfg.OfflineAP.SSID,
		Passphrase: m.cfg.OfflineAP.Passphrase,
	}
}

// UpdateAPConfig updates SSID and passphrase in config.yaml and reloads.
func (m *Manager) UpdateAPConfig(ssid, passphrase string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cfg.OfflineAP.SSID = ssid
	m.cfg.OfflineAP.Passphrase = passphrase

	if err := m.cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if m.active {
		m.stopAPLocked()
		m.startAPLocked()
	}

	return nil
}

func (m *Manager) writeHostapdConf(path string) error {
	const hostapdTmpl = `interface={{.VIF}}
driver=nl80211
ssid={{.SSID}}
hw_mode=g
channel={{.Channel}}
wmm_enabled=0
macaddr_acl=0
auth_algs=1
ignore_broadcast_ssid=0
wpa=2
wpa_passphrase={{.Passphrase}}
wpa_key_mgmt=WPA-PSK
wpa_pairwise=TKIP
rsn_pairwise=CCMP
`
	tmpl, err := template.New("hostapd").Parse(hostapdTmpl)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, map[string]any{
		"VIF":        m.cfg.OfflineAP.VirtualInterface,
		"SSID":       m.cfg.OfflineAP.SSID,
		"Channel":    m.cfg.OfflineAP.Channel,
		"Passphrase": m.cfg.OfflineAP.Passphrase,
	})
}

func (m *Manager) writeDnsmasqConf(path, gatewayIP string) error {
	const dnsmasqTmpl = `interface={{.VIF}}
bind-interfaces
dhcp-range={{.DHCPStart}},{{.DHCPEnd}},12h
dhcp-option=3,{{.GatewayIP}}
dhcp-option=6,{{.GatewayIP}}
address=/#/{{.GatewayIP}}
`
	tmpl, err := template.New("dnsmasq").Parse(dnsmasqTmpl)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, map[string]any{
		"VIF":       m.cfg.OfflineAP.VirtualInterface,
		"DHCPStart": m.cfg.OfflineAP.DHCPStart,
		"DHCPEnd":   m.cfg.OfflineAP.DHCPEnd,
		"GatewayIP": gatewayIP,
	})
}

func (m *Manager) recordAPStart() {
	state := map[string]any{
		"started_at": time.Now().Format(time.RFC3339),
		"ssid":       m.cfg.OfflineAP.SSID,
	}
	data, _ := json.Marshal(state)
	os.WriteFile(filepath.Join(runtimeDir, "ap_state.json"), data, 0644)
}

func (m *Manager) clearAPState() {
	os.Remove(filepath.Join(runtimeDir, "ap_state.json"))
}
