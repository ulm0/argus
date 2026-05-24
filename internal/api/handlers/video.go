package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/mux"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
	"github.com/ulm0/argus/internal/services/mode"
	"github.com/ulm0/argus/internal/services/video"
)

// thumbFlight deduplicates concurrent ffmpeg thumbnail generation for the same path.
type thumbFlight struct {
	done chan struct{}
	err  error
}

var thumbFlights sync.Map // key: thumbPath → *thumbFlight

func (h *VideoHandler) generateThumbnailOnce(videoPath, thumbPath string, width, height int) error {
	f := &thumbFlight{done: make(chan struct{})}
	if actual, loaded := thumbFlights.LoadOrStore(thumbPath, f); loaded {
		<-actual.(*thumbFlight).done
		return actual.(*thumbFlight).err
	}
	f.err = h.videoSvc.GenerateThumbnail(videoPath, thumbPath, width, height)
	close(f.done)
	thumbFlights.Delete(thumbPath)
	return f.err
}

type VideoHandler struct {
	cfg      *config.Config
	videoSvc *video.Service
	modeSvc  *mode.Service
}

func NewVideoHandler(cfg *config.Config) *VideoHandler {
	return &VideoHandler{
		cfg:      cfg,
		videoSvc: video.NewService(cfg),
		modeSvc:  mode.NewService(cfg),
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

	folderPath := h.resolveFolderPath(folder)
	if folderPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
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

// Telemetry extracts Tesla SEI telemetry from an MP4 file and serves it as JSON.
func (h *VideoHandler) Telemetry(w http.ResponseWriter, r *http.Request) {
	videoPath := h.resolveVideoPath(r)
	if videoPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video path"})
		return
	}

	if _, err := os.Stat(videoPath); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
		return
	}

	h.videoSvc.ExtractTelemetry(w, videoPath)
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

// DownloadEvent streams a ZIP of all videos + metadata for an event.
//
// The archive is generated on the fly with archive/zip rather than the
// external `zip(1)` binary: the previous implementation depended on the
// host having info-zip installed and tripped over a 0-byte temp file
// (CreateTemp + Close), which `zip` interpreted as a corrupted update
// target and rejected with exit status 3.
func (h *VideoHandler) DownloadEvent(w http.ResponseWriter, r *http.Request) {
	folder := mux.Vars(r)["folder"]
	event := mux.Vars(r)["event"]

	folderPath := h.resolveFolderPath(folder)
	if folderPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	files, err := h.videoSvc.CollectEventArchiveFiles(folderPath, event)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "no archivable files") {
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, event))
	if err := video.WriteEventZip(w, files); err != nil {
		// Headers are already flushed; we can't switch to a JSON error here.
		// Surface the failure in the logs so it isn't silently swallowed.
		logger.L.WithField("folder", folder).WithField("event", event).WithError(err).Error("event zip stream failed")
	}
}

// Thumbnail generates and serves a thumbnail for the first valid camera in an event.
func (h *VideoHandler) Thumbnail(w http.ResponseWriter, r *http.Request) {
	folder := mux.Vars(r)["folder"]
	event := mux.Vars(r)["event"]

	folderPath := h.resolveFolderPath(folder)
	if folderPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	details, err := h.videoSvc.GetEventDetails(folderPath, event)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	// Tesla pre-generates thumb.png for most events — serve it directly.
	if details.HasThumbnail {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, filepath.Join(folderPath, event, "thumb.png"))
		return
	}

	camera := r.URL.Query().Get("camera")
	if camera == "" {
		camera = "front"
	}
	// camera must be a single clean path component (no traversal, no separators).
	if strings.ContainsAny(camera, "/\\") || strings.Contains(camera, "..") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid camera"})
		return
	}

	// Pick the requested camera if it exists and is not encrypted.
	// Otherwise fall back to the first non-encrypted camera available.
	videoFile, ok := details.CameraVideos[camera]
	if !ok || details.Encrypted[camera] {
		videoFile = ""
		camera = ""
		for cam, f := range details.CameraVideos {
			if !details.Encrypted[cam] {
				camera = cam
				videoFile = f
				break
			}
		}
	}
	if videoFile == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no unencrypted video available for thumbnail"})
		return
	}

	videoFullPath := filepath.Join(folderPath, event, videoFile)
	hash := h.videoSvc.ThumbnailHash(videoFullPath)
	thumbPath := filepath.Join(h.cfg.ThumbnailDir, folder, event, camera+"_"+hash+".jpg")

	// Ensure the resolved thumbnail path stays within ThumbnailDir.
	thumbDir := filepath.Clean(h.cfg.ThumbnailDir)
	if !strings.HasPrefix(filepath.Clean(thumbPath), thumbDir+string(filepath.Separator)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	if _, err := os.Stat(thumbPath); err != nil {
		width, _ := strconv.Atoi(r.URL.Query().Get("w"))
		height, _ := strconv.Atoi(r.URL.Query().Get("h"))
		if width <= 0 {
			width = 320
		}
		if height <= 0 {
			height = 180
		}
		if err := h.generateThumbnailOnce(videoFullPath, thumbPath, width, height); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "thumbnail generation failed: " + err.Error()})
			return
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, thumbPath)
}

// SessionDetail returns a VideoEvent-shaped response for a RecentClips session.
func (h *VideoHandler) SessionDetail(w http.ResponseWriter, r *http.Request) {
	folder := mux.Vars(r)["folder"]
	session := mux.Vars(r)["session"]

	folderPath := h.resolveFolderPath(folder)
	if folderPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	filenames := h.videoSvc.GetSessionVideos(folderPath, session)
	if len(filenames) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}

	cameraVideos := make(map[string]string)
	encrypted := make(map[string]bool)
	var totalBytes int64

	for _, filename := range filenames {
		_, camera, ok := video.ParseSessionFromFilename(filename)
		if !ok {
			continue
		}
		cameraVideos[camera] = filename
		fullPath := filepath.Join(folderPath, filename)
		encrypted[camera] = !h.videoSvc.IsValidMP4(fullPath)
		if info, err := os.Stat(fullPath); err == nil {
			totalBytes += info.Size()
		}
	}

	if len(cameraVideos) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no valid videos in session"})
		return
	}

	// session format is "2024-01-01_12-00-00" → datetime "2024-01-01T12:00:00"
	datetime := ""
	if len(session) >= 19 {
		datetime = session[:10] + "T" + strings.ReplaceAll(session[11:19], "-", ":")
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":                session,
		"camera_videos":       cameraVideos,
		"encrypted_videos":    encrypted,
		"clips":               []string{session},
		"starting_clip_index": 0,
		"datetime":            datetime,
		"size_mb":             float64(totalBytes) / (1024 * 1024),
	})
}

// SessionThumbnail generates and serves a thumbnail for a session (RecentClips).
func (h *VideoHandler) SessionThumbnail(w http.ResponseWriter, r *http.Request) {
	folder := mux.Vars(r)["folder"]
	session := mux.Vars(r)["session"]

	folderPath := h.resolveFolderPath(folder)
	if folderPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	videos := h.videoSvc.GetSessionVideos(folderPath, session)
	if len(videos) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no videos found for session"})
		return
	}

	// Prefer front camera; skip encrypted (invalid MP4) files.
	var target string
	for _, v := range videos {
		if strings.Contains(v, "-front.") && h.videoSvc.IsValidMP4(filepath.Join(folderPath, v)) {
			target = v
			break
		}
	}
	if target == "" {
		for _, v := range videos {
			if h.videoSvc.IsValidMP4(filepath.Join(folderPath, v)) {
				target = v
				break
			}
		}
	}
	if target == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no unencrypted video available for thumbnail"})
		return
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
		if err := h.generateThumbnailOnce(videoFullPath, thumbPath, width, height); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "thumbnail generation failed: " + err.Error()})
			return
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, thumbPath)
}

// Delete removes an event and all its videos.
func (h *VideoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.modeSvc.CurrentMode().Token != "edit" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "delete requires edit mode"})
		return
	}

	folder := mux.Vars(r)["folder"]
	event := mux.Vars(r)["event"]

	folderPath := h.resolveFolderPath(folder)
	if folderPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	if err := h.videoSvc.DeleteEvent(folderPath, event); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "event": event})
}

// resolveFolderPath resolves a folder name (from the API) to its absolute path on disk.
// Folders prefixed with "archive/" are resolved against the archive TeslaCam path.
func (h *VideoHandler) resolveFolderPath(folder string) string {
	if strings.HasPrefix(folder, "archive/") {
		archivePath := h.videoSvc.GetArchivePath()
		if archivePath == "" {
			return ""
		}
		sub := strings.TrimPrefix(folder, "archive/")
		p := filepath.Join(archivePath, filepath.Clean(sub))
		if !strings.HasPrefix(p, archivePath) {
			return ""
		}
		return p
	}

	tcPath := h.videoSvc.GetTeslaCamPath()
	if tcPath == "" {
		return ""
	}
	p := filepath.Join(tcPath, filepath.Clean(folder))
	if !strings.HasPrefix(p, tcPath) {
		return ""
	}
	return p
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
