package ap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulm0/argus/internal/config"
)

func TestWriteDnsmasqConfCaptiveToggle(t *testing.T) {
	cfg := &config.Config{}
	cfg.OfflineAP.VirtualInterface = "uap0"
	cfg.OfflineAP.DHCPStart = "192.168.4.10"
	cfg.OfflineAP.DHCPEnd = "192.168.4.50"
	m := &Manager{cfg: cfg}

	dir := t.TempDir()

	// Captive: must redirect every lookup back to the gateway (no internet DNS).
	captivePath := filepath.Join(dir, "captive.conf")
	if err := m.writeDnsmasqConf(captivePath, "192.168.4.1", true); err != nil {
		t.Fatalf("writeDnsmasqConf captive: %v", err)
	}
	captive := readFile(t, captivePath)
	if !strings.Contains(captive, "address=/#/192.168.4.1") {
		t.Errorf("captive config missing wildcard DNS redirect:\n%s", captive)
	}

	// Sharing: must NOT hijack DNS so clients can resolve real domains.
	sharePath := filepath.Join(dir, "share.conf")
	if err := m.writeDnsmasqConf(sharePath, "192.168.4.1", false); err != nil {
		t.Fatalf("writeDnsmasqConf share: %v", err)
	}
	share := readFile(t, sharePath)
	if strings.Contains(share, "address=/#/") {
		t.Errorf("share-internet config must not hijack DNS:\n%s", share)
	}
	if !strings.Contains(share, "dhcp-range=192.168.4.10,192.168.4.50") {
		t.Errorf("share config missing dhcp-range:\n%s", share)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
