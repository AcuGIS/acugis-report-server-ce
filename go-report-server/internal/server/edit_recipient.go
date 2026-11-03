package server

import (
	_ "embed"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

//go:embed edit_recipient.html
var editRecipientHTML string

type editRecipientPageData struct {
	materialChromeData
	ID        int
	PageTitle string
	Name      string
	Email     string
	Notes     string
	BackURL   string
}

var editRecipientTemplate = template.Must(template.New("edit_recipient").Parse(editRecipientHTML))

func (s *Server) handleEditRecipientPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	id, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
	name, email, notes := "", "", ""
	pageTitle := "Add recipient"

	if id > 0 && s.deps.Recipients != nil {
		pageTitle = "Edit recipient"
		if rec, err := s.deps.Recipients.Get(r.Context(), id); err == nil && rec != nil {
			name = rec.Name
			email = rec.Email
			notes = rec.Notes
		}
	}

	data := editRecipientPageData{
		materialChromeData: s.materialChrome(pageTitle),
		ID:                 id,
		PageTitle:          pageTitle,
		Name:               name,
		Email:              email,
		Notes:              notes,
		BackURL:            s.goURL("/recipients"),
	}

	s.renderEditRecipient(w, data)
}

func (s *Server) renderEditRecipient(w http.ResponseWriter, data editRecipientPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := editRecipientTemplate.Execute(w, data); err != nil {
		s.logger.Error("render edit recipient", "error", err)
	}
}
