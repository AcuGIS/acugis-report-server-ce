package server

import (
	_ "embed"
	"html/template"
	"net/http"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

//go:embed recipients.html
var recipientsHTML string

type recipientRowView struct {
	ID    int
	Name  string
	Email string
	Notes string
}

type recipientsPageData struct {
	materialChromeData
	Error   string
	Rows    []recipientRowView
	EditURL string
}

var recipientsTemplate = template.Must(template.New("recipients").Parse(recipientsHTML))

func (s *Server) handleRecipientsPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	data := recipientsPageData{
		materialChromeData: s.materialChrome("Recipients"),
		EditURL:            s.goURL("/edit_recipient"),
	}

	if s.deps.Recipients == nil {
		data.Error = "Recipients repository unavailable."
		s.renderRecipients(w, data)
		return
	}

	rows, err := s.deps.Recipients.List(r.Context())
	if err != nil {
		data.Error = "Unable to load recipients."
		s.renderRecipients(w, data)
		return
	}

	data.Rows = make([]recipientRowView, 0, len(rows))
	for _, row := range rows {
		data.Rows = append(data.Rows, recipientRowView{
			ID:    row.ID,
			Name:  row.Name,
			Email: row.Email,
			Notes: row.Notes,
		})
	}

	s.renderRecipients(w, data)
}

func (s *Server) renderRecipients(w http.ResponseWriter, data recipientsPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := recipientsTemplate.Execute(w, data); err != nil {
		s.logger.Error("render recipients", "error", err)
	}
}
