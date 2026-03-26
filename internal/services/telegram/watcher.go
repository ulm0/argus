package telegram

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

// SentryWatcher watches for new Sentry Mode events using polling.
// On Linux with inotify (via fsnotify), this would be event-driven.
// For portability, we use a polling approach that checks for new directories.
type SentryWatcher struct {
	cfg      *config.Config
	callback func(SentryEvent)
	seen     map[string]bool
	stopCh   chan struct{}
	stopOnce sync.Once
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
			w.checkForNewEvents(ctx)
		}
	}
}

// Stop halts the watcher. Safe to call multiple times.
func (w *SentryWatcher) Stop() {
	w.stopOnce.Do(func() { close(w.stopCh) })
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

func (w *SentryWatcher) checkForNewEvents(ctx context.Context) {
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

		// Process each new event concurrently so that multiple events detected in
		// the same tick don't serialize their 5-second "write-settle" wait.
		go func(eventName string) {
			// Wait for the event to be fully written, honouring shutdown signals.
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			}

			eventDir := filepath.Join(sentryDir, eventName)
			videos := w.findVideos(eventDir)

			if len(videos) > 0 {
				event := SentryEvent{
					EventDir:  eventDir,
					EventName: eventName,
					Timestamp: time.Now(),
					Videos:    videos,
				}
				w.callback(event)
			}
		}(name)
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
