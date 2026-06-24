package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	onlineCacheTTL = 30 * time.Second
)

// cameraNumToName maps Tesla's event.json "camera" field to the video filename substring.
var cameraNumToName = map[int]string{
	0: "front",
	1: "fisheye",
	2: "narrow",
	3: "left_repeater",
	4: "right_repeater",
	5: "left_b_pillar",
	6: "right_b_pillar",
	7: "back",
	8: "cabin",
}

type SentryEvent struct {
	EventDir  string    `json:"event_dir"`
	EventName string    `json:"event_name"`
	Timestamp time.Time `json:"timestamp"`
	Videos    []string  `json:"videos"`
	Type      string    `json:"type"` // "sentry" or "saved"
}

// eventRingBuf is a fixed-capacity circular buffer for SentryEvents.
// It avoids the repeated slice header shuffling and GC pressure of a plain []SentryEvent.
type eventRingBuf struct {
	buf  []SentryEvent
	head int
	tail int
	size int
}

func newEventRingBuf(cap int) eventRingBuf {
	if cap <= 0 {
		cap = 50
	}
	return eventRingBuf{buf: make([]SentryEvent, cap)}
}

func (r *eventRingBuf) Len() int { return r.size }

func (r *eventRingBuf) toSlice() []SentryEvent {
	if r.size == 0 {
		return nil
	}
	out := make([]SentryEvent, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(r.head+i)%len(r.buf)]
	}
	return out
}

// push adds e, silently dropping the oldest entry if the buffer is full.
func (r *eventRingBuf) push(e SentryEvent) {
	if r.size == len(r.buf) {
		r.buf[r.head] = SentryEvent{}
		r.head = (r.head + 1) % len(r.buf)
		r.size--
	}
	r.buf[r.tail] = e
	r.tail = (r.tail + 1) % len(r.buf)
	r.size++
}

// pop removes and returns the oldest entry.
func (r *eventRingBuf) pop() (SentryEvent, bool) {
	if r.size == 0 {
		return SentryEvent{}, false
	}
	e := r.buf[r.head]
	r.buf[r.head] = SentryEvent{}
	r.head = (r.head + 1) % len(r.buf)
	r.size--
	return e, true
}

// pushFront re-inserts e at the front (highest priority), dropping the newest if full.
func (r *eventRingBuf) pushFront(e SentryEvent) {
	if r.size == len(r.buf) {
		r.tail = (r.tail - 1 + len(r.buf)) % len(r.buf)
		r.buf[r.tail] = SentryEvent{}
		r.size--
	}
	r.head = (r.head - 1 + len(r.buf)) % len(r.buf)
	r.buf[r.head] = e
	r.size++
}

type Service struct {
	cfg      *config.Config
	mu       sync.Mutex
	queue    eventRingBuf
	stopCh   chan struct{}
	drainCh  chan struct{}
	stopOnce sync.Once
	watcher  *SentryWatcher

	httpClient      *http.Client
	httpClientVideo *http.Client

	onlineMu  sync.Mutex
	onlineVal bool
	onlineExp time.Time
}

func NewService(cfg *config.Config) *Service {
	maxQ := cfg.Telegram.MaxQueueSize
	if maxQ <= 0 {
		maxQ = 50
	}
	return &Service{
		cfg:             cfg,
		queue:           newEventRingBuf(maxQ),
		stopCh:          make(chan struct{}),
		drainCh:         make(chan struct{}, 1),
		httpClient:      &http.Client{Timeout: 15 * time.Second},
		httpClientVideo: &http.Client{Timeout: 2 * time.Minute},
	}
}

// Start begins watching for Sentry events and processing the queue.
func (s *Service) Start(ctx context.Context) {
	if !s.cfg.Telegram.Enabled {
		logger.L.Debug("Telegram alerting disabled")
		return
	}

	s.loadQueue()

	s.watcher = NewSentryWatcher(s.cfg, s.onSentryEvent)
	go s.watcher.Start(ctx)

	go s.processQueue(ctx)

	logger.L.Info("Telegram alerting started")
}

// Stop halts the Telegram service. Safe to call multiple times.
func (s *Service) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		if s.watcher != nil {
			s.watcher.Stop()
		}
	})
}

// GetStatus returns the current Telegram service status.
func (s *Service) GetStatus() map[string]any {
	s.mu.Lock()
	qLen := s.queue.Len()
	s.mu.Unlock()

	return map[string]any{
		"enabled":        s.cfg.Telegram.Enabled,
		"queue_size":     qLen,
		"max_queue":      s.cfg.Telegram.MaxQueueSize,
		"online":         s.isOnline(),
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
	if s.cfg.Telegram.OfflineMode == "discard" && !s.isOnline() {
		logger.L.WithField("event", event.EventName).Debug("Telegram: discarding event (offline, mode=discard)")
		return
	}

	s.mu.Lock()
	s.queue.push(event)
	s.mu.Unlock()

	logger.L.WithField("event", event.EventName).WithField("videos", len(event.Videos)).Info("Telegram: queued sentry event")

	s.saveQueue()

	select {
	case s.drainCh <- struct{}{}:
	default:
	}
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
		case <-s.drainCh:
			s.drainQueue()
		}
	}
}

func (s *Service) drainQueue() {
	if !s.isOnline() {
		return
	}

	for {
		s.mu.Lock()
		event, ok := s.queue.pop()
		s.mu.Unlock()
		if !ok {
			return
		}

		if err := s.sendSentryAlert(event); err != nil {
			logger.L.WithError(err).WithField("event", event.EventName).Warn("Telegram: failed to send alert")
			s.mu.Lock()
			s.queue.pushFront(event)
			s.mu.Unlock()
			s.saveQueue()
			return
		}
		s.saveQueue()
	}
}

func (s *Service) sendSentryAlert(event SentryEvent) error {
	header := "🚨 *Sentry Mode Event*"
	if event.Type == "saved" {
		header = "💾 *Saved Clip*"
	}

	msg := fmt.Sprintf("%s\n\n"+
		"📅 Time: %s\n"+
		"📁 Event: `%s`\n"+
		"📹 Cameras: %d videos",
		header,
		event.Timestamp.Format("2006-01-02 15:04:05"),
		event.EventName,
		len(event.Videos),
	)

	if err := s.sendMessage(msg); err != nil {
		return err
	}

	camera := "front"
	if event.Type == "sentry" {
		camera = triggerCamera(event.EventDir)
	}
	for _, videoPath := range event.Videos {
		if strings.Contains(videoPath, camera) {
			if err := s.sendVideo(videoPath, event.EventName); err != nil {
				logger.L.WithError(err).WithField("video", filepath.Base(videoPath)).Warn("Telegram: failed to send video")
			}
			break
		}
	}

	return nil
}

// redactToken strips the bot token from an error before it is logged. HTTP
// client failures are *url.Error and embed the full request URL, which
// contains the token (…/bot<TOKEN>/sendMessage); logging it verbatim would
// leak the credential into the journal.
func (s *Service) redactToken(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if t := s.cfg.Telegram.BotToken; t != "" {
		msg = strings.ReplaceAll(msg, t, "***")
	}
	return fmt.Errorf("%s", msg)
}

func (s *Service) sendMessage(text string) error {
	url := fmt.Sprintf("%s%s/sendMessage", apiBaseURL, s.cfg.Telegram.BotToken)

	payload := map[string]string{
		"chat_id":    s.cfg.Telegram.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	resp, err := s.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("send message: %w", s.redactToken(err))
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

	if _, err := io.Copy(part, f); err != nil {
		return fmt.Errorf("copy video: %w", err)
	}
	writer.Close()

	resp, err := s.httpClientVideo.Post(url, writer.FormDataContentType(), &buf)
	if err != nil {
		return s.redactToken(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// triggerCamera reads event.json to find which camera detected the threat.
// Falls back to "front" if the file is missing, malformed, or the camera is unknown.
func triggerCamera(eventDir string) string {
	data, err := os.ReadFile(filepath.Join(eventDir, "event.json"))
	if err != nil {
		return "front"
	}
	var ej map[string]any
	if err := json.Unmarshal(data, &ej); err != nil {
		return "front"
	}
	camFloat, ok := ej["camera"].(float64)
	if !ok {
		return "front"
	}
	if name, ok := cameraNumToName[int(camFloat)]; ok {
		return name
	}
	return "front"
}

func (s *Service) queueFilePath() string {
	return filepath.Join(s.cfg.GadgetDir, "telegram_queue.json")
}

func (s *Service) saveQueue() {
	s.mu.Lock()
	events := s.queue.toSlice()
	s.mu.Unlock()

	data, err := json.Marshal(events)
	if err != nil {
		logger.L.WithError(err).Warn("Telegram: failed to marshal queue")
		return
	}

	tmp := s.queueFilePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		logger.L.WithError(err).Warn("Telegram: failed to write queue file")
		return
	}
	if err := os.Rename(tmp, s.queueFilePath()); err != nil {
		logger.L.WithError(err).Warn("Telegram: failed to save queue file")
	}
}

func (s *Service) loadQueue() {
	data, err := os.ReadFile(s.queueFilePath())
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		logger.L.WithError(err).Warn("Telegram: failed to read queue file")
		return
	}

	var events []SentryEvent
	if err := json.Unmarshal(data, &events); err != nil {
		logger.L.WithError(err).Warn("Telegram: failed to parse queue file")
		return
	}

	s.mu.Lock()
	for _, e := range events {
		s.queue.push(e)
	}
	s.mu.Unlock()

	if len(events) > 0 {
		logger.L.WithField("count", len(events)).Info("Telegram: restored queue from disk")
	}
}

// isOnline checks connectivity to Telegram's API, caching the result for onlineCacheTTL
// to avoid a blocking TCP dial on every queue tick and sentry event.
func (s *Service) isOnline() bool {
	s.onlineMu.Lock()
	if time.Now().Before(s.onlineExp) {
		v := s.onlineVal
		s.onlineMu.Unlock()
		return v
	}
	s.onlineMu.Unlock()

	conn, err := net.DialTimeout("tcp", "api.telegram.org:443", 5*time.Second)
	online := err == nil
	if online {
		conn.Close()
	}

	s.onlineMu.Lock()
	s.onlineVal = online
	s.onlineExp = time.Now().Add(onlineCacheTTL)
	s.onlineMu.Unlock()
	return online
}
