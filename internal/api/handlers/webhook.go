package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/webhook"
)

type WebhookHandler struct {
	cfg        *config.Config
	webhookSvc *webhook.Service
}

func NewWebhookHandler(cfg *config.Config, svc *webhook.Service) *WebhookHandler {
	return &WebhookHandler{cfg: cfg, webhookSvc: svc}
}

func (h *WebhookHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":    h.cfg.Webhook.Enabled,
		"url":        h.cfg.Webhook.URL,
		"has_secret": h.cfg.Webhook.Secret != "",
	})
}

type webhookConfigRequest struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Secret  string `json:"secret"`
}

func (h *WebhookHandler) Configure(w http.ResponseWriter, r *http.Request) {
	var req webhookConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	h.cfg.Webhook.Enabled = req.Enabled
	h.cfg.Webhook.URL = req.URL
	if req.Secret != "" {
		h.cfg.Webhook.Secret = req.Secret
	}
	if err := h.cfg.Save(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save config"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
