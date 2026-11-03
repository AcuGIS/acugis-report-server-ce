package server

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func (s *Server) handleAPIReportsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if s.deps.Reports == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "report store unavailable"})
		return
	}
	list, err := s.deps.Reports.List()
	if err != nil {
		s.logger.Error("list reports", "error", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list reports"})
		return
	}
	filtered := make([]reports.Summary, 0, len(list))
	for _, item := range list {
		if item.Type == "map" && !s.allowMapReports() {
			continue
		}
		ok, err := s.canViewReport(r, user, item.Key)
		if err != nil {
			s.writeAuthzJSON(w, err)
			return
		}
		if ok {
			filtered = append(filtered, item)
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"items": filtered,
		"count": len(filtered),
	})
}

func (s *Server) handleAPIReportGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	key := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	user := auth.FromContext(r.Context())
	def, err := s.loadAuthorizedReport(r, user, key)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, def.Raw)
}

func (s *Server) handleCatalogPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	base := s.mountPath()
	homeURL := base + "/"
	if base == "" {
		homeURL = "/"
	}
	isPlatformAdmin := s.isPlatformAdmin(user)
	isAdminOrEditor, err := s.canEditReportService(r, user)
	if err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	if isPlatformAdmin {
		isAdminOrEditor = true
	}

	username := ""
	if user != nil {
		username = user.Username
	}

	data := catalogPageData{
		ProductName:          productName(),
		BasePath:             base,
		APIBase:              base,
		HomeURL:              homeURL,
		AdminURL:             s.goURL("/hubs/reports"),
		AccountURL:           s.goURL("/profile"),
		LogoutURL:            s.goURL("/logout"),
		PublishURL:           s.goURL("/reports/publish_report"),
		DesignerCreateURL:    s.designerCreateURL(),
		DesignerImportURL:    s.designerImportURL(),
		ExplorerURL:          s.goURL("/explorer/"),
		PortalCSS:            base + "/reports/portal.css?v=20260804a",
		Username:             username,
		IsPlatformAdmin:      isPlatformAdmin,
		IsAdminOrEditor:      isAdminOrEditor,
		AllowRecursiveDelete: s.cfg.FolderAllowRecursiveDelete,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := catalogExplorerTemplate.Execute(w, data); err != nil {
		s.logger.Error("render catalog", "error", err)
	}
}

func (s *Server) buildCatalogCard(r *http.Request, user *auth.User, item reports.Summary, base string, isPlatformAdmin bool) (catalogCard, error) {
	isMap := item.Type == "map"
	typeLabel := "Jasper Report"
	badgeClass := "resource-type-badge--report"
	previewClass := "resource-preview--report"
	previewLabel := "Report Preview"
	desc := item.Description
	if isMap {
		typeLabel = "Map Report"
		badgeClass = "resource-type-badge--map"
		previewClass = "resource-preview--map"
		previewLabel = "Map Preview"
		if desc == "" {
			desc = "Map report"
		}
	} else if desc == "" {
		desc = "Jasper report"
	}

	canEdit := isPlatformAdmin
	canAdmin := isPlatformAdmin
	if !isPlatformAdmin {
		ok, err := s.canEditReport(r, user, item.Key)
		if err != nil {
			return catalogCard{}, err
		}
		canEdit = ok
		ok, err = s.canAdminReport(r, user, item.Key)
		if err != nil {
			return catalogCard{}, err
		}
		canAdmin = ok
	}

	thumbURL := ""
	if item.HasThumbnail {
		thumbURL = base + "/reports/report_thumbnail?report=" + urlQueryEscape(item.Key)
	}
	viewerURL := base + "/reports/" + item.Key
	if item.Datasource != "" {
		viewerURL += "?datasource=" + urlQueryEscape(item.Datasource)
	}
	searchBlob := strings.ToLower(item.Name + " " + desc + " " + typeLabel + " report")

	return catalogCard{
		Name:         item.Name,
		Description:  desc,
		TypeLabel:    typeLabel,
		BadgeClass:   badgeClass,
		PreviewClass: previewClass,
		PreviewLabel: previewLabel,
		ThumbURL:     thumbURL,
		ViewerURL:    viewerURL,
		EditURL:      s.goURL("/reports/edit_report?report=" + urlQueryEscape(item.Key)),
		DeleteURL:    s.goURL("/reports/delete_report?report=" + urlQueryEscape(item.Key)),
		CanEdit:      canEdit,
		CanAdmin:     canAdmin,
		SearchBlob:   searchBlob,
	}, nil
}

func (s *Server) canEditReport(r *http.Request, user *auth.User, key string) (bool, error) {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return true, nil
	}
	if s.deps.ACL == nil {
		return false, errStoreUnavailable
	}
	return s.deps.ACL.CanEditReport(r.Context(), user, key)
}

func (s *Server) canAdminReport(r *http.Request, user *auth.User, key string) (bool, error) {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return true, nil
	}
	if s.deps.ACL == nil {
		return false, errStoreUnavailable
	}
	return s.deps.ACL.CanAdminReport(r.Context(), user, key)
}

func (s *Server) canEditReportService(r *http.Request, user *auth.User) (bool, error) {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return true, nil
	}
	if user != nil && user.IsPlatformAdmin() {
		return true, nil
	}
	if s.deps.ACL == nil {
		return false, errStoreUnavailable
	}
	d, err := s.deps.ACL.CheckPermission(r.Context(), user, acl.ResourceTypeService, s.reportServiceKey(), acl.PermissionEdit)
	if err != nil {
		return false, errStoreUnavailable
	}
	return d.Allowed, nil
}

func urlQueryEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func (s *Server) loadAuthorizedReport(r *http.Request, user *auth.User, key string) (*reports.Definition, error) {
	if !jasper.IsValidReportKey(key) {
		return nil, errReportInvalid
	}
	if s.deps.Reports == nil {
		return nil, errStoreUnavailable
	}
	def, err := s.deps.Reports.Get(key)
	if err != nil {
		if errors.Is(err, reports.ErrNotFound) || errors.Is(err, reports.ErrInvalidKey) {
			return nil, errReportNotFound
		}
		return nil, err
	}
	ok, err := s.canViewReport(r, user, def.Key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errPermissionDenied
	}
	return def, nil
}

func (s *Server) requireCatalogAccess(r *http.Request, user *auth.User) error {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return nil
	}
	if user == nil || !auth.IsAuthenticated(r.Context()) {
		// Allow local-dev synthetic admin attached by middleware.
		if user != nil && user.Username == "local-dev" && user.IsAdmin {
			return nil
		}
		return errAuthRequired
	}
	if s.deps.ACL == nil {
		return errStoreUnavailable
	}
	ok, err := s.deps.ACL.CanAccessReportService(r.Context(), user)
	if err != nil {
		return errStoreUnavailable
	}
	if !ok {
		return errPermissionDenied
	}
	return nil
}

func (s *Server) canViewReport(r *http.Request, user *auth.User, key string) (bool, error) {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return true, nil
	}
	if s.deps.ACL == nil {
		return false, errStoreUnavailable
	}
	return s.deps.ACL.CanViewReport(r.Context(), user, key)
}

type httpError struct {
	Status  int
	Message string
}

func (e *httpError) Error() string { return e.Message }

var (
	errAuthRequired     = &httpError{Status: http.StatusUnauthorized, Message: "authentication required"}
	errPermissionDenied = &httpError{Status: http.StatusForbidden, Message: "Permission denied"}
	errReportNotFound   = &httpError{Status: http.StatusNotFound, Message: "Report not found."}
	errReportInvalid    = &httpError{Status: http.StatusBadRequest, Message: "Invalid report."}
	errStoreUnavailable = &httpError{Status: http.StatusServiceUnavailable, Message: "application database unavailable"}
)

func (s *Server) writeAuthzJSON(w http.ResponseWriter, err error) {
	var he *httpError
	if errors.As(err, &he) {
		s.writeJSON(w, he.Status, map[string]string{"error": he.Message})
		return
	}
	s.logger.Error("authz error", "error", err)
	s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}

func (s *Server) writeAuthzHTML(w http.ResponseWriter, r *http.Request, err error) {
	var he *httpError
	if errors.As(err, &he) {
		if he.Status == http.StatusUnauthorized {
			s.redirectUnauthenticatedBrowser(w, r)
			return
		}
		http.Error(w, he.Message, he.Status)
		return
	}
	s.logger.Error("authz error", "error", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

// redirectUnauthenticatedBrowser mirrors PHP jrs_require_login for HTML pages:
// 303 to /login (via goURL / JRS_BASE_PATH) with next= the requested path.
// API handlers must keep using writeAuthzJSON (401 JSON), not this helper.
func (s *Server) redirectUnauthenticatedBrowser(w http.ResponseWriter, r *http.Request) {
	login := s.loginURL()
	if r != nil && r.URL != nil && s.pathIsLogin(r.URL.Path) {
		http.Error(w, errAuthRequired.Message, http.StatusUnauthorized)
		return
	}
	next := "/"
	if r != nil && r.URL != nil {
		if uri := r.URL.RequestURI(); uri != "" {
			next = uri
		}
	}
	target := login + "?next=" + url.QueryEscape(next)
	if r == nil {
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) pathIsLogin(path string) bool {
	p := strings.TrimSuffix(path, "/")
	for _, cand := range []string{s.goURL("/login"), s.goURL("/login.php")} {
		if p == strings.TrimSuffix(cand, "/") {
			return true
		}
	}
	return false
}

type catalogPageData struct {
	ProductName          string
	BasePath             string
	APIBase              string
	HomeURL              string
	AdminURL             string
	AccountURL           string
	LogoutURL            string
	PublishURL           string
	DesignerCreateURL    string
	DesignerImportURL    string
	ExplorerURL          string
	PortalCSS            string
	Username             string
	IsPlatformAdmin      bool
	IsAdminOrEditor      bool
	AllowRecursiveDelete bool
	Cards                []catalogCard
}

type catalogCard struct {
	Name         string
	Description  string
	TypeLabel    string
	BadgeClass   string
	PreviewClass string
	PreviewLabel string
	ThumbURL     string
	ViewerURL    string
	EditURL      string
	DeleteURL    string
	CanEdit      bool
	CanAdmin     bool
	SearchBlob   string
}

type hubPageData struct {
	Title            string
	ProductName      string
	HubID            string
	BrandHome        string
	ShowSidebarBrand bool
	BasePath         string
	ShellMount       template.JS
	PHPMount         template.JS
	AssetsBase       template.JS
	AssetsURL        string
	CriticalCSS      template.CSS
	SidebarNav       template.HTML
	HubBody          template.HTML
}

var catalogTemplate = template.Must(template.New("catalog").Parse(catalogHTML))
var hubTemplate = template.Must(template.New("hub").Parse(hubHTML))
var workspaceTemplate = template.Must(template.New("workspace").Parse(workspaceHTML))

func stringify(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
