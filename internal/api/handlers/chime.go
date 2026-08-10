package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/chime"
	partutil "github.com/ulm0/argus/internal/services/partition"
)

type ChimeHandler struct {
	cfg      *config.Config
	chimeSvc *chime.Service
}

// NewChimeHandler wires the HTTP API to a chime service. svc must be the
// process-wide instance from run.go (the one whose scheduler tick actually
// fires and persists); a separate instance would keep an independent in-memory
// cache, so API edits to schedules/groups would never take effect and could be
// clobbered by the running scheduler's next save. If svc is nil, a new service
// is created (tests only).
func NewChimeHandler(cfg *config.Config, svc *chime.Service) *ChimeHandler {
	if svc == nil {
		svc = chime.NewService(cfg)
	}
	return &ChimeHandler{
		cfg:      cfg,
		chimeSvc: svc,
	}
}

// Lock chimes live on the LightShow partition (part2), same as TeslaUSB.
func (h *ChimeHandler) mountPath() string {
	p := partutil.AccessiblePath(h.cfg, "part2")
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}
	return ""
}

// editMountPath returns the read-write mount, which only exists in Edit mode.
// Present mode exposes part2 read-only, so writing through AccessiblePath would
// fail with EROFS halfway through instead of a clean 503.
func (h *ChimeHandler) editMountPath() string {
	return editMountPath(h.cfg, "part2")
}

func (h *ChimeHandler) chimesDir() string {
	return filepath.Join(h.mountPath(), h.cfg.Web.ChimesFolder)
}

func (h *ChimeHandler) boomboxDir() string {
	return filepath.Join(h.mountPath(), h.cfg.Web.BoomboxFolder)
}

// List returns all chimes plus active chime info.
func (h *ChimeHandler) List(w http.ResponseWriter, r *http.Request) {
	mp := h.mountPath()
	chimes := h.chimeSvc.ListChimes(mp, h.cfg.Web.ChimesFolder)
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
	h.serveAudio(w, r, h.chimesDir())
}

func (h *ChimeHandler) serveAudio(w http.ResponseWriter, r *http.Request, base string) {
	filename := mux.Vars(r)["filename"]
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

// maxChimeUploadSize caps a single chime upload. Chimes are short WAV clips;
// io.ReadAll would otherwise materialize the whole part in RAM, so on the
// ~400 MB device a large accidental upload (e.g. an MP4) could OOM-kill argus.
const maxChimeUploadSize = 32 << 20 // 32 MiB

// Upload handles a single chime file upload.
func (h *ChimeHandler) Upload(w http.ResponseWriter, r *http.Request) {
	h.uploadOne(w, r, h.cfg.Web.ChimesFolder)
}

// uploadOne runs the shared upload pipeline (re-encode, optional normalize,
// validate) into the given library folder.
func (h *ChimeHandler) uploadOne(w http.ResponseWriter, r *http.Request, folder string) {
	mp := h.editMountPath()
	if mp == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available for writing"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no file uploaded"})
		return
	}
	defer file.Close()

	if header.Size > maxChimeUploadSize {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file too large"})
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxChimeUploadSize))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	normalize := r.FormValue("normalize") == "true"
	targetLUFS := -14.0
	if v := r.FormValue("target_lufs"); v != "" {
		fmt.Sscanf(v, "%f", &targetLUFS)
	}

	if err := h.chimeSvc.UploadChime(data, header.Filename, mp, folder, normalize, targetLUFS); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "uploaded", "filename": header.Filename})
}

// UploadBulk handles multiple chime file uploads at once.
func (h *ChimeHandler) UploadBulk(w http.ResponseWriter, r *http.Request) {
	mp := h.editMountPath()
	if mp == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available for writing"})
		return
	}

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
		if fh.Size > maxChimeUploadSize {
			results = append(results, map[string]string{"filename": fh.Filename, "status": "error", "error": "file too large"})
			continue
		}

		f, err := fh.Open()
		if err != nil {
			results = append(results, map[string]string{"filename": fh.Filename, "status": "error", "error": err.Error()})
			continue
		}

		data, err := io.ReadAll(io.LimitReader(f, maxChimeUploadSize))
		f.Close()
		if err != nil {
			results = append(results, map[string]string{"filename": fh.Filename, "status": "error", "error": err.Error()})
			continue
		}

		if err := h.chimeSvc.UploadChime(data, fh.Filename, mp, h.cfg.Web.ChimesFolder, normalize, targetLUFS); err != nil {
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

	mp := h.editMountPath()
	if mp == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available for writing"})
		return
	}

	if err := h.chimeSvc.SetActiveChime(filename, mp); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "active", "chime": filename})
}

// Delete removes a chime file from the library.
func (h *ChimeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	h.deleteOne(w, r, h.cfg.Web.ChimesFolder)
}

func (h *ChimeHandler) deleteOne(w http.ResponseWriter, r *http.Request, folder string) {
	filename := mux.Vars(r)["filename"]

	mp := h.editMountPath()
	if mp == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available for writing"})
		return
	}

	if err := h.chimeSvc.DeleteChime(filename, mp, folder); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "filename": filename})
}

// Rename renames a chime file in the library.
func (h *ChimeHandler) Rename(w http.ResponseWriter, r *http.Request) {
	oldName := mux.Vars(r)["old"]
	newName := mux.Vars(r)["new"]

	mp := h.editMountPath()
	if mp == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available for writing"})
		return
	}

	if err := h.chimeSvc.RenameChime(oldName, newName, mp); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed", "old": oldName, "new": newName})
}

// Filenames returns just the list of chime filenames (lightweight endpoint).
func (h *ChimeHandler) Filenames(w http.ResponseWriter, r *http.Request) {
	chimes := h.chimeSvc.ListChimes(h.mountPath(), h.cfg.Web.ChimesFolder)
	if chimes == nil {
		chimes = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"filenames": chimes})
}

// --- Boombox endpoints (same pipeline as chimes, different folder) ---

// maxBoomboxPlayable is how many Boombox sounds the car actually offers: the
// first 5 files alphabetically. Extra files are stored but unreachable.
const maxBoomboxPlayable = 5

// ListBoombox returns the Boombox sound library.
func (h *ChimeHandler) ListBoombox(w http.ResponseWriter, r *http.Request) {
	sounds := h.chimeSvc.ListChimes(h.mountPath(), h.cfg.Web.BoomboxFolder)
	if sounds == nil {
		sounds = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sounds": sounds, "max_playable": maxBoomboxPlayable})
}

// UploadBoombox handles a single Boombox sound upload.
func (h *ChimeHandler) UploadBoombox(w http.ResponseWriter, r *http.Request) {
	h.uploadOne(w, r, h.cfg.Web.BoomboxFolder)
}

// PlayBoombox serves a Boombox sound for playback.
func (h *ChimeHandler) PlayBoombox(w http.ResponseWriter, r *http.Request) {
	h.serveAudio(w, r, h.boomboxDir())
}

// DeleteBoombox removes a Boombox sound.
func (h *ChimeHandler) DeleteBoombox(w http.ResponseWriter, r *http.Request) {
	h.deleteOne(w, r, h.cfg.Web.BoomboxFolder)
}

// --- Scheduler endpoints ---

// ListSchedules returns every schedule, enabled or not.
func (h *ChimeHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"schedules": h.chimeSvc.Scheduler().ListSchedules(false)})
}

// AddSchedule creates a new chime schedule.
func (h *ChimeHandler) AddSchedule(w http.ResponseWriter, r *http.Request) {
	var sched chime.Schedule
	if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	id, err := h.chimeSvc.Scheduler().AddSchedule(sched)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
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
		writeJSONError(w, http.StatusInternalServerError, err.Error())
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
		writeJSONError(w, http.StatusInternalServerError, err.Error())
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
		writeJSONError(w, http.StatusInternalServerError, err.Error())
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
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "id": id})
}

// DeleteGroup removes a chime group.
func (h *ChimeHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if err := h.chimeSvc.Groups().DeleteGroup(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
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
		writeJSONError(w, http.StatusInternalServerError, err.Error())
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
		writeJSONError(w, http.StatusInternalServerError, err.Error())
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
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
