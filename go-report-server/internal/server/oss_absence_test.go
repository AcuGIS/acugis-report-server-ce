//go:build oss

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/modules"
)

func TestOSSOmitsProfessionalRoutes(t *testing.T) {
	t.Parallel()
	if modules.Has(modules.IDAdHoc) || modules.Has(modules.IDDashboards) || modules.Has(modules.IDMapReports) ||
		modules.Has(modules.IDQGIS) || modules.Has(modules.IDSSO) || modules.Has(modules.IDAudit) {
		t.Fatal("OSS binary unexpectedly registered Professional modules")
	}

	cfg := testConfig()
	cfg.Edition = "professional"
	cfg.AuthDisabled = true
	cfg.MCP.Enabled = true
	s := New(cfg, Dependencies{}, slog.Default())
	handler := s.Handler()

	absent := []string{
		"/dashboards",
		"/dashboards/",
		"/explorer",
		"/explorer/",
		"/dataset-intelligence/",
		"/admin/audit",
		"/admin/usage",
		"/admin/governance",
		"/admin/audit-export",
		"/auth/oidc/login",
		"/auth/saml/login",
		"/qgis/",
		"/reports/qgis_connections",
		"/reports/arcgis_connections",
		"/api/v1/map/demo/features",
		"/api/v1/explorer/query",
		"/api/v1/mcp",
		"/api/v1/explorer/dashboards",
		"/embed/map-report/demo",
		"/embed/dashboard/alice/board",
		"/embed/view/alice/v1",
	}
	for _, path := range absent {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status=%d want 404 (absent from OSS binary); body=%s", path, rec.Code, rec.Body.String())
		}
		if path == "/api/v1/mcp" && strings.Contains(rec.Body.String(), "query_dataset") {
			t.Errorf("OSS MCP response advertised query_dataset: %s", rec.Body.String())
		}
	}
}

func TestOSSKeepsCoreRoutes(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.AuthDisabled = true
	s := New(cfg, Dependencies{}, slog.Default())
	handler := s.Handler()

	present := []string{
		"/health",
		"/api/v1/edition",
		"/api/openapi.yaml",
		"/reports/geoserver_connections",
		"/schedules",
		"/embed/report/demo",
	}
	for _, path := range present {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s: unexpectedly 404 in OSS build", path)
		}
	}
}

func TestOSSProfessionalEditionEnvDoesNotExposeModules(t *testing.T) {
	t.Setenv("ACUGIS_EDITION", "professional")
	cfg := testConfig()
	cfg.Edition = "professional"
	cfg.AuthDisabled = true
	cfg.MCP.Enabled = true
	s := New(cfg, Dependencies{}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/edition", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	caps, _ := body["capabilities"].(map[string]any)
	features, _ := caps["features"].(map[string]any)
	maps, _ := caps["maps"].(map[string]any)
	embedded, _ := maps["embedded"].(map[string]any)
	if features["dashboards"] != false || features["mapReports"] != false || features["dataExplorer"] != false ||
		features["dataIntelligence"] != false || features["audit"] != false || features["ssoOidc"] != false || features["ssoSaml"] != false {
		t.Fatalf("ACUGIS_EDITION=professional on OSS must not enable Pro features: %+v", features)
	}
	if embedded["qgis"] != false {
		t.Fatalf("QGIS must stay false on OSS: %+v", embedded)
	}
	if embedded["postgis"] != true || embedded["mysql"] != true || embedded["geoserver"] != true {
		t.Fatalf("embedded PostGIS/MySQL/GeoServer must remain available: %+v", embedded)
	}

	for _, path := range []string{"/dashboards/", "/explorer", "/auth/oidc/login", "/api/v1/map/x/features", "/api/v1/mcp"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("env professional still exposed %s status=%d", path, rec.Code)
		}
	}
}

func TestOSSOpenAPIOmitsProfessionalPaths(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	s := New(cfg, Dependencies{}, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("openapi status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, needle := range []string{"/api/v1/map/{key}/features", "/api/v1/explorer/query", "/api/v1/explorer/dashboards"} {
		if strings.Contains(body, needle) {
			t.Errorf("OSS OpenAPI still contains %s", needle)
		}
	}
}
