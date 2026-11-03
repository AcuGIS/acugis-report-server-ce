package server

import (
	"bytes"
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

const customersXML = `<?xml version="1.0" encoding="UTF-8"?>
<customers>
  <customer id="1001" active="true">
    <customer>Ada</customer>
    <region>North</region>
    <quantity>4</quantity>
  </customer>
  <customer id="1002" active="false">
    <customer>Bob</customer>
    <region>South</region>
    <quantity>7</quantity>
  </customer>
</customers>`

func writePublishedXMLReport(t *testing.T, resource, query string) (*reports.Store, string, string) {
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
	xmlPath := filepath.Join(pathsDir, "sales", "data", "customers.xml")
	if err := os.MkdirAll(filepath.Dir(xmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(xmlPath, []byte(customersXML), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "secret.xml")
	if err := os.WriteFile(secret, []byte("SENTINEL_SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="xml"/>
  <property name="acugis.datasource.resource" value="` + resource + `"/>
  <property name="acugis.datasource.xml.collection" value="/customers/customer"/>
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
	return reports.NewStore(jsonDir, "", jrxmlDir), pathsDir, secret
}

func TestPublishedXMLReportExecuteUsesCollectionFill(t *testing.T) {
	store, pathsDir, secret := writePublishedXMLReport(t, "data/customers.xml", `SELECT "customer", "region", "quantity" FROM "data"."/customers/customer" WHERE "region" = $P{region}`)
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
	if strings.Contains(body, secret) || strings.Contains(body, "SENTINEL_SECRET") {
		t.Fatalf("physical path leaked: %s", body)
	}
	method, form := tomcat.snapshot()
	if method != http.MethodPost {
		t.Fatalf("method %s", method)
	}
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
	if len(rows) != 1 || rows[0]["customer"] != "Ada" || rows[0]["quantity"] != float64(4) {
		t.Fatalf("rows %+v", rows)
	}
}

func TestPublishedXMLReportMissingAndTraversalDoNotUseJDBC(t *testing.T) {
	cases := []struct {
		resource string
		want     string
	}{
		{resource: "data/missing.xml", want: "Resource unavailable: data/missing.xml"},
		{resource: "data/../../secret.xml", want: "invalid resource path"},
	}
	for _, tc := range cases {
		t.Run(tc.resource, func(t *testing.T) {
			store, pathsDir, secret := writePublishedXMLReport(t, tc.resource, `SELECT "customer" FROM "data"."/customers/customer"`)
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
			if !strings.Contains(body, tc.want) {
				t.Fatalf("status %d body %s", rec.Code, body)
			}
			if strings.Contains(body, `"fillMode":"jdbc"`) || strings.Contains(body, "SENTINEL_SECRET") || strings.Contains(body, secret) {
				t.Fatalf("jdbc or secret leaked: %s", body)
			}
			secretBytes, err := os.ReadFile(secret)
			if err != nil || !bytes.Contains(secretBytes, []byte("SENTINEL_SECRET")) {
				t.Fatal("sentinel fixture missing")
			}
			if _, form := tomcat.snapshot(); form.Get("_collectionData") != "" || form.Get("_dataSource") != "" {
				t.Fatalf("tomcat was called with %+v", form)
			}
		})
	}
}
