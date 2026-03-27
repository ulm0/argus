package chime

import (
	"testing"
	"time"
)

func TestScheduleMatchesNow_Date(t *testing.T) {
	now := time.Date(2026, time.December, 25, 8, 30, 0, 0, time.Local)
	s := &Schedule{
		Type:  ScheduleDate,
		Month: 12,
		Day:   25,
		Time:  "08:30",
	}
	if !scheduleMatchesNow(s, now) {
		t.Fatal("expected date schedule to match")
	}
}

func TestScheduleMatchesNow_Holiday(t *testing.T) {
	now := time.Date(2026, time.December, 25, 9, 0, 0, 0, time.Local)
	s := &Schedule{
		Type:    ScheduleHoliday,
		Holiday: "christmas",
		Time:    "09:00",
	}
	if !scheduleMatchesNow(s, now) {
		t.Fatal("expected holiday schedule to match")
	}
}

func TestScheduleMatchesNow_Recurring(t *testing.T) {
	now := time.Now()
	last := now.Add(-16 * time.Minute)
	s := &Schedule{
		Type:     ScheduleRecurring,
		Interval: "15min",
		LastRun:  &last,
	}
	if !scheduleMatchesNow(s, now) {
		t.Fatal("expected recurring schedule to match")
	}
}

func TestParseRecurringInterval(t *testing.T) {
	cases := map[string]time.Duration{
		"15min":   15 * time.Minute,
		"2 hours": 2 * time.Hour,
		"45m":     45 * time.Minute,
		"1h":      1 * time.Hour,
	}
	for in, want := range cases {
		if got := parseRecurringInterval(in); got != want {
			t.Fatalf("parseRecurringInterval(%q) = %v, want %v", in, got, want)
		}
	}
}
