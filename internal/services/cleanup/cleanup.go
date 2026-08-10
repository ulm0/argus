package cleanup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
)

// Default number of events to retain when migrating an old config that did not
// have an explicit count, or when a freshly detected folder gets a placeholder
// policy.
const defaultKeepLast = 50

// sessionPattern matches RecentClips files of the form
// `<timestamp>-<camera>.<ext>` so we can group them as a single "event".
// Mirrors the regexp used by the video service.
var sessionPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2})-(.+)\.\w+$`)

// keepMarker is the file a user drops in an event directory (via the web UI) to
// pin it. Mirrors video.KeepMarker.
const keepMarker = ".argus-keep"

// FolderPolicy controls retention for a single TeslaCam subfolder.
//
// Old configs used three independent knobs (age/size/count) that operated on
// individual `.mp4` files, which caused half-deleted events with orphaned
// camera angles. The new model is intentionally a single dial:
//
//	enabled      → does cleanup touch this folder at all?
//	keep_last    → how many of the most-recent events to retain
//	boot_cleanup → also run automatically on service start
//
// "Event" means a self-contained recording: a subdirectory for SavedClips /
// SentryClips, or a session group (same timestamp prefix) for RecentClips.
type FolderPolicy struct {
	Enabled     bool `json:"enabled"`
	BootCleanup bool `json:"boot_cleanup"`
	KeepLast    int  `json:"keep_last"`
}

// EventToDelete describes a single event the plan will remove. The slice of
// files is informational so the UI can show what's about to disappear.
type EventToDelete struct {
	Folder   string    `json:"folder"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Files    []string  `json:"files"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

type CleanupPlan struct {
	TotalCount int                        `json:"total_count"`
	TotalSize  int64                      `json:"total_size"`
	Breakdown  map[string][]EventToDelete `json:"breakdown_by_folder"`
}

type CleanupReport struct {
	DryRun       bool     `json:"dry_run"`
	DeletedCount int      `json:"deleted_count"`
	DeletedSize  float64  `json:"deleted_size_gb"`
	Errors       []string `json:"errors,omitempty"`
}

type Service struct {
	cfg        *config.Config
	mu         sync.RWMutex
	policies   map[string]FolderPolicy
	configFile string
}

func NewService(cfg *config.Config) *Service {
	s := &Service{
		cfg:        cfg,
		policies:   make(map[string]FolderPolicy),
		configFile: filepath.Join(cfg.GadgetDir, "cleanup_config.json"),
	}
	s.loadPolicies()
	return s
}

// legacyPolicy mirrors the pre-simplification on-disk shape so we can migrate
// existing installs without forcing the user to re-configure from scratch.
type legacyPolicy struct {
	Enabled     bool `json:"enabled"`
	BootCleanup bool `json:"boot_cleanup"`
	KeepLast    int  `json:"keep_last"`
	CountBased  *struct {
		Enabled  bool `json:"enabled"`
		MaxCount int  `json:"max_count"`
	} `json:"count_based,omitempty"`
}

func (s *Service) loadPolicies() {
	data, err := os.ReadFile(s.configFile)
	if err != nil {
		return
	}
	var legacy map[string]legacyPolicy
	if err := json.Unmarshal(data, &legacy); err != nil {
		return
	}
	for folder, lp := range legacy {
		p := FolderPolicy{
			Enabled:     lp.Enabled,
			BootCleanup: lp.BootCleanup,
			KeepLast:    lp.KeepLast,
		}
		if p.KeepLast <= 0 {
			if lp.CountBased != nil && lp.CountBased.MaxCount > 0 {
				p.KeepLast = lp.CountBased.MaxCount
			} else {
				p.KeepLast = defaultKeepLast
			}
		}
		s.policies[folder] = p
	}
}

// isSafeFolderKey reports whether a cleanup policy key is a single, in-tree
// folder name. Policy keys are joined onto the TeslaCam path and ultimately
// fed to os.RemoveAll, so a key like "../../etc" must never be accepted — it
// would turn cleanup into an arbitrary-directory-delete primitive reachable
// from the unauthenticated local API (and the boot-time cleanup path).
func isSafeFolderKey(folder string) bool {
	return folder != "" && folder == filepath.Base(folder) &&
		folder != "." && folder != ".." && !strings.ContainsAny(folder, `/\`)
}

// SavePolicies persists cleanup policies to disk atomically.
// In-memory state is only updated after a successful disk write.
func (s *Service) SavePolicies(policies map[string]FolderPolicy) error {
	for folder, p := range policies {
		if !isSafeFolderKey(folder) {
			return fmt.Errorf("invalid cleanup folder name: %q", folder)
		}
		if p.KeepLast <= 0 {
			p.KeepLast = defaultKeepLast
			policies[folder] = p
		}
	}

	data, err := json.MarshalIndent(policies, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.configFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.configFile); err != nil {
		os.Remove(tmp)
		return err
	}
	s.mu.Lock()
	s.policies = policies
	s.mu.Unlock()
	return nil
}

// GetPolicies returns a copy of the current cleanup policies.
func (s *Service) GetPolicies() map[string]FolderPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]FolderPolicy, len(s.policies))
	for k, v := range s.policies {
		out[k] = v
	}
	return out
}

// DetectFolders finds TeslaCam subfolders on the partition.
func (s *Service) DetectFolders(partitionPath string) []string {
	tcPath := filepath.Join(partitionPath, "TeslaCam")
	entries, err := os.ReadDir(tcPath)
	if err != nil {
		return nil
	}

	var folders []string
	for _, e := range entries {
		if e.IsDir() {
			folders = append(folders, e.Name())
		}
	}
	return folders
}

// GetPoliciesForDetectedFolders returns policies merged with detected folders.
// Folders without an existing policy get a sensible default so the UI can show
// them with the knobs already populated.
func (s *Service) GetPoliciesForDetectedFolders(partitionPath string) map[string]FolderPolicy {
	folders := s.DetectFolders(partitionPath)
	policies := s.GetPolicies()
	result := make(map[string]FolderPolicy)

	for _, folder := range folders {
		if p, ok := policies[folder]; ok {
			if p.KeepLast <= 0 {
				p.KeepLast = defaultKeepLast
			}
			result[folder] = p
		} else {
			result[folder] = FolderPolicy{KeepLast: defaultKeepLast}
		}
	}
	return result
}

// CalculateCleanupPlan walks the configured folders and returns the events
// that should be removed to honor each folder's KeepLast policy.
func (s *Service) CalculateCleanupPlan(partitionPath string) (*CleanupPlan, error) {
	tcPath := filepath.Join(partitionPath, "TeslaCam")
	plan := &CleanupPlan{
		Breakdown: make(map[string][]EventToDelete),
	}

	for folder, policy := range s.GetPolicies() {
		if !policy.Enabled || policy.KeepLast <= 0 {
			continue
		}
		// Defense-in-depth against a poisoned/legacy policy file: never act on a
		// key that escapes the TeslaCam root, even if it slipped past SavePolicies.
		if !isSafeFolderKey(folder) {
			continue
		}

		folderPath := filepath.Join(tcPath, folder)
		// Confine the resolved path to the TeslaCam root before it is used to
		// build the event paths that ExecuteCleanup ultimately os.RemoveAll's.
		if folderPath != tcPath && !strings.HasPrefix(folderPath, tcPath+string(filepath.Separator)) {
			continue
		}
		if _, err := os.Stat(folderPath); err != nil {
			continue
		}

		events := listEvents(folder, folderPath)
		if len(events) <= policy.KeepLast {
			continue
		}

		// Sort newest-first by name (Tesla folders/sessions are timestamped),
		// fall back to modified time when names are equal.
		sort.Slice(events, func(i, j int) bool {
			if events[i].Name == events[j].Name {
				return events[i].Modified.After(events[j].Modified)
			}
			return events[i].Name > events[j].Name
		})

		toDelete := events[policy.KeepLast:]
		plan.Breakdown[folder] = toDelete
		for _, ev := range toDelete {
			plan.TotalCount++
			plan.TotalSize += ev.Size
		}
	}

	return plan, nil
}

// ExecuteCleanup deletes events according to the plan. For event-folder events
// (subdirectories) the whole directory is removed so cameras + metadata stay
// in sync. For session events (RecentClips) every file matching the session
// prefix is removed.
func (s *Service) ExecuteCleanup(plan *CleanupPlan, dryRun bool) CleanupReport {
	report := CleanupReport{DryRun: dryRun}

	for _, events := range plan.Breakdown {
		for _, ev := range events {
			if dryRun {
				report.DeletedCount++
				report.DeletedSize += float64(ev.Size) / (1024 * 1024 * 1024)
				continue
			}

			if err := deleteEvent(ev); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("delete %s/%s: %v", ev.Folder, ev.Name, err))
				continue
			}
			s.removeEventThumbnails(ev)
			report.DeletedCount++
			report.DeletedSize += float64(ev.Size) / (1024 * 1024 * 1024)
		}
	}

	return report
}

// removeEventThumbnails deletes the cached thumbnails belonging to a deleted
// event so the on-disk thumbnail cache doesn't accumulate orphans over the
// device's lifetime. Dir-based events (Saved/Sentry) cache their thumbnails
// under <ThumbnailDir>/<folder>/<event>/; session events (RecentClips) cache
// theirs under <ThumbnailDir>/<folder>/sessions/ as <session>_<hash>.jpg.
func (s *Service) removeEventThumbnails(ev EventToDelete) {
	if s.cfg.ThumbnailDir == "" {
		return
	}
	if isEventDir(ev) {
		os.RemoveAll(filepath.Join(s.cfg.ThumbnailDir, ev.Folder, ev.Name))
		return
	}
	matches, _ := filepath.Glob(filepath.Join(s.cfg.ThumbnailDir, ev.Folder, "sessions", ev.Name+"_*.jpg"))
	for _, m := range matches {
		os.Remove(m)
	}
}

// CleanupOrphanedThumbnails removes thumbnails that no longer have corresponding videos.
func (s *Service) CleanupOrphanedThumbnails(thumbnailDir string, videoPathsExist func(string) bool) int {
	entries, err := os.ReadDir(thumbnailDir)
	if err != nil {
		return 0
	}

	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		hash := strings.TrimSuffix(e.Name(), ".png")
		if !videoPathsExist(hash) {
			os.Remove(filepath.Join(thumbnailDir, e.Name()))
			removed++
		}
	}
	return removed
}

// listEvents returns every event under folderPath. Detection is automatic:
// if there are subdirectories we treat each as an event (Saved/Sentry layout);
// otherwise we group top-level files by their session prefix (RecentClips).
func listEvents(folder, folderPath string) []EventToDelete {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil
	}

	hasSubdirs := false
	for _, e := range entries {
		if e.IsDir() {
			hasSubdirs = true
			break
		}
	}

	if hasSubdirs {
		return collectEventDirs(folder, folderPath, entries)
	}
	return collectSessions(folder, folderPath, entries)
}

func collectEventDirs(folder, folderPath string, entries []os.DirEntry) []EventToDelete {
	var events []EventToDelete
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		eventDir := filepath.Join(folderPath, e.Name())
		// Pinned events are exempt from retention, so they also drop out of the
		// preview counts. Mirrors video.KeepMarker.
		if _, err := os.Stat(filepath.Join(eventDir, keepMarker)); err == nil {
			continue
		}
		size, files, mtime := summarizeDir(eventDir)
		events = append(events, EventToDelete{
			Folder:   folder,
			Name:     e.Name(),
			Path:     eventDir,
			Files:    files,
			Size:     size,
			Modified: mtime,
		})
	}
	return events
}

func collectSessions(folder, folderPath string, entries []os.DirEntry) []EventToDelete {
	type session struct {
		files []string
		size  int64
		mtime time.Time
	}
	groups := make(map[string]*session)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := sessionPattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		key := m[1]
		info, err := e.Info()
		if err != nil {
			continue
		}
		g, ok := groups[key]
		if !ok {
			g = &session{}
			groups[key] = g
		}
		g.files = append(g.files, e.Name())
		g.size += info.Size()
		if info.ModTime().After(g.mtime) {
			g.mtime = info.ModTime()
		}
	}

	events := make([]EventToDelete, 0, len(groups))
	for name, g := range groups {
		events = append(events, EventToDelete{
			Folder:   folder,
			Name:     name,
			Path:     folderPath,
			Files:    g.files,
			Size:     g.size,
			Modified: g.mtime,
		})
	}
	return events
}

// summarizeDir returns the total byte size, the names of contained files
// (relative to the dir) and the most recent modification time. Used to
// describe an event-folder event without having to walk it twice.
func summarizeDir(dir string) (int64, []string, time.Time) {
	var size int64
	var files []string
	var mtime time.Time

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, nil, mtime
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		size += info.Size()
		files = append(files, e.Name())
		if info.ModTime().After(mtime) {
			mtime = info.ModTime()
		}
	}
	return size, files, mtime
}

// deleteEvent removes the on-disk content for an event. For dir-based events
// the whole directory is removed; for session events each file is unlinked.
func deleteEvent(ev EventToDelete) error {
	if info, err := os.Stat(ev.Path); err == nil && info.IsDir() && isEventDir(ev) {
		return os.RemoveAll(ev.Path)
	}
	for _, f := range ev.Files {
		if err := os.Remove(filepath.Join(ev.Path, f)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// isEventDir reports whether ev.Path itself is the event directory
// (dir-based event) rather than the parent folder of session files.
// Heuristic: dir-based events have a Path ending in their Name.
func isEventDir(ev EventToDelete) bool {
	return filepath.Base(ev.Path) == ev.Name
}
