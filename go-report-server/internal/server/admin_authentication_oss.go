//go:build oss

package server

import (
	_ "embed"
	"html/template"
	"net/http"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

//go:embed admin_authentication_oss.html
var adminAuthenticationHTML string

type adminAuthPageData struct {
	materialChromeData
	AuthURL   string
	LogoutURL string
}

var adminAuthTemplate = template.Must(template.New("admin_authentication").Parse(adminAuthenticationHTML))

func (s *Server) handleAdminAuthenticationPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	data := adminAuthPageData{
		materialChromeData: s.materialChrome("Authentication"),
		AuthURL:            s.goURL("/admin/authentication"),
		LogoutURL:          s.goURL("/logout"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminAuthTemplate.Execute(w, data); err != nil {
		s.logger.Error("render admin authentication", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
