package schedules

import (
	"context"
	"encoding/json"
	"io"
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

type fakeArcGIS struct {
	cred  arcgis.Credentials
	asked string
}

func (f *fakeArcGIS) FindArcGIS(_ context.Context, name string) (arcgis.Credentials, error) {
	f.asked = name
	if name != f.cred.Name {
		return arcgis.Credentials{}, arcgis.ErrNotFound
	}
	return f.cred, nil
}

func TestPrepareArcGISCollectionScheduled(t *testing.T) {
	var where string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/query") {
			where = r.URL.Query().Get("where")
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
	defer srv.Close()

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
  <property name="acugis.datasource.provider" value="arcgis"/>
  <property name="acugis.datasource.connection" value="WeatherFS"/>
  <parameter name="p_div" class="java.util.Collection">
    <defaultValueExpression><![CDATA[java.util.Arrays.asList("Mtn", "W N Cen")]]></defaultValueExpression>
  </parameter>
  <query language="collection"><![CDATA[SELECT "division" FROM "layers"."Divisions" WHERE $X{IN, "division", p_div}]]></query>
</jasperReport>`
	if err := os.WriteFile(filepath.Join(jrxmlDir, "weather.jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{"name":"Weather","report":"weather","datasource":"WeatherFS","type":"jasper","parameters":[{"name":"p_div","multiple":true}]}`
	if err := os.WriteFile(filepath.Join(jsonDir, "weather.report.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := &fakeArcGIS{cred: arcgis.Credentials{Name: "WeatherFS", URL: srv.URL + "/FeatureServer"}}
	runner := &Runner{Reports: reports.NewStore(jsonDir, "", jrxmlDir), ArcGIS: cat}

	ok, method, _, ct, body, err := runner.prepareArcGISCollection(context.Background(), "weather", "WeatherFS", "_repFormat=pdf&_repName=weather")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if method != http.MethodPost || !strings.Contains(ct, "urlencoded") {
		t.Fatalf("method=%s ct=%s", method, ct)
	}
	raw, _ := io.ReadAll(body)
	vals, err := url.ParseQuery(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(vals.Get("_collectionData"), `"division":"Mtn"`) {
		t.Fatalf("body=%s", raw)
	}
	if vals.Get("_repFormat") != "pdf" {
		t.Fatalf("format=%q", vals.Get("_repFormat"))
	}
	if cat.asked != "WeatherFS" {
		t.Fatalf("connection=%q", cat.asked)
	}
	if !strings.Contains(where, "'Mtn'") || !strings.Contains(where, "'W N Cen'") {
		t.Fatalf("scheduled default where=%s", where)
	}

	ok, _, _, _, _, err = runner.prepareArcGISCollection(context.Background(), "weather", "WeatherFS", "p_div=Pac&p_div=NE&_repFormat=xlsx")
	if err != nil || !ok {
		t.Fatalf("filtered ok=%v err=%v", ok, err)
	}
	if !strings.Contains(where, "'Pac'") || !strings.Contains(where, "'NE'") || strings.Contains(where, "'Mtn, NE'") {
		t.Fatalf("scheduled viewer where=%s", where)
	}
}

func TestPrepareArcGISCollectionIgnoresCassandraAndBigQuery(t *testing.T) {
	dir := t.TempDir()
	jrxmlDir := filepath.Join(dir, "jrxml")
	jsonDir := filepath.Join(dir, "json")
	if err := os.MkdirAll(jrxmlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"cass.jrxml": `<jasperReport><property name="acugis.datasource.provider" value="cassandra"/><query language="cassandra"><![CDATA[SELECT id FROM ks.t]]></query></jasperReport>`,
		"bq.jrxml":   `<jasperReport><property name="acugis.datasource.provider" value="bigquery"/><query language="collection"><![CDATA[SELECT 1]]></query></jasperReport>`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(jrxmlDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runner := &Runner{Reports: reports.NewStore(jsonDir, "", jrxmlDir), ArcGIS: &fakeArcGIS{}}
	for _, key := range []string{"cass", "bq"} {
		ok, _, _, _, _, err := runner.prepareArcGISCollection(context.Background(), key, "SomeDS", "")
		if ok || err != nil {
			t.Fatalf("%s ok=%v err=%v", key, ok, err)
		}
	}
}
