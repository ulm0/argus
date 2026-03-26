package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/ap"
)

type APHandler struct {
	cfg     *config.Config
	manager *ap.Manager
}

func NewAPHandler(cfg *config.Config) *APHandler {
	return &APHandler{
		cfg:     cfg,
		manager: ap.NewManager(cfg),
	}
}

func (h *APHandler) Status(w http.ResponseWriter, r *http.Request) {
	status := h.manager.GetStatus()
	writeJSON(w, http.StatusOK, status)
}

func (h *APHandler) Force(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.manager.SetForceMode(req.Mode); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": req.Mode})
}

func (h *APHandler) Configure(w http.ResponseWriter, r *http.Request) {
	var apCfg ap.APConfig
	if err := json.NewDecoder(r.Body).Decode(&apCfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.manager.UpdateAPConfig(apCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"config": h.manager.GetAPConfig(),
	})
}
