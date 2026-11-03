package server

import (
	"bytes"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/xuri/excelize/v2"
)

func newDocumentAdmin(t *testing.T) (*Server, *memDSStore, string) {
	t.Helper()
	appRoot := t.TempDir()
	files := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), "/* shell */")
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.DatasourceFilesDir = files
	store := newMemDSStore()
	s := New(cfg, Dependencies{
		ACL:         acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: true}),
		Datasources: store,
	}, slog.Default())
	return s, store, files
}

func TestAddDatasourceRendersDocumentProviders(t *testing.T) {
	s, _, _ := newDocumentAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/reports/edit_datasource", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`value="postgresql"`, `value="mysql"`, `value="oracle"`, `value="sqlserver"`,
		`value="snowflake"`, `value="sqlite"`, `value="cassandra"`, `value="bigquery"`,
		`value="duckdb"`,
		`value="csv"`, `value="json"`, `value="excel"`, `value="xml"`,
		`id="document_fields"`, `name="delimiter"`, `name="json_source"`, `name="json_url"`,
		`name="sheet"`, `name="collection"`, `name="document"`,
		"PostgreSQL", "MySQL", "Oracle", "Microsoft SQL Server", "Snowflake",
		"SQLite / GeoPackage", "Apache Cassandra", "Google BigQuery", "DuckDB",
		"ArcGIS Feature Server", "GeoServer", "CSV", "JSON", "Excel", "XML",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
	duck := strings.Index(body, `value="duckdb"`)
	csv := strings.Index(body, `value="csv"`)
	xml := strings.Index(body, `value="xml"`)
	if duck < 0 || csv < duck || xml < csv {
		t.Fatal("document providers were not appended after the existing engines")
	}
}

func TestDocumentDatasourceSaveEditAndReject(t *testing.T) {
	s, store, root := newDocumentAdmin(t)

	rec := postDocument(t, s, map[string]string{
		"name": "Orders CSV", "db_type": "csv", "delimiter": "comma",
	}, "orders.csv", []byte("name,city\nAda,Paris\n"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("csv save %d %s", rec.Code, rec.Body.String())
	}
	csvRow := mustDS(t, store, "Orders CSV")
	if csvRow.DBType != "csv" || csvRow.Host != "" || csvRow.Username != "" || csvRow.Port != 0 || csvRow.Option(datasources.OptionDelimiter) != "comma" {
		t.Fatalf("%+v", csvRow)
	}
	if csvRow.DBName != csvRow.Option(datasources.OptionResourceFile) || !strings.HasPrefix(csvRow.DBName, "datasources/") {
		t.Fatal(csvRow.DBName)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(csvRow.DBName))); err != nil {
		t.Fatal(err)
	}

	edit := httptest.NewRequest(http.MethodGet, "/reports/edit_datasource?name="+url.QueryEscape(csvRow.Name)+"&db_type=csv", nil)
	editRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(editRec, edit)
	if editRec.Code != http.StatusOK || !strings.Contains(editRec.Body.String(), `data-doc-filename="orders.csv"`) || !strings.Contains(editRec.Body.String(), `data-doc-delimiter="comma"`) || !strings.Contains(editRec.Body.String(), csvRow.DBName) {
		t.Fatalf("edit page missing saved csv config: %s", editRec.Body.String())
	}
	again := postDocument(t, s, map[string]string{
		"name": "Orders CSV", "db_type": "csv", "delimiter": "comma", "document_resource": csvRow.DBName,
	}, "", nil)
	if again.Code != http.StatusSeeOther {
		t.Fatalf("csv edit %d %s", again.Code, again.Body.String())
	}
	edited := mustDS(t, store, "Orders CSV")
	if edited.DBName != csvRow.DBName || edited.ID != csvRow.ID {
		t.Fatalf("edit replaced the document: %+v", edited)
	}

	rec = postDocument(t, s, map[string]string{
		"name": "Orders JSON", "db_type": "json", "json_source": "file",
	}, "orders.json", []byte(`[{"name":"Ada"}]`))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("json file %d %s", rec.Code, rec.Body.String())
	}
	jsonRow := mustDS(t, store, "Orders JSON")
	if jsonRow.Option(datasources.OptionJSONSource) != "file" || jsonRow.Option(datasources.OptionCollection) == "" || jsonRow.Option(datasources.OptionResourceFile) != jsonRow.DBName {
		t.Fatalf("%+v", jsonRow)
	}

	rec = postDocument(t, s, map[string]string{
		"name": "Remote JSON", "db_type": "json", "json_source": "url",
		"json_url": "https://example.com/orders.json", "collection": "root",
	}, "", nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("json url %d %s", rec.Code, rec.Body.String())
	}
	urlRow := mustDS(t, store, "Remote JSON")
	if urlRow.DBName != "" || urlRow.Option(datasources.OptionJSONSource) != "url" || urlRow.Option(datasources.OptionJSONURL) != "https://example.com/orders.json" || urlRow.Option(datasources.OptionResourceFile) != "" || urlRow.Option(datasources.OptionCollection) != "root" {
		t.Fatalf("%+v", urlRow)
	}

	rec = postDocument(t, s, map[string]string{
		"name": "Orders Excel", "db_type": "excel", "sheet": "Orders",
	}, "orders.xlsx", ordersWorkbook(t))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("excel %d %s", rec.Code, rec.Body.String())
	}
	excelRow := mustDS(t, store, "Orders Excel")
	if excelRow.Option(datasources.OptionSheet) != "Orders" || !strings.HasSuffix(excelRow.Option(datasources.OptionResourceFile), "/orders.xlsx") {
		t.Fatalf("%+v", excelRow)
	}

	rec = postDocument(t, s, map[string]string{
		"name": "Orders XML", "db_type": "xml",
	}, "orders.xml", []byte("<customers><customer><name>Ada</name></customer><customer><name>Grace</name></customer></customers>"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("xml %d %s", rec.Code, rec.Body.String())
	}
	xmlRow := mustDS(t, store, "Orders XML")
	if xmlRow.Option(datasources.OptionCollection) == "" || !strings.HasSuffix(xmlRow.DBName, "/orders.xml") {
		t.Fatalf("%+v", xmlRow)
	}

	rec = postDocument(t, s, map[string]string{"name": "Missing", "db_type": "csv"}, "", nil)
	if rec.Code == http.StatusSeeOther || !strings.Contains(rec.Body.String(), "document file is required") {
		t.Fatalf("missing file status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = postDocument(t, s, map[string]string{"name": "Bad", "db_type": "csv"}, "notes.exe", []byte("name\nAda\n"))
	if rec.Code == http.StatusSeeOther {
		t.Fatal("invalid extension was saved")
	}
	rec = postDocument(t, s, map[string]string{"name": "Broken", "db_type": "csv"}, "broken.csv", []byte("\"unterminated"))
	if rec.Code == http.StatusSeeOther {
		t.Fatal("invalid csv was saved")
	}
	rec = postDocument(t, s, map[string]string{
		"name": "Escape", "db_type": "csv", "document_resource": `../../etc/passwd`,
	}, "", nil)
	if rec.Code == http.StatusSeeOther || !strings.Contains(rec.Body.String(), "not accepted") {
		t.Fatalf("traversal status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = postDocument(t, s, map[string]string{"name": "Escape2", "db_type": "csv"}, `a..b.csv`, []byte("name\nAda\n"))
	if rec.Code == http.StatusSeeOther {
		t.Fatal("traversal filename was saved")
	}
	rec = postDocument(t, s, map[string]string{
		"name": "Loop", "db_type": "json", "json_source": "url", "json_url": "http://127.0.0.1/orders.json",
	}, "", nil)
	if rec.Code == http.StatusSeeOther {
		t.Fatal("loopback JSON URL was saved")
	}
	if _, err := store.Find(t.Context(), "Missing"); err == nil {
		t.Fatal("invalid document was persisted")
	}

	drec := postDocumentTo(t, s, "/api/v1/datasources/document/discover", map[string]string{"db_type": "excel"}, "orders.xlsx", ordersWorkbook(t))
	if drec.Code != http.StatusOK || !strings.Contains(drec.Body.String(), "Orders") {
		t.Fatalf("excel discover %d %s", drec.Code, drec.Body.String())
	}
	xrec := postDocumentTo(t, s, "/api/v1/datasources/document/discover", map[string]string{"db_type": "xml"}, "orders.xml", []byte("<customers><customer><name>Ada</name></customer><customer><name>Grace</name></customer></customers>"))
	if xrec.Code != http.StatusOK || !strings.Contains(xrec.Body.String(), "customer") {
		t.Fatalf("xml discover %d %s", xrec.Code, xrec.Body.String())
	}
}

func postDocument(t *testing.T, s *Server, fields map[string]string, filename string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	return postDocumentTo(t, s, "/reports/edit_datasource", fields, filename, body)
}

func postDocumentTo(t *testing.T, s *Server, path string, fields map[string]string, filename string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if filename != "" {
		part, err := w.CreateFormFile("document", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(part, bytes.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func mustDS(t *testing.T, store *memDSStore, name string) datasources.Datasource {
	t.Helper()
	row, err := store.Find(t.Context(), name)
	if err != nil || row == nil {
		t.Fatalf("%s: %v", name, err)
	}
	return *row
}

func ordersWorkbook(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetName("Sheet1", "Orders")
	_ = f.SetCellValue("Orders", "A1", "Customer")
	_ = f.SetCellValue("Orders", "B1", "Region")
	_ = f.SetCellValue("Orders", "A2", "Ada")
	_ = f.SetCellValue("Orders", "B2", "North")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
