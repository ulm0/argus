package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
	"github.com/ulm0/argus/internal/services/telegram"
)

type payload struct {
	EventName string `json:"event_name"`
	Timestamp string `json:"timestamp"`
	Cameras   int    `json:"cameras"`
}

type Service struct {
	cfg     *config.Config
	watcher *telegram.SentryWatcher
	client  *http.Client
}

func NewService(cfg *config.Config) *Service {
	s := &Service{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
	s.watcher = telegram.NewSentryWatcher(cfg, s.onEvent)
	return s
}

func (s *Service) Start(ctx context.Context) {
	if !s.cfg.Webhook.Enabled || s.cfg.Webhook.URL == "" {
		return
	}
	go s.watcher.Start(ctx)
	logger.L.Info("Webhook notifications started")
}

func (s *Service) Stop() {
	s.watcher.Stop()
}

func (s *Service) onEvent(e telegram.SentryEvent) {
	p := payload{
		EventName: e.EventName,
		Timestamp: e.Timestamp.UTC().Format(time.RFC3339),
		Cameras:   len(e.Videos),
	}
	body, err := json.Marshal(p)
	if err != nil {
		return
	}

	req, err := http.NewRequest(http.MethodPost, s.cfg.Webhook.URL, bytes.NewReader(body))
	if err != nil {
		logger.L.WithError(err).Warn("webhook: failed to create request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "argus-webhook/1.0")

	if s.cfg.Webhook.Secret != "" {
		mac := hmac.New(sha256.New, []byte(s.cfg.Webhook.Secret))
		mac.Write(body)
		req.Header.Set("X-Argus-Signature", hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		logger.L.WithError(err).Warn("webhook: delivery failed")
		return
	}
	// Drain before closing so net/http can reuse the keep-alive connection.
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		logger.L.WithField("status", resp.StatusCode).Warn("webhook: non-2xx response")
	}
}
