package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/system/bluetooth"
)

type BluetoothHandler struct {
	manager *bluetooth.Manager
}

func NewBluetoothHandler() *BluetoothHandler {
	return &BluetoothHandler{manager: bluetooth.NewManager()}
}

func (h *BluetoothHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.manager.GetStatus())
}

func (h *BluetoothHandler) Devices(w http.ResponseWriter, r *http.Request) {
	devices := h.manager.ListDevices()
	if devices == nil {
		devices = []bluetooth.Device{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (h *BluetoothHandler) Connect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MAC string `json:"mac"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.manager.Connect(req.MAC); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *BluetoothHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MAC string `json:"mac"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.manager.Disconnect(req.MAC); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
