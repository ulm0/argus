package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

const (
	apiBaseURL  = "https://api.telegram.org/bot"
	maxFileSize = 50 * 1024 * 1024 // Telegram 50 MiB limit
)

type SentryEvent struct {
	EventDir  string    `json:"event_dir"`
	EventName string    `json:"event_name"`
	Timestamp time.Time `json:"timestamp"`
	Videos    []string  `json:"videos"`
}

type Service struct {
	cfg       *config.Config
	mu        sync.Mutex
	queue     []SentryEvent
	stopCh    chan struct{}
	watcher   *SentryWatcher
}

func NewService(cfg *config.Config) *Service {
	return &Service{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
}

// Start begins watching for Sentry events and processing the queue.
func (s *Service) Start(ctx context.Context) {
	if !s.cfg.Telegram.Enabled {
		logger.L.Debug("Telegram alerting disabled")
		return
	}

	// Start the sentry watcher
	s.watcher = NewSentryWatcher(s.cfg, s.onSentryEvent)
	go s.watcher.Start(ctx)

	// Start the queue processor
	go s.processQueue(ctx)

	logger.L.Info("Telegram alerting started")
}

// Stop halts the Telegram service.
func (s *Service) Stop() {
	close(s.stopCh)
	if s.watcher != nil {
		s.watcher.Stop()
	}
}

// GetStatus returns the current Telegram service status.
func (s *Service) GetStatus() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	return map[string]any{
		"enabled":    s.cfg.Telegram.Enabled,
		"queue_size": len(s.queue),
		"max_queue":  s.cfg.Telegram.MaxQueueSize,
		"online":     s.isOnline(),
		"bot_configured": s.cfg.Telegram.BotToken != "",
	}
}

// Configure updates the Telegram configuration.
func (s *Service) Configure(botToken, chatID, offlineMode, videoQuality string) error {
	s.cfg.Telegram.BotToken = botToken
	s.cfg.Telegram.ChatID = chatID
	if offlineMode != "" {
		s.cfg.Telegram.OfflineMode = offlineMode
	}
	if videoQuality != "" {
		s.cfg.Telegram.VideoQuality = videoQuality
	}
	return s.cfg.Save()
}

// TestMessage sends a test message to verify the configuration.
func (s *Service) TestMessage() error {
	if s.cfg.Telegram.BotToken == "" || s.cfg.Telegram.ChatID == "" {
		return fmt.Errorf("Telegram bot token and chat ID must be configured")
	}

	return s.sendMessage("Argus Telegram alerting test message. Configuration is working correctly.")
}

func (s *Service) onSentryEvent(event SentryEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.Telegram.OfflineMode == "discard" && !s.isOnline() {
		logger.L.WithField("event", event.EventName).Debug("Telegram: discarding event (offline, mode=discard)")
		return
	}

	if len(s.queue) >= s.cfg.Telegram.MaxQueueSize {
		// Drop oldest event
		s.queue = s.queue[1:]
	}

	s.queue = append(s.queue, event)
	logger.L.WithField("event", event.EventName).WithField("videos", len(event.Videos)).Info("Telegram: queued sentry event")
}

func (s *Service) processQueue(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.drainQueue()
		}
	}
}

func (s *Service) drainQueue() {
	if !s.isOnline() {
		return
	}

	s.mu.Lock()
	if len(s.queue) == 0 {
		s.mu.Unlock()
		return
	}

	// Take the first event
	event := s.queue[0]
	s.queue = s.queue[1:]
	s.mu.Unlock()

	if err := s.sendSentryAlert(event); err != nil {
		logger.L.WithError(err).WithField("event", event.EventName).Warn("Telegram: failed to send alert")
		// Re-queue the event
		s.mu.Lock()
		s.queue = append([]SentryEvent{event}, s.queue...)
		s.mu.Unlock()
	}
}

func (s *Service) sendSentryAlert(event SentryEvent) error {
	msg := fmt.Sprintf("🚨 *Sentry Mode Event*\n\n"+
		"📅 Time: %s\n"+
		"📁 Event: `%s`\n"+
		"📹 Cameras: %d videos",
		event.Timestamp.Format("2006-01-02 15:04:05"),
		event.EventName,
		len(event.Videos),
	)

	if err := s.sendMessage(msg); err != nil {
		return err
	}

	// Send video clips (front camera preferred)
	for _, videoPath := range event.Videos {
		if strings.Contains(videoPath, "front") {
			if err := s.sendVideo(videoPath, event.EventName); err != nil {
				logger.L.WithError(err).WithField("video", filepath.Base(videoPath)).Warn("Telegram: failed to send video")
			}
			break // only send front camera
		}
	}

	return nil
}

func (s *Service) sendMessage(text string) error {
	url := fmt.Sprintf("%s%s/sendMessage", apiBaseURL, s.cfg.Telegram.BotToken)

	payload := map[string]string{
		"chat_id":    s.cfg.Telegram.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	data, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// NotifyText sends a plain text message via the Telegram bot.
// It is a no-op if Telegram is not configured or if the service is offline.
func (s *Service) NotifyText(text string) error {
	if !s.cfg.Telegram.Enabled {
		return nil
	}
	if !s.isOnline() {
		return nil
	}
	return s.sendMessage(text)
}

func (s *Service) sendVideo(videoPath, caption string) error {
	info, err := os.Stat(videoPath)
	if err != nil {
		return err
	}
	if info.Size() > maxFileSize {
		return fmt.Errorf("video too large for Telegram (%d bytes)", info.Size())
	}

	url := fmt.Sprintf("%s%s/sendVideo", apiBaseURL, s.cfg.Telegram.BotToken)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("chat_id", s.cfg.Telegram.ChatID)
	writer.WriteField("caption", caption)

	part, err := writer.CreateFormFile("video", filepath.Base(videoPath))
	if err != nil {
		return err
	}

	f, err := os.Open(videoPath)
	if err != nil {
		return err
	}
	defer f.Close()

	io.Copy(part, f)
	writer.Close()

	resp, err := http.Post(url, writer.FormDataContentType(), &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (s *Service) isOnline() bool {
	conn, err := net.DialTimeout("tcp", "api.telegram.org:443", 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
