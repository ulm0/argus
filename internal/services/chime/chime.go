package chime

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/logger"

	"github.com/ulm0/argus/internal/config"
)

type Service struct {
	cfg       *config.Config
	scheduler *Scheduler
	groups    *GroupManager
}

func NewService(cfg *config.Config) *Service {
	return &Service{
		cfg:       cfg,
		scheduler: NewScheduler(cfg),
		groups:    NewGroupManager(cfg),
	}
}

func (s *Service) Scheduler() *Scheduler   { return s.scheduler }
func (s *Service) Groups() *GroupManager    { return s.groups }

// ValidateTeslaWAV checks if a WAV file meets Tesla lock chime requirements.
// Requirements: PCM, 16-bit, 44.1 or 48 kHz, mono or stereo, <1 MiB, <10s.
func (s *Service) ValidateTeslaWAV(path string) (bool, string) {
	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Sprintf("cannot open file: %v", err)
	}
	defer f.Close()

	info, _ := f.Stat()
	if info.Size() > s.cfg.Web.MaxLockChimeSize {
		return false, fmt.Sprintf("file too large: %d bytes (max %d)", info.Size(), s.cfg.Web.MaxLockChimeSize)
	}

	// Read RIFF/WAVE container header (12 bytes).
	riff := make([]byte, 12)
	if _, err := io.ReadFull(f, riff); err != nil {
		return false, "cannot read WAV header"
	}
	if string(riff[0:4]) != "RIFF" || string(riff[8:12]) != "WAVE" {
		return false, "not a valid WAV file"
	}

	// Scan chunks to find "fmt " regardless of ordering or extra metadata chunks.
	var fmtData []byte
	var dataSize uint32
	chunkHdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(f, chunkHdr); err != nil {
			break
		}
		chunkID := string(chunkHdr[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHdr[4:8])

		if chunkID == "fmt " {
			fmtData = make([]byte, chunkSize)
			if _, err := io.ReadFull(f, fmtData); err != nil {
				return false, "cannot read fmt chunk"
			}
		} else if chunkID == "data" {
			dataSize = chunkSize
			break
		} else {
			// Skip unknown chunk; chunks are padded to even size.
			skip := int64(chunkSize)
			if chunkSize%2 != 0 {
				skip++
			}
			if _, err := f.Seek(skip, io.SeekCurrent); err != nil {
				break
			}
		}
	}

	if len(fmtData) < 16 {
		return false, "fmt chunk missing or too short"
	}

	audioFormat := binary.LittleEndian.Uint16(fmtData[0:2])
	if audioFormat != 1 { // PCM
		return false, fmt.Sprintf("not PCM format (format=%d)", audioFormat)
	}

	channels := binary.LittleEndian.Uint16(fmtData[2:4])
	if channels != 1 && channels != 2 {
		return false, fmt.Sprintf("invalid channels: %d (need 1 or 2)", channels)
	}

	sampleRate := binary.LittleEndian.Uint32(fmtData[4:8])
	if sampleRate != 44100 && sampleRate != 48000 {
		return false, fmt.Sprintf("invalid sample rate: %d (need 44100 or 48000)", sampleRate)
	}

	bitsPerSample := binary.LittleEndian.Uint16(fmtData[14:16])
	if bitsPerSample != 16 {
		return false, fmt.Sprintf("invalid bit depth: %d (need 16)", bitsPerSample)
	}

	// Check duration using the data chunk size found during scanning.
	if dataSize > 0 {
		bytesPerSec := sampleRate * uint32(channels) * uint32(bitsPerSample/8)
		if bytesPerSec > 0 {
			duration := float64(dataSize) / float64(bytesPerSec)
			if duration > s.cfg.Web.MaxLockChimeDur {
				return false, fmt.Sprintf("too long: %.1fs (max %.1fs)", duration, s.cfg.Web.MaxLockChimeDur)
			}
			if duration < s.cfg.Web.MinLockChimeDur {
				return false, fmt.Sprintf("too short: %.1fs (min %.1fs)", duration, s.cfg.Web.MinLockChimeDur)
			}
		}
	}

	return true, ""
}

// ReencodeForTesla re-encodes an audio file to Tesla-compatible WAV using ffmpeg.
func (s *Service) ReencodeForTesla(inputPath, outputPath string) error {
	args := []string{
		"-i", inputPath,
		"-acodec", "pcm_s16le",
		"-ar", "44100",
		"-ac", "1",
		"-y", outputPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg reencode: %s: %w", string(output), err)
	}
	return nil
}

// NormalizeAudio performs two-pass LUFS loudness normalization.
func (s *Service) NormalizeAudio(inputPath string, targetLUFS float64) (string, error) {
	// Pass 1: measure
	args1 := []string{
		"-i", inputPath,
		"-af", fmt.Sprintf("loudnorm=I=%.1f:LRA=11:TP=-1.5:print_format=json", targetLUFS),
		"-f", "null", "-",
	}
	out1, err := exec.Command("ffmpeg", args1...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg loudnorm pass 1: %w", err)
	}

	// Extract measured values from JSON output
	jsonStart := strings.LastIndex(string(out1), "{")
	if jsonStart < 0 {
		return "", fmt.Errorf("loudnorm pass 1: no JSON output")
	}
	jsonStr := string(out1)[jsonStart:]
	var measured map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &measured); err != nil {
		return "", fmt.Errorf("parse loudnorm output: %w", err)
	}

	// Pass 2: apply
	outputPath := inputPath + ".normalized.wav"
	args2 := []string{
		"-i", inputPath,
		"-af", fmt.Sprintf("loudnorm=I=%.1f:LRA=11:TP=-1.5:measured_I=%v:measured_LRA=%v:measured_TP=%v:measured_thresh=%v:linear=true",
			targetLUFS,
			measured["input_i"],
			measured["input_lra"],
			measured["input_tp"],
			measured["input_thresh"],
		),
		"-ar", "44100",
		"-y", outputPath,
	}
	if out, err := exec.Command("ffmpeg", args2...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg loudnorm pass 2: %s: %w", string(out), err)
	}

	return outputPath, nil
}

// ReplaceLockChime atomically replaces LockChime.wav with fsync and MD5 verification.
func (s *Service) ReplaceLockChime(sourcePath, destPath string) error {
	srcData, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	srcMD5 := md5.Sum(srcData)

	tmpPath := destPath + ".tmp"

	// Write to temp file with fsync
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	if _, err := f.Write(srcData); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("fsync temp: %w", err)
	}
	f.Close()

	// Backup old chime
	backupPath := filepath.Join(filepath.Dir(destPath), "old"+filepath.Base(destPath))
	os.Rename(destPath, backupPath)

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		// Restore backup
		os.Rename(backupPath, destPath)
		return fmt.Errorf("rename: %w", err)
	}

	// Verify MD5
	written, err := os.ReadFile(destPath)
	if err != nil {
		return fmt.Errorf("verify read: %w", err)
	}
	writtenMD5 := md5.Sum(written)
	if srcMD5 != writtenMD5 {
		os.Rename(backupPath, destPath)
		return fmt.Errorf("MD5 verification failed")
	}

	// Cleanup backup
	os.Remove(backupPath)
	exec.Command("sync").Run()

	return nil
}

// SetActiveChime copies a chime from the library to LockChime.wav.
func (s *Service) SetActiveChime(chimeFilename, mountPath string) error {
	chimesDir := filepath.Join(mountPath, s.cfg.Web.ChimesFolder)
	srcPath := filepath.Join(chimesDir, chimeFilename)
	if !strings.HasPrefix(srcPath, chimesDir+string(filepath.Separator)) {
		return fmt.Errorf("invalid chime filename: path traversal detected")
	}
	destPath := filepath.Join(mountPath, s.cfg.Web.LockChimeFilename)
	return s.ReplaceLockChime(srcPath, destPath)
}

// UploadChime saves an uploaded file to the chimes library.
func (s *Service) UploadChime(data []byte, filename, mountPath string, normalize bool, targetLUFS float64) error {
	chimesDir := filepath.Join(mountPath, s.cfg.Web.ChimesFolder)
	os.MkdirAll(chimesDir, 0755)

	destPath := filepath.Join(chimesDir, filename)

	// Write raw upload to temp file.
	tmpPath := destPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}

	// cleanup tracks the current intermediate file so we can remove it on error.
	cleanup := func(path string) { os.Remove(path) }

	ext := strings.ToLower(filepath.Ext(filename))

	// Convert MP3 to WAV if needed.
	if ext == ".mp3" {
		wavPath := strings.TrimSuffix(tmpPath, ext) + ".wav"
		if err := s.ReencodeForTesla(tmpPath, wavPath); err != nil {
			cleanup(tmpPath)
			return err
		}
		cleanup(tmpPath)
		tmpPath = wavPath
		destPath = strings.TrimSuffix(destPath, ext) + ".wav"
	}

	// Re-encode if not valid Tesla WAV.
	if valid, _ := s.ValidateTeslaWAV(tmpPath); !valid {
		reencPath := tmpPath + ".reenc.wav"
		if err := s.ReencodeForTesla(tmpPath, reencPath); err != nil {
			cleanup(tmpPath)
			return err
		}
		cleanup(tmpPath)
		tmpPath = reencPath
	}

	// Normalize if requested.
	if normalize && targetLUFS != 0 {
		normPath, err := s.NormalizeAudio(tmpPath, targetLUFS)
		if err != nil {
			logger.L.WithError(err).Warn("normalization failed")
		} else {
			cleanup(tmpPath)
			tmpPath = normPath
		}
	}

	// Final rename; if it fails, remove the remaining temp file.
	if err := os.Rename(tmpPath, destPath); err != nil {
		cleanup(tmpPath)
		return err
	}
	return nil
}

// DeleteChime removes a chime file from the library.
func (s *Service) DeleteChime(filename, mountPath string) error {
	chimesDir := filepath.Join(mountPath, s.cfg.Web.ChimesFolder)
	chimePath := filepath.Join(chimesDir, filename)
	if !strings.HasPrefix(chimePath, chimesDir+string(filepath.Separator)) {
		return fmt.Errorf("invalid chime filename: path traversal detected")
	}
	return os.Remove(chimePath)
}

// RenameChime renames a chime in the library.
func (s *Service) RenameChime(oldName, newName, mountPath string) error {
	chimesDir := filepath.Join(mountPath, s.cfg.Web.ChimesFolder)
	oldPath := filepath.Join(chimesDir, oldName)
	newPath := filepath.Join(chimesDir, newName)
	if !strings.HasPrefix(oldPath, chimesDir+string(filepath.Separator)) {
		return fmt.Errorf("invalid chime filename: path traversal detected")
	}
	if !strings.HasPrefix(newPath, chimesDir+string(filepath.Separator)) {
		return fmt.Errorf("invalid chime filename: path traversal detected")
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	exec.Command("sync").Run()
	return nil
}

// ListChimes returns all chime files in the library.
func (s *Service) ListChimes(mountPath string) []string {
	chimesDir := filepath.Join(mountPath, s.cfg.Web.ChimesFolder)
	entries, err := os.ReadDir(chimesDir)
	if err != nil {
		return nil
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".wav") {
			files = append(files, e.Name())
		}
	}
	return files
}

// GetActiveChimeInfo returns info about the current LockChime.wav.
func (s *Service) GetActiveChimeInfo(mountPath string) (name string, exists bool) {
	chimePath := filepath.Join(mountPath, s.cfg.Web.LockChimeFilename)
	if _, err := os.Stat(chimePath); err != nil {
		return "", false
	}
	return s.cfg.Web.LockChimeFilename, true
}

// Scheduler types and implementation

type ScheduleType string

const (
	ScheduleWeekly    ScheduleType = "weekly"
	ScheduleDate      ScheduleType = "date"
	ScheduleHoliday   ScheduleType = "holiday"
	ScheduleRecurring ScheduleType = "recurring"
)

type Schedule struct {
	ID             string       `json:"id"`
	ChimeFilename  string       `json:"chime_filename"`
	Time           string       `json:"time"`
	Type           ScheduleType `json:"type"`
	Days           []int        `json:"days,omitempty"`
	Month          int          `json:"month,omitempty"`
	Day            int          `json:"day,omitempty"`
	Holiday        string       `json:"holiday,omitempty"`
	Interval       string       `json:"interval,omitempty"`
	Name           string       `json:"name,omitempty"`
	Enabled        bool         `json:"enabled"`
	LastRun        *time.Time   `json:"last_run,omitempty"`
}

type Scheduler struct {
	cfg       *config.Config
	mu        sync.Mutex
	schedules []Schedule
	filePath  string
}

func NewScheduler(cfg *config.Config) *Scheduler {
	s := &Scheduler{
		cfg:      cfg,
		filePath: filepath.Join(cfg.GadgetDir, "chime_schedules.json"),
	}
	s.load()
	return s
}

func (s *Scheduler) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.schedules)
}

func (s *Scheduler) save() error {
	data, err := json.MarshalIndent(s.schedules, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically via temp file + rename to prevent corruption on power loss.
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

func (s *Scheduler) AddSchedule(sched Schedule) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sched.ID = fmt.Sprintf("sched_%d", time.Now().UnixNano())
	s.schedules = append(s.schedules, sched)
	return sched.ID, s.save()
}

func (s *Scheduler) UpdateSchedule(id string, updates map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.schedules {
		if s.schedules[i].ID == id {
			if v, ok := updates["enabled"].(bool); ok {
				s.schedules[i].Enabled = v
			}
			if v, ok := updates["chime_filename"].(string); ok {
				s.schedules[i].ChimeFilename = v
			}
			if v, ok := updates["time"].(string); ok {
				s.schedules[i].Time = v
			}
			if v, ok := updates["name"].(string); ok {
				s.schedules[i].Name = v
			}
			return s.save()
		}
	}
	return fmt.Errorf("schedule %s not found", id)
}

func (s *Scheduler) DeleteSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.schedules {
		if s.schedules[i].ID == id {
			s.schedules = append(s.schedules[:i], s.schedules[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("schedule %s not found", id)
}

func (s *Scheduler) GetSchedule(id string) *Schedule {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.schedules {
		if s.schedules[i].ID == id {
			return &s.schedules[i]
		}
	}
	return nil
}

func (s *Scheduler) ListSchedules(enabledOnly bool) []Schedule {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !enabledOnly {
		result := make([]Schedule, len(s.schedules))
		copy(result, s.schedules)
		return result
	}

	var result []Schedule
	for _, sched := range s.schedules {
		if sched.Enabled {
			result = append(result, sched)
		}
	}
	return result
}

func (s *Scheduler) RecordExecution(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for i := range s.schedules {
		if s.schedules[i].ID == id {
			s.schedules[i].LastRun = &now
			break
		}
	}
	if err := s.save(); err != nil {
		logger.L.WithError(err).Warn("failed to persist schedule execution record")
	}
}

// Chime Groups

type ChimeGroup struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Chimes      []string `json:"chimes"`
}

type RandomConfig struct {
	Enabled  bool   `json:"enabled"`
	GroupID  string `json:"group_id"`
	LastUsed string `json:"last_used,omitempty"`
}

type GroupManager struct {
	cfg          *config.Config
	mu           sync.Mutex
	groups       []ChimeGroup
	randomConfig RandomConfig
	groupsFile   string
	randomFile   string
}

func NewGroupManager(cfg *config.Config) *GroupManager {
	gm := &GroupManager{
		cfg:        cfg,
		groupsFile: filepath.Join(cfg.GadgetDir, "chime_groups.json"),
		randomFile: filepath.Join(cfg.GadgetDir, "chime_random_config.json"),
	}
	gm.loadGroups()
	gm.loadRandomConfig()
	return gm
}

func (gm *GroupManager) loadGroups() {
	data, _ := os.ReadFile(gm.groupsFile)
	json.Unmarshal(data, &gm.groups)
}

func (gm *GroupManager) saveGroups() error {
	data, _ := json.MarshalIndent(gm.groups, "", "  ")
	tmp := gm.groupsFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, gm.groupsFile)
}

func (gm *GroupManager) loadRandomConfig() {
	data, _ := os.ReadFile(gm.randomFile)
	json.Unmarshal(data, &gm.randomConfig)
}

func (gm *GroupManager) saveRandomConfig() error {
	data, _ := json.MarshalIndent(gm.randomConfig, "", "  ")
	tmp := gm.randomFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, gm.randomFile)
}

func (gm *GroupManager) ListGroups() []ChimeGroup {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	result := make([]ChimeGroup, len(gm.groups))
	copy(result, gm.groups)
	return result
}

func (gm *GroupManager) CreateGroup(name, desc string, chimes []string) (string, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	g := ChimeGroup{
		ID:          fmt.Sprintf("grp_%d", time.Now().UnixNano()),
		Name:        name,
		Description: desc,
		Chimes:      chimes,
	}
	gm.groups = append(gm.groups, g)
	return g.ID, gm.saveGroups()
}

func (gm *GroupManager) UpdateGroup(id, name, desc string, chimes []string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	for i := range gm.groups {
		if gm.groups[i].ID == id {
			if name != "" {
				gm.groups[i].Name = name
			}
			if desc != "" {
				gm.groups[i].Description = desc
			}
			if chimes != nil {
				gm.groups[i].Chimes = chimes
			}
			return gm.saveGroups()
		}
	}
	return fmt.Errorf("group %s not found", id)
}

func (gm *GroupManager) DeleteGroup(id string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	for i := range gm.groups {
		if gm.groups[i].ID == id {
			gm.groups = append(gm.groups[:i], gm.groups[i+1:]...)
			return gm.saveGroups()
		}
	}
	return fmt.Errorf("group %s not found", id)
}

func (gm *GroupManager) AddChimeToGroup(groupID, chimeFilename string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	for i := range gm.groups {
		if gm.groups[i].ID == groupID {
			for _, c := range gm.groups[i].Chimes {
				if c == chimeFilename {
					return nil // already present
				}
			}
			gm.groups[i].Chimes = append(gm.groups[i].Chimes, chimeFilename)
			return gm.saveGroups()
		}
	}
	return fmt.Errorf("group %s not found", groupID)
}

func (gm *GroupManager) RemoveChimeFromGroup(groupID, chimeFilename string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	for i := range gm.groups {
		if gm.groups[i].ID == groupID {
			for j, c := range gm.groups[i].Chimes {
				if c == chimeFilename {
					gm.groups[i].Chimes = append(gm.groups[i].Chimes[:j], gm.groups[i].Chimes[j+1:]...)
					return gm.saveGroups()
				}
			}
			return nil
		}
	}
	return fmt.Errorf("group %s not found", groupID)
}

func (gm *GroupManager) GetRandomConfig() RandomConfig {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	return gm.randomConfig
}

func (gm *GroupManager) SetRandomMode(enabled bool, groupID string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	gm.randomConfig.Enabled = enabled
	gm.randomConfig.GroupID = groupID
	return gm.saveRandomConfig()
}

func (gm *GroupManager) SelectRandomChime(avoidChime string) string {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if !gm.randomConfig.Enabled || gm.randomConfig.GroupID == "" {
		return ""
	}

	for _, g := range gm.groups {
		if g.ID == gm.randomConfig.GroupID {
			if len(g.Chimes) == 0 {
				return ""
			}
			// Filter out the chime to avoid
			var candidates []string
			for _, c := range g.Chimes {
				if c != avoidChime {
					candidates = append(candidates, c)
				}
			}
			if len(candidates) == 0 {
				candidates = g.Chimes
			}

			selected := candidates[rand.Intn(len(candidates))]
			gm.randomConfig.LastUsed = selected
			if err := gm.saveRandomConfig(); err != nil {
				logger.L.WithError(err).Warn("failed to persist random chime selection")
			}
			return selected
		}
	}
	return ""
}
