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
	ForceModeAuto ForceMode = "auto"
	ForceModeOn   ForceMode = "on"
	ForceModeOff  ForceMode = "off"

	runtimeDir = "/run/argus-ap"
)

type Status struct {
	Enabled       bool      `json:"enabled"`
	APActive      bool      `json:"active"`
	ForceMode     ForceMode `json:"force_mode"`
	SSID          string    `json:"ssid"`
	Passphrase    string    `json:"passphrase"`
	StaticIP      string    `json:"static_ip"`
	DHCPStart     string    `json:"dhcp_range_start"`
	DHCPEnd       string    `json:"dhcp_range_end"`
	ClientCount   int       `json:"client_count"`
	ShareInternet bool      `json:"share_internet"`
	Upstream      string    `json:"upstream,omitempty"`
	Error         string    `json:"error,omitempty"`
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
	cfg        *config.Config
	mu         sync.Mutex
	active     bool
	forceMode  ForceMode
	dnsmasqCmd *exec.Cmd
}

// cleanupInterface tears down the virtual AP interface, used both on stop and
// when a startup step fails partway through.
func (m *Manager) cleanupInterface(vif string) {
	exec.Command("ip", "link", "set", vif, "down").Run()
	exec.Command("iw", "dev", vif, "del").Run()
}

// detectCountry returns the active wireless regulatory country code (e.g. "US")
// from `iw reg get`, or "" when it is unset/global ("00"). hostapd needs this to
// bring the radio up on many channels; without it the AP often fails to start.
func detectCountry() string {
	out, err := exec.Command("iw", "reg", "get").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "country ") {
			code := strings.TrimSpace(strings.TrimPrefix(line, "country "))
			if len(code) >= 2 && code[:2] != "00" {
				return code[:2]
			}
		}
	}
	return ""
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

	s := Status{
		Enabled:       m.cfg.OfflineAP.Enabled,
		APActive:      m.active,
		ForceMode:     m.forceMode,
		SSID:          m.cfg.OfflineAP.SSID,
		Passphrase:    m.cfg.OfflineAP.Passphrase,
		StaticIP:      m.cfg.OfflineAP.IPv4CIDR,
		DHCPStart:     m.cfg.OfflineAP.DHCPStart,
		DHCPEnd:       m.cfg.OfflineAP.DHCPEnd,
		ShareInternet: m.cfg.OfflineAP.ShareInternet,
	}
	if m.active {
		s.ClientCount = m.clientCount()
		if m.cfg.OfflineAP.ShareInternet {
			s.Upstream = upstreamInterface(m.cfg.OfflineAP.VirtualInterface)
		}
	}
	return s
}

func (m *Manager) clientCount() int {
	out, err := exec.Command("iw", "dev", m.cfg.OfflineAP.VirtualInterface, "station", "dump").CombinedOutput()
	if err != nil {
		return 0
	}
	return strings.Count(string(out), "Station")
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

// StartAP starts the access point. No-op when force mode is "off".
func (m *Manager) StartAP() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.forceMode == ForceModeOff {
		return nil
	}
	return m.startAPLocked()
}

// StopAP stops the access point. No-op when force mode is "on".
func (m *Manager) StopAP() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.forceMode == ForceModeOn {
		return nil
	}
	return m.stopAPLocked()
}

func (m *Manager) startAPLocked() error {
	if m.active {
		return nil
	}

	vif := m.cfg.OfflineAP.VirtualInterface
	iface := m.cfg.OfflineAP.Interface

	// Evict any stale hostapd/dnsmasq left by a previous crash before touching
	// the interface, so they don't hold the listening socket when we restart.
	exec.Command("pkill", "-f", "hostapd.*argus").Run()
	exec.Command("pkill", "-f", "dnsmasq.*argus").Run()
	time.Sleep(300 * time.Millisecond)

	// Create virtual interface
	logger.L.WithField("interface", vif).Info("creating virtual AP interface")
	exec.Command("iw", "dev", vif, "del").Run() // clean up stale
	if err := exec.Command("iw", "dev", iface, "interface", "add", vif, "type", "__ap").Run(); err != nil {
		return fmt.Errorf("create virtual interface: %w", err)
	}

	// Configure IP address. Flush first so a stale address left by a previous
	// run can't make "ip addr add" fail with EEXIST.
	ip := strings.Split(m.cfg.OfflineAP.IPv4CIDR, "/")[0]
	exec.Command("ip", "addr", "flush", "dev", vif).Run()
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

	// hostapd -B daemonizes and returns 0 once forked, so a config parse error
	// is caught here, but a post-fork radio failure (bad channel / missing
	// regulatory domain) is not — the launcher still exits 0. Verify the daemon
	// is actually alive afterwards so we never report the AP as active while it
	// is silently broadcasting nothing.
	if out, err := exec.Command("hostapd", "-B", hostapdConf).CombinedOutput(); err != nil {
		m.cleanupInterface(vif)
		return fmt.Errorf("hostapd failed to start: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	time.Sleep(1 * time.Second)
	if exec.Command("pgrep", "-f", "hostapd.*argus").Run() != nil {
		m.cleanupInterface(vif)
		return fmt.Errorf("hostapd exited after start (check channel and country/regulatory domain)")
	}

	// Write and start dnsmasq. In share-internet mode it resolves real domains
	// (clients reach the internet); otherwise it stays a captive portal that
	// points every lookup back at the admin UI.
	dnsmasqConf := filepath.Join(runtimeDir, "dnsmasq.conf")
	if err := m.writeDnsmasqConf(dnsmasqConf, ip, !m.cfg.OfflineAP.ShareInternet); err != nil {
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

	// In share-internet mode, NAT AP clients out to the current upstream
	// (Bluetooth tethering or WiFi) so a device joining the hotspot — e.g. the
	// car — gets real internet through the Pi. Best-effort: the AP itself is up
	// regardless, so a NAT failure is logged, not fatal.
	if m.cfg.OfflineAP.ShareInternet {
		if up := upstreamInterface(vif); up != "" {
			if err := enableNAT(vif, up); err != nil {
				logger.L.WithError(err).Warn("enable internet sharing NAT failed")
			} else {
				logger.L.WithField("upstream", up).Info("AP internet sharing enabled")
			}
		} else {
			logger.L.Warn("share_internet enabled but no upstream interface has internet")
		}
	}

	m.active = true
	m.recordAPStart()
	logger.L.WithField("ssid", m.cfg.OfflineAP.SSID).WithField("interface", vif).WithField("ip", ip).Info("access point started")
	return nil
}

func (m *Manager) stopAPLocked() error {
	if !m.active {
		return nil
	}

	// hostapd is daemonized (-B), so the only reliable handle is its cmdline.
	exec.Command("pkill", "-f", "hostapd.*argus").Run()
	if m.dnsmasqCmd != nil && m.dnsmasqCmd.Process != nil {
		m.dnsmasqCmd.Process.Kill()
		m.dnsmasqCmd = nil
	} else {
		exec.Command("pkill", "-f", "dnsmasq.*argus").Run()
	}
	time.Sleep(500 * time.Millisecond)

	// Remove any NAT rules we installed for internet sharing.
	disableNAT(m.cfg.OfflineAP.VirtualInterface)

	m.cleanupInterface(m.cfg.OfflineAP.VirtualInterface)

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

// GetAPConfig reads the AP configuration from config.
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

// UpdateAPConfig updates AP configuration fields and persists them. Restarts the AP if active.
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

	if err := m.cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if m.active {
		m.stopAPLocked()
		return m.startAPLocked()
	}
	return nil
}

// SetShareInternet toggles routing AP clients to the upstream internet and
// persists it. Restarts the AP when active so the NAT rules and dnsmasq mode
// (captive vs. resolving) are reapplied.
func (m *Manager) SetShareInternet(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cfg.OfflineAP.ShareInternet = enabled
	if err := m.cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if m.active {
		m.stopAPLocked()
		return m.startAPLocked()
	}
	return nil
}

func (m *Manager) writeHostapdConf(path string) error {
	const hostapdTmpl = `interface={{.VIF}}
driver=nl80211
ssid={{.SSID}}
hw_mode=g
channel={{.Channel}}
{{if .Country}}country_code={{.Country}}
ieee80211d=1
{{end}}wmm_enabled=0
macaddr_acl=0
auth_algs=1
ignore_broadcast_ssid=0
wpa=2
wpa_passphrase={{.Passphrase}}
wpa_key_mgmt=WPA-PSK
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

	// channel 0 means ACS (auto-select), which brcmfmac on the Pi does not
	// support — fall back to a sane fixed 2.4GHz channel.
	channel := m.cfg.OfflineAP.Channel
	if channel <= 0 {
		channel = 6
	}

	return tmpl.Execute(f, map[string]any{
		"VIF":        m.cfg.OfflineAP.VirtualInterface,
		"SSID":       m.cfg.OfflineAP.SSID,
		"Channel":    channel,
		"Passphrase": m.cfg.OfflineAP.Passphrase,
		"Country":    detectCountry(),
	})
}

func (m *Manager) writeDnsmasqConf(path, gatewayIP string, captive bool) error {
	const dnsmasqTmpl = `interface={{.VIF}}
bind-interfaces
dhcp-range={{.DHCPStart}},{{.DHCPEnd}},12h
dhcp-option=3,{{.GatewayIP}}
dhcp-option=6,{{.GatewayIP}}
{{if .Captive}}address=/#/{{.GatewayIP}}
{{end}}`
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
		"Captive":   captive,
	})
}

// upstreamInterface returns the interface that currently carries a default route
// (i.e. has internet), preferring Bluetooth tethering, then WiFi, then wired. The
// AP's own virtual interface is always excluded.
func upstreamInterface(vif string) string {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return ""
	}

	devs := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "dev" && i+1 < len(fields) {
				dev := fields[i+1]
				if dev != vif {
					devs[dev] = true
				}
			}
		}
	}

	for _, pref := range []string{"bnep0", "wlan0", "eth0", "usb0"} {
		if devs[pref] {
			return pref
		}
	}
	// Fall back to any remaining default-route device.
	for dev := range devs {
		return dev
	}
	return ""
}

// enableNAT masquerades AP-client traffic out through the upstream interface and
// turns on IPv4 forwarding. Rules are added only if not already present so a
// restart doesn't stack duplicates.
func enableNAT(vif, upstream string) error {
	if err := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run(); err != nil {
		return fmt.Errorf("enable ip_forward: %w", err)
	}
	rules := natRules(vif, upstream)
	for _, r := range rules {
		// -C checks for the rule; if absent (non-zero exit), append it with -A.
		check := append([]string{"-t", r.table, "-C"}, r.spec...)
		if exec.Command("iptables", check...).Run() != nil {
			add := append([]string{"-t", r.table, "-A"}, r.spec...)
			if err := exec.Command("iptables", add...).Run(); err != nil {
				return fmt.Errorf("iptables -A %v: %w", r.spec, err)
			}
		}
	}
	return nil
}

// disableNAT removes the masquerade/forward rules for vif across every known
// upstream interface. Best-effort: missing rules are ignored.
func disableNAT(vif string) {
	for _, upstream := range []string{"bnep0", "wlan0", "eth0", "usb0"} {
		for _, r := range natRules(vif, upstream) {
			del := append([]string{"-t", r.table, "-D"}, r.spec...)
			exec.Command("iptables", del...).Run()
		}
	}
}

type iptRule struct {
	table string
	spec  []string
}

func natRules(vif, upstream string) []iptRule {
	return []iptRule{
		{"nat", []string{"POSTROUTING", "-o", upstream, "-j", "MASQUERADE"}},
		{"filter", []string{"FORWARD", "-i", upstream, "-o", vif, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
		{"filter", []string{"FORWARD", "-i", vif, "-o", upstream, "-j", "ACCEPT"}},
	}
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
