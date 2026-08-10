package handlers

import (
	"net/http"

	"github.com/ulm0/argus/internal/services/archive"
)

type ArchiveHandler struct {
	archiveSvc *archive.Service
}

func NewArchiveHandler(svc *archive.Service) *ArchiveHandler {
	return &ArchiveHandler{archiveSvc: svc}
}

func (h *ArchiveHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.archiveSvc.Status())
}

func (h *ArchiveHandler) Run(w http.ResponseWriter, r *http.Request) {
	// The copy runs in the background; poll /api/archive/status for the result.
	if err := h.archiveSvc.Run(); err != nil {
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
