package embeddedmap

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

func TestNormalizeDefaults(t *testing.T) {
	d := Normalize(nil)
	if d.Enabled || d.Band != "summary" || d.Renderer != RendererID || d.ParameterName != ParamName {
		t.Fatalf("unexpected defaults: %+v", d)
	}
}

func TestNormalizeRenderer(t *testing.T) {
	d := Normalize(map[string]any{
		"enabled":  true,
		"renderer": "mysql",
		"mapType":  "choropleth",
		"field":    "pop",
	})
	if d.Renderer != RendererMySQL {
		t.Fatalf("expected mysql renderer, got %q", d.Renderer)
	}
	d = Normalize(map[string]any{"renderer": "geoserver", "geoserverWorkspace": "topp", "geoserverLayer": "states"})
	if d.Renderer != RendererGeoServer || d.GeoServerWorkspace != "topp" || d.GeoServerLayer != "states" {
		t.Fatalf("unexpected geoserver normalize: %+v", d)
	}
	d = Normalize(map[string]any{"renderer": "qgis", "qgisLayer": "parcels", "qgisProject": "planning.qgz"})
	if d.Renderer != RendererQGIS || d.QGISLayer != "parcels" || d.QGISProject != "planning.qgz" {
		t.Fatalf("unexpected qgis normalize: %+v", d)
	}
}

func TestResolveProvider(t *testing.T) {
	if got := ResolveProvider("geoserver", &datasources.Datasource{DBType: "postgresql"}); got != RendererGeoServer {
		t.Fatalf("geoserver must not be overwritten by datasource, got %q", got)
	}
	if got := ResolveProvider("qgis", &datasources.Datasource{DBType: "postgresql"}); got != RendererQGIS {
		t.Fatalf("qgis must not be overwritten by datasource, got %q", got)
	}
	if got := ResolveProvider("postgis", &datasources.Datasource{DBType: "mysql"}); got != RendererMySQL {
		t.Fatalf("mysql datasource should win for spatial renderer, got %q", got)
	}
	if got := ResolveProvider("mysql", &datasources.Datasource{DBType: "postgresql"}); got != RendererPostGIS {
		t.Fatalf("postgres datasource should win for spatial renderer, got %q", got)
	}
}

func TestFromForm(t *testing.T) {
	form := url.Values{}
	form.Set("embedded_map[enabled]", "1")
	form.Set("embedded_map[mapType]", "locator")
	form.Set("embedded_map[band]", "summary")
	form.Set("embedded_map[width]", "400")
	form.Set("embedded_map[includeLegend]", "1")
	form.Set("embedded_map[pointSymbol][shape]", "square")
	d := FromForm(form)
	if !d.Enabled || d.Width != 400 || d.PointSymbol["shape"] != "square" {
		t.Fatalf("unexpected from form: %+v", d)
	}
}

func TestSyncJRXMLInsertAndRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.jrxml")
	// Studio 7 / JasperReports 7: summary is the band (no nested <band>).
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="demo">
<summary height="40"></summary>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	emb := Default()
	emb.Enabled = true
	emb.Band = "summary"
	emb.Width = 400
	emb.Height = 300
	res := SyncJRXML(path, emb)
	if !res.OK || !res.JRXMLChanged {
		t.Fatalf("sync insert failed: %+v", res)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.Contains(text, ParamName) || !strings.Contains(text, ElementUUID) {
		t.Fatalf("missing markers after insert:\n%s", text)
	}
	if !strings.Contains(text, `name="`+ReportDirParam+`"`) {
		t.Fatalf("expected REPORT_DIR parameter:\n%s", text)
	}
	if !strings.Contains(text, "$P{"+ParamName+"}") || !strings.Contains(text, "java.io.File") {
		t.Fatalf("expected File-based P_MAP_IMAGE expression:\n%s", text)
	}
	if strings.Contains(strings.ToLower(text), "<summary") && strings.Contains(text, "<band") {
		// Nested band under summary breaks Jackson JRDesignBand.
		if regexpContainsSummaryBand(text) {
			t.Fatalf("summary must not nest <band>:\n%s", text)
		}
	}
	if !strings.Contains(text, `height="`) {
		t.Fatalf("expected summary height attr:\n%s", text)
	}

	res = SyncJRXML(path, emb)
	if !res.OK || res.JRXMLChanged {
		t.Fatalf("second sync should be no-op: %+v", res)
	}

	emb.Enabled = false
	res = SyncJRXML(path, emb)
	if !res.OK || !res.JRXMLChanged {
		t.Fatalf("sync remove failed: %+v", res)
	}
	data, _ = os.ReadFile(path)
	text = string(data)
	if strings.Contains(text, ElementUUID) {
		t.Fatalf("uuid still present after remove:\n%s", text)
	}
}

func TestSyncJRXMLFlattensNestedSummaryBand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.jrxml")
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="broken">
<summary height="40">
<band height="320">
<element kind="staticText" uuid="keep-me" x="0" y="0" width="10" height="10"/>
</band>
</summary>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	emb := Default()
	emb.Enabled = true
	emb.Band = "summary"
	res := SyncJRXML(path, emb)
	if !res.OK {
		t.Fatalf("sync: %+v", res)
	}
	text := string(mustRead(t, path))
	if regexpContainsSummaryBand(text) {
		t.Fatalf("expected flattened summary:\n%s", text)
	}
	if !strings.Contains(text, "keep-me") || !strings.Contains(text, ElementUUID) {
		t.Fatalf("expected existing + map elements:\n%s", text)
	}
}

func regexpContainsSummaryBand(text string) bool {
	return strings.Contains(strings.ToLower(text), "<summary") &&
		regexp.MustCompile(`(?is)<summary\b[^>]*>\s*<band\b`).MatchString(text)
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTomcatPath(t *testing.T) {
	p := Paths{
		JRXMLDir:                  `/opt/app/docker/report-server/reports`,
		JasperReportsContainerDir: `/usr/local/tomcat/jasper_reports/reports`,
	}
	got := p.TomcatPath(`/opt/app/docker/report-server/reports/_map_cache/demo.png`)
	got = strings.ReplaceAll(got, `\`, `/`)
	if got != `/usr/local/tomcat/jasper_reports/reports/_map_cache/demo.png` {
		t.Fatalf("tomcat path=%q", got)
	}

	// Standalone: same host/container dir → absolute host path.
	stand := Paths{
		JRXMLDir:                  `/home/tomcat/apache-tomcat-10.1.57/acugis/reports`,
		JasperReportsContainerDir: `/home/tomcat/apache-tomcat-10.1.57/acugis/reports`,
	}
	got = stand.TomcatPath(`/home/tomcat/apache-tomcat-10.1.57/acugis/reports/_map_cache/demo.png`)
	got = strings.ReplaceAll(got, `\`, `/`)
	if !strings.HasSuffix(got, `/acugis/reports/_map_cache/demo.png`) {
		t.Fatalf("standalone path=%q", got)
	}
}

func TestRenderPNGLocator(t *testing.T) {
	features := []map[string]any{
		{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []any{-73.9, 40.7},
			},
			"properties": map[string]any{"name": "a"},
		},
	}
	emb := Default()
	emb.Width = 200
	emb.Height = 150
	thematic := PrepareThematic(features, nil, emb, "Demo")
	png, err := RenderPNG(thematic, emb)
	if err != nil {
		t.Fatal(err)
	}
	if len(png) < 50 || png[0] != 0x89 {
		t.Fatalf("expected PNG bytes, got %d bytes", len(png))
	}
}
