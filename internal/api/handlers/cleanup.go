package handlers

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/cleanup"
	partutil "github.com/ulm0/argus/internal/services/partition"
)

type CleanupHandler struct {
	cfg        *config.Config
	cleanupSvc *cleanup.Service
}

func NewCleanupHandler(cfg *config.Config) *CleanupHandler {
	return &CleanupHandler{
		cfg:        cfg,
		cleanupSvc: cleanup.NewService(cfg),
	}
}

func (h *CleanupHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	partitionPath := h.resolvePartition("part1")

	var policies map[string]cleanup.FolderPolicy
	if partitionPath != "" {
		policies = h.cleanupSvc.GetPoliciesForDetectedFolders(partitionPath)
	} else {
		policies = h.cleanupSvc.GetPolicies()
	}

	writeJSON(w, http.StatusOK, map[string]any{"policies": policies})
}

func (h *CleanupHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Policies map[string]cleanup.FolderPolicy `json:"policies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.cleanupSvc.SavePolicies(req.Policies); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *CleanupHandler) Preview(w http.ResponseWriter, r *http.Request) {
	partitionPath := h.resolvePartition("part1")
	if partitionPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available"})
		return
	}

	plan, err := h.cleanupSvc.CalculateCleanupPlan(partitionPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	report := h.cleanupSvc.ExecuteCleanup(plan, true)
	writeJSON(w, http.StatusOK, map[string]any{
		"plan":   plan,
		"report": report,
	})
}

func (h *CleanupHandler) Execute(w http.ResponseWriter, r *http.Request) {
	partitionPath := h.resolvePartition("part1")
	if partitionPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available"})
		return
	}

	var req struct {
		DryRun bool `json:"dry_run"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	plan, err := h.cleanupSvc.CalculateCleanupPlan(partitionPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	report := h.cleanupSvc.ExecuteCleanup(plan, req.DryRun)
	writeJSON(w, http.StatusOK, report)
}

func (h *CleanupHandler) Calculate(w http.ResponseWriter, r *http.Request) {
	partitionPath := h.resolvePartition("part1")
	if partitionPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available"})
		return
	}

	plan, err := h.cleanupSvc.CalculateCleanupPlan(partitionPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func (h *CleanupHandler) resolvePartition(partition string) string {
	p := partutil.AccessiblePath(h.cfg, partition)
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}
	return ""
}
