package handlers

import (
	"net/http"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/analytics"
)

type AnalyticsHandler struct {
	cfg          *config.Config
	analyticsSvc *analytics.Service
}

func NewAnalyticsHandler(cfg *config.Config) *AnalyticsHandler {
	return &AnalyticsHandler{
		cfg:          cfg,
		analyticsSvc: analytics.NewService(cfg),
	}
}

func (h *AnalyticsHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	data := h.analyticsSvc.GetCompleteAnalytics()
	writeJSON(w, http.StatusOK, data)
}

func (h *AnalyticsHandler) SystemMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.analyticsSvc.GetSystemMetrics())
}

func (h *AnalyticsHandler) PartitionUsage(w http.ResponseWriter, r *http.Request) {
	usages := h.analyticsSvc.GetPartitionUsage()
	writeJSON(w, http.StatusOK, map[string]any{"partitions": usages})
}

func (h *AnalyticsHandler) VideoStats(w http.ResponseWriter, r *http.Request) {
	stats := h.analyticsSvc.GetVideoStatistics()
	writeJSON(w, http.StatusOK, map[string]any{"statistics": stats})
}

func (h *AnalyticsHandler) Health(w http.ResponseWriter, r *http.Request) {
	health := h.analyticsSvc.GetStorageHealth()
	writeJSON(w, http.StatusOK, health)
}
