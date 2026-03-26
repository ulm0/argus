package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/updater"
)

// UpdateHandler handles update-related API endpoints.
type UpdateHandler struct {
	cfg *config.Config
}

func NewUpdateHandler(cfg *config.Config) *UpdateHandler {
	return &UpdateHandler{cfg: cfg}
}

type updateStatusResponse struct {
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	PublishedAt     string `json:"published_at,omitempty"`
}

// Status returns the current and latest known version.
// The latest version is only populated if the startup check has already run.
func (h *UpdateHandler) Status(w http.ResponseWriter, r *http.Request) {
	resp := updateStatusResponse{
		Current: currentVersion(),
	}

	if release := updater.GetPendingRelease(); release != nil {
		resp.Latest = release.Version
		resp.UpdateAvailable = true
		resp.PublishedAt = release.PublishedAt.Format("2006-01-02")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// currentVersion reads the version from the cmd package at runtime.
// It falls back to "dev" if not set.
var currentVersionFn func() string

func currentVersion() string {
	if currentVersionFn != nil {
		return currentVersionFn()
	}
	return "dev"
}

// SetVersionProvider wires the version string from the cmd package into this handler.
func SetVersionProvider(fn func() string) {
	currentVersionFn = fn
}
