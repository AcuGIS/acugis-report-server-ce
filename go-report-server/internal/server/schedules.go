package server

import (
	_ "embed"
	"html/template"
	"net/http"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/schedules"
)

//go:embed schedules.html
var schedulesHTML string

type scheduleRowView struct {
	ID         int
	Summary    string
	ReportName string
	Datasource string
	Format     string
	Filename   string
	Email      string
	NoEmail    bool
	Enabled    bool
}

type schedulesPageData struct {
	materialChromeData
	Error   string
	Rows    []scheduleRowView
	EditURL string
}

var schedulesTemplate = template.Must(template.New("schedules").Parse(schedulesHTML))

func (s *Server) handleSchedulesPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	data := schedulesPageData{
		materialChromeData: s.materialChrome("Schedules"),
		EditURL:            s.goURL("/edit_schedule"),
	}

	repo := s.deps.Schedules
	if repo == nil || !repo.Configured() {
		data.Error = "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables."
		s.renderSchedules(w, data)
		return
	}
	if !repo.TableAvailable(r.Context()) {
		data.Error = "Missing jasper_schedule table. Load database/schema.sql into the application database."
		s.renderSchedules(w, data)
		return
	}

	rows, err := repo.List(r.Context())
	if err != nil {
		data.Error = "Unable to load schedules."
		s.renderSchedules(w, data)
		return
	}

	data.Rows = make([]scheduleRowView, 0, len(rows))
	for _, row := range rows {
		email := row.Email
		if row.NoEmail {
			email = "—"
		}
		data.Rows = append(data.Rows, scheduleRowView{
			ID:         row.ID,
			Summary:    schedules.Summary(row),
			ReportName: row.ReportName,
			Datasource: row.DatasourceName,
			Format:     row.Format,
			Filename:   row.Filename,
			Email:      email,
			NoEmail:    row.NoEmail,
			Enabled:    schedules.IsEnabled(row),
		})
	}

	s.renderSchedules(w, data)
}

func (s *Server) renderSchedules(w http.ResponseWriter, data schedulesPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := schedulesTemplate.Execute(w, data); err != nil {
		s.logger.Error("render schedules", "error", err)
	}
}
