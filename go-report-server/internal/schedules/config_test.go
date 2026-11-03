package schedules_test

import (
	"context"
	"errors"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/schedules"
)

type stubReportChecker struct {
	ds  string
	err error
}

func (s stubReportChecker) ReadReport(_ context.Context, _ string) (string, error) {
	return s.ds, s.err
}

type stubDatasourceChecker struct {
	err error
}

func (s stubDatasourceChecker) FindDatasource(_ context.Context, _ string) error {
	return s.err
}

func TestValidateModeFields(t *testing.T) {
	ok, msg, fields := schedules.ValidateModeFields("repeat_minutes", map[string]any{"repeat_minutes": "15"})
	if !ok || msg != "" || fields.RepeatMinutes == nil || *fields.RepeatMinutes != 15 {
		t.Fatalf("repeat_minutes: ok=%v msg=%q fields=%+v", ok, msg, fields)
	}

	ok, msg, _ = schedules.ValidateModeFields("repeat_minutes", map[string]any{"repeat_minutes": "2000"})
	if ok || msg == "" {
		t.Fatalf("expected repeat_minutes range error, got ok=%v msg=%q", ok, msg)
	}

	ok, _, fields = schedules.ValidateModeFields("daily", map[string]any{"daily_time": "08:30"})
	if !ok || fields.DailyTime != "08:30" {
		t.Fatalf("daily: %+v", fields)
	}

	ok, msg, _ = schedules.ValidateModeFields("daily", map[string]any{"daily_time": "99:00"})
	if ok {
		t.Fatal("expected invalid daily time")
	}

	ok, _, fields = schedules.ValidateModeFields("weekly", map[string]any{
		"weekly_day": "Monday", "weekly_time": "09:15",
	})
	if !ok || fields.WeeklyDay != "Monday" || fields.WeeklyTime != "09:15" {
		t.Fatalf("weekly: %+v", fields)
	}

	ok, _, fields = schedules.ValidateModeFields("once", map[string]any{"once_datetime": "2026-07-27T14:30"})
	if !ok || fields.OnceDatetime != "2026-07-27T14:30" {
		t.Fatalf("once: %+v", fields)
	}

	ok, _, fields = schedules.ValidateModeFields("custom", map[string]any{"cron_custom": "0 8 * * *"})
	if !ok || fields.CronCustom != "0 8 * * *" {
		t.Fatalf("custom: %+v", fields)
	}
}

func TestDeriveLegacyCron(t *testing.T) {
	n := 10
	got := schedules.DeriveLegacyCron("repeat_minutes", schedules.ModeFields{RepeatMinutes: &n})
	if got.CronPeriod != "custom" || got.CronCustom != "*/10 * * * *" {
		t.Fatalf("repeat_minutes: %+v", got)
	}

	min := 15
	got = schedules.DeriveLegacyCron("hourly", schedules.ModeFields{HourlyMinute: &min})
	if got.CronPeriod != "custom" || got.CronCustom != "15 * * * *" {
		t.Fatalf("hourly with minute: %+v", got)
	}

	zero := 0
	got = schedules.DeriveLegacyCron("hourly", schedules.ModeFields{HourlyMinute: &zero})
	if got.CronPeriod != "hourly" || got.CronCustom != "" {
		t.Fatalf("hourly on the hour: %+v", got)
	}

	got = schedules.DeriveLegacyCron("daily", schedules.ModeFields{DailyTime: "08:00"})
	if got.CronPeriod != "daily" {
		t.Fatalf("daily: %+v", got)
	}

	got = schedules.DeriveLegacyCron("once", schedules.ModeFields{})
	if got.CronPeriod != "once" {
		t.Fatalf("once: %+v", got)
	}
}

func TestEffectiveMode(t *testing.T) {
	cases := []struct {
		name string
		in   schedules.Schedule
		want string
	}{
		{"schedule_mode", schedules.Schedule{ScheduleMode: "weekly"}, "weekly"},
		{"legacy repeat", schedules.Schedule{CronPeriod: "repeat_minutes"}, "repeat_minutes"},
		{"legacy now", schedules.Schedule{CronPeriod: "now"}, "once"},
		{"never", schedules.Schedule{CronPeriod: "never"}, "never"},
	}
	for _, tc := range cases {
		if got := schedules.EffectiveMode(tc.in); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestSummary(t *testing.T) {
	n := 5
	s := schedules.Schedule{CronPeriod: "custom", ScheduleMode: "repeat_minutes", RepeatMinutes: &n}
	if got := schedules.Summary(s); got != "Runs every 5 minutes" {
		t.Fatalf("summary repeat: %q", got)
	}

	if got := schedules.Summary(schedules.Schedule{CronPeriod: "never"}); got != "Not scheduled" {
		t.Fatalf("summary never: %q", got)
	}

	h := 30
	if got := schedules.Summary(schedules.Schedule{
		CronPeriod: "custom", ScheduleMode: "hourly", HourlyMinute: &h,
	}); got != "Runs hourly at minute 30" {
		t.Fatalf("summary hourly: %q", got)
	}
}

func TestBuildTimerSpec(t *testing.T) {
	n := 10
	spec := schedules.BuildTimerSpec(schedules.Schedule{
		ScheduleMode: "repeat_minutes", RepeatMinutes: &n, CronPeriod: "custom", CronCustom: "*/10 * * * *",
	})
	if spec.Type != "interval" || spec.Interval != "10min" {
		t.Fatalf("interval spec: %+v", spec)
	}

	spec = schedules.BuildTimerSpec(schedules.Schedule{ScheduleMode: "daily", DailyTime: "08:30", CronPeriod: "daily"})
	if spec.Type != "calendar" || spec.Calendar != "*-*-* 08:30:00" {
		t.Fatalf("daily spec: %+v", spec)
	}

	spec = schedules.BuildTimerSpec(schedules.Schedule{CronPeriod: "never"})
	if spec.Type != "none" {
		t.Fatalf("never spec: %+v", spec)
	}
}

func TestCronToSystemd(t *testing.T) {
	if got := schedules.CronToSystemd("*/5 * * * *"); got != "*:0/5" {
		t.Fatalf("every 5 min: %q", got)
	}
	if got := schedules.CronToSystemd("30 8 * * *"); got != "*-*-* 08:30:00" {
		t.Fatalf("fixed hm: %q", got)
	}
}

func TestNormalizePost(t *testing.T) {
	ctx := context.Background()
	reports := stubReportChecker{ds: "main_db"}
	datasources := stubDatasourceChecker{}

	post := map[string]any{
		"schedule_mode":   "daily",
		"daily_time":      "09:00",
		"report_name":     "demo-report",
		"datasource_name": "main_db",
		"format":          "pdf",
		"filename":        "monthly.pdf",
		"email":           "ops@example.com",
		"enabled":         "1",
	}
	res := schedules.NormalizePost(ctx, post, reports, datasources)
	if !res.OK || res.Data == nil {
		t.Fatalf("normalize failed: %+v", res)
	}
	if res.Data.CronPeriod != "daily" || res.Data.ReportName != "demo-report" {
		t.Fatalf("data: %+v", res.Data)
	}
	if res.Data.DailyTime != "09:00" {
		t.Fatalf("daily_time: %q", res.Data.DailyTime)
	}

	post = map[string]any{
		"schedule_mode": "daily",
		"daily_time":    "09:00",
		"report_name":   "!!!",
		"datasource":    "main_db",
		"filename":      "x.pdf",
		"email":         "a@b.com",
	}
	res = schedules.NormalizePost(ctx, post, reports, datasources)
	if res.OK {
		t.Fatal("expected invalid report error")
	}

	post["report_name"] = "demo-report"
	post["datasource_name"] = "missing"
	res = schedules.NormalizePost(ctx, post, reports, stubDatasourceChecker{err: errors.New("missing")})
	if res.OK || res.Err != "Unknown datasource." {
		t.Fatalf("datasource: %+v", res)
	}

	post["datasource_name"] = "main_db"
	post["filename"] = ""
	res = schedules.NormalizePost(ctx, post, reports, datasources)
	if res.OK {
		t.Fatal("expected filename required")
	}

	post["filename"] = "x.pdf"
	delete(post, "email")
	post["noemail"] = "1"
	res = schedules.NormalizePost(ctx, post, reports, datasources)
	if !res.OK || !res.Data.NoEmail {
		t.Fatalf("noemail: %+v", res)
	}
}

func TestEnvLine(t *testing.T) {
	if got := schedules.EnvLine("JRS_EMAIL", "plain@example.com"); got != "JRS_EMAIL=plain@example.com" {
		t.Fatalf("plain: %q", got)
	}
	if got := schedules.EnvLine("JRS_EMAIL_BODY", "hello world"); got != "JRS_EMAIL_BODY='hello world'" {
		t.Fatalf("quoted: %q", got)
	}
}

func TestParseEnvValue(t *testing.T) {
	if got := schedules.ParseEnvValue(`'a'\''b'`); got != "a'b" {
		t.Fatalf("single quoted: %q", got)
	}
	if got := schedules.ParseEnvValue("plain"); got != "plain" {
		t.Fatalf("plain: %q", got)
	}
}

func TestParseURLParams(t *testing.T) {
	params := schedules.ParseURLParams("foo=bar, baz=qux")
	if params["foo"] != "bar" || params["baz"] != "qux" {
		t.Fatalf("pairs: %+v", params)
	}
	params = schedules.ParseURLParams("alpha, beta")
	if params["alpha"] != "" || params["beta"] != "" {
		t.Fatalf("tokens: %+v", params)
	}
}
