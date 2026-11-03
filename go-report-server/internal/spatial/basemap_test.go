package spatial

import (
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestDefaultMapReportBasemap(t *testing.T) {
	def := DefaultMapDefinition("geom")
	if def["basemap"] != DefaultMapReportBasemap {
		t.Fatalf("default basemap=%v", def["basemap"])
	}
	if DefaultMapReportBasemap != BasemapOFMPositron {
		t.Fatalf("canonical default=%s", DefaultMapReportBasemap)
	}
	if def["geometryColumn"] != "geom" {
		t.Fatalf("geometryColumn=%v", def["geometryColumn"])
	}
}

func TestNormalizeMapReportBasemapLegacy(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", BasemapOFMPositron},
		{"osm", BasemapOFMPositron},
		{"OpenStreetMap", BasemapOFMPositron},
		{"https://tile.openstreetmap.org/{z}/{x}/{y}.png", BasemapOFMPositron},
		{"carto_light", BasemapOFMPositron},
		{"Carto Light", BasemapOFMPositron},
		{"carto-light", BasemapOFMPositron},
		{"https://a.basemaps.cartocdn.com/light_all/{z}/{x}/{y}.png", BasemapOFMPositron},
		{"carto_dark", BasemapOFMDark},
		{"Carto Dark", BasemapOFMDark},
		{"carto-dark", BasemapOFMDark},
		{"https://a.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png", BasemapOFMDark},
		{"carto_voyager", BasemapOFMBright},
		{"Carto Voyager", BasemapOFMBright},
		{"carto-voyager", BasemapOFMBright},
		{"https://a.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}.png", BasemapOFMBright},
		{"ofm_liberty", BasemapOFMLiberty},
		{"ofm_positron", BasemapOFMPositron},
		{"openfreemap_light", BasemapOFMPositron},
		{"OpenFreeMap Light", BasemapOFMPositron},
		{"esri_imagery", BasemapEsriImagery},
		{"esri satellite", BasemapEsriImagery},
		{"not-a-basemap", BasemapOFMPositron},
	}
	for _, tc := range cases {
		if got := NormalizeMapReportBasemap(tc.in); got != tc.want {
			t.Errorf("NormalizeMapReportBasemap(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeMapDefinitionPreservesSpatialFields(t *testing.T) {
	raw := map[string]any{
		"basemap":        "carto_light",
		"geometryColumn": "shape",
		"renderer":       "heatmap",
		"sync": map[string]any{
			"filterMapFromReport": true,
			"filterReportFromMap": true,
		},
	}
	out := NormalizeMapDefinition(raw, "shape")
	if out["basemap"] != BasemapOFMPositron {
		t.Fatalf("basemap=%v", out["basemap"])
	}
	if out["geometryColumn"] != "shape" {
		t.Fatalf("geometryColumn=%v", out["geometryColumn"])
	}
	if out["renderer"] != "heatmap" {
		t.Fatalf("renderer=%v", out["renderer"])
	}
	sync, _ := out["sync"].(map[string]any)
	if sync["filterReportFromMap"] != true || sync["filterMapFromReport"] != true {
		t.Fatalf("sync=%v", sync)
	}
	if raw["basemap"] != "carto_light" {
		t.Fatalf("raw basemap rewritten: %v", raw["basemap"])
	}
}

func TestReportMapDefinitionDoesNotRewriteStoredBasemap(t *testing.T) {
	stored := map[string]any{
		"basemap":        "osm",
		"geometryColumn": "geom",
	}
	def := &reports.Definition{
		Key: "m1",
		Raw: map[string]any{
			"type":          "map",
			"mapDefinition": stored,
		},
	}
	out := ReportMapDefinition(def)
	if out["basemap"] != BasemapOFMPositron {
		t.Fatalf("basemap=%v", out["basemap"])
	}
	if stored["basemap"] != "osm" {
		t.Fatalf("stored basemap rewritten: %v", stored["basemap"])
	}
}

func TestMapReportViewerCSPAllowsOpenFreeMapWithoutWildcard(t *testing.T) {
	csp := MapReportViewerCSP()
	for _, want := range []string{
		"connect-src 'self' https://tiles.openfreemap.org",
		"img-src 'self' data: blob: https://tiles.openfreemap.org https://server.arcgisonline.com",
		"style-src 'self' https://cdn.jsdelivr.net https://fonts.googleapis.com 'unsafe-inline'",
		"worker-src 'self' blob:",
		"font-src 'self' data: https://fonts.gstatic.com",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("csp missing %q\n%s", want, csp)
		}
	}
	if strings.Contains(csp, "*") {
		t.Fatalf("csp uses a wildcard: %s", csp)
	}
	if strings.Contains(csp, "cartocdn.com") || strings.Contains(csp, "tile.openstreetmap.org") {
		t.Fatalf("csp still allows retired basemap hosts: %s", csp)
	}
}
