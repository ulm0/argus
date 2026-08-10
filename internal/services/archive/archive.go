// Package archive copies TeslaCam event directories off the car onto a mounted
// target (a CIFS share, an external drive) whenever the Pi joins a known
// network, so the footage survives the car.
//
// Clips are written to <archive.target_path>/TeslaCam/<folder>/<event>, the
// layout video.Service.GetArchivePath expects, so pointing
// installation.archive_path at the same directory makes archived clips
// browsable in the UI.
package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
	"github.com/ulm0/argus/internal/services/video"
	"github.com/ulm0/argus/internal/services/wifi"
	"github.com/ulm0/argus/internal/system/mount"
)

// pollInterval is how often we check whether we are on the configured network.
// Polling keeps the AP failover monitor's callbacks with the AP manager, which
// owns them, and a sync pass over already-archived events costs two ReadDirs.
const pollInterval = 2 * time.Minute

// archivedMarker marks a fully copied event directory. It lives at the
// destination, not inside the source event: in present mode the TeslaCam mount
// is read-only, so the source cannot be written. A target directory without the
// marker is a copy a power cut interrupted, and gets retried next pass.
const archivedMarker = ".argus-archived"

// eventQuietPeriod is how long an event must go untouched before we archive it.
// The car writes an event directory incrementally while Sentry records, and a
// directory copied mid-write would get the marker and never be retried, losing
// every clip written after the copy.
const eventQuietPeriod = 5 * time.Minute

type Status struct {
	Enabled       bool   `json:"enabled"`
	SSID          string `json:"ssid"`
	TargetPath    string `json:"target_path"`
	IncludeRecent bool   `json:"include_recent"`
	Running       bool   `json:"running"`
	LastRun       string `json:"last_run,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	CopiedTotal   int    `json:"copied_total"`
}

type Service struct {
	cfg   *config.Config
	video *video.Service
	wifi  *wifi.Monitor

	mu          sync.Mutex
	running     bool
	lastRun     time.Time
	lastError   string
	copiedTotal int

	// cfgMu serializes this service's own reads of the Archive config so the loop,
	// Status() and sync() always see one consistent snapshot, same as
	// telegram.Service.cfgMu.
	//
	// ponytail: reader-side only. PATCH /api/config writes cfg.Archive (and
	// cfg.Telegram) directly on the shared *config.Config without taking any
	// service lock, so a concurrent save is still technically unsynchronised.
	// Closing that properly means one mutex on config.Config itself, taken by
	// Patch/Save and every service snapshot — a repo-wide change, not an
	// archive-specific one.
	cfgMu sync.Mutex
}

// archiveCfg snapshots the mutable config in one guarded read, so a tick sees a
// consistent SSID/target/flags set even if a PATCH lands mid-pass.
func (s *Service) archiveCfg() config.ArchiveConfig {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	return s.cfg.Archive
}

func NewService(cfg *config.Config) *Service {
	s := &Service{cfg: cfg}
	if cfg != nil {
		s.video = video.NewService(cfg)
		s.wifi = wifi.NewMonitor(cfg)
	}
	return s
}

// Start runs the sync loop regardless of Archive.Enabled so that enabling
// archiving at runtime (PATCH /api/config) takes effect without a restart. A
// disabled tick only reads a bool.
func (s *Service) Start(ctx context.Context) {
	if s == nil || s.cfg == nil {
		return
	}
	go s.loop(ctx)
	logger.L.Info("Archive sync started")
}

func (s *Service) loop(ctx context.Context) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Enabled is checked first so a disabled tick never shells out to
			// iwgetid.
			if ac := s.archiveCfg(); ac.Enabled && s.onTargetNetwork(ac.SSID) {
				s.Run()
			}
		}
	}
}

// onTargetNetwork reports whether we are on the WiFi network that triggers a
// sync. An empty configured SSID means any WiFi connection will do.
func (s *Service) onTargetNetwork(wantSSID string) bool {
	conn := s.wifi.GetCurrentConnection()
	if !conn.Connected {
		return false
	}
	return wantSSID == "" || conn.SSID == wantSSID
}

// Run starts a sync in the background and returns immediately; the copy itself
// can take minutes over the Pi's link.
func (s *Service) Run() error {
	if s == nil || s.cfg == nil {
		return errors.New("archive service unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return errors.New("sync already running")
	}
	s.running = true

	go func() {
		copied, err := s.sync()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.running = false
		s.lastRun = time.Now()
		s.copiedTotal += copied
		s.lastError = ""
		if err != nil {
			s.lastError = err.Error()
			logger.L.WithError(err).Warn("Archive: sync failed")
		} else {
			logger.L.WithField("copied", copied).Info("Archive: sync finished")
		}
	}()
	return nil
}

func (s *Service) Status() Status {
	if s == nil || s.cfg == nil {
		return Status{}
	}
	ac := s.archiveCfg()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{
		Enabled:       ac.Enabled,
		SSID:          ac.SSID,
		TargetPath:    ac.TargetPath,
		IncludeRecent: ac.IncludeRecent,
		Running:       s.running,
		LastError:     s.lastError,
		CopiedTotal:   s.copiedTotal,
	}
	if !s.lastRun.IsZero() {
		st.LastRun = s.lastRun.Format(time.RFC3339)
	}
	return st
}

func (s *Service) sync() (int, error) {
	ac := s.archiveCfg()
	target := ac.TargetPath
	if target == "" {
		return 0, errors.New("no target path configured")
	}
	// The target is a mount point someone else set up. Test that something is
	// actually mounted there, not merely that the directory exists — an
	// unmounted CIFS share leaves its mountpoint behind as an empty local
	// directory, and copying into it would fill the Pi's SD card instead, marking
	// each event archived so it is never retried once the share comes back.
	if !mount.NewManager().IsMounted(target) {
		return 0, fmt.Errorf("target path %s is not mounted", target)
	}
	tcPath := s.video.GetTeslaCamPath()
	if tcPath == "" {
		return 0, errors.New("TeslaCam path not available")
	}

	folders := []string{"SavedClips", "SentryClips"}
	if ac.IncludeRecent {
		folders = append(folders, "RecentClips")
	}

	// Mirror the on-car layout under the target so that pointing
	// installation.archive_path at the same directory makes the copies
	// browsable (video.Service.GetArchivePath appends TeslaCam).
	dstRoot := filepath.Join(target, "TeslaCam")

	total := 0
	var firstErr error
	for _, folder := range folders {
		n, err := syncFolder(filepath.Join(tcPath, folder), filepath.Join(dstRoot, folder))
		total += n
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return total, firstErr
}

// syncFolder copies event directories (SavedClips, SentryClips) and loose clip
// files (RecentClips) from src to dst one at a time, skipping whatever is
// already archived, and returns how many it copied.
//
// ponytail: sequential, no bandwidth control — the Pi Zero's link is the
// bottleneck. Add parallelism only if a real transfer proves too slow.
func syncFolder(src, dst string) (int, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	copied := 0
	var firstErr error
	for _, e := range entries {
		var err error
		switch {
		case e.IsDir():
			if _, serr := os.Stat(filepath.Join(dst, e.Name(), archivedMarker)); serr == nil {
				continue
			}
			if stillWriting(filepath.Join(src, e.Name())) {
				continue // the car may still be adding clips; retry next pass
			}
			err = copyEvent(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
		case e.Type().IsRegular():
			if _, serr := os.Stat(filepath.Join(dst, e.Name())); serr == nil {
				continue
			}
			// Same trap as an event directory: a clip still being written would
			// be copied truncated, and its presence at the destination stops it
			// ever being copied again.
			if info, ierr := e.Info(); ierr != nil || time.Since(info.ModTime()) < eventQuietPeriod {
				continue
			}
			if err = os.MkdirAll(dst, 0755); err == nil {
				err = copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
			}
		default:
			continue // symlinks and devices are not TeslaCam output
		}
		if err != nil {
			logger.L.WithError(err).WithField("event", e.Name()).Warn("Archive: failed to copy event")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		copied++
	}
	return copied, firstErr
}

// stillWriting reports whether anything in an event directory changed inside
// eventQuietPeriod, i.e. the car is probably not done with it yet. An
// unreadable directory is left to copyEvent to fail on.
func stillWriting(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) < eventQuietPeriod {
			return true
		}
	}
	return false
}

// copyEvent copies one event directory, then marks it complete.
func copyEvent(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue // an event is flat: clips, event.json, thumb.png
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dst, archivedMarker), nil, 0644)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Stream through a temp name and rename, so an interrupted transfer never
	// leaves a truncated clip that looks like a finished one.
	tmp := dst + ".partial"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
