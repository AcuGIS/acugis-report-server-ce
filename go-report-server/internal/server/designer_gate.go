package server

import (
	_ "embed"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

//go:embed designer_gate.html
var designerGateHTML string

var designerGateTemplate = template.Must(template.New("designer_gate").Parse(designerGateHTML))

type designerGateData struct {
	materialChromeData
	SidebarNav      template.HTML
	LogoutURL       string
	ContactAdmin    bool
	Error           string
	ShowStart       bool
	ButtonLabel     string
	TrialDays       int
	ShowExpired     bool
	ShowUsed        bool
	ShowUnavailable bool
	ActivateURL     string
}

func (s *Server) renderDesignerLicenseGate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	data := designerGateData{
		materialChromeData: s.materialChrome("Report Designer Web"),
		SidebarNav:         s.renderSidebarNavHTML("reports"),
		LogoutURL:          s.goURL("/logout"),
		ActivateURL:        s.goURL("/admin/license") + "#report-designer-web",
	}
	if !s.isPlatformAdmin(user) {
		data.ContactAdmin = true
		s.writeDesignerGate(w, http.StatusForbidden, data)
		return
	}
	view := trialView{}
	if s.designerLicense != nil {
		view = composeTrialView(s.designerLicense.Status(), s.designerLicense.TrialOffer(r.Context()), time.Now().UTC())
	}
	if r.Method == http.MethodPost {
		if !sameOriginPost(r) {
			data.Error = "The license request was rejected."
		} else if err := r.ParseForm(); err != nil {
			data.Error = "The license request was rejected."
		} else if strings.TrimSpace(r.FormValue("action")) == "start_trial_designer" {
			if s.designerLicense == nil {
				data.Error = "Report Designer Web is not installed."
			} else if _, err := s.designerLicense.StartTrial(r.Context()); err != nil {
				data.Error = safeDesignerLicenseErr(err)
			} else {
				http.Redirect(w, r, r.URL.RequestURI(), http.StatusSeeOther)
				return
			}
			if s.designerLicense != nil {
				view = composeTrialView(s.designerLicense.Status(), s.designerLicense.TrialOffer(r.Context()), time.Now().UTC())
			}
		}
	}
	data.ShowStart = view.ShowStart
	data.ButtonLabel = view.ButtonLabel
	data.TrialDays = view.TrialDays
	data.ShowExpired = view.ShowExpired
	data.ShowUsed = view.ShowUsed
	data.ShowUnavailable = view.ShowUnavailable
	s.writeDesignerGate(w, http.StatusOK, data)
}

func (s *Server) writeDesignerGate(w http.ResponseWriter, status int, data designerGateData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := designerGateTemplate.Execute(w, data); err != nil && s.logger != nil {
		s.logger.Error("render designer license gate", "error", err)
	}
}
