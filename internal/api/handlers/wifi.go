package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/wifi"
)

type WifiHandler struct {
	cfg     *config.Config
	monitor *wifi.Monitor
}

func NewWifiHandler(cfg *config.Config) *WifiHandler {
	return &WifiHandler{
		cfg:     cfg,
		monitor: wifi.NewMonitor(cfg),
	}
}

func (h *WifiHandler) Status(w http.ResponseWriter, r *http.Request) {
	conn := h.monitor.GetCurrentConnection()
	changeStatus := h.monitor.GetWifiChangeStatus()

	writeJSON(w, http.StatusOK, map[string]any{
		"connection":    conn,
		"change_status": changeStatus,
	})
}

func (h *WifiHandler) Scan(w http.ResponseWriter, r *http.Request) {
	networks, err := h.monitor.ScanNetworks()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"networks": networks})
}

func (h *WifiHandler) Configure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.monitor.UpdateCredentials(req.SSID, req.Password); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *WifiHandler) DismissStatus(w http.ResponseWriter, r *http.Request) {
	h.monitor.ClearWifiChangeStatus()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
