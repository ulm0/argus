package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/mode"
)

type ModeHandler struct {
	cfg     *config.Config
	modeSvc *mode.Service
}

func NewModeHandler(cfg *config.Config) *ModeHandler {
	return &ModeHandler{
		cfg:     cfg,
		modeSvc: mode.NewService(cfg),
	}
}

func (h *ModeHandler) Status(w http.ResponseWriter, r *http.Request) {
	info := h.modeSvc.CurrentMode()
	features := h.modeSvc.FeatureAvailability()

	resp := map[string]any{
		"mode":       info.Token,
		"mode_label": info.Label,
		"hostname":   h.modeSvc.Hostname(),
		"features":   features,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *ModeHandler) PresentUSB(w http.ResponseWriter, r *http.Request) {
	if err := h.modeSvc.SwitchToPresent(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": "present"})
}

func (h *ModeHandler) EditUSB(w http.ResponseWriter, r *http.Request) {
	if err := h.modeSvc.SwitchToEdit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": "edit"})
}

func (h *ModeHandler) GadgetState(w http.ResponseWriter, r *http.Request) {
	state := h.modeSvc.GadgetState()
	writeJSON(w, http.StatusOK, state)
}

func (h *ModeHandler) RecoverGadget(w http.ResponseWriter, r *http.Request) {
	result, err := h.modeSvc.RecoverGadget()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *ModeHandler) OperationStatus(w http.ResponseWriter, r *http.Request) {
	status := h.modeSvc.OperationStatus()
	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
