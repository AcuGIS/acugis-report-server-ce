package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func writePublishedJSONReport(t *testing.T, resource, query, extra string) (*reports.Store, string) {
	t.Helper()
	dir := t.TempDir()
	jrxmlDir := filepath.Join(dir, "jrxml")
	jsonDir := filepath.Join(dir, "json")
	pathsDir := filepath.Join(dir, "resource-paths")
	if err := os.MkdirAll(jrxmlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jsonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(pathsDir, "sales", "data", "customers.json")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `[
	  {"customer":"Ada","region":"North","quantity":4,"amount":19.95,"active":true},
	  {"customer":"Bob","region":"South","quantity":7,"amount":42.5,"active":false}
	]`
	if err := os.WriteFile(docPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="json"/>
  <property name="acugis.datasource.json.source" value="file"/>
  <property name="acugis.datasource.resource" value="` + resource + `"/>
  <property name="acugis.datasource.json.collection" value="root"/>
  ` + extra + `
  <query language="collection"><![CDATA[` + query + `]]></query>
</jasperReport>`
	if err := os.WriteFile(filepath.Join(jrxmlDir, "customers.jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jrxmlDir, "customers.resource-base"), []byte("sales\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{"name":"Customers","report":"customers","datasource":"","type":"jasper","format":"html"}`
	if err := os.WriteFile(filepath.Join(jsonDir, "customers.report.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	return reports.NewStore(jsonDir, "", jrxmlDir), pathsDir
}

func TestPublishedJSONReportExecuteUsesCollectionFill(t *testing.T) {
	store, pathsDir := writePublishedJSONReport(t, "data/customers.json", `SELECT "customer", "region", "quantity", "amount", "active" FROM "data"."root" WHERE "region" = $P{region}`, "")
	tomcat := &fillModeTomcat{}
	jasperSrv := httptest.NewServer(tomcat)
	defer jasperSrv.Close()
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Jasper.ReportURL = jasperSrv.URL
	cfg.Jasper.Timeout = 5 * time.Second
	cfg.Paths.ResourcePathsDir = pathsDir
	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{ReportExists: func(string) bool { return true }})
	s := New(cfg, Dependencies{
		Gateway: gw,
		Reports: store,
		ACL:     authDisabledACL(),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/reports/execute?_repName=customers&_repFormat=html&_dataSource=&region=North", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "collection-fill") {
		t.Fatalf("status %d body %s", rec.Code, body)
	}
	if strings.Contains(body, `"fillMode":"jdbc"`) || strings.Contains(body, "Missing required parameter: _dataSource") {
		t.Fatalf("fell through to JDBC: %s", body)
	}
	_, form := tomcat.snapshot()
	if strings.TrimSpace(form.Get("_dataSource")) != "" {
		t.Fatalf("dataSource %q", form.Get("_dataSource"))
	}
	raw := form.Get("_collectionData")
	if raw == "" {
		t.Fatal("missing _collectionData")
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["customer"] != "Ada" || rows[0]["quantity"] != float64(4) || rows[0]["active"] != true || rows[0]["amount"] != 19.95 {
		t.Fatalf("rows %+v", rows)
	}
}

func TestPublishedJSONURLRejectsLoopbackWithoutJDBC(t *testing.T) {
	store, pathsDir := writePublishedJSONReport(t, "data/customers.json", `SELECT "customer" FROM "data"."root"`, `<property name="acugis.datasource.json.source" value="url"/><property name="acugis.datasource.json.url" value="http://127.0.0.1/latest/meta-data"/>`)
	tomcat := &fillModeTomcat{}
	jasperSrv := httptest.NewServer(tomcat)
	defer jasperSrv.Close()
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Jasper.ReportURL = jasperSrv.URL
	cfg.Jasper.Timeout = 5 * time.Second
	cfg.Paths.ResourcePathsDir = pathsDir
	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{ReportExists: func(string) bool { return true }})
	s := New(cfg, Dependencies{
		Gateway: gw,
		Reports: store,
		ACL:     authDisabledACL(),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/reports/execute?_repName=customers&_repFormat=html&_dataSource=", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "JSON URL is not allowed") || strings.Contains(body, "127.0.0.1") || strings.Contains(body, "meta-data") {
		t.Fatalf("status %d body %s", rec.Code, body)
	}
	if strings.Contains(body, `"fillMode":"jdbc"`) {
		t.Fatalf("jdbc %s", body)
	}
	if _, form := tomcat.snapshot(); form.Get("_collectionData") != "" || form.Get("_dataSource") != "" {
		t.Fatalf("tomcat was called")
	}
}
