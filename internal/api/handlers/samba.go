package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/system/samba"
)

type SambaHandler struct {
	cfg     *config.Config
	manager *samba.Manager
}

func NewSambaHandler(cfg *config.Config) *SambaHandler {
	return &SambaHandler{
		cfg:     cfg,
		manager: samba.NewManager(cfg),
	}
}

func (h *SambaHandler) Status(w http.ResponseWriter, r *http.Request) {
	shares := []map[string]string{
		{"name": "gadget_part1", "label": "TeslaCam", "path": h.cfg.Installation.MountDir + "/part1"},
		{"name": "gadget_part2", "label": "LightShow", "path": h.cfg.Installation.MountDir + "/part2"},
	}
	if h.cfg.DiskImages.MusicEnabled {
		shares = append(shares, map[string]string{
			"name": "gadget_part3", "label": "Music", "path": h.cfg.Installation.MountDir + "/part3",
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user":         h.cfg.Installation.TargetUser,
		"config_path":  h.cfg.System.SambaConf,
		"password_set": h.cfg.Network.SambaPassword != "",
		"enabled":      h.cfg.SambaEnabled(),
		"shares":       shares,
	})
}

func (h *SambaHandler) SetEnabled(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if *req.Enabled {
		if err := h.manager.GenerateConfig(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to write smb.conf: "+err.Error())
			return
		}
		if err := h.manager.EnableServices(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		if err := h.manager.DisableServices(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	h.cfg.SetSambaEnabled(*req.Enabled)
	if err := h.cfg.Save(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "applied but failed to persist config: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"enabled": *req.Enabled})
}

func (h *SambaHandler) SetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password is required"})
		return
	}

	if err := h.manager.SetPassword(req.Password); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.cfg.Network.SambaPassword = req.Password
	if err := h.cfg.Save(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "password set but failed to persist config: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *SambaHandler) Restart(w http.ResponseWriter, r *http.Request) {
	if err := h.manager.RestartSambaServices(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *SambaHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
	if err := h.manager.GenerateConfig(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := h.manager.RestartSambaServices(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "config written but restart failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
