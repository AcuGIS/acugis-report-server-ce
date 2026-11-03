package server

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/config"
)

// Known federation hubs from hubs/*.php / shell-config.js.
var hubPages = map[string]hubMeta{
	"reports":        {Title: "Reports", RequirePlatformAdmin: false},
	"data-explorer":  {Title: "Ad Hoc", RequirePlatformAdmin: false},
	"data":           {Title: "Data Sources", RequirePlatformAdmin: false},
	"scheduling":     {Title: "Scheduling", RequirePlatformAdmin: true},
	"administration": {Title: "Administration", RequirePlatformAdmin: true},
}

type hubMeta struct {
	Title                string
	RequirePlatformAdmin bool
}

func (s *Server) handleHubPage(w http.ResponseWriter, r *http.Request) {
	hubID := strings.TrimSpace(chi.URLParam(r, "hub"))
	hubID = strings.TrimSuffix(hubID, ".php")
	meta, ok := hubPages[hubID]
	if !ok {
		http.NotFound(w, r)
		return
	}

	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	if meta.RequirePlatformAdmin && !s.isPlatformAdmin(user) {
		s.writeAuthzHTML(w, r, errPermissionDenied)
		return
	}

	mount := s.shellMount()
	data := hubPageData{
		Title:            meta.Title,
		ProductName:      productName(),
		HubID:            hubID,
		BrandHome:        s.goURL("/hubs/reports"),
		ShowSidebarBrand: !s.qcartaProfile(),
		BasePath:         s.mountPath(),
		ShellMount:  jsQuoted(mount),
		PHPMount:    jsQuoted(strings.TrimRight(config.PHPAppMount, "/")),
		AssetsBase:  jsQuoted(s.assetsBase()),
		AssetsURL:   s.assetsBase(),
		CriticalCSS: s.shellCriticalCSS(),
		SidebarNav:  s.renderSidebarNavHTML(hubID),
		HubBody:     s.renderHubBodyHTML(hubID, meta.Title),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := hubTemplate.Execute(w, data); err != nil {
		s.logger.Error("render hub", "error", err, "hub", hubID)
	}
}

func (s *Server) handleWorkspacePage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	mount := s.shellMount()
	data := hubPageData{
		ProductName:      productName(),
		BrandHome:        s.goURL("/hubs/reports"),
		ShowSidebarBrand: !s.qcartaProfile(),
		BasePath:         s.mountPath(),
		ShellMount:  jsQuoted(mount),
		PHPMount:    jsQuoted(strings.TrimRight(config.PHPAppMount, "/")),
		AssetsBase:  jsQuoted(s.assetsBase()),
		AssetsURL:   s.assetsBase(),
		CriticalCSS: s.shellCriticalCSS(),
		SidebarNav:  s.renderSidebarNavHTML(""),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := workspaceTemplate.Execute(w, data); err != nil {
		s.logger.Error("render workspace", "error", err)
	}
}

func (s *Server) isPlatformAdmin(user *auth.User) bool {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return true
	}
	return user != nil && user.IsPlatformAdmin()
}

// requirePlatformAdmin mirrors PHP jrs_require_login + jrs_require_platform_admin.
func (s *Server) requirePlatformAdmin(r *http.Request, user *auth.User) error {
	if err := s.requireCatalogAccess(r, user); err != nil {
		return err
	}
	if !s.isPlatformAdmin(user) {
		return errPermissionDenied
	}
	return nil
}

func (s *Server) goURL(rel string) string {
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	base := s.mountPath()
	if base == "" {
		return rel
	}
	return base + rel
}

func (s *Server) shellMount() string {
	if base := s.mountPath(); base != "" {
		return base
	}
	// Root deploy: shell-config still needs a mount string for urlPrefix.
	return ""
}

func (s *Server) assetsBase() string {
	if s.qcartaProfile() {
		return "/shared/ui"
	}
	if base := s.mountPath(); base != "" {
		return base + "/shared/ui"
	}
	return "/shared/ui"
}

func jsQuoted(s string) template.JS {
	b, err := json.Marshal(s)
	if err != nil {
		return template.JS(`""`)
	}
	return template.JS(b)
}

func productName() string {
	return "AcuGIS Report Server"
}
