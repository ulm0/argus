package handlers

import (
	"net/http"
	"os/exec"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

// SystemPowerHandler exposes reboot/poweroff endpoints. The argus service runs
// as root, so it can invoke `systemctl` directly without sudo.
type SystemPowerHandler struct {
	cfg *config.Config
}

func NewSystemPowerHandler(cfg *config.Config) *SystemPowerHandler {
	return &SystemPowerHandler{cfg: cfg}
}

// Reboot schedules a reboot. We delay slightly so the HTTP response can flush
// and the client can show a "rebooting…" toast before the network drops.
func (h *SystemPowerHandler) Reboot(w http.ResponseWriter, r *http.Request) {
	logger.L.Warn("system: reboot requested via API")
	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := exec.Command("systemctl", "reboot").Run(); err != nil {
			logger.L.WithError(err).Error("systemctl reboot failed")
		}
	}()
	writeJSON(w, http.StatusOK, map[string]string{"status": "rebooting"})
}

// PowerOff schedules a graceful shutdown.
func (h *SystemPowerHandler) PowerOff(w http.ResponseWriter, r *http.Request) {
	logger.L.Warn("system: power off requested via API")
	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := exec.Command("systemctl", "poweroff").Run(); err != nil {
			logger.L.WithError(err).Error("systemctl poweroff failed")
		}
	}()
	writeJSON(w, http.StatusOK, map[string]string{"status": "powering off"})
}
