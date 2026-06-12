package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
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
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": "present"})
}

func (h *ModeHandler) EditUSB(w http.ResponseWriter, r *http.Request) {
	if err := h.modeSvc.SwitchToEdit(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
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
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *ModeHandler) OperationStatus(w http.ResponseWriter, r *http.Request) {
	status := h.modeSvc.OperationStatus()
	writeJSON(w, http.StatusOK, status)
}

// writeJSON serializes v as the response body. It deliberately does no logging:
// payloads can carry secrets (e.g. the AP passphrase in /ap/status), so 5xx
// logging lives in writeJSONError, which only ever receives the error string.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeJSONError writes {"error": msg} and logs server-side (5xx) failures.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	if status >= 500 && msg != "" {
		logger.L.WithField("status", status).WithField("detail", msg).Error("request error")
	}
	writeJSON(w, status, map[string]string{"error": msg})
}
