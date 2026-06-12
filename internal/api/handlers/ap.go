package handlers

import (
	"encoding/json"
	"net/http"

	system_ap "github.com/ulm0/argus/internal/system/ap"
)

type APHandler struct {
	manager *system_ap.Manager
}

func NewAPHandler(mgr *system_ap.Manager) *APHandler {
	return &APHandler{manager: mgr}
}

func (h *APHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.manager.GetStatus())
}

func (h *APHandler) Force(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.manager.SetForceMode(system_ap.ForceMode(req.Mode)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": req.Mode})
}

func (h *APHandler) ShareInternet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.manager.SetShareInternet(req.Enabled); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "share_internet": req.Enabled})
}

func (h *APHandler) Configure(w http.ResponseWriter, r *http.Request) {
	var apCfg system_ap.APConfig
	if err := json.NewDecoder(r.Body).Decode(&apCfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.manager.UpdateAPConfig(apCfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"config": h.manager.GetAPConfig(),
	})
}
