package embeddedmap

import (
	"testing"
)

func TestBuildCQLFilter(t *testing.T) {
	mappings := []GeoServerFilterMapping{
		{ReportParameter: "STATE", GeoServerField: "STATE_NAME", Operator: "="},
		{ReportParameter: "EMPTY", GeoServerField: "x", Operator: "="},
	}
	params := map[string]any{
		"STATE": "New York",
		"EMPTY": "",
	}
	got := BuildCQLFilter(mappings, params)
	want := "STATE_NAME='New York'"
	if got != want {
		t.Fatalf("cql=%q want %q", got, want)
	}
}

func TestBuildCQLFilterIN(t *testing.T) {
	mappings := []GeoServerFilterMapping{
		{ReportParameter: "CODES", GeoServerField: "code", Operator: "="},
	}
	params := map[string]any{
		"CODES": []string{"A", "B"},
	}
	got := BuildCQLFilter(mappings, params)
	if got != "code IN ('A','B')" {
		t.Fatalf("cql=%q", got)
	}
}

func TestBuildCQLFilterNumeric(t *testing.T) {
	mappings := []GeoServerFilterMapping{
		{ReportParameter: "POP", GeoServerField: "pop", Operator: ">="},
	}
	params := map[string]any{"POP": "100000"}
	got := BuildCQLFilter(mappings, params)
	if got != "pop>=100000" {
		t.Fatalf("cql=%q", got)
	}
}

func TestFitBBoxToAspect(t *testing.T) {
	b := fitBBoxToAspect(wmsBBox{0, 0, 1, 1}, 200, 100)
	w := b.MaxX - b.MinX
	h := b.MaxY - b.MinY
	aspect := w / h
	if aspect < 1.9 || aspect > 2.1 {
		t.Fatalf("expected ~2:1 aspect, got %f (%v)", aspect, b)
	}
}

func TestParseCapabilitiesExtent(t *testing.T) {
	xml := `<?xml version="1.0"?>
<WMS_Capabilities>
  <Layer>
    <Name>topp:states</Name>
    <LatLonBoundingBox minx="-125" miny="24" maxx="-66" maxy="50"/>
  </Layer>
</WMS_Capabilities>`
	b, crs, ok := parseCapabilitiesExtent(xml, "topp:states")
	if !ok || crs != "EPSG:4326" {
		t.Fatalf("parse failed: ok=%v crs=%s", ok, crs)
	}
	if b.MinX != -125 || b.MaxY != 50 {
		t.Fatalf("bbox=%+v", b)
	}
}

func TestBBoxCSV(t *testing.T) {
	got := bboxCSV(wmsBBox{-1.5, 2, 3.25, 4})
	if got != "-1.50000000,2.00000000,3.25000000,4.00000000" {
		t.Fatalf("csv=%q", got)
	}
}
