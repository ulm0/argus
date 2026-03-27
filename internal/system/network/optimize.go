package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ulm0/argus/internal/logger"
)

// Optimizer applies runtime network and WiFi performance tuning.
type Optimizer struct{}

func NewOptimizer() *Optimizer {
	return &Optimizer{}
}

// Apply runs all network optimizations. Non-fatal errors are logged but don't stop execution.
func (o *Optimizer) Apply() {
	o.setCPUGovernor("performance")
	o.disableWiFiPowerSave()
	o.setTXQueueLen("wlan0", 2000)
	o.enableRTSCTS("wlan0")
	o.applyTCPTuning()
	o.setRegulatoryDomain("US")
	o.setMMCReadAheadKB(2048)
}

// setMMCReadAheadKB sets read-ahead for the SD card block device when present (TeslaUSB optimize_network.sh).
func (o *Optimizer) setMMCReadAheadKB(kb int) {
	p := "/sys/block/mmcblk0/queue/read_ahead_kb"
	if _, err := os.Stat(p); err != nil {
		return
	}
	if err := os.WriteFile(p, []byte(fmt.Sprintf("%d", kb)), 0644); err != nil {
		logger.L.WithError(err).Warn("set mmc read_ahead_kb failed")
	}
}

func (o *Optimizer) setCPUGovernor(governor string) {
	paths, _ := findGlobPaths("/sys/devices/system/cpu/cpu*/cpufreq/scaling_governor")
	for _, p := range paths {
		if err := os.WriteFile(p, []byte(governor), 0644); err != nil {
			logger.L.WithError(err).Warn("set CPU governor failed")
		}
	}
}

func (o *Optimizer) disableWiFiPowerSave() {
	if err := exec.Command("iwconfig", "wlan0", "power", "off").Run(); err != nil {
		logger.L.WithError(err).Warn("disable WiFi power save failed")
	}
}

func (o *Optimizer) setTXQueueLen(iface string, length int) {
	if err := exec.Command("ip", "link", "set", iface, "txqueuelen", fmt.Sprintf("%d", length)).Run(); err != nil {
		logger.L.WithError(err).WithField("iface", iface).Warn("set txqueuelen failed")
	}
}

func (o *Optimizer) enableRTSCTS(iface string) {
	if err := exec.Command("iw", "dev", iface, "set", "rts", "500").Run(); err != nil {
		logger.L.WithError(err).Warn("enable RTS/CTS failed")
	}
}

func (o *Optimizer) applyTCPTuning() {
	sysctls := map[string]string{
		"net.core.rmem_max":               "16777216",
		"net.core.wmem_max":               "16777216",
		"net.ipv4.tcp_rmem":               "4096 87380 16777216",
		"net.ipv4.tcp_wmem":               "4096 65536 16777216",
		"net.ipv4.tcp_congestion_control": "bbr",
		"net.ipv4.tcp_fastopen":           "3",
		"net.ipv4.tcp_mtu_probing":        "1",
	}

	for key, val := range sysctls {
		path := "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
		if err := os.WriteFile(path, []byte(val), 0644); err != nil {
			logger.L.WithError(err).WithField("key", key).Warn("sysctl write failed")
		}
	}
}

func (o *Optimizer) setRegulatoryDomain(domain string) {
	if err := exec.Command("iw", "reg", "set", domain).Run(); err != nil {
		logger.L.WithError(err).WithField("domain", domain).Warn("set regulatory domain failed")
	}
}

func findGlobPaths(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}
