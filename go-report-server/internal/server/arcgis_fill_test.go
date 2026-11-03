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

	"github.com/acugis/acugis-report-server-go/internal/arcgis"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

type fakeArcGISCatalog struct {
	cred  arcgis.Credentials
	asked string
	calls int
}

func (f *fakeArcGISCatalog) ListEnabled(context.Context) ([]arcgis.Connection, error) {
	return nil, nil
}
func (f *fakeArcGISCatalog) FindArcGIS(_ context.Context, name string) (arcgis.Credentials, error) {
	f.calls++
	f.asked = name
	if name != f.cred.Name {
		return arcgis.Credentials{}, arcgis.ErrNotFound
	}
	return f.cred, nil
}
func (f *fakeArcGISCatalog) Upsert(context.Context, arcgis.Connection) error { return nil }
func (f *fakeArcGISCatalog) DeleteByID(context.Context, int64) error         { return nil }
func (f *fakeArcGISCatalog) TableAvailable(context.Context) bool             { return true }

func writeArcReport(t *testing.T, provider, query string) (*reports.Store, string) {
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
  <property name="acugis.datasource.connection" value="WeatherFS"/>
  <parameter name="p_div" class="java.util.Collection">
    <defaultValueExpression><![CDATA[java.util.Arrays.asList("Mtn", "W N Cen")]]></defaultValueExpression>
  </parameter>
  <query language="` + map[bool]string{true: "collection", false: provider}[provider != "cassandra"] + `"><![CDATA[` + query + `]]></query>
</jasperReport>`
	if provider == "cassandra" {
		jrxml = `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="cassandra"/>
  <query language="cassandra"><![CDATA[SELECT id FROM ks.t]]></query>
</jasperReport>`
	}
	if provider == "bigquery" {
		jrxml = `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="bigquery"/>
  <query language="collection"><![CDATA[SELECT 1]]></query>
</jasperReport>`
	}
	if err := os.WriteFile(filepath.Join(jrxmlDir, "weather.jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{
		"name": "Weather",
		"report": "weather",
		"datasource": "WeatherFS",
		"type": "jasper",
		"format": "html",
		"parameters": [{"name": "p_div", "label": "Division", "type": "select", "multiple": true}]
	}`
	if err := os.WriteFile(filepath.Join(jsonDir, "weather.report.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	return reports.NewStore(jsonDir, "", jrxmlDir), "weather"
}

func TestPrepareArcGISCollectionFillInteractive(t *testing.T) {
	var wheres []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/query") {
			wheres = append(wheres, r.URL.Query().Get("where"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"features": []any{map[string]any{"attributes": map[string]any{
					"division":    "Mtn",
					"stationcode": 6275,
					"lat":         52.07,
				}}},
			})
			return
		}
		if strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/0") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "Divisions", "id": 0, "supportsQuery": true, "maxRecordCount": 1000,
				"fields": []any{
					map[string]any{"name": "division", "type": "esriFieldTypeString"},
					map[string]any{"name": "stationcode", "type": "esriFieldTypeInteger"},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"layers": []any{map[string]any{"id": 0, "name": "Divisions", "supportsQuery": true}},
		})
	}))
	defer srv.Close()

	store, name := writeArcReport(t, "arcgis", `SELECT "division", "stationcode" FROM "layers"."Divisions" WHERE $X{IN, "division", p_div}`)
	cat := &fakeArcGISCatalog{cred: arcgis.Credentials{Name: "WeatherFS", URL: srv.URL + "/arcgis/rest/services/demo/FeatureServer"}}
	s := &Server{
		deps:   Dependencies{Reports: store, ArcGIS: cat},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, format := range []string{"html", "pdf", "xlsx"} {
		wheres = nil
		ok, method, _, ct, body, err := s.prepareArcGISCollectionFill(context.Background(), name, "_repFormat="+format+"&_dataSource=ignored")
		if err != nil || !ok {
			t.Fatalf("format %s: ok=%v err=%v", format, ok, err)
		}
		if method != http.MethodPost || !strings.Contains(ct, "application/x-www-form-urlencoded") {
			t.Fatalf("format %s method=%s ct=%s", format, method, ct)
		}
		raw, _ := io.ReadAll(body)
		vals, err := url.ParseQuery(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		if vals.Get("_repFormat") != format {
			t.Fatalf("format lost: %v", vals)
		}
		payload := vals.Get("_collectionData")
		if !strings.Contains(payload, `"division":"Mtn"`) || !strings.Contains(payload, `"stationcode":6275`) {
			t.Fatalf("collection %s", payload)
		}
		if cat.asked != "WeatherFS" {
			t.Fatalf("resolved %q", cat.asked)
		}
		if len(wheres) != 1 || !strings.Contains(wheres[0], "'Mtn'") || !strings.Contains(wheres[0], "'W N Cen'") {
			t.Fatalf("default where=%v", wheres)
		}
	}

	wheres = nil
	_, _, _, _, _, err := s.prepareArcGISCollectionFill(context.Background(), name, "p_div=Pac&p_div=NE&_repFormat=html")
	if err != nil {
		t.Fatal(err)
	}
	if len(wheres) != 1 || !strings.Contains(wheres[0], "'Pac'") || !strings.Contains(wheres[0], "'NE'") || strings.Contains(wheres[0], "'Mtn'") {
		t.Fatalf("viewer where=%v", wheres)
	}
	if strings.Contains(wheres[0], "'Pac, NE'") {
		t.Fatalf("flattened viewer where: %s", wheres[0])
	}
}

func TestPrepareArcGISSkipsOtherCollectionProviders(t *testing.T) {
	for _, provider := range []string{"cassandra", "bigquery"} {
		t.Run(provider, func(t *testing.T) {
			store, name := writeArcReport(t, provider, "SELECT 1")
			cat := &fakeArcGISCatalog{}
			s := &Server{deps: Dependencies{Reports: store, ArcGIS: cat}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			ok, _, _, _, _, err := s.prepareArcGISCollectionFill(context.Background(), name, "")
			if ok || err != nil {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
			if cat.calls != 0 {
				t.Fatal("arcgis catalog was queried for a non-arcgis report")
			}
		})
	}
}

func TestPrepareArcGISMissingConnection(t *testing.T) {
	store, name := writeArcReport(t, "arcgis", `SELECT "division" FROM "layers"."Divisions"`)
	cat := &fakeArcGISCatalog{cred: arcgis.Credentials{Name: "Other"}}
	s := &Server{deps: Dependencies{Reports: store, ArcGIS: cat}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ok, _, _, _, _, err := s.prepareArcGISCollectionFill(context.Background(), name, "")
	if !ok || err == nil || !strings.Contains(err.Error(), "WeatherFS") {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
