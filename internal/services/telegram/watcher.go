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

// maxConcurrentEvents limits how many sentry event goroutines run simultaneously.
// On a single-core Pi Zero, more than a few concurrent goroutines just cause context switching overhead.
const maxConcurrentEvents = 3

// SentryWatcher watches for new Sentry Mode and Saved Clip events using polling.
// On Linux with inotify (via fsnotify), this would be event-driven.
// For portability, we use a polling approach that checks for new directories.
type SentryWatcher struct {
	cfg        *config.Config
	callback   func(SentryEvent)
	seenSentry map[string]bool
	seenSaved  map[string]bool
	stopCh     chan struct{}
	stopOnce   sync.Once
	sem        chan struct{}
}

func NewSentryWatcher(cfg *config.Config, callback func(SentryEvent)) *SentryWatcher {
	return &SentryWatcher{
		cfg:        cfg,
		callback:   callback,
		seenSentry: make(map[string]bool),
		seenSaved:  make(map[string]bool),
		stopCh:     make(chan struct{}),
		sem:        make(chan struct{}, maxConcurrentEvents),
	}
}

// Start begins polling for new Sentry and Saved Clip events.
func (w *SentryWatcher) Start(ctx context.Context) {
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
			w.checkDir(ctx, w.sentryPath(), w.seenSentry, "sentry")
			w.checkDir(ctx, w.savedPath(), w.seenSaved, "saved")
		}
	}
}

// Stop halts the watcher. Safe to call multiple times.
func (w *SentryWatcher) Stop() {
	w.stopOnce.Do(func() { close(w.stopCh) })
}

func (w *SentryWatcher) clipPath(folder string) string {
	for _, ro := range []bool{true, false} {
		base := w.cfg.MountPath("part1", ro)
		p := filepath.Join(base, "TeslaCam", folder)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return ""
}

func (w *SentryWatcher) sentryPath() string { return w.clipPath("SentryClips") }
func (w *SentryWatcher) savedPath() string  { return w.clipPath("SavedClips") }

func (w *SentryWatcher) seedExistingEvents() {
	for _, dir := range []struct {
		path  string
		seen  map[string]bool
		label string
	}{
		{w.sentryPath(), w.seenSentry, "sentry"},
		{w.savedPath(), w.seenSaved, "saved"},
	} {
		if dir.path == "" {
			continue
		}
		entries, err := os.ReadDir(dir.path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				dir.seen[e.Name()] = true
			}
		}
		logger.L.WithField("count", len(dir.seen)).WithField("type", dir.label).Debug("Telegram watcher: seeded existing events")
	}
}

func (w *SentryWatcher) checkDir(ctx context.Context, dir string, seen map[string]bool, eventType string) {
	if dir == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		name := e.Name()
		if seen[name] {
			continue
		}

		seen[name] = true

		// Process each new event concurrently but bounded by the semaphore so we
		// never flood the single-core Pi Zero with unbounded goroutines.
		go func(eventName string) {
			// Acquire semaphore slot, honouring shutdown signals.
			select {
			case w.sem <- struct{}{}:
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			}
			defer func() { <-w.sem }()

			// Wait for the event to be fully written.
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			}

			eventDir := filepath.Join(dir, eventName)
			videos := w.findVideos(eventDir)

			if len(videos) > 0 {
				event := SentryEvent{
					EventDir:  eventDir,
					EventName: eventName,
					Timestamp: time.Now(),
					Videos:    videos,
					Type:      eventType,
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
