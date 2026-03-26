package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/video"
)

type VideoHandler struct {
	cfg      *config.Config
	videoSvc *video.Service
}

func NewVideoHandler(cfg *config.Config) *VideoHandler {
	return &VideoHandler{
		cfg:      cfg,
		videoSvc: video.NewService(cfg),
	}
}

// List returns TeslaCam folders or paginated events/sessions within a folder.
func (h *VideoHandler) List(w http.ResponseWriter, r *http.Request) {
	folder := r.URL.Query().Get("folder")

	if folder == "" {
		folders := h.videoSvc.GetFolders()
		if folders == nil {
			folders = []video.Folder{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"folders":       folders,
			"teslacam_path": h.videoSvc.GetTeslaCamPath(),
		})
		return
	}

	tcPath := h.videoSvc.GetTeslaCamPath()
	if tcPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "TeslaCam directory not found"})
		return
	}

	folderPath := filepath.Join(tcPath, filepath.Clean(folder))
	if !strings.HasPrefix(folderPath, tcPath) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder path"})
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage <= 0 {
		perPage = 20
	}

	mode := r.URL.Query().Get("mode")
	if mode == "sessions" {
		sessions, hasNext := h.videoSvc.GroupVideosBySession(folderPath, page, perPage)
		if sessions == nil {
			sessions = []video.SessionGroup{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"sessions": sessions,
			"page":     page,
			"per_page": perPage,
			"has_next": hasNext,
		})
		return
	}

	events, hasNext := h.videoSvc.GetEvents(folderPath, page, perPage)
	if events == nil {
		events = []video.Event{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":   events,
		"page":     page,
		"per_page": perPage,
		"has_next": hasNext,
	})
}

// Event returns details for a specific event within a folder.
func (h *VideoHandler) Event(w http.ResponseWriter, r *http.Request) {
	folder := mux.Vars(r)["folder"]
	event := mux.Vars(r)["event"]

	tcPath := h.videoSvc.GetTeslaCamPath()
	if tcPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "TeslaCam directory not found"})
		return
	}

	folderPath := filepath.Join(tcPath, filepath.Clean(folder))
	if !strings.HasPrefix(folderPath, tcPath) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder path"})
		return
	}

	details, err := h.videoSvc.GetEventDetails(folderPath, event)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, details)
}

// Stream serves a video file with HTTP Range support.
func (h *VideoHandler) Stream(w http.ResponseWriter, r *http.Request) {
	videoPath := h.resolveVideoPath(r)
	if videoPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video path"})
		return
	}

	if _, err := os.Stat(videoPath); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
		return
	}

	h.videoSvc.StreamVideo(w, r, videoPath)
}

// SEI serves a video file for client-side SEI metadata parsing (no range support).
func (h *VideoHandler) SEI(w http.ResponseWriter, r *http.Request) {
	videoPath := h.resolveVideoPath(r)
	if videoPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video path"})
		return
	}

	if _, err := os.Stat(videoPath); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
		return
	}

	h.videoSvc.ReadSEIData(w, r, videoPath)
}

// Download serves a single video file as an attachment.
func (h *VideoHandler) Download(w http.ResponseWriter, r *http.Request) {
	videoPath := h.resolveVideoPath(r)
	if videoPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video path"})
		return
	}

	if _, err := os.Stat(videoPath); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(videoPath)))
	http.ServeFile(w, r, videoPath)
}

// DownloadEvent creates and serves a ZIP of all videos in an event.
func (h *VideoHandler) DownloadEvent(w http.ResponseWriter, r *http.Request) {
	folder := mux.Vars(r)["folder"]
	event := mux.Vars(r)["event"]

	tcPath := h.videoSvc.GetTeslaCamPath()
	if tcPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "TeslaCam directory not found"})
		return
	}

	folderPath := filepath.Join(tcPath, filepath.Clean(folder))
	if !strings.HasPrefix(folderPath, tcPath) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder path"})
		return
	}

	zipPath, err := h.videoSvc.CreateEventZip(folderPath, event)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer os.Remove(zipPath)

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, event))
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, zipPath)
}

// Thumbnail generates and serves a thumbnail for the first valid camera in an event.
func (h *VideoHandler) Thumbnail(w http.ResponseWriter, r *http.Request) {
	folder := mux.Vars(r)["folder"]
	event := mux.Vars(r)["event"]

	tcPath := h.videoSvc.GetTeslaCamPath()
	if tcPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "TeslaCam directory not found"})
		return
	}

	folderPath := filepath.Join(tcPath, filepath.Clean(folder))
	if !strings.HasPrefix(folderPath, tcPath) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder path"})
		return
	}

	details, err := h.videoSvc.GetEventDetails(folderPath, event)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	camera := r.URL.Query().Get("camera")
	if camera == "" {
		camera = "front"
	}

	videoFile, ok := details.CameraVideos[camera]
	if !ok {
		for cam, f := range details.CameraVideos {
			camera = cam
			videoFile = f
			break
		}
	}
	if videoFile == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no video found for thumbnail"})
		return
	}

	videoFullPath := filepath.Join(folderPath, event, videoFile)
	hash := h.videoSvc.ThumbnailHash(videoFullPath)
	thumbPath := filepath.Join(h.cfg.ThumbnailDir, folder, event, camera+"_"+hash+".jpg")

	if _, err := os.Stat(thumbPath); err != nil {
		width, _ := strconv.Atoi(r.URL.Query().Get("w"))
		height, _ := strconv.Atoi(r.URL.Query().Get("h"))
		if width <= 0 {
			width = 320
		}
		if height <= 0 {
			height = 180
		}
		if err := h.videoSvc.GenerateThumbnail(videoFullPath, thumbPath, width, height); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "thumbnail generation failed: " + err.Error()})
			return
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, thumbPath)
}

// SessionThumbnail generates and serves a thumbnail for a session (RecentClips).
func (h *VideoHandler) SessionThumbnail(w http.ResponseWriter, r *http.Request) {
	folder := mux.Vars(r)["folder"]
	session := mux.Vars(r)["session"]

	tcPath := h.videoSvc.GetTeslaCamPath()
	if tcPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "TeslaCam directory not found"})
		return
	}

	folderPath := filepath.Join(tcPath, filepath.Clean(folder))
	if !strings.HasPrefix(folderPath, tcPath) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder path"})
		return
	}

	videos := h.videoSvc.GetSessionVideos(folderPath, session)
	if len(videos) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no videos found for session"})
		return
	}

	// Prefer front camera
	var target string
	for _, v := range videos {
		if strings.Contains(v, "-front.") {
			target = v
			break
		}
	}
	if target == "" {
		target = videos[0]
	}

	videoFullPath := filepath.Join(folderPath, target)
	hash := h.videoSvc.ThumbnailHash(videoFullPath)
	thumbPath := filepath.Join(h.cfg.ThumbnailDir, folder, "sessions", session+"_"+hash+".jpg")

	if _, err := os.Stat(thumbPath); err != nil {
		width, _ := strconv.Atoi(r.URL.Query().Get("w"))
		height, _ := strconv.Atoi(r.URL.Query().Get("h"))
		if width <= 0 {
			width = 320
		}
		if height <= 0 {
			height = 180
		}
		if err := h.videoSvc.GenerateThumbnail(videoFullPath, thumbPath, width, height); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "thumbnail generation failed: " + err.Error()})
			return
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, thumbPath)
}

// Delete removes an event and all its videos.
func (h *VideoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	folder := mux.Vars(r)["folder"]
	event := mux.Vars(r)["event"]

	tcPath := h.videoSvc.GetTeslaCamPath()
	if tcPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "TeslaCam directory not found"})
		return
	}

	folderPath := filepath.Join(tcPath, filepath.Clean(folder))
	if !strings.HasPrefix(folderPath, tcPath) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid folder path"})
		return
	}

	if err := h.videoSvc.DeleteEvent(folderPath, event); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "event": event})
}

// resolveVideoPath extracts and validates the video path from the wildcard URL segment.
func (h *VideoHandler) resolveVideoPath(r *http.Request) string {
	tcPath := h.videoSvc.GetTeslaCamPath()
	if tcPath == "" {
		return ""
	}

	wildcard := mux.Vars(r)["rest"]
	if wildcard == "" {
		return ""
	}

	fullPath := filepath.Join(tcPath, filepath.Clean(wildcard))
	if !strings.HasPrefix(fullPath, tcPath) {
		return ""
	}
	return fullPath
}
