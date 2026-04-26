package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/telegram"
)

type sentryEventPayload struct {
	EventName string `json:"event_name"`
	Timestamp string `json:"timestamp"`
	Cameras   int    `json:"cameras"`
}

// SentryEventsHandler manages SSE clients and broadcasts sentry events to them.
type SentryEventsHandler struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
	watcher *telegram.SentryWatcher
}

func NewSentryEventsHandler(cfg *config.Config) *SentryEventsHandler {
	h := &SentryEventsHandler{
		clients: make(map[chan string]struct{}),
	}
	h.watcher = telegram.NewSentryWatcher(cfg, h.broadcast)
	return h
}

func (h *SentryEventsHandler) Start(ctx context.Context) {
	go h.watcher.Start(ctx)
}

func (h *SentryEventsHandler) broadcast(e telegram.SentryEvent) {
	p := sentryEventPayload{
		EventName: e.EventName,
		Timestamp: e.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
		Cameras:   len(e.Videos),
	}
	data, _ := json.Marshal(p)

	h.mu.Lock()
	for ch := range h.clients {
		select {
		case ch <- string(data):
		default: // slow client: drop event
		}
	}
	h.mu.Unlock()
}

// Stream is the SSE handler for GET /api/events/stream.
func (h *SentryEventsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ch := make(chan string, 8)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Initial keepalive comment so the client knows the connection is alive.
	fmt.Fprintf(w, ": connected\n\n")
	f.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			f.Flush()
		}
	}
}
