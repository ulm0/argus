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
	"time"

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

// thumbSem caps concurrent ffmpeg thumbnail processes. A cold list page makes
// the browser open ~6 thumbnail requests at once, and that many ffmpeg
// processes starve the video stream (or the OOM killer) on a 512MB Pi Zero.
// ponytail: single global slot; raise to 2 only if list scrolling measurably drags.
var thumbSem = make(chan struct{}, 1)

func (h *VideoHandler) generateThumbnailOnce(videoPath, thumbPath string, width, height int) error {
	f := &thumbFlight{done: make(chan struct{})}
	if actual, loaded := thumbFlights.LoadOrStore(thumbPath, f); loaded {
		<-actual.(*thumbFlight).done
		return actual.(*thumbFlight).err
	}
	thumbSem <- struct{}{}
	f.err = h.videoSvc.GenerateThumbnail(videoPath, thumbPath, width, height)
	<-thumbSem
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

	folderPath := h.resolveFolderPath(folder)
	if folderPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	if page < 0 {
		page = 0
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

	before := r.URL.Query().Get("before")
	if before != "" {
		if _, err := time.Parse("2006-01-02", before); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "before must be YYYY-MM-DD"})
			return
		}
	}

	events, hasNext := h.videoSvc.GetEvents(folderPath, page, perPage, before)
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
	details.Prev, details.Next = h.videoSvc.EventNeighbors(folderPath, event)
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

	h.videoSvc.ExtractTelemetry(r.Context(), w, videoPath)
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

// maxExportSeconds bounds a trim export. The point is a shareable clip, and an
// unbounded window would just be the whole-file download with ffmpeg attached.
const maxExportSeconds = 300

// exportSem caps concurrent ffmpeg export processes. Kept separate from thumbSem
// so an export the user is waiting on never queues behind background thumbnail
// work — but it still has to be bounded, or N tabs hitting export fork N ffmpegs
// on a single-core 512MB Pi.
// ponytail: single global slot; raise to 2 only if concurrent exports become real.
var exportSem = make(chan struct{}, 1)

// deferredAttachment holds back the download headers until the encoder actually
// produces a byte. Setting them up front committed the response as a successful
// file download, so any ffmpeg failure (bad range, unreadable file, ffmpeg
// missing) delivered a 0-byte .mp4 with a 200 and only a log line to explain it.
// It stays a pass-through writer rather than a buffer: clips are tens of MB and
// the device has 512MB.
type deferredAttachment struct {
	w       http.ResponseWriter
	setup   func()
	written bool
}

func (d *deferredAttachment) Write(p []byte) (int, error) {
	if !d.written && len(p) > 0 {
		d.written = true
		d.setup()
	}
	return d.w.Write(p)
}

// Export streams a trimmed section of a single camera file as MP4.
func (h *VideoHandler) Export(w http.ResponseWriter, r *http.Request) {
	videoPath := h.resolveVideoPath(r)
	if videoPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video path"})
		return
	}

	if _, err := os.Stat(videoPath); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
		return
	}

	start, errStart := strconv.ParseFloat(r.URL.Query().Get("start"), 64)
	end, errEnd := strconv.ParseFloat(r.URL.Query().Get("end"), 64)
	if errStart != nil || errEnd != nil || start < 0 || end <= start || end-start > maxExportSeconds {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "start/end must be seconds, end > start, at most 300s apart"})
		return
	}

	// Wait for a slot before any header goes out, so a client that gives up on the
	// queue (flaky link, closed tab) costs nothing instead of holding an ffmpeg.
	select {
	case exportSem <- struct{}{}:
	case <-r.Context().Done():
		return
	}
	defer func() { <-exportSem }()

	name := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	out := &deferredAttachment{w: w, setup: func() {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%.0f.mp4"`, name, start))
	}}

	err := h.videoSvc.ExportClip(r.Context(), out, videoPath, start, end)
	if err != nil {
		logger.L.WithField("video", videoPath).WithError(err).Error("clip export failed")
	}
	if !out.written {
		// Nothing was committed yet, so the failure can still be an honest error
		// instead of an empty attachment. A zero-byte success counts as failure.
		writeJSONError(w, http.StatusInternalServerError, "clip export failed")
	}
	// Past the first byte the response is already a file download; the log line
	// above is the only place a mid-stream failure can be reported.
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

	camera := r.URL.Query().Get("camera")

	// Tesla pre-generates thumb.png for most events — serve it directly. It is the
	// front view, so the shortcut is only valid for the front camera: taking it for
	// every ?camera= would render all six tiles of the player's camera strip with
	// the same image. Keeping it for "front" matters, because that is the tile the
	// event list and the strip both request, and the fallback path forks ffmpeg.
	if (camera == "" || camera == "front") && details.HasThumbnail {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, filepath.Join(folderPath, event, "thumb.png"))
		return
	}

	if camera == "" {
		camera = "front"
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
	if !withinBase(thumbPath, h.cfg.ThumbnailDir) {
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
		if width > 1920 {
			width = 1920
		}
		if height > 1080 {
			height = 1080
		}
		if err := h.generateThumbnailOnce(videoFullPath, thumbPath, width, height); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "thumbnail generation failed: "+err.Error())
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
	if !withinBase(thumbPath, h.cfg.ThumbnailDir) {
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
		if width > 1920 {
			width = 1920
		}
		if height > 1080 {
			height = 1080
		}
		if err := h.generateThumbnailOnce(videoFullPath, thumbPath, width, height); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "thumbnail generation failed: "+err.Error())
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
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Prune any generated thumbnails for this event so they don't accumulate on
	// the SD card once the source videos are gone. Best-effort: they are written
	// to ThumbnailDir/{folder}/{event}/ (see Thumbnail). folder and event are
	// already validated against traversal by resolveFolderPath/DeleteEvent above,
	// but this is a recursive delete built from request input, so it gets the same
	// containment check as every other ThumbnailDir path in this file.
	thumbDir := filepath.Join(h.cfg.ThumbnailDir, folder, event)
	if withinBase(thumbDir, h.cfg.ThumbnailDir) {
		if err := os.RemoveAll(thumbDir); err != nil {
			logger.L.WithField("folder", folder).WithField("event", event).WithError(err).Warn("failed to prune event thumbnails")
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "event": event})
}

// Keep toggles the marker that exempts an event from automatic cleanup.
func (h *VideoHandler) Keep(w http.ResponseWriter, r *http.Request) {
	if h.modeSvc.CurrentMode().Token != "edit" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "keep requires edit mode"})
		return
	}

	folder := mux.Vars(r)["folder"]
	event := mux.Vars(r)["event"]

	folderPath := h.resolveFolderPath(folder)
	if folderPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	kept, err := h.videoSvc.ToggleKeep(folderPath, event)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"kept": kept})
}

// withinBase reports whether p is base itself or nested under it. The trailing
// separator prevents sibling-directory escapes that a bare strings.HasPrefix
// allows (e.g. base="/mnt/TeslaCam" p="/mnt/TeslaCam-evil").
func withinBase(p, base string) bool {
	return p == base || strings.HasPrefix(p, base+string(filepath.Separator))
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
		if !withinBase(p, archivePath) {
			return ""
		}
		return p
	}

	tcPath := h.videoSvc.GetTeslaCamPath()
	if tcPath == "" {
		return ""
	}
	p := filepath.Join(tcPath, filepath.Clean(folder))
	if !withinBase(p, tcPath) {
		return ""
	}
	return p
}

// resolveVideoPath extracts and validates the video path from the wildcard URL segment.
// The wildcard is a folder-relative path, so it obeys the same "archive/" prefix rule
// as resolveFolderPath — the UI lists archive folders as "archive/<name>" and streams
// files from them under that same prefix.
func (h *VideoHandler) resolveVideoPath(r *http.Request) string {
	wildcard := mux.Vars(r)["rest"]
	if wildcard == "" {
		return ""
	}
	p := h.resolveFolderPath(wildcard)
	if p == "" {
		return ""
	}
	// A wildcard that cleans to a directory ("." or "archive/") resolves to the
	// media root, and http.ServeFile would answer with a directory listing.
	// These routes only ever serve one file.
	if info, err := os.Stat(p); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return p
}
