package server

import (
	_ "embed"
	"html/template"
	"net/http"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

//go:embed profile.html
var profileHTML string

type profilePageData struct {
	materialChromeData
	Username   string
	Email      string
	IsAdmin    bool
	Groups     []string
	APIBase    string
	APIBaseJS  template.JS
	LogoutURL  string
	CatalogURL string
}

var profileTemplate = template.Must(template.New("profile").Parse(profileHTML))

func (s *Server) handleProfilePage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		http.Error(w, "Profile is unavailable while authentication is disabled.", http.StatusBadRequest)
		return
	}
	user := auth.FromContext(r.Context())
	if user == nil || !auth.IsAuthenticated(r.Context()) || user.ID <= 0 {
		s.writeAuthzHTML(w, r, errAuthRequired)
		return
	}
	groups := user.Groups
	if groups == nil {
		groups = []string{}
	}
	data := profilePageData{
		materialChromeData: s.materialChrome("Profile"),
		Username:           user.Username,
		Email:              user.Email,
		IsAdmin:            user.IsPlatformAdmin(),
		Groups:             groups,
		APIBase:            s.goURL("/api/v1/auth"),
		APIBaseJS:          jsQuoted(s.goURL("/api/v1/auth")),
		LogoutURL:          s.goURL("/logout"),
		CatalogURL:         s.goURL("/reports/"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := profileTemplate.Execute(w, data); err != nil {
		s.logger.Error("render profile", "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
