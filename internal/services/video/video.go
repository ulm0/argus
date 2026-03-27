package video

import (
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
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

var sessionPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2})-(.+)\.\w+$`)

type Service struct {
	cfg *config.Config
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

type Event struct {
	Name         string            `json:"name"`
	Datetime     string            `json:"datetime"`
	City         string            `json:"city"`
	Reason       string            `json:"reason"`
	SizeMB       float64           `json:"size_mb"`
	HasThumbnail bool              `json:"has_thumbnail"`
	CameraVideos map[string]string `json:"camera_videos"`
	Encrypted    map[string]bool   `json:"encrypted_videos"`
	Clips        []string          `json:"clips,omitempty"`
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
func (s *Service) GetTeslaCamPath() string {
	for _, ro := range []bool{true, false} {
		base := s.cfg.MountPath("part1", ro)
		tcPath := filepath.Join(base, "TeslaCam")
		if info, err := os.Stat(tcPath); err == nil && info.IsDir() {
			return tcPath
		}
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
	return folders
}

// GetEvents returns paginated events from a TeslaCam subfolder.
func (s *Service) GetEvents(folderPath string, page, perPage int) ([]Event, bool) {
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

	return events, hasNext
}

// GetEventDetails returns full details for a single event.
func (s *Service) GetEventDetails(folderPath, eventName string) (*Event, error) {
	eventDir := filepath.Join(folderPath, eventName)
	if _, err := os.Stat(eventDir); err != nil {
		return nil, fmt.Errorf("event not found: %s", eventName)
	}

	event := s.parseEvent(eventDir, eventName)
	return &event, nil
}

// GroupVideosBySession groups videos by their timestamp session for RecentClips.
func (s *Service) GroupVideosBySession(folderPath string, page, perPage int) ([]SessionGroup, bool) {
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
func (s *Service) IsValidMP4(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 12)
	if _, err := io.ReadFull(f, buf); err != nil {
		return false
	}

	// Check for ftyp box
	return string(buf[4:8]) == "ftyp"
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
	return os.RemoveAll(eventDir)
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
		}
	}

	if event.Datetime == "" {
		event.Datetime = name
	}

	// Check for thumbnail
	event.HasThumbnail = fileExists(filepath.Join(eventDir, "thumb.png"))

	// Scan for video files
	entries, err := os.ReadDir(eventDir)
	if err != nil {
		return event
	}

	var totalSize int64
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
		}
	}

	event.SizeMB = float64(totalSize) / (1024 * 1024)

	// Collect clip timestamps
	clipMap := make(map[string]bool)
	for _, name := range event.CameraVideos {
		m := sessionPattern.FindStringSubmatch(name)
		if m != nil {
			clipMap[m[1]] = true
		}
	}
	for clip := range clipMap {
		event.Clips = append(event.Clips, clip)
	}
	sort.Strings(event.Clips)

	return event
}

// CreateEventZip creates a ZIP archive of all videos in an event.
func (s *Service) CreateEventZip(folderPath, eventName string) (string, error) {
	eventDir := filepath.Join(folderPath, filepath.Clean(eventName))
	if !strings.HasPrefix(eventDir, folderPath+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid event name: path traversal detected")
	}

	entries, err := os.ReadDir(eventDir)
	if err != nil {
		return "", err
	}

	// Use a randomly-named temp file to avoid name-collision races.
	tmpFile, err := os.CreateTemp("", "argus-event-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp zip: %w", err)
	}
	zipPath := tmpFile.Name()
	tmpFile.Close()

	args := []string{"-j", zipPath}
	for _, e := range entries {
		if !e.IsDir() && isVideoFile(e.Name()) {
			args = append(args, filepath.Join(eventDir, e.Name()))
		}
	}

	if err := exec.Command("zip", args...).Run(); err != nil {
		os.Remove(zipPath)
		return "", err
	}
	return zipPath, nil
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

func countVideoFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			// Count subdirectory video files
			subEntries, _ := os.ReadDir(filepath.Join(dir, e.Name()))
			for _, se := range subEntries {
				if !se.IsDir() && isVideoFile(se.Name()) {
					count++
				}
			}
		} else if isVideoFile(e.Name()) {
			count++
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
			payload := make([]byte, boxSize-8)
			if _, err := io.ReadFull(f, payload); err != nil {
				return 0, fmt.Errorf("read mvhd: %w", err)
			}

			version := payload[0]
			var timescale uint32
			var duration uint64

			if version == 0 {
				timescale = binary.BigEndian.Uint32(payload[12:16])
				duration = uint64(binary.BigEndian.Uint32(payload[16:20]))
			} else {
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
