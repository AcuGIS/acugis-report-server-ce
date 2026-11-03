package server

import (
	_ "embed"
	"html/template"
	"net/http"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

//go:embed admin_embed.html
var adminEmbedHTML string

type adminEmbedPageData struct {
	materialChromeData
	APIBase    template.JS
	SidebarNav template.HTML
}

var adminEmbedTemplate = template.Must(template.New("admin_embed").Parse(adminEmbedHTML))

func (s *Server) handleAdminEmbeddedAnalyticsPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	chrome := s.materialChrome("Embedded Analytics")
	data := adminEmbedPageData{
		materialChromeData: chrome,
		APIBase:            jsQuoted(s.goURL("/api/v1/admin/embed/origins")),
		SidebarNav:         s.renderSidebarNavHTML("administration"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := adminEmbedTemplate.Execute(w, data); err != nil {
		s.logger.Error("render embedded analytics page", "error", err)
	}
}
