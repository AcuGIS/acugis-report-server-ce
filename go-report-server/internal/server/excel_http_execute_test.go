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
	"github.com/xuri/excelize/v2"
)

func writeOrdersWorkbook(t *testing.T, path string) {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	_, _ = f.NewSheet("Orders")
	_, _ = f.NewSheet("Customers")
	_ = f.SetCellValue("Orders", "A1", "Customer")
	_ = f.SetCellValue("Orders", "B1", "Region")
	_ = f.SetCellValue("Orders", "C1", "Quantity")
	_ = f.SetCellValue("Orders", "A2", "Ada")
	_ = f.SetCellValue("Orders", "B2", "North")
	_ = f.SetCellValue("Orders", "C2", 4)
	_ = f.SetCellValue("Orders", "A3", "Bob")
	_ = f.SetCellValue("Orders", "B3", "South")
	_ = f.SetCellValue("Orders", "C3", 7)
	_ = f.SetCellValue("Customers", "A1", "Customer")
	_ = f.SetCellValue("Customers", "A2", "Ada")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
}

func writePublishedExcelReport(t *testing.T, resource, query string) (*reports.Store, string, string) {
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
	xlsxPath := filepath.Join(pathsDir, "sales", "data", "sales.xlsx")
	if err := os.MkdirAll(filepath.Dir(xlsxPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeOrdersWorkbook(t, xlsxPath)
	secret := filepath.Join(dir, "secret.xlsx")
	if err := os.WriteFile(secret, []byte("SENTINEL_SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="excel"/>
  <property name="acugis.datasource.resource" value="` + resource + `"/>
  <property name="acugis.datasource.excel.sheet" value="Orders"/>
  <query language="collection"><![CDATA[` + query + `]]></query>
</jasperReport>`
	if err := os.WriteFile(filepath.Join(jrxmlDir, "orders.jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jrxmlDir, "orders.resource-base"), []byte("sales\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{"name":"Orders","report":"orders","datasource":"","type":"jasper","format":"html"}`
	if err := os.WriteFile(filepath.Join(jsonDir, "orders.report.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	return reports.NewStore(jsonDir, "", jrxmlDir), pathsDir, secret
}

func TestPublishedExcelReportExecuteUsesCollectionFill(t *testing.T) {
	store, pathsDir, secret := writePublishedExcelReport(t, "data/sales.xlsx", `SELECT "Customer", "Region", "Quantity" FROM "data"."Orders" WHERE "Region" = $P{region}`)
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

	req := httptest.NewRequest(http.MethodPost, "/reports/execute?_repName=orders&_repFormat=html&_dataSource=&region=North", nil)
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
	if len(rows) != 1 || rows[0]["Customer"] != "Ada" || rows[0]["Quantity"] != float64(4) {
		t.Fatalf("rows %+v", rows)
	}
}

func TestPublishedExcelReportMissingAndTraversalDoNotUseJDBC(t *testing.T) {
	cases := []struct {
		resource string
		want     string
	}{
		{resource: "data/missing.xlsx", want: "Resource unavailable: data/missing.xlsx"},
		{resource: "data/../../secret.xlsx", want: "invalid resource path"},
	}
	for _, tc := range cases {
		t.Run(tc.resource, func(t *testing.T) {
			store, pathsDir, secret := writePublishedExcelReport(t, tc.resource, `SELECT "Customer" FROM "data"."Orders"`)
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
			req := httptest.NewRequest(http.MethodPost, "/reports/execute?_repName=orders&_repFormat=html&_dataSource=", nil)
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
