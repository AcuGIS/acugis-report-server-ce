package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/arcgis"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// fillModeTomcat is the published-report Jasper endpoint. It returns the same
// JSON Tomcat emits when a run arrives without _collectionData and without
// _dataSource, and HTML only when collection data is present.
type fillModeTomcat struct {
	mu       sync.Mutex
	method   string
	form     url.Values
	rawQuery string
}

func (t *fillModeTomcat) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	t.mu.Lock()
	t.method = r.Method
	t.form = url.Values{}
	for k, vs := range r.Form {
		t.form[k] = append([]string(nil), vs...)
	}
	t.rawQuery = r.URL.RawQuery
	collection := strings.TrimSpace(r.Form.Get("_collectionData"))
	dataSource := strings.TrimSpace(r.Form.Get("_dataSource"))
	t.mu.Unlock()
	if collection != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html>collection-fill "+dataSource+"</html>")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	if dataSource == "" {
		_, _ = io.WriteString(w, `{"fillMode":"jdbc","error":"Missing required parameter: _dataSource"}`)
		return
	}
	_, _ = io.WriteString(w, `{"fillMode":"jdbc","error":"jdbc fill"}`)
}

func (t *fillModeTomcat) snapshot() (string, url.Values) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.method, t.form
}

func writePublishedCollectionReport(t *testing.T, key, provider, connection, query string) *reports.Store {
	t.Helper()
	dir := t.TempDir()
	jrxmlDir := filepath.Join(dir, "jrxml")
	jsonDir := filepath.Join(dir, "json")
	if err := os.MkdirAll(jrxmlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jsonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="` + provider + `"/>
  <property name="acugis.datasource.connection" value="` + connection + `"/>
  <query language="collection"><![CDATA[` + query + `]]></query>
</jasperReport>`
	if err := os.WriteFile(filepath.Join(jrxmlDir, key+".jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	// Catalog datasource is blank, matching a published report whose viewer
	// sends _dataSource= because Add Datasource could not bind the name.
	meta := `{
		"name": "Published",
		"report": "` + key + `",
		"datasource": "",
		"type": "jasper",
		"format": "html"
	}`
	if err := os.WriteFile(filepath.Join(jsonDir, key+".report.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	return reports.NewStore(jsonDir, "", jrxmlDir)
}

func TestPublishedGeoServerReportExecuteUsesCollectionFill(t *testing.T) {
	wfs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.EqualFold(r.URL.Query().Get("request"), "DescribeFeatureType") {
			_, _ = io.WriteString(w, `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"><xsd:complexType><xsd:sequence><xsd:element name="name" type="xsd:string"/></xsd:sequence></xsd:complexType></xsd:schema>`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "FeatureCollection",
			"features": []any{map[string]any{
				"id":         "roads.1",
				"properties": map[string]any{"name": "Main"},
			}},
		})
	}))
	defer wfs.Close()

	tomcat := &fillModeTomcat{}
	jasperSrv := httptest.NewServer(tomcat)
	defer jasperSrv.Close()

	store := writePublishedCollectionReport(t, "roads", "geoserver", "ProdGeoServer", `SELECT "name" FROM "transport"."roads"`)
	cat := &fakeGeoServerCatalog{cred: geoserver.Credentials{Name: "ProdGeoServer", URL: wfs.URL + "/geoserver"}}
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Jasper.ReportURL = jasperSrv.URL
	cfg.Jasper.Timeout = 5 * time.Second
	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{ReportExists: func(string) bool { return true }})
	s := New(cfg, Dependencies{
		Gateway:   gw,
		Reports:   store,
		GeoServer: cat,
		ACL:       authDisabledACL(),
		Auth:      auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/reports/execute?_repName=roads&_dataSource=&_repFormat=html", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, `"fillMode":"jdbc"`) || strings.Contains(body, "Missing required parameter: _dataSource") {
		t.Fatalf("published GeoServer run entered JDBC fill: status=%d body=%s", rec.Code, body)
	}
	if rec.Code != http.StatusOK || !strings.Contains(body, "collection-fill ProdGeoServer") {
		t.Fatalf("status=%d body=%s", rec.Code, body)
	}
	method, form := tomcat.snapshot()
	if method != http.MethodPost {
		t.Fatalf("tomcat method=%s", method)
	}
	if !strings.Contains(form.Get("_collectionData"), `"name":"Main"`) {
		t.Fatalf("collection=%s", form.Get("_collectionData"))
	}
	if form.Get("_dataSource") != "ProdGeoServer" {
		t.Fatalf("datasource sent to tomcat=%q", form.Get("_dataSource"))
	}
}

func TestPublishedArcGISReportExecuteUsesCollectionFill(t *testing.T) {
	features := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/query") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"features": []any{map[string]any{"attributes": map[string]any{"division": "Mtn"}}},
			})
			return
		}
		if strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/0") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "Divisions", "id": 0, "supportsQuery": true, "maxRecordCount": 1000,
				"fields": []any{map[string]any{"name": "division", "type": "esriFieldTypeString"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"layers": []any{map[string]any{"id": 0, "name": "Divisions", "supportsQuery": true}},
		})
	}))
	defer features.Close()

	tomcat := &fillModeTomcat{}
	jasperSrv := httptest.NewServer(tomcat)
	defer jasperSrv.Close()

	store := writePublishedCollectionReport(t, "weather", "arcgis", "WeatherFS", `SELECT "division" FROM "layers"."Divisions"`)
	cat := &fakeArcGISCatalog{cred: arcgis.Credentials{Name: "WeatherFS", URL: features.URL + "/arcgis/rest/services/demo/FeatureServer"}}
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Jasper.ReportURL = jasperSrv.URL
	cfg.Jasper.Timeout = 5 * time.Second
	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{ReportExists: func(string) bool { return true }})
	s := New(cfg, Dependencies{
		Gateway: gw,
		Reports: store,
		ArcGIS:  cat,
		ACL:     authDisabledACL(),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/reports/execute?_repName=weather&_dataSource=&_repFormat=html", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, `"fillMode":"jdbc"`) || strings.Contains(body, "Missing required parameter: _dataSource") {
		t.Fatalf("ArcGIS run entered JDBC fill: status=%d body=%s", rec.Code, body)
	}
	if rec.Code != http.StatusOK || !strings.Contains(body, "collection-fill WeatherFS") {
		t.Fatalf("status=%d body=%s", rec.Code, body)
	}
	_, form := tomcat.snapshot()
	if !strings.Contains(form.Get("_collectionData"), `"division":"Mtn"`) {
		t.Fatalf("collection=%s", form.Get("_collectionData"))
	}
}
