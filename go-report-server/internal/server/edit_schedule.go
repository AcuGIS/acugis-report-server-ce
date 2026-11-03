package server

import (
	_ "embed"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/internal/schedules"
)

//go:embed edit_schedule.html
var editScheduleHTML string

type scheduleFormView struct {
	ScheduleMode  string
	RepeatMinutes int
	HourlyMinute  int
	DailyTime     string
	WeeklyDay     string
	WeeklyTime    string
	MonthlyDay    int
	MonthlyTime   string
	OnceDatetime  string
	CronCustom    string
}

type editSchedulePageData struct {
	materialChromeData
	EditID           int
	PageTitle        string
	Error            string
	IsUpdate         bool
	ScheduleSummary  string
	ScheduleEnabled  bool
	Form             scheduleFormView
	ReportName       string
	DatasourceName   string
	Format           string
	Filename         string
	Email            string
	EmailSubject     string
	EmailBody        string
	EmailTemplate    string
	NoEmail          bool
	URLOptParams     string
	Reports          []reports.Summary
	Datasources      []string
	Recipients       []schedules.Recipient
	EmailTemplates   []string
	EmailTmplDir     string
	Modes            []string
	ModeLabels       map[string]string
	WeeklyDays       []string
	OutputFormats    []string
	BackURL          string
}

var editScheduleTemplate = template.Must(template.New("edit_schedule").Funcs(template.FuncMap{
	"eq": func(a, b string) bool { return a == b },
	"modeLabel": func(mode string) string {
		if label, ok := scheduleModeLabels[mode]; ok {
			return label
		}
		return mode
	},
}).Parse(editScheduleHTML))

func (s *Server) handleEditSchedulePage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	editID, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
	sched := defaultScheduleRow()
	errorMsg := strings.TrimSpace(r.URL.Query().Get("error"))

	repo := s.deps.Schedules
	if repo != nil && repo.Configured() && editID > 0 {
		loaded, err := repo.Get(r.Context(), editID)
		if err != nil {
			errorMsg = "Schedule not found."
		} else {
			sched = *loaded
		}
	}

	pageTitle := "Add schedule"
	if editID > 0 {
		pageTitle = "Edit schedule #" + strconv.Itoa(editID)
	}

	defaults := schedules.FormFieldDefaults(sched)
	form := scheduleFormView{
		ScheduleMode:  strFromMap(defaults, "schedule_mode"),
		RepeatMinutes: intFromMap(defaults, "repeat_minutes", 10),
		HourlyMinute:  intFromMap(defaults, "hourly_minute", 0),
		DailyTime:     strFromMap(defaults, "daily_time"),
		WeeklyDay:     strFromMap(defaults, "weekly_day"),
		WeeklyTime:    strFromMap(defaults, "weekly_time"),
		MonthlyDay:    intFromMap(defaults, "monthly_day", 1),
		MonthlyTime:   strFromMap(defaults, "monthly_time"),
		OnceDatetime:  strFromMap(defaults, "once_datetime"),
		CronCustom:    strFromMap(defaults, "cron_custom"),
	}

	reportList := []reports.Summary{}
	if s.deps.Reports != nil {
		if list, err := s.deps.Reports.List(); err == nil {
			reportList = list
		}
	}

	recipients := []schedules.Recipient{}
	if s.deps.Recipients != nil {
		if list, err := s.deps.Recipients.List(r.Context()); err == nil {
			recipients = list
		}
	}

	templates := []string{}
	emailDir := ""
	if s.deps.EmailTmpls != nil {
		templates = s.deps.EmailTmpls.List()
	}
	if s.cfg.Paths.EmailTmplDir != "" {
		emailDir = s.cfg.Paths.EmailTmplDir
	}

	data := editSchedulePageData{
		materialChromeData: s.materialChrome(pageTitle),
		EditID:             editID,
		PageTitle:          pageTitle,
		Error:              errorMsg,
		IsUpdate:           editID > 0,
		ScheduleSummary:    schedules.Summary(sched),
		ScheduleEnabled:    scheduleEnabledForForm(sched, editID),
		Form:               form,
		ReportName:         sched.ReportName,
		DatasourceName:     sched.DatasourceName,
		Format:             defaultString(sched.Format, "pdf"),
		Filename:           sched.Filename,
		Email:              sched.Email,
		EmailSubject:       sched.EmailSubject,
		EmailBody:          sched.EmailBody,
		EmailTemplate:      sched.EmailTemplate,
		NoEmail:            sched.NoEmail,
		URLOptParams:       sched.URLOptParams,
		Reports:            reportList,
		Datasources:        s.listDatasourceNamesForSchedule(r),
		Recipients:         recipients,
		EmailTemplates:     templates,
		EmailTmplDir:       emailDir,
		Modes:              schedules.Modes,
		ModeLabels:         scheduleModeLabels,
		WeeklyDays:         schedules.WeeklyDays,
		OutputFormats:      schedules.OutputFormats,
		BackURL:            s.goURL("/schedules"),
	}

	s.renderEditSchedule(w, data)
}

func defaultScheduleRow() schedules.Schedule {
	return schedules.Schedule{
		CronPeriod:   "daily",
		ScheduleMode: "daily",
		CronCustom:   "*/10 * * * *",
		Format:       "pdf",
		Enabled:      true,
	}
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (s *Server) renderEditSchedule(w http.ResponseWriter, data editSchedulePageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := editScheduleTemplate.Execute(w, data); err != nil {
		s.logger.Error("render edit schedule", "error", err)
	}
}
