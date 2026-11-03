package server

import (
	_ "embed"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

//go:embed history.html
var historyHTML string

type historyRowView struct {
	ID           int
	ScheduleID   string
	ReportName   string
	Status       string
	StartedAt    string
	FinishedAt   string
	EmailStatus  string
	OutputFile   string
	ErrorMessage string
}

type historyScheduleOpt struct {
	ID         int
	ReportName string
}

type historyPageData struct {
	materialChromeData
	Error           string
	Rows            []historyRowView
	Schedules       []historyScheduleOpt
	FilterScheduleID int
}

var historyTemplate = template.Must(template.New("history").Funcs(template.FuncMap{
	"selectedSchedule": func(filter, id int) bool {
		return filter > 0 && filter == id
	},
}).Parse(historyHTML))

func (s *Server) handleHistoryPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	filterID, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("schedule_id")))

	data := historyPageData{
		materialChromeData: s.materialChrome("History"),
		FilterScheduleID: filterID,
	}

	if s.deps.Executions == nil {
		data.Error = "Execution history unavailable."
		s.renderHistory(w, data)
		return
	}

	var filter *int
	if filterID > 0 {
		filter = &filterID
	}

	rows, err := s.deps.Executions.List(r.Context(), filter, 200)
	if err != nil {
		data.Error = "Unable to load execution history."
		s.renderHistory(w, data)
		return
	}

	data.Rows = make([]historyRowView, 0, len(rows))
	for _, row := range rows {
		schedID := "—"
		if row.ScheduleID != nil {
			schedID = strconv.Itoa(*row.ScheduleID)
		}
		data.Rows = append(data.Rows, historyRowView{
			ID:           row.ID,
			ScheduleID:   schedID,
			ReportName:   row.ReportName,
			Status:       row.Status,
			StartedAt:    formatScheduleTime(row.StartedAt),
			FinishedAt:   formatScheduleTimePtr(row.FinishedAt),
			EmailStatus:  row.EmailStatus,
			OutputFile:   row.OutputFile,
			ErrorMessage: row.ErrorMessage,
		})
	}

	if s.deps.Schedules != nil {
		if list, err := s.deps.Schedules.List(r.Context()); err == nil {
			data.Schedules = make([]historyScheduleOpt, 0, len(list))
			for _, sched := range list {
				data.Schedules = append(data.Schedules, historyScheduleOpt{
					ID:         sched.ID,
					ReportName: sched.ReportName,
				})
			}
		}
	}

	s.renderHistory(w, data)
}

func (s *Server) renderHistory(w http.ResponseWriter, data historyPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := historyTemplate.Execute(w, data); err != nil {
		s.logger.Error("render history", "error", err)
	}
}
