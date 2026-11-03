package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/internal/schedules"
)

type scheduleReportChecker struct {
	store *reports.Store
}

func (c scheduleReportChecker) ReadReport(_ context.Context, key string) (string, error) {
	if c.store == nil {
		return "", reports.ErrNotFound
	}
	def, err := c.store.Get(key)
	if err != nil {
		return "", err
	}
	if def == nil || def.Raw == nil {
		return "", reports.ErrNotFound
	}
	if v, ok := def.Raw["datasource"].(string); ok {
		return strings.TrimSpace(v), nil
	}
	return "", nil
}

type scheduleDatasourceChecker struct {
	finder datasources.Finder
}

func (c scheduleDatasourceChecker) FindDatasource(ctx context.Context, name string) error {
	if c.finder == nil {
		return fmt.Errorf("datasource unavailable")
	}
	_, err := c.finder.Find(ctx, name)
	return err
}

func (s *Server) scheduleCheckers() (schedules.ReportChecker, schedules.DatasourceChecker) {
	return scheduleReportChecker{store: s.deps.Reports}, scheduleDatasourceChecker{finder: s.deps.Datasources}
}

func formToMap(r *http.Request) map[string]any {
	m := make(map[string]any)
	for k, vals := range r.PostForm {
		if len(vals) == 1 {
			m[k] = vals[0]
		} else {
			m[k] = vals
		}
	}
	return m
}

// parsePostedForm parses urlencoded or multipart POST bodies (PHP $_POST parity).
// Browser FormData submissions are multipart; ParseForm alone ignores those fields.
func parsePostedForm(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "multipart/form-data") {
		return r.ParseMultipartForm(32 << 20) // 32 MiB
	}
	return r.ParseForm()
}

func scheduleEnabledForForm(sched schedules.Schedule, editID int) bool {
	enabled := schedules.IsEnabled(sched)
	if editID > 0 && !enabled {
		cron := strings.ToLower(strings.TrimSpace(sched.CronPeriod))
		if cron != "never" && cron != "now" {
			enabled = true
		}
	}
	return enabled
}

func intFromMap(m map[string]any, key string, fallback int) int {
	if m == nil {
		return fallback
	}
	v, ok := m[key]
	if !ok {
		return fallback
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			return fallback
		}
		parsed, err := strconv.Atoi(s)
		if err != nil {
			return fallback
		}
		return parsed
	}
}

func strFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func (s *Server) listDatasourceNamesForSchedule(r *http.Request) []string {
	return s.listDatasourceNames(r)
}

func (s *Server) runScheduleAsync(scheduleID int) {
	if s.deps.ScheduleRun == nil || scheduleID <= 0 {
		return
	}
	timeout := s.cfg.Jasper.Timeout + 30*time.Second
	if timeout < 90*time.Second {
		timeout = 90 * time.Second
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_ = s.deps.ScheduleRun.Execute(ctx, scheduleID, "manual")
	}()
}

func applyNoEmailFromPost(data *schedules.Schedule, post map[string]any) {
	noemail := schedules.BoolValue(post["noemail"], false)
	if !noemail {
		return
	}
	data.NoEmail = true
	data.Email = ""
	data.EmailSubject = ""
	data.EmailBody = ""
	data.EmailTemplate = ""
}

func scheduleActionError(err error) string {
	switch {
	case errors.Is(err, schedules.ErrNotFound):
		return "No such schedule."
	case errors.Is(err, schedules.ErrTableMissing):
		return "Schedule table missing. Load database/schema.sql."
	case errors.Is(err, schedules.ErrUnavailable):
		return "Application database unavailable."
	default:
		return "Error while processing your request."
	}
}

func formatScheduleTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04:05")
}

func formatScheduleTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04:05")
}

var scheduleModeLabels = map[string]string{
	"once":           "Once",
	"repeat_minutes": "Every N Minutes",
	"hourly":         "Hourly",
	"daily":          "Daily",
	"weekly":         "Weekly",
	"monthly":        "Monthly",
	"custom":         "Custom (Advanced)",
}
