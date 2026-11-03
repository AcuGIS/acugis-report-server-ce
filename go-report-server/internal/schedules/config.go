package schedules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	repeatMinutesCronRe = regexp.MustCompile(`^\*/(\d+)\s+\*\s+\*\s+\*\s+\*$`)
	hourlyMinuteCronRe  = regexp.MustCompile(`^(\d+)\s+\*\s+\*\s+\*\s+\*$`)
	timeHMRe            = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
	cronEveryNMinRe     = regexp.MustCompile(`^\*/(\d+)$`)
	cronFixedHMRe       = regexp.MustCompile(`^(\d+)$`)
)

// BoolValue parses PostgreSQL / form boolean (PHP jrs_schedule_bool_value).
func BoolValue(value any, defaultVal bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int:
		if v == 1 {
			return true
		}
		if v == 0 {
			return false
		}
	case int64:
		if v == 1 {
			return true
		}
		if v == 0 {
			return false
		}
	case nil:
		return defaultVal
	}
	s := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	if s == "" {
		return defaultVal
	}
	switch s {
	case "t", "true", "1", "yes", "on":
		return true
	case "f", "false", "0", "no", "off":
		return false
	default:
		return defaultVal
	}
}

// IsEnabled reports whether a schedule row is enabled (PHP jrs_schedule_is_enabled).
func IsEnabled(s Schedule) bool {
	return s.Enabled
}

// EnabledFromPost reads the enabled flag from form POST (PHP jrs_schedule_enabled_from_post).
func EnabledFromPost(post map[string]any, mode string) bool {
	raw, ok := post["enabled"]
	if !ok {
		return containsString(Modes, mode)
	}
	if arr, ok := raw.([]any); ok && len(arr) > 0 {
		raw = arr[len(arr)-1]
	}
	if arr, ok := raw.([]string); ok && len(arr) > 0 {
		raw = arr[len(arr)-1]
	}
	return BoolValue(raw, true)
}

// EffectiveMode returns the effective schedule mode (PHP jrs_schedule_effective_mode).
func EffectiveMode(s Schedule) string {
	mode := strings.ToLower(strings.TrimSpace(s.ScheduleMode))
	if mode != "" && containsString(Modes, mode) {
		return mode
	}

	cron := strings.ToLower(strings.TrimSpace(s.CronPeriod))
	if cron == "" {
		cron = "never"
	}
	switch cron {
	case "repeat_minutes":
		return "repeat_minutes"
	case "once":
		return "once"
	case "hourly", "daily", "weekly", "monthly", "custom":
		return cron
	case "now":
		return "once"
	default:
		return "never"
	}
}

// FormFieldDefaults returns UI defaults for repeat fields (PHP jrs_schedule_form_field_defaults).
func FormFieldDefaults(s Schedule) map[string]any {
	mode := EffectiveMode(s)
	cron := strings.ToLower(strings.TrimSpace(s.CronPeriod))
	if cron == "" {
		cron = "never"
	}

	repeatMinutes := 10
	if s.RepeatMinutes != nil && *s.RepeatMinutes >= 1 {
		repeatMinutes = *s.RepeatMinutes
	}

	hourlyMinute := 0
	if s.HourlyMinute != nil {
		hourlyMinute = *s.HourlyMinute
	}

	dailyTime := strings.TrimSpace(s.DailyTime)
	if dailyTime == "" {
		dailyTime = "08:00"
	}

	weeklyDay := strings.TrimSpace(s.WeeklyDay)
	if weeklyDay == "" {
		weeklyDay = "Monday"
	}

	weeklyTime := strings.TrimSpace(s.WeeklyTime)
	if weeklyTime == "" {
		weeklyTime = "08:00"
	}

	monthlyDay := 1
	if s.MonthlyDay != nil && *s.MonthlyDay >= 1 && *s.MonthlyDay <= 31 {
		monthlyDay = *s.MonthlyDay
	}

	monthlyTime := strings.TrimSpace(s.MonthlyTime)
	if monthlyTime == "" {
		monthlyTime = "08:00"
	}

	onceDatetime := strings.TrimSpace(s.OnceDatetime)
	cronCustom := strings.TrimSpace(s.CronCustom)
	if cronCustom == "" {
		cronCustom = "*/10 * * * *"
	}

	if mode == "custom" && cronCustom != "" {
		if m := repeatMinutesCronRe.FindStringSubmatch(cronCustom); len(m) == 2 {
			mode = "repeat_minutes"
			if n, err := strconv.Atoi(m[1]); err == nil {
				repeatMinutes = max(1, n)
			}
		}
	}

	if cron == "never" && strings.TrimSpace(s.ScheduleMode) == "" {
		mode = "daily"
	}

	if mode == "never" {
		mode = "daily"
	}

	return map[string]any{
		"schedule_mode":  mode,
		"repeat_minutes": repeatMinutes,
		"hourly_minute":  hourlyMinute,
		"daily_time":     dailyTime,
		"weekly_day":     weeklyDay,
		"weekly_time":    weeklyTime,
		"monthly_day":    monthlyDay,
		"monthly_time":   monthlyTime,
		"once_datetime":  onceDatetime,
		"cron_custom":    cronCustom,
	}
}

// ParseTimeHM parses HH:MM (PHP jrs_schedule_parse_time_hm).
func ParseTimeHM(timeStr string) (hour, minute int, ok bool) {
	timeStr = strings.TrimSpace(timeStr)
	m := timeHMRe.FindStringSubmatch(timeStr)
	if len(m) != 3 {
		return 0, 0, false
	}
	hour, _ = strconv.Atoi(m[1])
	minute, _ = strconv.Atoi(m[2])
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}

// ParseDatetimeLocal parses Y-m-d H:i or datetime-local input (PHP jrs_schedule_parse_datetime_local).
func ParseDatetimeLocal(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	value = strings.ReplaceAll(value, "T", " ")
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ValidateModeFields validates mode-specific POST fields (PHP jrs_schedule_validate_mode_fields).
func ValidateModeFields(mode string, post map[string]any) (ok bool, errMsg string, fields ModeFields) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if !containsString(Modes, mode) {
		return false, "Invalid schedule mode.", ModeFields{}
	}

	fields = ModeFields{ScheduleMode: mode}

	switch mode {
	case "repeat_minutes":
		n := intFromPost(post, "repeat_minutes", 0)
		if n < 1 || n > 1440 {
			return false, "Repeat interval must be between 1 and 1440 minutes.", ModeFields{}
		}
		fields.RepeatMinutes = &n

	case "hourly":
		min := intFromPost(post, "hourly_minute", -1)
		if min < 0 || min > 59 {
			return false, "Hourly minute must be between 0 and 59.", ModeFields{}
		}
		fields.HourlyMinute = &min

	case "daily":
		t := stringFromPost(post, "daily_time")
		if _, _, valid := ParseTimeHM(t); !valid {
			return false, "Daily time must be a valid HH:MM value.", ModeFields{}
		}
		fields.DailyTime = t

	case "weekly":
		day := stringFromPost(post, "weekly_day")
		if !containsString(WeeklyDays, day) {
			return false, "Select a valid day of the week.", ModeFields{}
		}
		t := stringFromPost(post, "weekly_time")
		if _, _, valid := ParseTimeHM(t); !valid {
			return false, "Weekly time must be a valid HH:MM value.", ModeFields{}
		}
		fields.WeeklyDay = day
		fields.WeeklyTime = t

	case "monthly":
		dom := intFromPost(post, "monthly_day", 0)
		if dom < 1 || dom > 31 {
			return false, "Monthly day must be between 1 and 31.", ModeFields{}
		}
		t := stringFromPost(post, "monthly_time")
		if _, _, valid := ParseTimeHM(t); !valid {
			return false, "Monthly time must be a valid HH:MM value.", ModeFields{}
		}
		fields.MonthlyDay = &dom
		fields.MonthlyTime = t

	case "once":
		raw := stringFromPost(post, "once_datetime")
		dt, valid := ParseDatetimeLocal(raw)
		if !valid {
			return false, "Run date and time is required for a one-time schedule.", ModeFields{}
		}
		fields.OnceDatetime = dt.Format("2006-01-02T15:04")

	case "custom":
		cronCustom := stringFromPost(post, "cron_custom")
		if cronCustom == "" {
			return false, "Cron expression is required for custom schedules.", ModeFields{}
		}
		parts := strings.Fields(cronCustom)
		if len(parts) != 5 {
			return false, "Cron expression must have five fields (minute hour day month weekday).", ModeFields{}
		}
		fields.CronCustom = cronCustom
	}

	return true, "", fields
}

// LegacyCron holds derived cron_period and cron_custom.
type LegacyCron struct {
	CronPeriod string
	CronCustom string
}

// DeriveLegacyCron maps repeat fields to legacy cron columns (PHP jrs_schedule_derive_legacy_cron).
func DeriveLegacyCron(mode string, fields ModeFields) LegacyCron {
	switch mode {
	case "repeat_minutes":
		n := 10
		if fields.RepeatMinutes != nil {
			n = *fields.RepeatMinutes
		}
		n = max(1, min(1440, n))
		return LegacyCron{CronPeriod: "custom", CronCustom: fmt.Sprintf("*/%d * * * *", n)}

	case "hourly":
		minute := 0
		if fields.HourlyMinute != nil {
			minute = *fields.HourlyMinute
		}
		if minute > 0 && minute <= 59 {
			return LegacyCron{CronPeriod: "custom", CronCustom: fmt.Sprintf("%d * * * *", minute)}
		}
		return LegacyCron{CronPeriod: "hourly", CronCustom: ""}

	case "daily":
		return LegacyCron{CronPeriod: "daily", CronCustom: ""}
	case "weekly":
		return LegacyCron{CronPeriod: "weekly", CronCustom: ""}
	case "monthly":
		return LegacyCron{CronPeriod: "monthly", CronCustom: ""}
	case "custom":
		return LegacyCron{CronPeriod: "custom", CronCustom: strings.TrimSpace(fields.CronCustom)}
	case "once":
		return LegacyCron{CronPeriod: "once", CronCustom: ""}
	default:
		return LegacyCron{CronPeriod: "never", CronCustom: ""}
	}
}

// PrepareForSystemd hydrates schedule_mode from legacy cron before systemd install.
func PrepareForSystemd(s Schedule) Schedule {
	cron := strings.ToLower(strings.TrimSpace(s.CronPeriod))
	cronCustom := strings.TrimSpace(s.CronCustom)

	if cron == "repeat_minutes" {
		n := 10
		if s.RepeatMinutes != nil {
			n = *s.RepeatMinutes
		}
		n = max(1, min(1440, n))
		s.CronPeriod = "custom"
		s.CronCustom = fmt.Sprintf("*/%d * * * *", n)
		s.ScheduleMode = "repeat_minutes"
		s.RepeatMinutes = &n
		cron = "custom"
		cronCustom = s.CronCustom
	}

	if m := repeatMinutesCronRe.FindStringSubmatch(cronCustom); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		s.ScheduleMode = "repeat_minutes"
		s.RepeatMinutes = &n
		s.CronPeriod = "custom"
		s.CronCustom = cronCustom
	} else if cron == "custom" && cronCustom != "" {
		s.ScheduleMode = "custom"
	} else if cron == "once" {
		s.ScheduleMode = "once"
	} else if containsString(Modes, cron) {
		s.ScheduleMode = cron
	} else if cron == "hourly" || cron == "daily" || cron == "weekly" || cron == "monthly" {
		s.ScheduleMode = cron
	}

	if m := hourlyMinuteCronRe.FindStringSubmatch(cronCustom); len(m) == 2 && cron == "custom" {
		minute, _ := strconv.Atoi(m[1])
		s.ScheduleMode = "hourly"
		s.HourlyMinute = &minute
	}

	return s
}

// Summary returns a human-readable schedule summary (PHP jrs_schedule_summary).
func Summary(s Schedule) string {
	cron := strings.ToLower(strings.TrimSpace(s.CronPeriod))
	if cron == "never" {
		return "Not scheduled"
	}
	if cron == "now" {
		return "Runs immediately on save"
	}

	mode := EffectiveMode(s)
	switch mode {
	case "repeat_minutes":
		n := 10
		if s.RepeatMinutes != nil {
			n = *s.RepeatMinutes
		}
		suffix := "s"
		if n == 1 {
			suffix = ""
		}
		return fmt.Sprintf("Runs every %d minute%s", n, suffix)

	case "hourly":
		m := 0
		if s.HourlyMinute != nil {
			m = *s.HourlyMinute
		}
		return fmt.Sprintf("Runs hourly at minute %02d", m)

	case "daily":
		t := strings.TrimSpace(s.DailyTime)
		if t == "" && cron == "daily" && strings.TrimSpace(s.ScheduleMode) == "" {
			t = "00:00"
		}
		if t == "" {
			t = "08:00"
		}
		return "Runs daily at " + t

	case "weekly":
		day := strings.TrimSpace(s.WeeklyDay)
		t := strings.TrimSpace(s.WeeklyTime)
		if t == "" {
			t = "08:00"
		}
		if day == "" && strings.TrimSpace(s.ScheduleMode) == "" {
			return "Runs weekly (legacy midnight)"
		}
		if day == "" {
			day = "Monday"
		}
		return fmt.Sprintf("Runs %ss at %s", day, t)

	case "monthly":
		dom := 1
		if s.MonthlyDay != nil {
			dom = *s.MonthlyDay
		}
		if dom < 1 && strings.TrimSpace(s.ScheduleMode) == "" {
			dom = 1
		}
		t := strings.TrimSpace(s.MonthlyTime)
		if t == "" {
			t = "08:00"
		}
		return fmt.Sprintf("Runs monthly on day %d at %s", dom, t)

	case "once":
		raw := strings.TrimSpace(s.OnceDatetime)
		if raw != "" {
			if dt, ok := ParseDatetimeLocal(raw); ok {
				return "Runs once at " + dt.Format("2006-01-02 15:04")
			}
		}
		if cron == "now" {
			return "Runs immediately on save"
		}
		return "Runs once"

	case "custom":
		expr := strings.TrimSpace(s.CronCustom)
		if expr != "" {
			return "Custom: " + expr
		}
		return "Custom schedule"

	default:
		if cron == "" {
			return "Not scheduled"
		}
		return strings.ToUpper(cron[:1]) + cron[1:]
	}
}

// WeeklyDayToSystemd converts a weekday name to systemd format.
func WeeklyDayToSystemd(day string) string {
	mapDay := map[string]string{
		"monday": "Mon", "tuesday": "Tue", "wednesday": "Wed", "thursday": "Thu",
		"friday": "Fri", "saturday": "Sat", "sunday": "Sun",
	}
	key := strings.ToLower(strings.TrimSpace(day))
	if v, ok := mapDay[key]; ok {
		return v
	}
	return "Mon"
}

// TimerSpec builds systemd timer directives (PHP jrs_schedule_timer_spec).
func BuildTimerSpec(s Schedule) TimerSpec {
	mode := EffectiveMode(s)
	if mode == "never" {
		return TimerSpec{Type: "none", Error: "no schedule"}
	}

	switch mode {
	case "repeat_minutes":
		n := 0
		if s.RepeatMinutes != nil {
			n = *s.RepeatMinutes
		}
		if n < 1 || n > 1440 {
			return TimerSpec{Type: "none", Error: "invalid repeat_minutes"}
		}
		return TimerSpec{Type: "interval", Interval: fmt.Sprintf("%dmin", n)}

	case "hourly":
		if s.HourlyMinute == nil && strings.TrimSpace(s.ScheduleMode) == "" {
			return TimerSpec{Type: "calendar", Calendar: "*-*-* *:00:00"}
		}
		min := 0
		if s.HourlyMinute != nil {
			min = *s.HourlyMinute
		}
		if min < 0 || min > 59 {
			return TimerSpec{Type: "none", Error: "invalid hourly_minute"}
		}
		return TimerSpec{Type: "calendar", Calendar: fmt.Sprintf("*-*-* *:%02d:00", min)}

	case "daily":
		dailyTime := strings.TrimSpace(s.DailyTime)
		if dailyTime == "" && strings.TrimSpace(s.ScheduleMode) == "" {
			return TimerSpec{Type: "calendar", Calendar: "*-*-* 00:00:00"}
		}
		if dailyTime == "" {
			dailyTime = "08:00"
		}
		hour, minute, ok := ParseTimeHM(dailyTime)
		if !ok {
			hour, minute = 8, 0
		}
		return TimerSpec{
			Type:     "calendar",
			Calendar: fmt.Sprintf("*-*-* %02d:%02d:00", hour, minute),
		}

	case "weekly":
		if strings.TrimSpace(s.WeeklyDay) == "" && strings.TrimSpace(s.ScheduleMode) == "" {
			return TimerSpec{Type: "calendar", Calendar: "Mon *-*-* 00:00:00"}
		}
		day := WeeklyDayToSystemd(s.WeeklyDay)
		if day == "Mon" && strings.TrimSpace(s.WeeklyDay) == "" {
			day = WeeklyDayToSystemd("Monday")
		}
		weeklyTime := strings.TrimSpace(s.WeeklyTime)
		if weeklyTime == "" {
			weeklyTime = "08:00"
		}
		hour, minute, ok := ParseTimeHM(weeklyTime)
		if !ok {
			hour, minute = 8, 0
		}
		return TimerSpec{
			Type:     "calendar",
			Calendar: fmt.Sprintf("%s *-*-* %02d:%02d:00", day, hour, minute),
		}

	case "monthly":
		if strings.TrimSpace(s.ScheduleMode) == "" && s.MonthlyDay == nil {
			return TimerSpec{Type: "calendar", Calendar: "*-*-01 00:00:00"}
		}
		dom := 1
		if s.MonthlyDay != nil {
			dom = *s.MonthlyDay
		}
		if dom < 1 || dom > 31 {
			return TimerSpec{Type: "none", Error: "invalid monthly_day"}
		}
		monthlyTime := strings.TrimSpace(s.MonthlyTime)
		if monthlyTime == "" {
			monthlyTime = "08:00"
		}
		hour, minute, ok := ParseTimeHM(monthlyTime)
		if !ok {
			hour, minute = 8, 0
		}
		return TimerSpec{
			Type:     "calendar",
			Calendar: fmt.Sprintf("*-*-%02d %02d:%02d:00", dom, hour, minute),
		}

	case "once":
		raw := strings.TrimSpace(s.OnceDatetime)
		dt, ok := ParseDatetimeLocal(raw)
		if !ok {
			return TimerSpec{Type: "none", Error: "invalid once_datetime"}
		}
		return TimerSpec{Type: "calendar", Calendar: dt.Format("2006-01-02 15:04:05")}

	case "custom":
		cron := strings.TrimSpace(s.CronCustom)
		if cron == "" {
			return TimerSpec{Type: "none", Error: "empty cron"}
		}
		calendar := CronToSystemd(cron)
		if calendar == "" {
			return TimerSpec{Type: "none", Error: "invalid cron"}
		}
		return TimerSpec{Type: "calendar", Calendar: calendar}

	default:
		return TimerSpec{Type: "none", Error: "unknown mode"}
	}
}

// CronToSystemd converts a five-field cron expression to systemd calendar syntax.
func CronToSystemd(cron string) string {
	cron = strings.TrimSpace(cron)
	if cron == "" {
		return ""
	}
	parts := strings.Fields(cron)
	if len(parts) != 5 {
		return cron
	}
	min, hour, dom, mon, dow := parts[0], parts[1], parts[2], parts[3], parts[4]

	if m := cronEveryNMinRe.FindStringSubmatch(min); len(m) == 2 && hour == "*" && dom == "*" && mon == "*" && dow == "*" {
		n, _ := strconv.Atoi(m[1])
		return fmt.Sprintf("*:0/%d", n)
	}
	if mMin := cronFixedHMRe.FindStringSubmatch(min); len(mMin) == 2 {
		if mHour := cronFixedHMRe.FindStringSubmatch(hour); len(mHour) == 2 && dom == "*" && mon == "*" && dow == "*" {
			h, _ := strconv.Atoi(mHour[1])
			mi, _ := strconv.Atoi(mMin[1])
			return fmt.Sprintf("*-*-* %02d:%02d:00", h, mi)
		}
	}

	return fmt.Sprintf("%s %s %s %s %s", min, hour, dom, mon, dow)
}

// NeedsSystemd reports whether a schedule should have a systemd timer.
func NeedsSystemd(s Schedule) bool {
	if !IsEnabled(s) {
		return false
	}
	cron := strings.ToLower(strings.TrimSpace(s.CronPeriod))
	if cron == "never" || cron == "now" {
		return false
	}
	if containsString([]string{"hourly", "daily", "weekly", "monthly", "custom", "once", "repeat_minutes"}, cron) {
		return true
	}
	return EffectiveMode(s) != "never"
}

func stringFromPost(post map[string]any, key string) string {
	if post == nil {
		return ""
	}
	v, ok := post[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func intFromPost(post map[string]any, key string, fallback int) int {
	s := stringFromPost(post, key)
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func isValidOutputFormat(format string) bool {
	format = strings.ToLower(strings.TrimSpace(format))
	return containsString(OutputFormats, format)
}
