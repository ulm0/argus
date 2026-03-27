package chime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ulm0/argus/internal/logger"
	"github.com/ulm0/argus/internal/services/partition"
	"github.com/ulm0/argus/internal/system/gadget"
	"github.com/ulm0/argus/internal/system/loop"
	"github.com/ulm0/argus/internal/system/mount"
)

// RunSchedulerTick evaluates enabled weekly schedules once (call every minute from run).
// Matches TeslaUSB check_chime_schedule.py at a minimal level: weekly + time-of-day.
func (s *Service) RunSchedulerTick(ctx context.Context) {
	lockFiles := []string{
		filepath.Join(s.cfg.GadgetDir, ".quick_edit_part2.lock"),
		filepath.Join(s.cfg.GadgetDir, ".quick_edit_part3.lock"),
	}
	for _, f := range lockFiles {
		if info, err := os.Stat(f); err == nil {
			if time.Since(info.ModTime()) < 2*time.Minute {
				return
			}
			_ = os.Remove(f)
		}
	}

	schedules := s.scheduler.ListSchedules(true)
	now := time.Now()
	for i := range schedules {
		sch := &schedules[i]
		if !sch.Enabled || sch.Type != ScheduleWeekly {
			continue
		}
		if !weeklyMatchesNow(sch, now) {
			continue
		}
		if sch.LastRun != nil && sch.LastRun.Year() == now.Year() && sch.LastRun.YearDay() == now.YearDay() &&
			sch.LastRun.Hour() == now.Hour() && sch.LastRun.Minute() == now.Minute() {
			continue
		}

		chimeFile := sch.ChimeFilename
		if chimeFile == "" {
			continue
		}
		if strings.EqualFold(chimeFile, "RANDOM") {
			gm := s.Groups()
			chimeFile = gm.SelectRandomChime("")
			if chimeFile == "" {
				continue
			}
		}

		if err := s.ApplyScheduledChime(ctx, chimeFile); err != nil {
			logger.L.WithError(err).WithField("schedule_id", sch.ID).Warn("scheduled chime apply failed")
			continue
		}
		s.scheduler.RecordExecution(sch.ID)
		logger.L.WithField("schedule_id", sch.ID).WithField("chime", chimeFile).Info("scheduled chime applied")
	}
}

func weeklyMatchesNow(sch *Schedule, t time.Time) bool {
	h, m, ok := parseHHMM(sch.Time)
	if !ok {
		return false
	}
	if t.Hour() != h || t.Minute() != m {
		return false
	}
	if len(sch.Days) == 0 {
		return false
	}
	wd := int(t.Weekday())
	monFirst := (wd + 6) % 7
	for _, d := range sch.Days {
		if d == monFirst || d == wd {
			return true
		}
	}
	return false
}

func parseHHMM(s string) (hour, min int, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return h, m, true
}

// ApplyScheduledChime sets the active lock chime in edit mode (RW mount), quick-edit in present mode,
// or a temporary RW loop mount when mode is unknown (e.g. boot before state is written).
func (s *Service) ApplyScheduledChime(ctx context.Context, chimeFilename string) error {
	mode := strings.TrimSpace(string(readFileOrEmpty(s.cfg.StateFile)))
	if mode == "edit" {
		mp := partition.AccessiblePath(s.cfg, "part2")
		return s.SetActiveChime(chimeFilename, mp)
	}
	if mode == "present" {
		g := gadget.NewManager()
		l := loop.NewManager()
		m := mount.NewManager()
		q := mount.NewQuickEditor(s.cfg, g, l, m)
		return q.QuickEditPart2(func(ctx context.Context, rwPath string) error {
			return s.SetActiveChime(chimeFilename, rwPath)
		}, 5*time.Minute)
	}
	return s.setActiveChimeWithLoopMount(chimeFilename)
}

func (s *Service) setActiveChimeWithLoopMount(chimeFilename string) error {
	if !s.cfg.DiskImages.Part2Enabled {
		return fmt.Errorf("part2 disabled")
	}
	mnt := mount.NewManager()
	loop := loop.NewManager()
	mp := s.cfg.MountPath("part2", false)
	if mnt.IsMounted(mp) {
		return s.SetActiveChime(chimeFilename, mp)
	}
	dev, err := loop.Create(s.cfg.ImgLightshow, false)
	if err != nil {
		return err
	}
	defer func() {
		_ = mnt.SafeUnmountDir(mp)
		_ = loop.Detach(dev)
	}()
	if err := os.MkdirAll(mp, 0755); err != nil {
		return err
	}
	fs, _ := mnt.DetectFSType(dev)
	if fs == "" {
		fs = "vfat"
	}
	if err := mnt.Mount(dev, mp, fs, false); err != nil {
		return err
	}
	return s.SetActiveChime(chimeFilename, mp)
}

func readFileOrEmpty(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return b
}
