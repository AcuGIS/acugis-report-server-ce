package schedules_test

import (
	"context"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/schedules"
)

func nyLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load America/New_York: %v", err)
	}
	return loc
}

func TestLastOccurrenceDaily(t *testing.T) {
	loc := time.UTC
	spec := schedules.TimerSpec{Type: "calendar", Calendar: "*-*-* 08:30:00"}
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, loc)
	got, ok := schedules.LastOccurrence(spec, now)
	if !ok || !got.Equal(time.Date(2026, 8, 13, 8, 30, 0, 0, loc)) {
		t.Fatalf("after daily slot: got %v ok=%v", got, ok)
	}
	now = time.Date(2026, 8, 13, 8, 0, 0, 0, loc)
	got, ok = schedules.LastOccurrence(spec, now)
	if !ok || !got.Equal(time.Date(2026, 8, 12, 8, 30, 0, 0, loc)) {
		t.Fatalf("before daily slot: got %v ok=%v", got, ok)
	}
}

func TestLastOccurrenceHourlyWeeklyMonthly(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 8, 13, 10, 20, 0, 0, loc) // Thursday

	got, ok := schedules.LastOccurrence(schedules.TimerSpec{Type: "calendar", Calendar: "*-*-* *:15:00"}, now)
	if !ok || !got.Equal(time.Date(2026, 8, 13, 10, 15, 0, 0, loc)) {
		t.Fatalf("hourly: %v ok=%v", got, ok)
	}

	got, ok = schedules.LastOccurrence(schedules.TimerSpec{Type: "calendar", Calendar: "Thu *-*-* 09:00:00"}, now)
	if !ok || !got.Equal(time.Date(2026, 8, 13, 9, 0, 0, 0, loc)) {
		t.Fatalf("weekly this week: %v ok=%v", got, ok)
	}

	got, ok = schedules.LastOccurrence(schedules.TimerSpec{Type: "calendar", Calendar: "*-*-01 08:00:00"}, now)
	if !ok || !got.Equal(time.Date(2026, 8, 1, 8, 0, 0, 0, loc)) {
		t.Fatalf("monthly: %v ok=%v", got, ok)
	}
}

func TestLastOccurrenceOnceAndTimezone(t *testing.T) {
	loc := nyLoc(t)
	spec := schedules.TimerSpec{Type: "calendar", Calendar: "2026-08-13 14:05:00"}
	before := time.Date(2026, 8, 13, 14, 0, 0, 0, loc)
	if _, ok := schedules.LastOccurrence(spec, before); ok {
		t.Fatal("once should not be due before wall-clock time in America/New_York")
	}
	after := time.Date(2026, 8, 13, 14, 5, 0, 0, loc)
	got, ok := schedules.LastOccurrence(spec, after)
	if !ok || !got.Equal(time.Date(2026, 8, 13, 14, 5, 0, 0, loc)) {
		t.Fatalf("once at NY 14:05: got %v ok=%v", got, ok)
	}
	// Same instant as UTC 18:05 in August (EDT, UTC-4) must still match NY wall clock.
	utc := after.UTC()
	if utc.Hour() != 18 {
		t.Fatalf("expected EDT offset, utc hour=%d", utc.Hour())
	}
}

func TestIsDueOnceDoesNotRepeat(t *testing.T) {
	loc := nyLoc(t)
	s := schedules.Schedule{ID: 1, Enabled: true, ScheduleMode: "once", CronPeriod: "once", OnceDatetime: "2026-08-13T14:05"}
	spec := schedules.TimerSpec{Type: "calendar", Calendar: "2026-08-13 14:05:00"}
	now := time.Date(2026, 8, 13, 14, 6, 0, 0, loc)
	if !schedules.IsDue(s, spec, now, nil) {
		t.Fatal("expected first fire")
	}
	last := &schedules.Execution{StartedAt: time.Date(2026, 8, 13, 14, 5, 1, 0, loc), Status: "success"}
	if schedules.IsDue(s, spec, now.Add(time.Hour), last) {
		t.Fatal("once must not fire twice")
	}
}

func TestIsDueDailyMissedOccurrenceFiresOnce(t *testing.T) {
	loc := time.UTC
	s := schedules.Schedule{ID: 2, Enabled: true, ScheduleMode: "daily", CronPeriod: "daily", DailyTime: "08:00"}
	spec := schedules.BuildTimerSpec(s)
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, loc)
	if !schedules.IsDue(s, spec, now, nil) {
		t.Fatal("persistent catch-up should fire missed 08:00")
	}
	last := &schedules.Execution{StartedAt: time.Date(2026, 8, 13, 8, 0, 5, 0, loc), Status: "success"}
	if schedules.IsDue(s, spec, now, last) {
		t.Fatal("already executed this occurrence")
	}
}

func TestIsDueInterval(t *testing.T) {
	loc := time.UTC
	n := 10
	s := schedules.Schedule{
		ID: 3, Enabled: true, ScheduleMode: "repeat_minutes", CronPeriod: "repeat_minutes",
		RepeatMinutes: &n, CreatedAt: time.Date(2026, 8, 13, 8, 0, 0, 0, loc),
	}
	spec := schedules.BuildTimerSpec(s)
	now := time.Date(2026, 8, 13, 8, 9, 0, 0, loc)
	if schedules.IsDue(s, spec, now, nil) {
		t.Fatal("interval should wait until createdAt+10m")
	}
	now = time.Date(2026, 8, 13, 8, 10, 0, 0, loc)
	if !schedules.IsDue(s, spec, now, nil) {
		t.Fatal("interval due at createdAt+10m")
	}
	last := &schedules.Execution{StartedAt: now, Status: "success"}
	if schedules.IsDue(s, spec, now.Add(9*time.Minute), last) {
		t.Fatal("interval not due yet")
	}
	if !schedules.IsDue(s, spec, now.Add(10*time.Minute), last) {
		t.Fatal("interval due 10m after last start")
	}
}

func TestIsDueDisabledAndNone(t *testing.T) {
	s := schedules.Schedule{Enabled: false, ScheduleMode: "daily", CronPeriod: "daily", DailyTime: "08:00"}
	spec := schedules.BuildTimerSpec(s)
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	if schedules.IsDue(s, spec, now, nil) {
		t.Fatal("disabled schedule must not be due")
	}
	if schedules.IsDue(s, schedules.TimerSpec{Type: "none"}, now, nil) {
		t.Fatal("none spec must not be due")
	}
}

func TestDispatcherTickNilRepo(t *testing.T) {
	d := schedules.NewDispatcher(schedules.DispatcherOptions{
		Run: func(_ context.Context, _ int) error {
			t.Fatal("run must not be called without a repository")
			return nil
		},
		Location: time.UTC,
	})
	if d.PollInterval() != schedules.DefaultDispatcherPollInterval {
		t.Fatalf("poll: %s", d.PollInterval())
	}
	d.Tick(context.Background())
}

func TestScheduleRuntimeDefaultSystemd(t *testing.T) {
	t.Setenv("JRS_SCHEDULE_RUNTIME", "")
	if got := schedules.ScheduleRuntime(); got != "systemd" {
		t.Fatalf("default runtime=%q", got)
	}
	t.Setenv("JRS_SCHEDULE_RUNTIME", "dispatcher")
	if got := schedules.ScheduleRuntime(); got != "dispatcher" {
		t.Fatalf("dispatcher runtime=%q", got)
	}
}
