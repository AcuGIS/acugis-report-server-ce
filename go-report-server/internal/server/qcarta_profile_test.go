package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/modules"
)

func TestQCartaProfileOmitsExcludedRoutes(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Profile = config.ProfileQCarta
	cfg.AuthDisabled = true
	cfg.CentralLoginURL = "/login"
	s := New(cfg, Dependencies{}, slog.Default())
	handler := s.Handler()

	absent := []string{
		"/schedules",
		"/schedules.php",
		"/edit_schedule",
		"/executions",
		"/history",
		"/recipients",
		"/embed/report/demo",
		"/api/v1/embed/token",
		"/sdk/embed.js",
		"/api/v1/map/demo/features",
		"/api/v1/map/demo/extent",
		"/api/v1/map/demo/columns",
		"/explorer",
		"/dashboards",
		"/admin/users",
		"/api/v1/auth/tokens",
		"/hubs/reports",
	}
	for _, path := range absent {
		code, _ := followQCARTAStatus(handler, path)
		if code != http.StatusNotFound {
			t.Errorf("%s: status=%d want 404", path, code)
		}
	}
}

func TestQCartaProfileKeepsCoreRoutes(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Profile = config.ProfileQCarta
	cfg.AuthDisabled = true
	s := New(cfg, Dependencies{}, slog.Default())
	handler := s.Handler()

	present := []string{
		"/health",
		"/workspace",
		"/reports",
		"/reports/publish_report",
		"/reports/datasources",
		"/reports/execute",
		"/reports/edit_report",
		"/reports/delete_report",
		"/api/v1/reports",
		"/api/v1/datasources",
		"/api/discovery",
		"/reports/geoserver_connections",
	}
	// QGIS Server is a Professional module. The OSS build must not require it.
	if modules.Has(modules.IDQGIS) {
		present = append(present, "/reports/qgis_connections")
	}
	for _, path := range present {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s: unexpectedly 404", path)
		}
	}
}

func TestQCartaProfileRedirectsLogin(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Profile = config.ProfileQCarta
	cfg.CentralLoginURL = "https://qcarta.example/login"
	s := New(cfg, Dependencies{}, slog.Default())
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/login?next=/reports/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want 303 body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "https://qcarta.example/login") {
		t.Fatalf("Location=%s", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/login.php?next=/reports/x", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("login.php status=%d want 301", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "/login") {
		t.Fatalf("login.php Location=%s", rec.Header().Get("Location"))
	}
}

func TestQCartaWorkspaceIsDistinctFromCatalog(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Profile = config.ProfileQCarta
	cfg.AuthDisabled = true
	s := New(cfg, Dependencies{}, slog.Default())
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/workspace", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/workspace status=%d want 200", rec.Code)
	}
	ws := rec.Body.String()
	if !strings.Contains(ws, `data-federation-workspace`) {
		t.Fatal("/workspace must be a shell landing so hub cards can render")
	}
	if strings.Contains(ws, "No reports are visible yet.") {
		t.Fatal("/workspace must not render the published-report list")
	}

	req = httptest.NewRequest(http.MethodGet, "/reports", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/reports status=%d want 200", rec.Code)
	}
	list := rec.Body.String()
	if !strings.Contains(list, `data-federation-workspace-static="true"`) {
		t.Fatal("/reports must keep the catalog so the shell does not replace it")
	}
	if !strings.Contains(list, "No reports are visible yet.") && !strings.Contains(list, `class="qcarta-reports-table"`) {
		t.Fatal("/reports must render the published-report list")
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/ status=%d want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `data-federation-workspace`) {
		t.Fatal("/ must be the same shell landing as /workspace")
	}
	if strings.Contains(rec.Body.String(), "No reports are visible yet.") {
		t.Fatal("/ must not render the published-report list")
	}
}

func TestStandaloneProfileStillExposesSchedules(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.AuthDisabled = true
	s := New(cfg, Dependencies{}, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/schedules", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound {
		t.Fatal("standalone must keep /schedules")
	}
}

func followQCARTAStatus(handler http.Handler, path string) (int, string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusFound {
		return rec.Code, rec.Body.String()
	}
	loc := rec.Header().Get("Location")
	if loc == "" {
		return rec.Code, rec.Body.String()
	}
	if !strings.HasPrefix(loc, "http://") && !strings.HasPrefix(loc, "https://") && !strings.HasPrefix(loc, "/") {
		loc = "/" + loc
	}
	req = httptest.NewRequest(http.MethodGet, loc, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}
