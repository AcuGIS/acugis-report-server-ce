package schedules

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
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

type fakeGeoServer struct {
	cred  geoserver.Credentials
	asked string
}

func (f *fakeGeoServer) FindGeoServer(_ context.Context, name string) (geoserver.Credentials, error) {
	f.asked = name
	if name != f.cred.Name {
		return geoserver.Credentials{}, geoserver.ErrNotFound
	}
	return f.cred, nil
}

func TestPrepareGeoServerCollectionScheduled(t *testing.T) {
	var cql string
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if strings.EqualFold(r.URL.Query().Get("request"), "DescribeFeatureType") {
			_, _ = io.WriteString(w, `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"><xsd:complexType><xsd:sequence><xsd:element name="name" type="xsd:string"/></xsd:sequence></xsd:complexType></xsd:schema>`)
			return
		}
		cql = r.URL.Query().Get("CQL_FILTER")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "FeatureCollection",
			"features": []any{map[string]any{
				"id": "roads.1", "properties": map[string]any{"name": "Main"},
			}},
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
  <property name="acugis.datasource.provider" value="geoserver"/>
  <property name="acugis.datasource.connection" value="ProdGeoServer"/>
  <query language="collection"><![CDATA[SELECT "name" FROM "transport"."roads" WHERE "lanes" > $P{minlanes}]]></query>
</jasperReport>`
	if err := os.WriteFile(filepath.Join(jrxmlDir, "roads.jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{"name":"Roads","report":"roads","datasource":"ProdGeoServer","type":"jasper"}`
	if err := os.WriteFile(filepath.Join(jsonDir, "roads.report.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := &fakeGeoServer{cred: geoserver.Credentials{
		Name: "ProdGeoServer", URL: srv.URL + "/geoserver", Username: "admin", Password: "secret",
	}}
	runner := &Runner{Reports: reports.NewStore(jsonDir, "", jrxmlDir), GeoServer: cat}
	ok, method, _, ct, body, err := runner.prepareGeoServerCollection(context.Background(), "roads", "ProdGeoServer", "minlanes=2&_repFormat=pdf")
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
	if !strings.Contains(vals.Get("_collectionData"), `"name":"Main"`) {
		t.Fatalf("body=%s", raw)
	}
	if cat.asked != "ProdGeoServer" {
		t.Fatalf("connection=%q", cat.asked)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	if auth != want {
		t.Fatalf("auth=%q", auth)
	}
	if !strings.Contains(cql, "> 2") || strings.Contains(cql, "'2'") {
		t.Fatalf("cql=%s", cql)
	}
}

func TestPrepareGeoServerCollectionIgnoresOtherProviders(t *testing.T) {
	dir := t.TempDir()
	jrxmlDir := filepath.Join(dir, "jrxml")
	jsonDir := filepath.Join(dir, "json")
	if err := os.MkdirAll(jrxmlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"arc.jrxml":  `<jasperReport><property name="acugis.datasource.provider" value="arcgis"/><query language="collection"><![CDATA[SELECT "a" FROM "layers"."t"]]></query></jasperReport>`,
		"cass.jrxml": `<jasperReport><property name="acugis.datasource.provider" value="cassandra"/><query language="cassandra"><![CDATA[SELECT id FROM ks.t]]></query></jasperReport>`,
		"bq.jrxml":   `<jasperReport><property name="acugis.datasource.provider" value="bigquery"/><query language="collection"><![CDATA[SELECT 1]]></query></jasperReport>`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(jrxmlDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runner := &Runner{Reports: reports.NewStore(jsonDir, "", jrxmlDir), GeoServer: &fakeGeoServer{}}
	for _, key := range []string{"arc", "cass", "bq"} {
		ok, _, _, _, _, err := runner.prepareGeoServerCollection(context.Background(), key, "SomeDS", "")
		if ok || err != nil {
			t.Fatalf("%s ok=%v err=%v", key, ok, err)
		}
	}
}
