package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/telegram"
)

type TelegramHandler struct {
	cfg         *config.Config
	telegramSvc *telegram.Service
}

func NewTelegramHandler(cfg *config.Config) *TelegramHandler {
	return &TelegramHandler{
		cfg:         cfg,
		telegramSvc: telegram.NewService(cfg),
	}
}

func (h *TelegramHandler) Status(w http.ResponseWriter, r *http.Request) {
	status := h.telegramSvc.GetStatus()
	writeJSON(w, http.StatusOK, status)
}

func (h *TelegramHandler) Configure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BotToken     string `json:"bot_token"`
		ChatID       string `json:"chat_id"`
		OfflineMode  string `json:"offline_mode"`
		VideoQuality string `json:"video_quality"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.telegramSvc.Configure(req.BotToken, req.ChatID, req.OfflineMode, req.VideoQuality); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *TelegramHandler) Test(w http.ResponseWriter, r *http.Request) {
	if err := h.telegramSvc.TestMessage(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "test message sent"})
}
