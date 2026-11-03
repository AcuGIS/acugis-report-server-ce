package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

type fakeGeoServerCatalog struct {
	cred  geoserver.Credentials
	rows  []geoserver.Connection
	asked string
	calls int
}

func (f *fakeGeoServerCatalog) ListEnabled(context.Context) ([]geoserver.Connection, error) {
	return f.rows, nil
}
func (f *fakeGeoServerCatalog) FindGeoServer(_ context.Context, name string) (geoserver.Credentials, error) {
	f.calls++
	f.asked = name
	if name != f.cred.Name {
		return geoserver.Credentials{}, geoserver.ErrNotFound
	}
	return f.cred, nil
}
func (f *fakeGeoServerCatalog) Upsert(context.Context, geoserver.Connection) error { return nil }
func (f *fakeGeoServerCatalog) DeleteByID(context.Context, int64) error            { return nil }
func (f *fakeGeoServerCatalog) TableAvailable(context.Context) bool                { return true }

func writeGeoReport(t *testing.T, provider, query string) (*reports.Store, string) {
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
  <property name="acugis.datasource.connection" value="ProdGeoServer"/>
  <parameter name="classes" class="java.util.Collection"/>
  <query language="collection"><![CDATA[` + query + `]]></query>
</jasperReport>`
	if err := os.WriteFile(filepath.Join(jrxmlDir, "roads.jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{
		"name": "Roads",
		"report": "roads",
		"datasource": "ProdGeoServer",
		"type": "jasper",
		"format": "html",
		"parameters": [{"name": "classes", "label": "Class", "type": "select", "multiple": true}]
	}`
	if err := os.WriteFile(filepath.Join(jsonDir, "roads.report.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	return reports.NewStore(jsonDir, "", jrxmlDir), "roads"
}

func TestPrepareGeoServerCollectionFill(t *testing.T) {
	var filters []string
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawAuth = true
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.EqualFold(r.URL.Query().Get("request"), "DescribeFeatureType") {
			_, _ = io.WriteString(w, `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"><xsd:complexType><xsd:sequence><xsd:element name="name" type="xsd:string"/><xsd:element name="lanes" type="xsd:int"/></xsd:sequence></xsd:complexType></xsd:schema>`)
			return
		}
		filters = append(filters, r.URL.Query().Get("CQL_FILTER"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "FeatureCollection",
			"features": []any{map[string]any{
				"id":         "roads.1",
				"properties": map[string]any{"name": "Main", "lanes": 4},
			}},
		})
	}))
	defer srv.Close()

	store, name := writeGeoReport(t, "geoserver", `SELECT "name", "lanes" FROM "transport"."roads" WHERE $X{IN, "class", classes}`)
	cat := &fakeGeoServerCatalog{cred: geoserver.Credentials{Name: "ProdGeoServer", URL: srv.URL + "/geoserver"}}
	s := &Server{
		deps:   Dependencies{Reports: store, GeoServer: cat},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ok, method, _, ct, body, err := s.prepareGeoServerCollectionFill(context.Background(), name, "classes=highway&classes=local&_repFormat=pdf")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if method != http.MethodPost || !strings.Contains(ct, "application/x-www-form-urlencoded") {
		t.Fatalf("method=%s ct=%s", method, ct)
	}
	raw, _ := io.ReadAll(body)
	vals, err := url.ParseQuery(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	payload := vals.Get("_collectionData")
	if !strings.Contains(payload, `"name":"Main"`) || !strings.Contains(payload, `"lanes":4`) {
		t.Fatalf("collection %s", payload)
	}
	if vals.Get("_repFormat") != "pdf" {
		t.Fatalf("format=%q", vals.Get("_repFormat"))
	}
	if cat.asked != "ProdGeoServer" {
		t.Fatalf("resolved %q", cat.asked)
	}
	if sawAuth {
		t.Fatal("anonymous connection sent Authorization")
	}
	if len(filters) != 1 || !strings.Contains(filters[0], "'highway'") || !strings.Contains(filters[0], "'local'") {
		t.Fatalf("cql=%v", filters)
	}
}

func TestPrepareGeoServerSkipsOtherProviders(t *testing.T) {
	for _, provider := range []string{"arcgis", "cassandra", "bigquery"} {
		t.Run(provider, func(t *testing.T) {
			store, name := writeGeoReport(t, provider, `SELECT "name" FROM "transport"."roads"`)
			cat := &fakeGeoServerCatalog{}
			s := &Server{deps: Dependencies{Reports: store, GeoServer: cat}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			ok, _, _, _, _, err := s.prepareGeoServerCollectionFill(context.Background(), name, "")
			if ok || err != nil {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
			if cat.calls != 0 {
				t.Fatal("geoserver catalog was queried")
			}
		})
	}
}

func TestPrepareGeoServerMissingConnection(t *testing.T) {
	store, name := writeGeoReport(t, "geoserver", `SELECT "name" FROM "transport"."roads"`)
	cat := &fakeGeoServerCatalog{cred: geoserver.Credentials{Name: "Other", URL: "http://127.0.0.1/geoserver"}}
	s := &Server{deps: Dependencies{Reports: store, GeoServer: cat}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ok, _, _, _, _, err := s.prepareGeoServerCollectionFill(context.Background(), name, "")
	if !ok || err == nil || !strings.Contains(err.Error(), "ProdGeoServer") {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestGeoServerPublishDatasourceValidation(t *testing.T) {
	jrxml := []byte(`<jasperReport>
  <property name="acugis.datasource.provider" value="geoserver"/>
  <property name="acugis.datasource.connection" value="ProdGeoServer"/>
  <query language="collection"><![CDATA[SELECT "name" FROM "transport"."roads"]]></query>
</jasperReport>`)
	cat := &fakeGeoServerCatalog{
		cred: geoserver.Credentials{Name: "ProdGeoServer", URL: "http://127.0.0.1/geoserver"},
		rows: []geoserver.Connection{{Name: "ProdGeoServer"}},
	}
	s := &Server{deps: Dependencies{GeoServer: cat}}
	req := httptest.NewRequest(http.MethodPost, "/reports", nil)
	msg, note := s.geoServerPublishDatasourceError(req, jrxml, "ProdGeoServer")
	if msg != "" || note == "" {
		t.Fatalf("msg=%q note=%q", msg, note)
	}
	msg, _ = s.geoServerPublishDatasourceError(req, jrxml, "Other")
	if !strings.Contains(msg, "ProdGeoServer") {
		t.Fatalf("mismatch msg=%q", msg)
	}
	cat.cred.Name = "Other"
	msg, _ = s.geoServerPublishDatasourceError(req, jrxml, "ProdGeoServer")
	if !strings.Contains(msg, "not configured") || !strings.Contains(msg, "ProdGeoServer") {
		t.Fatalf("missing msg=%q", msg)
	}
	names := s.geoServerDatasourceNames(req.Context())
	if len(names) != 1 || names[0] != "ProdGeoServer" {
		t.Fatalf("names=%v", names)
	}
}
