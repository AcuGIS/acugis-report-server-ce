package server

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// handleViewPage serves reports/view.php parity at GET /reports/{key}.
func (s *Server) handleViewPage(w http.ResponseWriter, r *http.Request) {
	key := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	s.renderViewPage(w, r, key)
}

// handleViewPagePHP serves GET /reports/view.php?report=… (PHP entry URL).
func (s *Server) handleViewPagePHP(w http.ResponseWriter, r *http.Request) {
	key := jasper.NormalizeReportKey(r.URL.Query().Get("report"))
	if !jasper.IsValidReportKey(key) {
		http.Error(w, "Report not found.", http.StatusNotFound)
		return
	}
	target := s.goURL("/reports/" + key)
	if ds := strings.TrimSpace(r.URL.Query().Get("datasource")); ds != "" {
		target += "?datasource=" + url.QueryEscape(ds)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func mapReportCSP(isMap bool) string {
	if !isMap {
		return ""
	}
	return mapreport.MapReportViewerCSP()
}

func (s *Server) renderViewPage(w http.ResponseWriter, r *http.Request, key string) {
	user := auth.FromContext(r.Context())
	def, err := s.loadAuthorizedReport(r, user, key)
	if err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	if mapreport.IsMapReport(def) {
		if err := s.requireEditionFeature(edition.FeatureMapReports); err != nil {
			s.writeAuthzHTML(w, r, err)
			return
		}
	}
	ds := strings.TrimSpace(r.URL.Query().Get("datasource"))
	if ds == "" {
		ds = def.Datasource()
	}
	isMap := mapreport.IsMapReport(def)
	// Same browser endpoint as report_runner: Go /reports/execute so
	// HTML chart/QR image URLs are rewritten to /report-image (Tomcat
	// /acugis_report_image is not reachable from the browser).
	reportEndpoint := s.browserReportEndpoint()

	chrome := s.materialChrome(def.DisplayName())
	chrome.BrandHome = s.goURL("/reports/")
	data := viewPageData{
		materialChromeData: chrome,
		IsMap:              isMap,
		ReportJS:           mustJSON(def.Key),
		DatasourceJS:       mustJSON(ds),
		ReportEndpointJS:   mustJSON(reportEndpoint),
		IsMapJS:            template.JS(boolJS(isMap)),
		CloseURL:           s.goURL("/reports/"),
		MapReportCSP:       mapReportCSP(isMap),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := viewTemplate.Execute(w, data); err != nil {
		s.logger.Error("render view", "error", err)
		return
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryReport,
		Action:     audit.ActionReportView,
		ObjectType: audit.ObjectReport,
		ObjectID:   key,
		ObjectName: def.DisplayName(),
		Status:     audit.StatusSuccess,
	})
}

// handleReportConfig mirrors reports/get_report_config.php (?report=).
func (s *Server) handleReportConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	key := jasper.NormalizeReportKey(r.URL.Query().Get("report"))
	s.writeReportConfig(w, r, key)
}

// handleAPIReportConfig mirrors GET /api/v1/reports/{key}/config.
func (s *Server) handleAPIReportConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	key := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	s.writeReportConfig(w, r, key)
}

func (s *Server) writeReportConfig(w http.ResponseWriter, r *http.Request, key string) {
	if !jasper.IsValidReportKey(key) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid report"})
		return
	}
	if err := s.enforceEmbedContentScope(r, key); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	user := auth.FromContext(r.Context())
	def, err := s.loadAuthorizedReport(r, user, key)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if mapreport.IsMapReport(def) {
		if err := s.requireEditionFeature(edition.FeatureMapReports); err != nil {
			s.writeAuthzJSON(w, err)
			return
		}
	}

	payload := cloneRawMap(def.Raw)
	if strings.TrimSpace(stringify(payload["report"])) == "" {
		payload["report"] = key
	}
	typ := def.Type()
	payload["type"] = typ
	if typ == "map" {
		payload["reportTypeLabel"] = "Map Report"
	} else {
		payload["reportTypeLabel"] = "Jasper Report"
	}

	// PHP get_report_config.php uses jrs_read_report($report, true) — expand sql: options.
	finder, connector := s.datasourceAccess()
	if finder != nil && connector != nil {
		payload = datasources.ExpandSQLParameterOptions(r.Context(), payload, finder, connector)
	}

	// Merge cascading lookup metadata from JRXML (Designer acugis.parameter.*.lookup.*).
	if lookups, jrxmlParams, lerr := s.loadReportLookups(def); lerr == nil {
		payload = enrichConfigWithJRXMLLookups(payload, lookups, jrxmlParams)
	}

	payload["map"] = s.resolveMapMeta(r, def)
	payload["mapDefinition"] = mapreport.ReportMapDefinition(def)

	w.Header().Set("Cache-Control", "no-store")
	s.writeJSON(w, http.StatusOK, payload)
}

func (s *Server) datasourceAccess() (datasources.Finder, datasources.Connector) {
	finder := s.deps.Datasources
	connector := s.deps.DSConnector
	if finder == nil && s.deps.Map != nil {
		finder = s.deps.Map.Datasources
	}
	if connector == nil && s.deps.Map != nil {
		connector = s.deps.Map.Connector
	}
	return finder, connector
}

func (s *Server) resolveMapMeta(r *http.Request, def *reports.Definition) mapreport.LayerMeta {
	meta := mapreport.LayerMeta{
		Type:         def.Type(),
		ReportFields: []map[string]any{},
	}
	if s.deps.Map == nil {
		return meta
	}
	layer, err := s.deps.Map.ResolveLayer(r.Context(), def)
	if err != nil && layer.Error == "" {
		layer.Error = err.Error()
	}
	if layer.Type == "" {
		layer.Type = def.Type()
	}
	if layer.ReportFields == nil {
		layer.ReportFields = []map[string]any{}
	}
	return layer
}

func mustJSON(v any) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return template.JS("null")
	}
	return template.JS(b)
}

func boolJS(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func cloneRawMap(raw map[string]any) map[string]any {
	if raw == nil {
		return map[string]any{}
	}
	// Shallow clone is enough: viewer only reads top-level keys and nested maps
	// already owned by the store.
	out := make(map[string]any, len(raw)+4)
	for k, v := range raw {
		out[k] = v
	}
	return out
}
