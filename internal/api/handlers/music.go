package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/music"
	partutil "github.com/ulm0/argus/internal/services/partition"
)

type MusicHandler struct {
	cfg      *config.Config
	musicSvc *music.Service
}

func NewMusicHandler(cfg *config.Config) *MusicHandler {
	return &MusicHandler{
		cfg:      cfg,
		musicSvc: music.NewService(cfg),
	}
}

func (h *MusicHandler) List(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")

	mountPath := h.resolveMusicMount()
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "music partition not available"})
		return
	}

	result, err := h.musicSvc.ListFiles(mountPath, relPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *MusicHandler) Upload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file field"})
		return
	}
	defer file.Close()

	relPath := r.FormValue("path")

	mountPath := h.editMusicMount()
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "music partition not available for writing"})
		return
	}

	if err := h.musicSvc.SaveFile(file, header.Filename, mountPath, relPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "filename": header.Filename})
}

func (h *MusicHandler) UploadChunk(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("chunk")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing chunk field"})
		return
	}
	defer file.Close()

	uploadID := r.FormValue("upload_id")
	filename := r.FormValue("filename")
	relPath := r.FormValue("path")

	chunkIndex, err := strconv.Atoi(r.FormValue("chunk_index"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid chunk_index"})
		return
	}

	totalChunks, err := strconv.Atoi(r.FormValue("total_chunks"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid total_chunks"})
		return
	}

	if uploadID == "" {
		uploadID = music.GenerateUploadID()
	}

	mountPath := h.editMusicMount()
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "music partition not available for writing"})
		return
	}

	complete, err := h.musicSvc.HandleChunk(uploadID, filename, chunkIndex, totalChunks, file, mountPath, relPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"upload_id": uploadID,
		"complete":  complete,
	})
}

func (h *MusicHandler) Delete(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/music/delete/")

	mountPath := h.editMusicMount()
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "music partition not available for writing"})
		return
	}

	if err := h.musicSvc.DeleteFile(mountPath, relPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MusicHandler) DeleteDir(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/music/delete-dir/")

	mountPath := h.editMusicMount()
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "music partition not available for writing"})
		return
	}

	if err := h.musicSvc.DeleteDirectory(mountPath, relPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MusicHandler) Move(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
		NewName     string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	mountPath := h.editMusicMount()
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "music partition not available for writing"})
		return
	}

	if err := h.musicSvc.MoveFile(mountPath, req.Source, req.Destination, req.NewName); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MusicHandler) Mkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	mountPath := h.editMusicMount()
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "music partition not available for writing"})
		return
	}

	if err := h.musicSvc.CreateDirectory(mountPath, req.Path, req.Name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MusicHandler) Play(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/music/play/")

	mountPath := h.resolveMusicMount()
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "music partition not available"})
		return
	}

	fullPath, err := h.musicSvc.ResolvePath(mountPath, relPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	http.ServeFile(w, r, fullPath)
}

func (h *MusicHandler) resolveMusicMount() string {
	if !h.cfg.DiskImages.MusicEnabled {
		return ""
	}
	p := partutil.AccessiblePath(h.cfg, "part3")
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}
	return ""
}

func (h *MusicHandler) editMusicMount() string {
	if !h.cfg.DiskImages.MusicEnabled {
		return ""
	}
	path := h.cfg.MountPath("part3", false)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return ""
}
