package video

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

var sessionPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2})-(.+)\.\w+$`)

const tcPathCacheTTL = 30 * time.Second
const eventCacheTTL = 30 * time.Second

// maxCacheEntries bounds each in-memory cache so a client paging with arbitrary
// page/per_page values (distinct keys) can't grow the maps without limit on a
// memory-constrained Pi. When the limit is hit the cache is reset wholesale —
// it is only an optimization, so correctness is unaffected.
const maxCacheEntries = 512

type mp4CacheEntry struct {
	valid bool
	mtime int64
	size  int64
}

type eventCacheEntry struct {
	events  []Event
	hasNext bool
	expiry  time.Time
}

type sessionCacheEntry struct {
	sessions []SessionGroup
	hasNext  bool
	expiry   time.Time
}

type Service struct {
	cfg *config.Config

	tcPathMu  sync.Mutex
	tcPath    string
	tcPathExp time.Time

	mp4Mu    sync.RWMutex
	mp4Cache map[string]mp4CacheEntry

	eventMu    sync.RWMutex
	eventCache map[string]eventCacheEntry

	sessionMu    sync.RWMutex
	sessionCache map[string]sessionCacheEntry
}

func NewService(cfg *config.Config) *Service {
	return &Service{
		cfg:          cfg,
		mp4Cache:     make(map[string]mp4CacheEntry),
		eventCache:   make(map[string]eventCacheEntry),
		sessionCache: make(map[string]sessionCacheEntry),
	}
}

// KeepMarker is an empty file placed inside an event directory to exempt that
// event from automatic cleanup.
const KeepMarker = ".argus-keep"

type Event struct {
	Name              string            `json:"name"`
	Datetime          string            `json:"datetime"`
	City              string            `json:"city"`
	Reason            string            `json:"reason"`
	SizeMB            float64           `json:"size_mb"`
	HasThumbnail      bool              `json:"has_thumbnail"`
	CameraVideos      map[string]string `json:"camera_videos"`
	Encrypted         map[string]bool   `json:"encrypted_videos"`
	Clips             []string          `json:"clips,omitempty"`
	StartingClipIndex int               `json:"starting_clip_index"`
	TriggerOffsetSec  float64           `json:"trigger_offset_sec"`
	Kept              bool              `json:"kept"`
	// Pointers because 0 is a legal coordinate: with plain float64 + omitempty an
	// event on the equator or the prime meridian would drop the field entirely.
	EstLat *float64 `json:"est_lat,omitempty"`
	EstLon *float64 `json:"est_lon,omitempty"`
	Camera string   `json:"camera,omitempty"`
	// Neighbouring event directory names in newest-first order. Only the
	// single-event path fills these in (see EventNeighbors).
	Prev string `json:"prev,omitempty"`
	Next string `json:"next,omitempty"`
}

type Folder struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Count int    `json:"count"`
}

type SessionGroup struct {
	Session   string   `json:"session"`
	Cameras   []string `json:"cameras"`
	Timestamp string   `json:"timestamp"`
}

// GetTeslaCamPath finds the TeslaCam directory on the mounted partition.
// The result is cached for tcPathCacheTTL to avoid repeated stat calls.
func (s *Service) GetTeslaCamPath() string {
	s.tcPathMu.Lock()
	if s.tcPath != "" && time.Now().Before(s.tcPathExp) {
		p := s.tcPath
		s.tcPathMu.Unlock()
		return p
	}
	s.tcPathMu.Unlock()

	var found string
	for _, ro := range []bool{true, false} {
		base := s.cfg.MountPath("part1", ro)
		tcPath := filepath.Join(base, "TeslaCam")
		if info, err := os.Stat(tcPath); err == nil && info.IsDir() {
			found = tcPath
			break
		}
	}

	s.tcPathMu.Lock()
	s.tcPath = found
	s.tcPathExp = time.Now().Add(tcPathCacheTTL)
	s.tcPathMu.Unlock()
	return found
}

// GetArchivePath returns the TeslaCam path inside the configured archive directory, or "".
func (s *Service) GetArchivePath() string {
	if s.cfg.Installation.ArchivePath == "" {
		return ""
	}
	p := filepath.Join(s.cfg.Installation.ArchivePath, "TeslaCam")
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}
	return ""
}

// GetFolders returns the TeslaCam subfolders (SavedClips, SentryClips, RecentClips, etc.).
func (s *Service) GetFolders() []Folder {
	tcPath := s.GetTeslaCamPath()
	if tcPath == "" {
		return nil
	}

	entries, err := os.ReadDir(tcPath)
	if err != nil {
		return nil
	}

	var folders []Folder
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		count := countVideoFiles(filepath.Join(tcPath, name))
		folders = append(folders, Folder{Name: name, Path: name, Count: count})
	}

	if archivePath := s.GetArchivePath(); archivePath != "" {
		archiveEntries, err := os.ReadDir(archivePath)
		if err == nil {
			for _, e := range archiveEntries {
				if !e.IsDir() || e.Name() == "." || e.Name() == ".." {
					continue
				}
				count := countVideoFiles(filepath.Join(archivePath, e.Name()))
				folders = append(folders, Folder{
					Name:  e.Name() + " (Archive)",
					Path:  "archive/" + e.Name(),
					Count: count,
				})
			}
		}
	}

	return folders
}

// GetEvents returns paginated events from a TeslaCam subfolder. A non-empty
// before ("YYYY-MM-DD") skips everything recorded after that day.
func (s *Service) GetEvents(folderPath string, page, perPage int, before string) ([]Event, bool) {
	key := fmt.Sprintf("%s:%d:%d:%s", folderPath, page, perPage, before)

	s.eventMu.RLock()
	if entry, ok := s.eventCache[key]; ok && time.Now().Before(entry.expiry) {
		events, hasNext := entry.events, entry.hasNext
		s.eventMu.RUnlock()
		return events, hasNext
	}
	s.eventMu.RUnlock()

	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, false
	}

	// Filter to directories only (events)
	var dirs []fs.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}

	// Sort by name descending (newest first)
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Name() > dirs[j].Name()
	})

	// Event directories are timestamp-named ("2024-01-02_15-04-05"), so a
	// lexical upper bound is a date bound.
	if before != "" {
		cutoff := before + "_99"
		kept := dirs[:0]
		for _, d := range dirs {
			if d.Name() <= cutoff {
				kept = append(kept, d)
			}
		}
		dirs = kept
	}

	// Paginate
	start := page * perPage
	if start >= len(dirs) {
		return nil, false
	}
	end := start + perPage
	hasNext := end < len(dirs)
	if end > len(dirs) {
		end = len(dirs)
	}

	var events []Event
	for _, d := range dirs[start:end] {
		eventDir := filepath.Join(folderPath, d.Name())
		event := s.parseEvent(eventDir, d.Name())
		events = append(events, event)
	}

	s.eventMu.Lock()
	if len(s.eventCache) >= maxCacheEntries {
		s.eventCache = make(map[string]eventCacheEntry)
	}
	s.eventCache[key] = eventCacheEntry{events: events, hasNext: hasNext, expiry: time.Now().Add(eventCacheTTL)}
	s.eventMu.Unlock()

	return events, hasNext
}

// GetEventDetails returns full details for a single event.
func (s *Service) GetEventDetails(folderPath, eventName string) (*Event, error) {
	eventDir := filepath.Join(folderPath, filepath.Clean(eventName))
	if !strings.HasPrefix(eventDir, folderPath+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid event name: path traversal detected")
	}
	if _, err := os.Stat(eventDir); err != nil {
		return nil, fmt.Errorf("event not found: %s", eventName)
	}

	event := s.parseEvent(eventDir, eventName)
	return &event, nil
}

// EventNeighbors returns the event directory names surrounding eventName in
// newest-first order ("" when there is none). It only reads the parent
// directory — no per-event parsing — so the UI can render prev/next chips
// without paging the whole folder through the API.
func (s *Service) EventNeighbors(folderPath, eventName string) (prev, next string) {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return "", ""
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))

	for i, n := range names {
		if n != eventName {
			continue
		}
		if i > 0 {
			prev = names[i-1]
		}
		if i < len(names)-1 {
			next = names[i+1]
		}
		break
	}
	return prev, next
}

// ToggleKeep flips the keep marker on an event and reports its new state.
func (s *Service) ToggleKeep(folderPath, eventName string) (bool, error) {
	eventDir := filepath.Join(folderPath, filepath.Clean(eventName))
	if !strings.HasPrefix(eventDir, folderPath+string(filepath.Separator)) {
		return false, fmt.Errorf("invalid event name: path traversal detected")
	}
	if info, err := os.Stat(eventDir); err != nil || !info.IsDir() {
		return false, fmt.Errorf("event not found: %s", eventName)
	}

	marker := filepath.Join(eventDir, KeepMarker)
	if fileExists(marker) {
		if err := os.Remove(marker); err != nil {
			return false, err
		}
		s.invalidateFolderCache(folderPath)
		return false, nil
	}

	f, err := os.Create(marker)
	if err != nil {
		return false, err
	}
	f.Close()
	s.invalidateFolderCache(folderPath)
	return true, nil
}

// GroupVideosBySession groups videos by their timestamp session for RecentClips.
func (s *Service) GroupVideosBySession(folderPath string, page, perPage int) ([]SessionGroup, bool) {
	key := fmt.Sprintf("%s:%d:%d", folderPath, page, perPage)

	s.sessionMu.RLock()
	if entry, ok := s.sessionCache[key]; ok && time.Now().Before(entry.expiry) {
		sessions, hasNext := entry.sessions, entry.hasNext
		s.sessionMu.RUnlock()
		return sessions, hasNext
	}
	s.sessionMu.RUnlock()

	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, false
	}

	sessions := make(map[string][]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := sessionPattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		session := m[1]
		camera := m[2]
		sessions[session] = append(sessions[session], camera)
	}

	// Sort sessions descending
	var keys []string
	for k := range sessions {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	start := page * perPage
	if start >= len(keys) {
		return nil, false
	}
	end := start + perPage
	hasNext := end < len(keys)
	if end > len(keys) {
		end = len(keys)
	}

	var groups []SessionGroup
	for _, k := range keys[start:end] {
		groups = append(groups, SessionGroup{
			Session:   k,
			Cameras:   sessions[k],
			Timestamp: k,
		})
	}

	s.sessionMu.Lock()
	if len(s.sessionCache) >= maxCacheEntries {
		s.sessionCache = make(map[string]sessionCacheEntry)
	}
	s.sessionCache[key] = sessionCacheEntry{sessions: groups, hasNext: hasNext, expiry: time.Now().Add(eventCacheTTL)}
	s.sessionMu.Unlock()

	return groups, hasNext
}

// GetSessionVideos returns all video files for a given session ID.
func (s *Service) GetSessionVideos(folderPath, sessionID string) []string {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil
	}

	var videos []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), sessionID+"-") {
			videos = append(videos, e.Name())
		}
	}
	return videos
}

// IsValidMP4 checks if a file starts with a valid MP4 ftyp box.
// Results are cached by path+mtime to avoid reopening the same file on each event listing.
func (s *Service) IsValidMP4(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	mtime := info.ModTime().UnixNano()
	size := info.Size()

	s.mp4Mu.RLock()
	if entry, ok := s.mp4Cache[path]; ok && entry.mtime == mtime && entry.size == size {
		v := entry.valid
		s.mp4Mu.RUnlock()
		return v
	}
	s.mp4Mu.RUnlock()

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 12)
	if _, err := io.ReadFull(f, buf); err != nil {
		s.storeMP4Cache(path, mp4CacheEntry{valid: false, mtime: mtime, size: size})
		return false
	}

	valid := string(buf[4:8]) == "ftyp"
	s.storeMP4Cache(path, mp4CacheEntry{valid: valid, mtime: mtime, size: size})
	return valid
}

// storeMP4Cache records an mp4 validity verdict, bounding the cache size.
func (s *Service) storeMP4Cache(path string, entry mp4CacheEntry) {
	s.mp4Mu.Lock()
	if len(s.mp4Cache) >= maxCacheEntries {
		s.mp4Cache = make(map[string]mp4CacheEntry)
	}
	s.mp4Cache[path] = entry
	s.mp4Mu.Unlock()
}

// StreamVideo serves a video file with HTTP Range support.
func (s *Service) StreamVideo(w http.ResponseWriter, r *http.Request, videoPath string) {
	http.ServeFile(w, r, videoPath)
}

// GenerateThumbnail creates a thumbnail image from a video file.
// Returns an error if the file is not a valid MP4 or if ffmpeg fails.
func (s *Service) GenerateThumbnail(videoPath, outputPath string, width, height int) error {
	if !s.IsValidMP4(videoPath) {
		return fmt.Errorf("not a valid MP4 file (possibly encrypted or corrupt)")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("create thumbnail dir: %w", err)
	}

	var stderr strings.Builder
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-ss", "00:00:01",
		"-vframes", "1",
		"-vf", fmt.Sprintf("scale=%d:%d", width, height),
		"-y", outputPath,
	)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ExportClip stream-copies the [start, end] window of a video into w as MP4.
// `-c copy` keeps CPU near zero on a Pi Zero; the output has to be fragmented
// because the regular mp4 muxer cannot rewrite its index on a pipe.
func (s *Service) ExportClip(ctx context.Context, w io.Writer, videoPath string, start, end float64) error {
	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-ss", strconv.FormatFloat(start, 'f', 3, 64),
		"-to", strconv.FormatFloat(end, 'f', 3, 64),
		"-i", videoPath,
		"-c", "copy",
		"-movflags", "frag_keyframe+empty_moov",
		"-f", "mp4", "pipe:1",
	)
	cmd.Stdout = w
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ThumbnailHash generates a unique hash for cache-busting.
func (s *Service) ThumbnailHash(videoPath string) string {
	info, err := os.Stat(videoPath)
	if err != nil {
		return ""
	}
	data := fmt.Sprintf("%s_%d_%d", videoPath, info.ModTime().UnixNano(), info.Size())
	h := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", h)
}

// DeleteEvent removes all files in an event directory.
func (s *Service) DeleteEvent(folderPath, eventName string) error {
	eventDir := filepath.Join(folderPath, filepath.Clean(eventName))
	if !strings.HasPrefix(eventDir, folderPath+string(filepath.Separator)) {
		return fmt.Errorf("invalid event name: path traversal detected")
	}
	if err := os.RemoveAll(eventDir); err != nil {
		return err
	}
	s.invalidateFolderCache(folderPath)
	return nil
}

// invalidateFolderCache removes all cached entries for a given folder path.
func (s *Service) invalidateFolderCache(folderPath string) {
	prefix := folderPath + ":"
	s.eventMu.Lock()
	for key := range s.eventCache {
		if strings.HasPrefix(key, prefix) {
			delete(s.eventCache, key)
		}
	}
	s.eventMu.Unlock()

	s.sessionMu.Lock()
	for key := range s.sessionCache {
		if strings.HasPrefix(key, prefix) {
			delete(s.sessionCache, key)
		}
	}
	s.sessionMu.Unlock()
}

func (s *Service) parseEvent(eventDir, name string) Event {
	event := Event{
		Name:         name,
		CameraVideos: make(map[string]string),
		Encrypted:    make(map[string]bool),
	}

	// Try to parse event.json
	ejPath := filepath.Join(eventDir, "event.json")
	if data, err := os.ReadFile(ejPath); err == nil {
		var ej map[string]any
		if json.Unmarshal(data, &ej) == nil {
			if city, ok := ej["city"].(string); ok {
				event.City = city
			}
			if reason, ok := ej["reason"].(string); ok {
				event.Reason = reason
			}
			if ts, ok := ej["timestamp"].(string); ok {
				event.Datetime = ts
			}
			// est_lat/est_lon are the only location source for events whose
			// video is encrypted or corrupt; Tesla writes them as strings.
			event.EstLat = parseJSONFloat(ej["est_lat"])
			event.EstLon = parseJSONFloat(ej["est_lon"])
			switch c := ej["camera"].(type) {
			case string:
				event.Camera = c
			case float64:
				event.Camera = strconv.FormatInt(int64(c), 10)
			}
		}
	}

	if event.Datetime == "" {
		event.Datetime = name
	}

	// Check for thumbnail
	event.HasThumbnail = fileExists(filepath.Join(eventDir, "thumb.png"))
	event.Kept = fileExists(filepath.Join(eventDir, KeepMarker))

	// Scan for video files
	entries, err := os.ReadDir(eventDir)
	if err != nil {
		return event
	}

	var totalSize int64
	clipSet := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isVideoFile(name) {
			continue
		}

		info, _ := e.Info()
		if info != nil {
			totalSize += info.Size()
		}

		m := sessionPattern.FindStringSubmatch(name)
		if m != nil {
			camera := m[2]
			event.CameraVideos[camera] = name
			fullPath := filepath.Join(eventDir, name)
			event.Encrypted[camera] = !s.IsValidMP4(fullPath)
			clipSet[m[1]] = true
		}
	}

	event.SizeMB = float64(totalSize) / (1024 * 1024)

	for clip := range clipSet {
		event.Clips = append(event.Clips, clip)
	}
	sort.Strings(event.Clips)

	event.StartingClipIndex = computeStartingClipIndex(event.Clips, event.Datetime)
	event.TriggerOffsetSec = computeTriggerOffset(event.Clips, event.Datetime)

	return event
}

// parseJSONFloat reads a number Tesla writes as a JSON string (est_lat/est_lon).
// nil for absent or unparseable, so callers can tell "no location" from 0.
func parseJSONFloat(v any) *float64 {
	switch t := v.(type) {
	case float64:
		return &t
	case string:
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil
		}
		return &f
	}
	return nil
}

// parseEventTime parses a trigger timestamp in any of the formats Tesla writes
// to event.json, falling back to the event directory naming. Zero when unparseable.
func parseEventTime(datetime string) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02_15-04-05",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, datetime); err == nil {
			return t
		}
	}
	return time.Time{}
}

// computeTriggerOffset returns the seconds from the start of the first clip to
// the event trigger, for marking the moment on the player timeline. 0 when
// either timestamp is unparseable or the trigger predates the first clip.
func computeTriggerOffset(clips []string, datetime string) float64 {
	if len(clips) == 0 {
		return 0
	}
	eventTime := parseEventTime(datetime)
	first, err := time.Parse("2006-01-02_15-04-05", clips[0])
	if err != nil || eventTime.IsZero() {
		return 0
	}
	offset := eventTime.Sub(first).Seconds()
	if offset < 0 {
		return 0
	}
	return offset
}

// computeStartingClipIndex finds the index of the clip that contains or immediately
// precedes the event's trigger timestamp. Falls back to 0 if parsing fails.
func computeStartingClipIndex(clips []string, datetime string) int {
	if len(clips) == 0 {
		return 0
	}
	if len(clips) == 1 {
		return 0
	}

	eventTime := parseEventTime(datetime)
	if eventTime.IsZero() {
		return 0
	}

	// Clip timestamps are in "YYYY-MM-DD_HH-MM-SS" format (already sorted ascending)
	best := 0
	for i, clip := range clips {
		t, err := time.Parse("2006-01-02_15-04-05", clip)
		if err != nil {
			continue
		}
		if !t.After(eventTime) {
			best = i
		}
	}
	return best
}

// EventArchiveFile is a single file scheduled to be added to an event archive.
type EventArchiveFile struct {
	Path string // absolute source path on disk
	Name string // entry name inside the archive
}

// CollectEventArchiveFiles returns all video and metadata files that belong
// to an event, sorted by archive name. Returns an error if the event
// directory is missing or contains nothing archivable. Performing this scan
// up-front lets HTTP callers respond with a JSON error before they start
// streaming the ZIP body.
func (s *Service) CollectEventArchiveFiles(folderPath, eventName string) ([]EventArchiveFile, error) {
	eventDir := filepath.Join(folderPath, filepath.Clean(eventName))
	if !strings.HasPrefix(eventDir, folderPath+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid event name: path traversal detected")
	}

	entries, err := os.ReadDir(eventDir)
	if err != nil {
		return nil, fmt.Errorf("read event dir: %w", err)
	}

	var files []EventArchiveFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isVideoFile(name) && !isEventMetadataFile(name) {
			continue
		}
		files = append(files, EventArchiveFile{
			Path: filepath.Join(eventDir, name),
			Name: name,
		})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("event %q has no archivable files", eventName)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

// WriteEventZip streams a ZIP of files into w. The caller
// must invoke CollectEventArchiveFiles first to validate the event; mid-
// stream errors here cannot be surfaced to the HTTP client (headers will
// already be flushed) and should be logged instead.
func WriteEventZip(w io.Writer, files []EventArchiveFile) error {
	zw := zip.NewWriter(w)
	for _, f := range files {
		if err := addFileToZip(zw, f); err != nil {
			_ = zw.Close()
			return fmt.Errorf("zip %q: %w", f.Name, err)
		}
	}
	return zw.Close()
}

func addFileToZip(zw *zip.Writer, f EventArchiveFile) error {
	info, err := os.Stat(f.Path)
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = f.Name
	// H.264 and PNG/JPEG are already compressed, so DEFLATE on the hundreds of
	// megabytes of video in an event buys ~nothing and costs minutes of Pi Zero
	// CPU while the user waits on the download. Only the small text metadata is
	// worth compressing.
	switch strings.ToLower(filepath.Ext(f.Name)) {
	case ".mp4", ".png", ".jpg", ".jpeg":
		header.Method = zip.Store
	default:
		header.Method = zip.Deflate
	}
	src, err := os.Open(f.Path)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}

func isEventMetadataFile(name string) bool {
	switch strings.ToLower(name) {
	case "event.json", "thumb.png":
		return true
	}
	return false
}

// FormatFileSize returns a human-readable file size string.
func FormatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %s", float64(bytes)/float64(div), []string{"KB", "MB", "GB", "TB"}[exp])
}

// ParseSessionFromFilename extracts session and camera from a Tesla video filename.
func ParseSessionFromFilename(filename string) (session, camera string, ok bool) {
	m := sessionPattern.FindStringSubmatch(filename)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// countFolderItems counts the event-level items in a TeslaCam folder without
// recursing into subdirectories. For SavedClips/SentryClips each event is a
// subdirectory; for RecentClips videos are top-level files.
// Avoids the O(N) directory reads that the old recursive walk caused on large
// folders (e.g. 1325 SentryClips events → 1325 extra ReadDir calls).
func countVideoFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++ // each subdirectory is one event
		} else if isVideoFile(e.Name()) {
			count++ // top-level video file (RecentClips)
		}
	}
	return count
}

func isVideoFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".mp4" || ext == ".avi" || ext == ".mov" || ext == ".mkv"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadSEIData reads the entire video file for client-side SEI metadata parsing.
func (s *Service) ReadSEIData(w http.ResponseWriter, r *http.Request, videoPath string) {
	info, err := os.Stat(videoPath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("Accept-Ranges", "none")

	f, err := os.Open(videoPath)
	if err != nil {
		http.Error(w, "cannot open file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if _, err := io.Copy(w, f); err != nil {
		logger.L.WithError(err).Warn("ReadSEIData: stream interrupted")
	}
}

// GetMP4Duration extracts the duration from an MP4 file's moov/mvhd box.
func GetMP4Duration(path string) (time.Duration, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	fInfo, err := f.Stat()
	if err != nil {
		return 0, err
	}

	return scanBoxes(f, 0, fInfo.Size())
}

// scanBoxes walks MP4 boxes in the byte range [start, start+length) looking for mvhd.
// It descends into moov boxes automatically.
func scanBoxes(f *os.File, start, length int64) (time.Duration, error) {
	pos := start
	end := start + length

	for pos < end {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return 0, fmt.Errorf("seek: %w", err)
		}

		header := make([]byte, 8)
		if _, err := io.ReadFull(f, header); err != nil {
			break
		}

		boxSize := int64(binary.BigEndian.Uint32(header[0:4]))
		boxType := string(header[4:8])

		if boxSize < 8 {
			break
		}

		switch boxType {
		case "moov":
			// Descend into moov: its children start right after the 8-byte header.
			if dur, err := scanBoxes(f, pos+8, boxSize-8); err == nil {
				return dur, nil
			}
		case "mvhd":
			// boxSize is an attacker-controlled uint32 from the file. An mvhd box
			// is only a few dozen bytes; cap it before allocating so a crafted
			// header declaring a ~4 GiB box can't OOM a memory-constrained Pi.
			if boxSize-8 > 64<<10 {
				return 0, fmt.Errorf("mvhd box implausibly large: %d bytes", boxSize)
			}
			payload := make([]byte, boxSize-8)
			if _, err := io.ReadFull(f, payload); err != nil {
				return 0, fmt.Errorf("read mvhd: %w", err)
			}

			if len(payload) < 1 {
				return 0, fmt.Errorf("mvhd too short")
			}
			version := payload[0]
			var timescale uint32
			var duration uint64

			if version == 0 {
				if len(payload) < 20 {
					return 0, fmt.Errorf("mvhd v0 truncated")
				}
				timescale = binary.BigEndian.Uint32(payload[12:16])
				duration = uint64(binary.BigEndian.Uint32(payload[16:20]))
			} else {
				if len(payload) < 32 {
					return 0, fmt.Errorf("mvhd v1 truncated")
				}
				timescale = binary.BigEndian.Uint32(payload[20:24])
				duration = binary.BigEndian.Uint64(payload[24:32])
			}

			if timescale == 0 {
				return 0, fmt.Errorf("invalid timescale")
			}

			return time.Duration(duration) * time.Second / time.Duration(timescale), nil
		}

		pos += boxSize
	}

	return 0, fmt.Errorf("mvhd box not found")
}
