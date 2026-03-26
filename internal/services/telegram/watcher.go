package telegram

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

// SentryWatcher watches for new Sentry Mode events using polling.
// On Linux with inotify (via fsnotify), this would be event-driven.
// For portability, we use a polling approach that checks for new directories.
type SentryWatcher struct {
	cfg       *config.Config
	callback  func(SentryEvent)
	seen      map[string]bool
	stopCh    chan struct{}
}

func NewSentryWatcher(cfg *config.Config, callback func(SentryEvent)) *SentryWatcher {
	return &SentryWatcher{
		cfg:      cfg,
		callback: callback,
		seen:     make(map[string]bool),
		stopCh:   make(chan struct{}),
	}
}

// Start begins polling for new Sentry events.
func (w *SentryWatcher) Start(ctx context.Context) {
	// Seed the seen map with existing events
	w.seedExistingEvents()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.checkForNewEvents()
		}
	}
}

// Stop halts the watcher.
func (w *SentryWatcher) Stop() {
	close(w.stopCh)
}

func (w *SentryWatcher) sentryPath() string {
	for _, ro := range []bool{true, false} {
		base := w.cfg.MountPath("part1", ro)
		p := filepath.Join(base, "TeslaCam", "SentryClips")
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return ""
}

func (w *SentryWatcher) seedExistingEvents() {
	sentryDir := w.sentryPath()
	if sentryDir == "" {
		return
	}

	entries, err := os.ReadDir(sentryDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			w.seen[e.Name()] = true
		}
	}

	logger.L.WithField("count", len(w.seen)).Debug("Telegram watcher: seeded existing sentry events")
}

func (w *SentryWatcher) checkForNewEvents() {
	sentryDir := w.sentryPath()
	if sentryDir == "" {
		return
	}

	entries, err := os.ReadDir(sentryDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		name := e.Name()
		if w.seen[name] {
			continue
		}

		w.seen[name] = true

		// Wait a moment for the event to be fully written
		time.Sleep(5 * time.Second)

		eventDir := filepath.Join(sentryDir, name)
		videos := w.findVideos(eventDir)

		if len(videos) > 0 {
			event := SentryEvent{
				EventDir:  eventDir,
				EventName: name,
				Timestamp: time.Now(),
				Videos:    videos,
			}
			w.callback(event)
		}
	}
}

func (w *SentryWatcher) findVideos(eventDir string) []string {
	entries, err := os.ReadDir(eventDir)
	if err != nil {
		return nil
	}

	var videos []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".mp4" {
			videos = append(videos, filepath.Join(eventDir, e.Name()))
		}
	}
	return videos
}
