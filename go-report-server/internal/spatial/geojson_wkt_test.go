package spatial

import "testing"

func TestWKTToGeoJSON_PointAndPolygon(t *testing.T) {
	g, ok := WKTToGeoJSON("POINT(-73.9 40.7)")
	if !ok || g["type"] != "Point" {
		t.Fatalf("point: ok=%v g=%v", ok, g)
	}
	coords := g["coordinates"].([]float64)
	if coords[0] != -73.9 || coords[1] != 40.7 {
		t.Fatalf("coords=%v", coords)
	}

	g, ok = WKTToGeoJSON("POINT Z (-73.9 40.7 10)")
	if !ok || g["type"] != "Point" {
		t.Fatalf("point z: ok=%v g=%v", ok, g)
	}

	g, ok = WKTToGeoJSON("POLYGON((0 0,1 0,1 1,0 1,0 0))")
	if !ok || g["type"] != "Polygon" {
		t.Fatalf("polygon: ok=%v g=%v", ok, g)
	}
}

func TestWKTToGeoJSON_MultiPointParenForm(t *testing.T) {
	g, ok := WKTToGeoJSON("MULTIPOINT((1 2),(3 4))")
	if !ok || g["type"] != "MultiPoint" {
		t.Fatalf("multipoint: ok=%v g=%v", ok, g)
	}
}
