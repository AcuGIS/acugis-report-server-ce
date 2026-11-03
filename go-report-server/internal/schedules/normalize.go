package schedules

import (
	"context"
	"fmt"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

// ReportChecker validates report keys and returns default datasource (PHP jrs_read_report).
type ReportChecker interface {
	ReadReport(ctx context.Context, key string) (datasource string, err error)
}

// DatasourceChecker verifies a datasource exists (PHP jrs_find_datasource).
type DatasourceChecker interface {
	FindDatasource(ctx context.Context, name string) error
}

// NormalizeResult is returned by NormalizePost.
type NormalizeResult struct {
	OK   bool
	Data *Schedule
	Err  string
}

// NormalizePost normalizes form POST into schedule row fields (PHP jrs_schedule_normalize_post).
func NormalizePost(ctx context.Context, post map[string]any, reports ReportChecker, datasources DatasourceChecker) NormalizeResult {
	legacyCron := strings.ToLower(strings.TrimSpace(stringFromPost(post, "cron_period")))
	mode := strings.ToLower(strings.TrimSpace(stringFromPost(post, "schedule_mode")))

	if mode == "" && legacyCron != "" {
		if containsString(Modes, legacyCron) {
			mode = legacyCron
		} else if legacyCron == "never" || legacyCron == "now" {
			return NormalizeResult{OK: false, Err: "Select how often this schedule should run."}
		}
	}
	if mode == "" {
		mode = "daily"
	}

	ok, errMsg, modeFields := ValidateModeFields(mode, post)
	if !ok {
		if errMsg == "" {
			errMsg = "Invalid schedule."
		}
		return NormalizeResult{OK: false, Err: errMsg}
	}

	legacy := DeriveLegacyCron(mode, modeFields)

	reportRaw := stringFromPost(post, "report_name")
	if reportRaw == "" {
		reportRaw = stringFromPost(post, "name")
	}
	report := jasper.NormalizeReportKey(reportRaw)
	if !jasper.IsValidReportKey(report) {
		return NormalizeResult{OK: false, Err: "Select a valid Jasper report."}
	}

	var defaultDS string
	if reports != nil {
		ds, err := reports.ReadReport(ctx, report)
		if err != nil {
			return NormalizeResult{OK: false, Err: "Report definition not found."}
		}
		defaultDS = strings.TrimSpace(ds)
	} else {
		return NormalizeResult{OK: false, Err: "Report definition not found."}
	}

	datasource := stringFromPost(post, "datasource_name")
	if datasource == "" {
		datasource = stringFromPost(post, "datasource")
	}
	if datasource == "" {
		datasource = defaultDS
	}
	datasource = strings.TrimSpace(datasource)
	if datasource == "" {
		return NormalizeResult{OK: false, Err: "Datasource is required."}
	}
	if datasources != nil {
		if err := datasources.FindDatasource(ctx, datasource); err != nil {
			return NormalizeResult{OK: false, Err: "Unknown datasource."}
		}
	} else {
		return NormalizeResult{OK: false, Err: "Unknown datasource."}
	}

	format := strings.ToLower(strings.TrimSpace(stringFromPost(post, "format")))
	if format == "" {
		format = "pdf"
	}
	if !isValidOutputFormat(format) {
		return NormalizeResult{OK: false, Err: "Unsupported output format."}
	}

	filename := stringFromPost(post, "filename")
	if filename == "" {
		return NormalizeResult{OK: false, Err: "Filename is required."}
	}

	noemail := boolFromPost(post, "noemail")
	email := stringFromPost(post, "email")
	if !noemail && email == "" {
		return NormalizeResult{OK: false, Err: "Email recipients are required unless email is disabled."}
	}

	row := &Schedule{
		CronPeriod:     legacy.CronPeriod,
		CronCustom:     legacy.CronCustom,
		ScheduleMode:   mode,
		ReportName:     report,
		DatasourceName: datasource,
		Format:         format,
		Filename:       filename,
		Email:          "",
		EmailSubject:   "",
		EmailBody:      "",
		EmailTemplate:  "",
		NoEmail:        noemail,
		URLOptParams:   stringFromPost(post, "url_opt_params"),
		Enabled:        EnabledFromPost(post, mode),
	}

	if !noemail {
		row.Email = email
		subject := stringFromPost(post, "email_subject")
		if subject == "" {
			subject = stringFromPost(post, "email_subj")
		}
		row.EmailSubject = subject
		row.EmailBody = stringFromPost(post, "email_body")
		tmpl := stringFromPost(post, "email_template")
		if tmpl == "" {
			tmpl = stringFromPost(post, "email_tmpl")
		}
		row.EmailTemplate = tmpl
	}

	switch mode {
	case "repeat_minutes":
		n := 10
		if modeFields.RepeatMinutes != nil {
			n = *modeFields.RepeatMinutes
		}
		row.RepeatMinutes = &n
	case "hourly":
		min := 0
		if modeFields.HourlyMinute != nil {
			min = *modeFields.HourlyMinute
		}
		row.HourlyMinute = &min
	case "daily":
		row.DailyTime = modeFields.DailyTime
	case "weekly":
		row.WeeklyDay = modeFields.WeeklyDay
		row.WeeklyTime = modeFields.WeeklyTime
	case "monthly":
		row.MonthlyDay = modeFields.MonthlyDay
		row.MonthlyTime = modeFields.MonthlyTime
	case "once":
		row.OnceDatetime = modeFields.OnceDatetime
	case "custom":
		row.CronCustom = modeFields.CronCustom
	}

	return NormalizeResult{OK: true, Data: row}
}

func boolFromPost(post map[string]any, key string) bool {
	raw, ok := post[key]
	if !ok {
		return false
	}
	s := strings.TrimSpace(fmt.Sprint(raw))
	return s != "" && s != "f" && s != "false" && s != "0"
}
