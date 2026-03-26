package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/fsck"
)

type FsckHandler struct {
	cfg    *config.Config
	runner *fsck.Runner
}

func NewFsckHandler(cfg *config.Config) *FsckHandler {
	return &FsckHandler{
		cfg:    cfg,
		runner: fsck.NewRunner(cfg),
	}
}

func (h *FsckHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Partitions []string `json:"partitions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Partitions) == 0 {
		req.Partitions = h.cfg.USBPartitions()
	}

	if err := h.runner.Start(req.Partitions); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (h *FsckHandler) Status(w http.ResponseWriter, r *http.Request) {
	status := h.runner.GetStatus()
	writeJSON(w, http.StatusOK, status)
}

func (h *FsckHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if err := h.runner.Cancel(); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (h *FsckHandler) History(w http.ResponseWriter, r *http.Request) {
	history := h.runner.GetHistory()
	writeJSON(w, http.StatusOK, map[string]any{"history": history})
}

func (h *FsckHandler) LastCheck(w http.ResponseWriter, r *http.Request) {
	partition := mux.Vars(r)["partition"]

	result := h.runner.GetLastCheck(partition)
	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no check found for partition " + partition})
		return
	}

	writeJSON(w, http.StatusOK, result)
}
