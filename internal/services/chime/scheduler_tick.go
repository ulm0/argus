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

// RunSchedulerTick evaluates enabled schedules once (call every minute from run).
// Supports weekly/date/holiday/recurring schedule types.
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
		if !sch.Enabled {
			continue
		}
		if !scheduleMatchesNow(sch, now) {
			continue
		}
		if alreadyRanThisMinute(sch, now) {
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

func alreadyRanThisMinute(sch *Schedule, t time.Time) bool {
	if sch.LastRun == nil {
		return false
	}
	return sch.LastRun.Year() == t.Year() &&
		sch.LastRun.YearDay() == t.YearDay() &&
		sch.LastRun.Hour() == t.Hour() &&
		sch.LastRun.Minute() == t.Minute()
}

func scheduleMatchesNow(sch *Schedule, t time.Time) bool {
	switch sch.Type {
	case ScheduleWeekly:
		return weeklyMatchesNow(sch, t)
	case ScheduleDate:
		return dateMatchesNow(sch, t)
	case ScheduleHoliday:
		return holidayMatchesNow(sch, t)
	case ScheduleRecurring:
		return recurringMatchesNow(sch, t)
	default:
		return false
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

func dateMatchesNow(sch *Schedule, t time.Time) bool {
	h, m, ok := parseHHMM(sch.Time)
	if !ok {
		return false
	}
	if t.Hour() != h || t.Minute() != m {
		return false
	}
	return t.Month() == time.Month(sch.Month) && t.Day() == sch.Day
}

func holidayMatchesNow(sch *Schedule, t time.Time) bool {
	h, m, ok := parseHHMM(sch.Time)
	if !ok {
		return false
	}
	if t.Hour() != h || t.Minute() != m {
		return false
	}
	month, day, ok := holidayDate(strings.TrimSpace(strings.ToLower(sch.Holiday)), t.Year())
	if !ok {
		return false
	}
	return t.Month() == month && t.Day() == day
}

func recurringMatchesNow(sch *Schedule, t time.Time) bool {
	interval := parseRecurringInterval(sch.Interval)
	if interval <= 0 {
		return false
	}
	if sch.LastRun == nil {
		return true
	}
	return t.Sub(*sch.LastRun) >= interval
}

func parseRecurringInterval(raw string) time.Duration {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return 0
	}
	v = strings.ReplaceAll(v, "mins", "m")
	v = strings.ReplaceAll(v, "min", "m")
	v = strings.ReplaceAll(v, "hours", "h")
	v = strings.ReplaceAll(v, "hour", "h")
	v = strings.ReplaceAll(v, " ", "")
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0
	}
	return d
}

func holidayDate(name string, year int) (time.Month, int, bool) {
	switch name {
	case "newyear", "newyears", "new_year", "new_years":
		return time.January, 1, true
	case "valentines", "valentinesday", "valentine", "valentine_day":
		return time.February, 14, true
	case "halloween":
		return time.October, 31, true
	case "christmas", "christmasday":
		return time.December, 25, true
	case "thanksgiving":
		d := nthWeekdayOfMonth(year, time.November, time.Thursday, 4)
		return d.Month(), d.Day(), true
	case "easter":
		d := easterSunday(year)
		return d.Month(), d.Day(), true
	default:
		return 0, 0, false
	}
}

func nthWeekdayOfMonth(year int, month time.Month, weekday time.Weekday, n int) time.Time {
	d := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	for d.Weekday() != weekday {
		d = d.AddDate(0, 0, 1)
	}
	return d.AddDate(0, 0, 7*(n-1))
}

// easterSunday computes Gregorian Easter date for the provided year.
func easterSunday(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
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
