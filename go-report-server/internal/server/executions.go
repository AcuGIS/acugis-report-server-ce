package server

import (
	_ "embed"
	"html/template"
	"net/http"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/schedules"
)

//go:embed executions.html
var executionsHTML string

type executionRowView struct {
	ID           int
	ReportName   string
	Summary      string
	Enabled      bool
	LastStatus   string
	LastStarted  string
}

type executionsPageData struct {
	materialChromeData
	Error     string
	Rows      []executionRowView
	HistoryURL string
}

var executionsTemplate = template.Must(template.New("executions").Parse(executionsHTML))

func (s *Server) handleExecutionsPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	data := executionsPageData{
		materialChromeData: s.materialChrome("Executions"),
		HistoryURL:         s.goURL("/history"),
	}

	repo := s.deps.Schedules
	if repo == nil || !repo.Configured() {
		data.Error = "Unable to connect to the application PostgreSQL database."
		s.renderExecutions(w, data)
		return
	}

	schedulesList, err := repo.List(r.Context())
	if err != nil {
		data.Error = "Unable to load schedules."
		s.renderExecutions(w, data)
		return
	}

	data.Rows = make([]executionRowView, 0, len(schedulesList))
	for _, sched := range schedulesList {
		row := executionRowView{
			ID:         sched.ID,
			ReportName: sched.ReportName,
			Summary:    schedules.Summary(sched),
			Enabled:    schedules.IsEnabled(sched),
			LastStatus: "—",
			LastStarted: "—",
		}
		if s.deps.Executions != nil {
			if latest, err := s.deps.Executions.LatestForSchedule(r.Context(), sched.ID); err == nil && latest != nil {
				row.LastStatus = latest.Status
				row.LastStarted = formatScheduleTime(latest.StartedAt)
			}
		}
		data.Rows = append(data.Rows, row)
	}

	s.renderExecutions(w, data)
}

func (s *Server) renderExecutions(w http.ResponseWriter, data executionsPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := executionsTemplate.Execute(w, data); err != nil {
		s.logger.Error("render executions", "error", err)
	}
}
