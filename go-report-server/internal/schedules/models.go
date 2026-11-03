package schedules

import "time"

// Output formats supported for scheduled reports (PHP jrs_schedule_output_formats).
var OutputFormats = []string{"pdf", "xlsx", "csv", "html", "xls", "docx"}

// Modes are friendly repeat-scheduler modes (PHP jrs_schedule_modes).
var Modes = []string{"once", "repeat_minutes", "hourly", "daily", "weekly", "monthly", "custom"}

// CronPeriods includes legacy cron_period values (PHP jrs_schedule_cron_periods).
var CronPeriods = []string{"never", "now", "once", "repeat_minutes", "hourly", "daily", "weekly", "monthly", "custom"}

// WeeklyDays are valid weekly_day form values.
var WeeklyDays = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

// Schedule mirrors a jasper_schedule row.
type Schedule struct {
	ID             int
	CronPeriod     string
	CronCustom     string
	ScheduleMode   string
	RepeatMinutes  *int
	HourlyMinute   *int
	DailyTime      string
	WeeklyDay      string
	WeeklyTime     string
	MonthlyDay     *int
	MonthlyTime    string
	OnceDatetime   string
	ReportName     string
	DatasourceName string
	Format         string
	Filename       string
	Email          string
	EmailSubject   string
	EmailBody      string
	EmailTemplate  string
	NoEmail        bool
	URLOptParams   string
	Enabled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Recipient mirrors a jasper_recipient row.
type Recipient struct {
	ID    int
	Name  string
	Email string
	Notes string
}

// Execution mirrors a jasper_schedule_execution row with optional schedule join fields.
type Execution struct {
	ID               int
	ScheduleID       *int
	Status           string
	StartedAt        time.Time
	FinishedAt       *time.Time
	OutputFile       string
	EmailStatus      string
	ErrorMessage     string
	ReportName       string
	ScheduleFilename string
}

// ModeFields holds validated repeat-scheduler field values.
type ModeFields struct {
	ScheduleMode   string
	RepeatMinutes  *int
	HourlyMinute   *int
	DailyTime      string
	WeeklyDay      string
	WeeklyTime     string
	MonthlyDay     *int
	MonthlyTime    string
	OnceDatetime   string
	CronCustom     string
}

// TimerSpec describes a systemd timer directive (PHP jrs_schedule_timer_spec).
type TimerSpec struct {
	Type     string // none, interval, calendar
	Calendar string
	Interval string
	Error    string
}

// SyncResult is returned by SystemdManager.SyncAfterSave.
type SyncResult struct {
	OK            bool
	TimerRequired bool
	Error         string
}

// ExecuteResult is returned by Runner.Execute.
type ExecuteResult struct {
	OK           bool
	ExecutionID  int
	OutputFile   string
	EmailStatus  string
	Message      string
}
