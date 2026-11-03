package schedules

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const dueGrace = time.Second

// IsDue reports whether a schedule should fire at now, given its existing
// systemd timer spec and the latest execution (if any).
//
// Calendar schedules match systemd Persistent=true: a missed last occurrence
// is due once. Interval schedules fire when at least Interval has elapsed
// since the last start (or since CreatedAt when never run).
func IsDue(s Schedule, spec TimerSpec, now time.Time, last *Execution) bool {
	if spec.Type == "none" || spec.Error != "" {
		return false
	}
	if !IsEnabled(s) {
		return false
	}
	now = now.In(now.Location())

	if spec.Type == "interval" {
		d, err := ParseSystemdInterval(spec.Interval)
		if err != nil || d <= 0 {
			return false
		}
		anchor := s.CreatedAt
		if last != nil && !last.StartedAt.IsZero() {
			anchor = last.StartedAt
		}
		if anchor.IsZero() {
			return false
		}
		return !now.Before(anchor.Add(d))
	}

	occ, ok := LastOccurrence(spec, now)
	if !ok {
		return false
	}
	if now.Before(occ) {
		return false
	}
	if last != nil && !last.StartedAt.IsZero() && !last.StartedAt.Before(occ.Add(-dueGrace)) {
		return false
	}
	return true
}

// LastOccurrence returns the most recent scheduled instant <= now for a
// calendar TimerSpec produced by BuildTimerSpec. Times are wall-clock in
// now.Location() (typically TZ / time.Local).
func LastOccurrence(spec TimerSpec, now time.Time) (time.Time, bool) {
	if spec.Type != "calendar" {
		return time.Time{}, false
	}
	cal := strings.TrimSpace(spec.Calendar)
	if cal == "" {
		return time.Time{}, false
	}
	loc := now.Location()
	now = now.In(loc)

	if t, ok := parseOnceCalendar(cal, loc); ok {
		if now.Before(t) {
			return time.Time{}, false
		}
		return t, true
	}
	if t, ok := lastStarSlashMinutes(cal, now); ok {
		return t, true
	}
	if t, ok := lastHourlyCalendar(cal, now); ok {
		return t, true
	}
	if t, ok := lastDailyCalendar(cal, now); ok {
		return t, true
	}
	if t, ok := lastWeeklyCalendar(cal, now); ok {
		return t, true
	}
	if t, ok := lastMonthlyCalendar(cal, now); ok {
		return t, true
	}
	return time.Time{}, false
}

// ParseSystemdInterval parses BuildTimerSpec interval values such as "10min".
func ParseSystemdInterval(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, fmt.Errorf("empty interval")
	}
	if strings.HasSuffix(raw, "min") {
		n, err := strconv.Atoi(strings.TrimSuffix(raw, "min"))
		if err != nil || n < 1 {
			return 0, fmt.Errorf("invalid interval %q", raw)
		}
		return time.Duration(n) * time.Minute, nil
	}
	return time.ParseDuration(raw)
}

func parseOnceCalendar(cal string, loc *time.Location) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, cal, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// *:0/N  → every N minutes, systemd-style.
func lastStarSlashMinutes(cal string, now time.Time) (time.Time, bool) {
	cal = strings.TrimSpace(cal)
	if !strings.HasPrefix(cal, "*:0/") {
		return time.Time{}, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(cal, "*:0/"))
	if err != nil || n < 1 {
		return time.Time{}, false
	}
	truncated := now.Truncate(time.Minute)
	min := truncated.Minute()
	aligned := min - (min % n)
	t := time.Date(truncated.Year(), truncated.Month(), truncated.Day(), truncated.Hour(), aligned, 0, 0, truncated.Location())
	if t.After(now) {
		t = t.Add(-time.Duration(n) * time.Minute)
	}
	return t, true
}

// *-*-* *:MM:00
func lastHourlyCalendar(cal string, now time.Time) (time.Time, bool) {
	var minute int
	if _, err := fmt.Sscanf(cal, "*-*-* *:%d:00", &minute); err != nil {
		return time.Time{}, false
	}
	if minute < 0 || minute > 59 {
		return time.Time{}, false
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), minute, 0, 0, now.Location())
	if t.After(now) {
		t = t.Add(-time.Hour)
	}
	return t, true
}

// *-*-* HH:MM:00
func lastDailyCalendar(cal string, now time.Time) (time.Time, bool) {
	var hour, minute int
	n, err := fmt.Sscanf(cal, "*-*-* %d:%d:00", &hour, &minute)
	if err != nil || n != 2 {
		return time.Time{}, false
	}
	// Reject the hourly form "*-*-* *:MM:00" which also matches this scan
	// when Sscanf treats '*' poorly — require the hour token to be numeric.
	if strings.HasPrefix(strings.TrimPrefix(cal, "*-*-* "), "*:") {
		return time.Time{}, false
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return time.Time{}, false
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if t.After(now) {
		t = t.AddDate(0, 0, -1)
	}
	return t, true
}

var systemdWeekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
	"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// Mon *-*-* HH:MM:00
func lastWeeklyCalendar(cal string, now time.Time) (time.Time, bool) {
	parts := strings.Fields(cal)
	if len(parts) != 3 {
		return time.Time{}, false
	}
	day, ok := systemdWeekdays[strings.ToLower(parts[0])]
	if !ok || parts[1] != "*-*-*" {
		return time.Time{}, false
	}
	var hour, minute int
	if _, err := fmt.Sscanf(parts[2], "%d:%d:00", &hour, &minute); err != nil {
		return time.Time{}, false
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	for t.Weekday() != day {
		t = t.AddDate(0, 0, -1)
	}
	if t.After(now) {
		t = t.AddDate(0, 0, -7)
	}
	return t, true
}

// *-*-DD HH:MM:00
func lastMonthlyCalendar(cal string, now time.Time) (time.Time, bool) {
	var dom, hour, minute int
	if _, err := fmt.Sscanf(cal, "*-*-%d %d:%d:00", &dom, &hour, &minute); err != nil {
		return time.Time{}, false
	}
	if dom < 1 || dom > 31 {
		return time.Time{}, false
	}
	t := monthDateClamped(now.Year(), now.Month(), dom, hour, minute, now.Location())
	if t.After(now) {
		y, m := now.Year(), now.Month()-1
		if m < 1 {
			m = 12
			y--
		}
		t = monthDateClamped(y, m, dom, hour, minute, now.Location())
	}
	return t, true
}

func monthDateClamped(year int, month time.Month, day, hour, minute int, loc *time.Location) time.Time {
	first := time.Date(year, month, 1, hour, minute, 0, 0, loc)
	lastDay := first.AddDate(0, 1, -1).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, hour, minute, 0, 0, loc)
}
