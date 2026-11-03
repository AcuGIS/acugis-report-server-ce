package embeddedmap

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"testing"
	"time"
)

func TestNormalizeBasemap(t *testing.T) {
	cases := map[string]string{
		"":                  BasemapNone,
		"none":              BasemapNone,
		"osm":               BasemapOSM,
		"OpenStreetMap":     BasemapOSM,
		"carto_light":       "carto_light",
		"Carto Light":       "carto_light",
		"carto_dark":        "carto_dark",
		"Carto Dark":        "carto_dark",
		"openfreemap_light": BasemapOpenFreeMapLight,
		"openfreemap_dark":  BasemapOpenFreeMapDark,
		"satellite":         BasemapEsriImagery,
		"esri_streets":      BasemapEsriStreets,
		"unknown":           BasemapNone,
	}
	for in, want := range cases {
		if got := NormalizeBasemap(in); got != want {
			t.Errorf("NormalizeBasemap(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRenderBasemapAliases(t *testing.T) {
	if RenderBasemap("openfreemap_light") != BasemapOpenFreeMapLight {
		t.Fatal("light")
	}
	if RenderBasemap("openfreemap_dark") != BasemapOpenFreeMapDark {
		t.Fatal("dark")
	}
	if RenderBasemap("carto_light") != BasemapOpenFreeMapLight || RenderBasemap("Carto Light") != BasemapOpenFreeMapLight {
		t.Fatal("carto light alias")
	}
	if RenderBasemap("carto_dark") != BasemapOpenFreeMapDark || RenderBasemap("Carto Dark") != BasemapOpenFreeMapDark {
		t.Fatal("carto dark alias")
	}
	if RenderBasemap("none") != BasemapNone || RenderBasemap("") != BasemapNone {
		t.Fatal("none")
	}
	if !SameBasemap("carto_light", BasemapOpenFreeMapLight) || SameBasemap("carto_dark", BasemapOpenFreeMapLight) {
		t.Fatal("same basemap")
	}
}

func TestHasBasemap(t *testing.T) {
	if HasBasemap("none") || HasBasemap("") {
		t.Fatal("none should not fetch tiles")
	}
	if !HasBasemap("osm") || !HasBasemap("openfreemap_light") || !HasBasemap("openfreemap_dark") {
		t.Fatal("expected tile basemaps")
	}
	if !HasBasemap("carto_light") || !HasBasemap("carto_dark") {
		t.Fatal("legacy ids should still paint")
	}
}

func TestBasemapOptionsDropCarto(t *testing.T) {
	seen := map[string]bool{}
	for _, opt := range BasemapOptions {
		if strings.Contains(strings.ToLower(opt.Key), "carto") || strings.Contains(strings.ToLower(opt.Label), "carto") {
			t.Fatalf("menu still offers CARTO: %+v", opt)
		}
		seen[opt.Key] = true
	}
	for _, key := range []string{BasemapNone, BasemapOSM, BasemapOpenFreeMapLight, BasemapOpenFreeMapDark, BasemapEsriImagery} {
		if !seen[key] {
			t.Fatalf("missing option %s", key)
		}
	}
	for _, prov := range basemapProviders {
		if strings.Contains(prov.URLTemplate, "cartocdn") || strings.Contains(prov.VectorStyle, "cartocdn") || strings.Contains(prov.VectorStyle, "api_key") {
			t.Fatalf("provider still references CARTO or an API key: %+v", prov)
		}
	}
}

func TestBasemapZoom(t *testing.T) {
	z := basemapZoom(100000, 512)
	if z < 1 || z > 18 {
		t.Fatalf("unexpected zoom %d", z)
	}
}

func TestBasemapTileURL(t *testing.T) {
	u := basemapTileURL(basemapProviders[BasemapOSM], 3, 1, 2)
	if u != "https://tile.openstreetmap.org/3/1/2.png" {
		t.Fatalf("url=%q", u)
	}
	u = basemapTileURL(basemapProviders[BasemapEsriImagery], 4, 5, 6)
	if u != "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/4/6/5" {
		t.Fatalf("esri url=%q", u)
	}
}

func TestNormalizeKeepsBasemap(t *testing.T) {
	d := Normalize(map[string]any{"basemap": "carto_light", "renderer": "postgis"})
	if d.Basemap != "carto_light" {
		t.Fatalf("basemap=%q", d.Basemap)
	}
	if got := d.ToMap()["basemap"]; got != "carto_light" {
		t.Fatalf("ToMap rewrote stored basemap: %v", got)
	}
	dark := Normalize(map[string]any{"basemap": "carto_dark", "renderer": "postgis"})
	if dark.Basemap != "carto_dark" || dark.ToMap()["basemap"] != "carto_dark" {
		t.Fatalf("dark stored basemap=%q", dark.Basemap)
	}
	if RenderBasemap(d.Basemap) != BasemapOpenFreeMapLight || RenderBasemap(dark.Basemap) != BasemapOpenFreeMapDark {
		t.Fatal("stored ids should render as OpenFreeMap")
	}
}

func TestOpenFreeMapTilesRender(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	ext := bbox{-74.02, 40.70, -73.96, 40.75}

	lightURL := openFreeMapTileURL(ctx, openFreeMapLightStyle, 12, 1206, 1539)
	darkURL := openFreeMapTileURL(ctx, openFreeMapDarkStyle, 12, 1206, 1539)
	for _, u := range []string{lightURL, darkURL} {
		if !strings.Contains(u, "https://tiles.openfreemap.org/") || !strings.Contains(u, ".pbf") {
			t.Fatalf("tile url=%s", u)
		}
		if strings.Contains(u, "api_key") || strings.Contains(u, "apikey") || strings.Contains(u, "cartocdn") {
			t.Fatalf("tile url must not use an API key or CARTO: %s", u)
		}
	}

	light := renderBasemap(ctx, ext, "openfreemap_light")
	dark := renderBasemap(ctx, ext, "openfreemap_dark")
	legacy := renderBasemap(ctx, ext, "carto_light")
	none := renderBasemap(ctx, ext, "none")

	lightLuma, lightDev := imageLuma(light)
	darkLuma, darkDev := imageLuma(dark)
	legacyLuma, _ := imageLuma(legacy)
	noneLuma, noneDev := imageLuma(none)
	t.Logf("light luma=%.2f stdev=%.2f dark luma=%.2f stdev=%.2f legacy luma=%.2f none luma=%.2f", lightLuma, lightDev, darkLuma, darkDev, legacyLuma, noneLuma)
	if lightLuma < 160 || lightDev < 8 {
		t.Fatalf("OpenFreeMap light did not render a street basemap luma=%.2f stdev=%.2f", lightLuma, lightDev)
	}
	if darkLuma > 80 || darkDev < 4 {
		t.Fatalf("OpenFreeMap dark did not render a street basemap luma=%.2f stdev=%.2f", darkLuma, darkDev)
	}
	if math.Abs(lightLuma-legacyLuma) > 8 {
		t.Fatalf("carto_light luma %.2f diverged from openfreemap_light %.2f", legacyLuma, lightLuma)
	}
	if noneLuma < 250 || noneDev > 1 {
		t.Fatalf("none fetched a basemap luma=%.2f stdev=%.2f", noneLuma, noneDev)
	}
}

func renderBasemap(ctx context.Context, ext bbox, id string) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	drawBasemapTiles(ctx, img, ext, 320, 240, 0, 0, id)
	return img
}

func imageLuma(img image.Image) (mean, stdev float64) {
	b := img.Bounds()
	n := float64(b.Dx() * b.Dy())
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			sum += 0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(bl>>8)
		}
	}
	mean = sum / n
	var variance float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			l := 0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(bl>>8)
			d := l - mean
			variance += d * d
		}
	}
	return mean, math.Sqrt(variance / n)
}

func TestMercatorProjectorAlignsWithTiles(t *testing.T) {
	ext := bbox{-74.1, 40.6, -73.8, 40.9}
	p := newMercatorProjector(ext, 400, 300, 0, 0)
	if !p.mercator || p.scale <= 0 {
		t.Fatalf("bad projector: %+v", p)
	}
	x, y := p.xy(-74.0, 40.75)
	if x < 0 || x > 400 || y < 0 || y > 300 {
		t.Fatalf("xy out of bounds: %d,%d", x, y)
	}
}
