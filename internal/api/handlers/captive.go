package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/config"
)

type CaptiveHandler struct {
	cfg *config.Config
}

func NewCaptiveHandler(cfg *config.Config) *CaptiveHandler {
	return &CaptiveHandler{cfg: cfg}
}

// Detect handles all captive portal detection endpoints for Apple, Android, Windows, Firefox.
func (h *CaptiveHandler) Detect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"captive": true,
		"ssid":    h.cfg.OfflineAP.SSID,
		"url":     "/",
	})
}
