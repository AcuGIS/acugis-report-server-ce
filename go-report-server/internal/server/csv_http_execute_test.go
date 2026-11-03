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

const customersCSV = "customer,region,quantity,amount,active,order_date\n" +
	"Ada,North,2,19.50,true,2024-03-01\n" +
	"Bea,South,3,4.00,false,2024-03-02\n" +
	"\"José, \"\"P\"\"\",North,1,8.25,true,2024-04-15\n" +
	"Cara,North,10,100.00,true,2024-06-01\n"

func writePublishedCSVReport(t *testing.T, resource, query string) (*reports.Store, string, string) {
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
	csvPath := filepath.Join(pathsDir, "sales", "data", "customers.csv")
	if err := os.MkdirAll(filepath.Dir(csvPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(csvPath, []byte(customersCSV), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "secret.csv")
	if err := os.WriteFile(secret, []byte("SENTINEL_SECRET\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="csv"/>
  <property name="acugis.datasource.resource" value="` + resource + `"/>
  <property name="acugis.datasource.delimiter" value="comma"/>
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

func TestPublishedCSVReportExecuteUsesCollectionFill(t *testing.T) {
	store, pathsDir, _ := writePublishedCSVReport(t, "data/customers.csv", `SELECT "customer", "region", "quantity", "amount", "active", "order_date" FROM "data"."customers" WHERE "region" = $P{region}`)
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
	if len(rows) != 3 {
		t.Fatalf("rows %+v", rows)
	}
	var ada map[string]any
	for _, row := range rows {
		if row["customer"] == "Ada" {
			ada = row
		}
		if row["customer"] == "Bea" {
			t.Fatal("South row was not filtered")
		}
	}
	if ada == nil || ada["quantity"] != float64(2) || ada["amount"] != 19.5 || ada["active"] != true || ada["order_date"] != "2024-03-01" {
		t.Fatalf("ada %+v", ada)
	}
	foundJose := false
	for _, row := range rows {
		if name, _ := row["customer"].(string); strings.Contains(name, "José") {
			foundJose = true
		}
	}
	if !foundJose {
		t.Fatalf("utf-8 row missing: %+v", rows)
	}
}

func TestPublishedCSVReportMissingAndTraversalDoNotUseJDBC(t *testing.T) {
	cases := []struct {
		resource string
		want     string
	}{
		{resource: "data/missing.csv", want: "Resource unavailable: data/missing.csv"},
		{resource: "data/../../secret.csv", want: "invalid resource path"},
	}
	for _, tc := range cases {
		t.Run(tc.resource, func(t *testing.T) {
			store, pathsDir, secret := writePublishedCSVReport(t, tc.resource, `SELECT "customer" FROM "data"."customers"`)
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
			if strings.Contains(body, `"fillMode":"jdbc"`) || strings.Contains(body, "SENTINEL_SECRET") {
				t.Fatalf("jdbc or secret leaked: %s", body)
			}
			secretBytes, err := os.ReadFile(secret)
			if err != nil || !strings.Contains(string(secretBytes), "SENTINEL_SECRET") {
				t.Fatal("sentinel fixture missing")
			}
			if _, form := tomcat.snapshot(); form.Get("_collectionData") != "" || form.Get("_dataSource") != "" {
				t.Fatalf("tomcat was called with %+v", form)
			}
		})
	}
}
