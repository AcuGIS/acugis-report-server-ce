package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/modules"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestCatalogPageAndAPI(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "demo.report.json"), `{
		"name": "Demo",
		"report": "demo",
		"datasource": "DS",
		"description": "A demo",
		"type": "jasper",
		"format": "html",
		"parameters": [{"name": "p_x", "label": "X", "type": "text"}]
	}`)

	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "reports", "portal.css"), `.portal-home{color:#26a69a}`)
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell.css"), `/* css */`)

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.ReportJSONDir = dir
	cfg.Paths.ThumbnailDir = filepath.Join(appRoot, "assets", "reports")
	store := reports.NewStore(dir, cfg.Paths.ThumbnailDir, "")
	s := New(cfg, Dependencies{
		Reports: store,
		ACL:     acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`class="portal-home"`,
		`fe-layout`,
		`fe-new-menu`,
		`data-new="resource"`,
		`/reports/portal.css`,
		`/reports/publish_report`,
		`AcuGIS Report Server`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "var(--bg): #0f1419") || strings.Contains(body, "a.card") {
		t.Fatal("catalog still using temporary dark shell markup")
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/portal.css", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("portal.css status=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Fatalf("portal.css Content-Type=%q", ct)
	}
	if !strings.Contains(rec.Body.String(), "portal-home") {
		t.Fatalf("portal.css body=%s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/reports", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("api status=%d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if int(payload["count"].(float64)) != 1 {
		t.Fatalf("payload=%v", payload)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/demo", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("view status=%d body=%s", rec.Code, rec.Body.String())
	}
	viewBody := rec.Body.String()
	for _, want := range []string{
		`Report Viewer`,
		`id="paramSidebar"`,
		`/reports/execute`,
		`get_report_config?report=`,
		`Options`,
		`Export PDF`,
		`rv-btn-zoom-in`,
		`rv-btn-zoom-out`,
		`Fit Width`,
	} {
		if !strings.Contains(viewBody, want) {
			t.Fatalf("view missing %q: %s", want, viewBody)
		}
	}
	if strings.Contains(viewBody, "var(--bg)") || strings.Contains(viewBody, "← Reports") {
		t.Fatal("view still using temporary thin shell markup")
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/get_report_config?report=demo", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status=%d body=%s", rec.Code, rec.Body.String())
	}
	var cfgPayload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cfgPayload); err != nil {
		t.Fatal(err)
	}
	if cfgPayload["report"] != "demo" || cfgPayload["reportTypeLabel"] != "Jasper Report" {
		t.Fatalf("config=%v", cfgPayload)
	}
	if _, ok := cfgPayload["map"]; !ok {
		t.Fatalf("config missing map: %v", cfgPayload)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/reports/demo", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("def status=%d", rec.Code)
	}
}

func TestPortalCSSNotSwallowedByReportViewer(t *testing.T) {
	dir := t.TempDir()
	appRoot := t.TempDir()
	// AppRoot exists but portal.css is intentionally absent.
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell.css"), `/* css */`)

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.ReportJSONDir = dir
	s := New(cfg, Dependencies{
		Reports: reports.NewStore(dir, "", ""),
		ACL:     acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/reports/portal.css", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Report not found") {
		t.Fatalf("portal.css fell through to report viewer: %s", rec.Body.String())
	}
}

func TestCatalogRequiresAuthWhenEnabled(t *testing.T) {
	cfg := testConfig()
	cfg.AuthDisabled = false
	s := New(cfg, Dependencies{
		Reports: reports.NewStore(t.TempDir(), "", ""),
		ACL:     denyACL{},
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: false}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/login?next=%2F" {
		t.Fatalf("Location=%q", loc)
	}
}

func TestHubsChromeAndSharedUI(t *testing.T) {
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* bootstrap */`)
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell.css"), `.fed-shell-sidebar{}`)
	mustWrite(t, filepath.Join(appRoot, "reports", "portal.css"), `.portal-home{}`)

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	s := New(cfg, Dependencies{
		Reports: reports.NewStore(t.TempDir(), "", ""),
		ACL:     acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/hubs/reports", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("hub status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-federation-hub="reports"`,
		`fed-shell-sidebar`,
		`/shared/ui/shell-bootstrap.js`,
		`fed-shell-main`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("hub body missing %q: %s", want, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/shared/ui/shell.css", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("shell.css status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "fed-shell-sidebar") {
		t.Fatalf("shell.css body=%s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/hubs/not-a-hub", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown hub status=%d", rec.Code)
	}
}

func TestBasePathGoMountLinks(t *testing.T) {
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* bootstrap */`)
	mustWrite(t, filepath.Join(appRoot, "reports", "portal.css"), `.portal-home{}`)
	mustWrite(t, filepath.Join(appRoot, "assets", "js", "map-style.js"), `window.JrsMapStyle={};`)
	mustWrite(t, filepath.Join(appRoot, "assets", "js", "spatial-selection.js"), `window.JrsSpatialSelection={};`)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "demo.report.json"), `{
		"name": "Demo", "report": "demo", "datasource": "DS", "type": "jasper"
	}`)
	mustWrite(t, filepath.Join(dir, "mapdemo.report.json"), `{
		"name": "Map Demo", "report": "mapdemo", "datasource": "DS", "type": "map",
		"mapDefinition": {"enabled": true, "basemap": "osm", "geometryColumn": "geom"}
	}`)

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.BasePath = "/go"
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.ReportJSONDir = dir
	s := New(cfg, Dependencies{
		Reports: reports.NewStore(dir, "", ""),
		ACL:     acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/go/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="/go/hubs/reports"`,
		`/go/reports/portal.css`,
		`/go/reports/publish_report`,
		`data-api-base="/go"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("catalog missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `href="/acugis-report-server/hubs/`) {
		t.Fatal("Administration still points at PHP hubs mount")
	}

	req = httptest.NewRequest(http.MethodGet, "/go/hubs/reports.php", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("hub.php status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/go/hubs/reports" {
		t.Fatalf("hub.php Location=%s", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/hubs/reports", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("hub.php status=%d body=%s", rec.Code, rec.Body.String())
	}
	hubBody := rec.Body.String()
	for _, want := range []string{
		`FEDERATION_SHELL_MOUNT = "/go"`,
		`FEDERATION_SHELL_ASSETS = "/go/shared/ui"`,
		`/go/shared/ui/shell-bootstrap.js`,
		`data-federation-hub="reports"`,
	} {
		if !strings.Contains(hubBody, want) {
			t.Fatalf("hub missing %q: %s", want, hubBody)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/go/shared/ui/shell-bootstrap.js", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mounted shared/ui status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/reports/index.php", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("reports/index.php alias status=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/go/reports/" {
		t.Fatalf("reports/index.php Location=%s", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/reports/", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reports/ alias status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/reports/mapdemo", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if modules.Has(modules.IDMapReports) {
		if rec.Code != http.StatusOK {
			t.Fatalf("map view status=%d", rec.Code)
		}
		mapBody := rec.Body.String()
		for _, want := range []string{
			`Map Report Viewer`,
			`../assets/js/map-style.js`,
			`../assets/js/spatial-selection.js`,
			`map_features?report=`,
			`map_extent?report=`,
			`spatialToolbar`,
			`RefreshReport`,
			`id="splitBody"`,
		} {
			if !strings.Contains(mapBody, want) {
				t.Fatalf("map view missing %q", want)
			}
		}
	} else if rec.Code != http.StatusForbidden {
		t.Fatalf("OSS map view status=%d want 403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/assets/js/map-style.js", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("map-style.js status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/reports/view.php?report=demo&datasource=DS", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("view.php redirect status=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/go/reports/view?report=demo&datasource=DS" {
		t.Fatalf("view.php Location=%s", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/reports/view?report=demo&datasource=DS", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("view redirect status=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "/go/reports/demo") {
		t.Fatalf("view Location=%s", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/reports/execute_report.php", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("execute_report.php status=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/go/reports/execute" {
		t.Fatalf("execute_report.php Location=%s", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/reports/execute", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	// Without Jasper gateway this still hits the handler (not 404).
	if rec.Code == http.StatusNotFound {
		t.Fatal("execute alias not registered")
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
