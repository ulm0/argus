package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/chime"
)

type ChimeHandler struct {
	cfg      *config.Config
	chimeSvc *chime.Service
}

func NewChimeHandler(cfg *config.Config) *ChimeHandler {
	return &ChimeHandler{
		cfg:      cfg,
		chimeSvc: chime.NewService(cfg),
	}
}

func (h *ChimeHandler) mountPath() string {
	return h.cfg.MountPath("part1", false)
}

func (h *ChimeHandler) chimesDir() string {
	return filepath.Join(h.mountPath(), h.cfg.Web.ChimesFolder)
}

// List returns all chimes plus active chime info.
func (h *ChimeHandler) List(w http.ResponseWriter, r *http.Request) {
	mp := h.mountPath()
	chimes := h.chimeSvc.ListChimes(mp)
	if chimes == nil {
		chimes = []string{}
	}

	activeName, activeExists := h.chimeSvc.GetActiveChimeInfo(mp)
	randomCfg := h.chimeSvc.Groups().GetRandomConfig()

	writeJSON(w, http.StatusOK, map[string]any{
		"chimes":        chimes,
		"active":        activeName,
		"active_exists": activeExists,
		"random_mode":   randomCfg,
	})
}

// PlayActive serves the current LockChime.wav for playback.
func (h *ChimeHandler) PlayActive(w http.ResponseWriter, r *http.Request) {
	chimePath := filepath.Join(h.mountPath(), h.cfg.Web.LockChimeFilename)
	w.Header().Set("Content-Type", "audio/wav")
	http.ServeFile(w, r, chimePath)
}

// Play serves a specific chime file for playback.
func (h *ChimeHandler) Play(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]
	base := h.chimesDir()
	chimePath := filepath.Join(base, filepath.Clean(filename))
	if !strings.HasPrefix(chimePath, base+string(filepath.Separator)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	http.ServeFile(w, r, chimePath)
}

// Download serves a chime file as a download attachment.
func (h *ChimeHandler) Download(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]
	base := h.chimesDir()
	chimePath := filepath.Join(base, filepath.Clean(filename))
	if !strings.HasPrefix(chimePath, base+string(filepath.Separator)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, chimePath)
}

// Upload handles a single chime file upload.
func (h *ChimeHandler) Upload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no file uploaded"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read file"})
		return
	}

	normalize := r.FormValue("normalize") == "true"
	targetLUFS := -14.0
	if v := r.FormValue("target_lufs"); v != "" {
		fmt.Sscanf(v, "%f", &targetLUFS)
	}

	if err := h.chimeSvc.UploadChime(data, header.Filename, h.mountPath(), normalize, targetLUFS); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "uploaded", "filename": header.Filename})
}

// UploadBulk handles multiple chime file uploads at once.
func (h *ChimeHandler) UploadBulk(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(int64(h.cfg.Web.MaxUploadSizeMB) * 1024 * 1024); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse multipart form"})
		return
	}

	normalize := r.FormValue("normalize") == "true"
	targetLUFS := -14.0
	if v := r.FormValue("target_lufs"); v != "" {
		fmt.Sscanf(v, "%f", &targetLUFS)
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no files uploaded"})
		return
	}

	var results []map[string]string
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			results = append(results, map[string]string{"filename": fh.Filename, "status": "error", "error": err.Error()})
			continue
		}

		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			results = append(results, map[string]string{"filename": fh.Filename, "status": "error", "error": err.Error()})
			continue
		}

		if err := h.chimeSvc.UploadChime(data, fh.Filename, h.mountPath(), normalize, targetLUFS); err != nil {
			results = append(results, map[string]string{"filename": fh.Filename, "status": "error", "error": err.Error()})
			continue
		}
		results = append(results, map[string]string{"filename": fh.Filename, "status": "uploaded"})
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// SetActive copies the named chime to LockChime.wav.
func (h *ChimeHandler) SetActive(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]

	if err := h.chimeSvc.SetActiveChime(filename, h.mountPath()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "active", "chime": filename})
}

// Delete removes a chime file from the library.
func (h *ChimeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]

	if err := h.chimeSvc.DeleteChime(filename, h.mountPath()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "filename": filename})
}

// Rename renames a chime file in the library.
func (h *ChimeHandler) Rename(w http.ResponseWriter, r *http.Request) {
	oldName := mux.Vars(r)["old"]
	newName := mux.Vars(r)["new"]

	if err := h.chimeSvc.RenameChime(oldName, newName, h.mountPath()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed", "old": oldName, "new": newName})
}

// Filenames returns just the list of chime filenames (lightweight endpoint).
func (h *ChimeHandler) Filenames(w http.ResponseWriter, r *http.Request) {
	chimes := h.chimeSvc.ListChimes(h.mountPath())
	if chimes == nil {
		chimes = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"filenames": chimes})
}

// --- Scheduler endpoints ---

// AddSchedule creates a new chime schedule.
func (h *ChimeHandler) AddSchedule(w http.ResponseWriter, r *http.Request) {
	var sched chime.Schedule
	if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	id, err := h.chimeSvc.Scheduler().AddSchedule(sched)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "created", "id": id})
}

// ToggleSchedule enables or disables a schedule.
func (h *ChimeHandler) ToggleSchedule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	sched := h.chimeSvc.Scheduler().GetSchedule(id)
	if sched == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}

	if err := h.chimeSvc.Scheduler().UpdateSchedule(id, map[string]any{"enabled": !sched.Enabled}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "toggled", "id": id})
}

// DeleteSchedule removes a schedule by ID.
func (h *ChimeHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if err := h.chimeSvc.Scheduler().DeleteSchedule(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// GetSchedule returns a single schedule by ID.
func (h *ChimeHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	sched := h.chimeSvc.Scheduler().GetSchedule(id)
	if sched == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}

	writeJSON(w, http.StatusOK, sched)
}

// EditSchedule updates an existing schedule with provided fields.
func (h *ChimeHandler) EditSchedule(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if err := h.chimeSvc.Scheduler().UpdateSchedule(id, updates); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "id": id})
}

// --- Group endpoints ---

// ListGroups returns all chime groups.
func (h *ChimeHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	groups := h.chimeSvc.Groups().ListGroups()
	if groups == nil {
		groups = []chime.ChimeGroup{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

// CreateGroup creates a new chime group.
func (h *ChimeHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Chimes      []string `json:"chimes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	id, err := h.chimeSvc.Groups().CreateGroup(body.Name, body.Description, body.Chimes)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "created", "id": id})
}

// UpdateGroup updates a chime group's name, description, or chimes.
func (h *ChimeHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Chimes      []string `json:"chimes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if err := h.chimeSvc.Groups().UpdateGroup(id, body.Name, body.Description, body.Chimes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "id": id})
}

// DeleteGroup removes a chime group.
func (h *ChimeHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if err := h.chimeSvc.Groups().DeleteGroup(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// AddChimeToGroup adds a chime filename to a group.
func (h *ChimeHandler) AddChimeToGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var body struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if body.Filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "filename is required"})
		return
	}

	if err := h.chimeSvc.Groups().AddChimeToGroup(id, body.Filename); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "added", "group_id": id, "filename": body.Filename})
}

// RemoveChimeFromGroup removes a chime filename from a group.
func (h *ChimeHandler) RemoveChimeFromGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var body struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if body.Filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "filename is required"})
		return
	}

	if err := h.chimeSvc.Groups().RemoveChimeFromGroup(id, body.Filename); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed", "group_id": id, "filename": body.Filename})
}

// RandomMode enables or disables random chime selection from a group.
func (h *ChimeHandler) RandomMode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool   `json:"enabled"`
		GroupID string `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if err := h.chimeSvc.Groups().SetRandomMode(body.Enabled, body.GroupID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
