package embeddedmap

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/mvt"
	"github.com/paulmach/orb/geojson"
	"golang.org/x/image/vector"
)

// Public OpenFreeMap styles. Same endpoints as the Designer map renderer.
// https://tiles.openfreemap.org/styles/positron is the light Positron style.
// https://tiles.openfreemap.org/styles/dark is the dark style.
// Neither requires an API key. Vector tiles come from the style TileJSON,
// with planet/latest as the documented fallback.
const (
	openFreeMapLightStyle   = "https://tiles.openfreemap.org/styles/positron"
	openFreeMapDarkStyle    = "https://tiles.openfreemap.org/styles/dark"
	openFreeMapTileFallback = "https://tiles.openfreemap.org/planet/latest/{z}/{x}/{y}.pbf"
	openFreeMapHost         = "tiles.openfreemap.org"
	openFreeMapDefaultMax   = 14
	ofmTilePx               = 512
	ofmTileCache            = 48
)

type ofmLayer struct {
	kind        string
	sourceLayer string
	color       any
	opacity     any
	width       any
	outline     any
	filter      any
	minZoom     float64
	maxZoom     float64
}

type ofmParsed struct {
	template string
	maxZoom  int
	source   string
	bg       color.RGBA
	layers   []ofmLayer
}

var (
	ofmMu      sync.Mutex
	ofmStyles  = map[string]*ofmParsed{}
	ofmTiles   = map[string]*image.RGBA{}
	ofmTileLRU []string
)

func prepareOpenFreeMap(ctx context.Context, styleURL string) {
	if !ofmAllowed(styleURL) {
		return
	}
	ofmMu.Lock()
	_, ok := ofmStyles[styleURL]
	ofmMu.Unlock()
	if ok {
		return
	}
	parsed, err := loadOFMStyle(ctx, styleURL)
	if err != nil || parsed == nil {
		return
	}
	ofmMu.Lock()
	if _, exists := ofmStyles[styleURL]; !exists {
		ofmStyles[styleURL] = parsed
	}
	ofmMu.Unlock()
}

func openFreeMapMaxZoom(styleURL string) int {
	ofmMu.Lock()
	parsed := ofmStyles[styleURL]
	ofmMu.Unlock()
	if parsed == nil || parsed.maxZoom <= 0 {
		return openFreeMapDefaultMax
	}
	return parsed.maxZoom
}

func openFreeMapTileURL(ctx context.Context, styleURL string, z, x, y int) string {
	prepareOpenFreeMap(ctx, styleURL)
	ofmMu.Lock()
	parsed := ofmStyles[styleURL]
	ofmMu.Unlock()
	template := openFreeMapTileFallback
	if parsed != nil && parsed.template != "" {
		template = parsed.template
	}
	return strings.NewReplacer(
		"{z}", strconv.Itoa(z),
		"{x}", strconv.Itoa(x),
		"{y}", strconv.Itoa(y),
	).Replace(template)
}

func renderOpenFreeMapTile(ctx context.Context, styleURL string, z, x, y int) (image.Image, error) {
	if !ofmAllowed(styleURL) {
		return nil, fmt.Errorf("openfreemap style rejected")
	}
	key := fmt.Sprintf("%s/%d/%d/%d", styleURL, z, x, y)
	ofmMu.Lock()
	if img := ofmTiles[key]; img != nil {
		ofmMu.Unlock()
		return img, nil
	}
	ofmMu.Unlock()

	prepareOpenFreeMap(ctx, styleURL)
	ofmMu.Lock()
	parsed := ofmStyles[styleURL]
	ofmMu.Unlock()
	if parsed == nil {
		return nil, fmt.Errorf("openfreemap style unavailable")
	}
	if parsed.maxZoom > 0 && z > parsed.maxZoom {
		return nil, fmt.Errorf("openfreemap zoom %d above %d", z, parsed.maxZoom)
	}
	tileURL := openFreeMapTileURL(ctx, styleURL, z, x, y)
	body, err := ofmGet(ctx, tileURL, "application/vnd.mapbox-vector-tile,application/x-protobuf,*/*")
	if err != nil {
		return nil, err
	}
	layers, err := decodeMVT(body)
	if err != nil {
		return nil, err
	}
	img := paintOFMTile(parsed, layers, z)
	ofmMu.Lock()
	if prev := ofmTiles[key]; prev != nil {
		img = prev
	} else {
		ofmTiles[key] = img
		ofmTileLRU = append(ofmTileLRU, key)
		for len(ofmTileLRU) > ofmTileCache {
			drop := ofmTileLRU[0]
			ofmTileLRU = ofmTileLRU[1:]
			delete(ofmTiles, drop)
		}
	}
	ofmMu.Unlock()
	return img, nil
}

func loadOFMStyle(ctx context.Context, styleURL string) (*ofmParsed, error) {
	raw, err := ofmGet(ctx, styleURL, "application/json,*/*")
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	parsed := &ofmParsed{
		template: openFreeMapTileFallback,
		maxZoom:  openFreeMapDefaultMax,
		source:   "openmaptiles",
		bg:       color.RGBA{242, 243, 240, 255},
	}
	if strings.Contains(styleURL, "/dark") {
		parsed.bg = color.RGBA{12, 12, 12, 255}
	}
	sources, _ := root["sources"].(map[string]any)
	vectorID := "openmaptiles"
	vector, _ := sources[vectorID].(map[string]any)
	if vector == nil {
		for name, src := range sources {
			obj, _ := src.(map[string]any)
			if obj != nil && obj["type"] == "vector" {
				vector = obj
				vectorID = name
				break
			}
		}
	}
	parsed.source = vectorID
	if vector != nil {
		if tiles, ok := vector["tiles"].([]any); ok && len(tiles) > 0 {
			if s, ok := tiles[0].(string); ok && ofmAllowed(s) {
				parsed.template = s
			}
			parsed.maxZoom = clampZoom(asInt(vector["maxzoom"], openFreeMapDefaultMax))
		} else if tileJSON, ok := vector["url"].(string); ok && ofmAllowed(tileJSON) {
			if tjRaw, err := ofmGet(ctx, tileJSON, "application/json,*/*"); err == nil {
				var tj map[string]any
				if json.Unmarshal(tjRaw, &tj) == nil {
					if tiles, ok := tj["tiles"].([]any); ok && len(tiles) > 0 {
						if s, ok := tiles[0].(string); ok && ofmAllowed(s) {
							parsed.template = s
						}
					}
					parsed.maxZoom = clampZoom(asInt(tj["maxzoom"], openFreeMapDefaultMax))
				}
			}
		}
	}
	if !ofmAllowed(parsed.template) {
		parsed.template = openFreeMapTileFallback
	}
	layers, _ := root["layers"].([]any)
	for _, item := range layers {
		layer, _ := item.(map[string]any)
		if layer == nil {
			continue
		}
		kind, _ := layer["type"].(string)
		paint, _ := layer["paint"].(map[string]any)
		if kind == "background" {
			if c, ok := evalColor(paint["background-color"], 0); ok {
				parsed.bg = c
			}
			continue
		}
		if kind != "fill" && kind != "line" {
			continue
		}
		if layer["source"] != vectorID || paint == nil {
			continue
		}
		sourceLayer, _ := layer["source-layer"].(string)
		if sourceLayer == "" {
			continue
		}
		colorKey := "fill-color"
		opacityKey := "fill-opacity"
		if kind == "line" {
			colorKey = "line-color"
			opacityKey = "line-opacity"
		}
		if _, exists := paint[colorKey]; !exists || paint[colorKey] == nil {
			continue
		}
		lp := ofmLayer{
			kind:        kind,
			sourceLayer: sourceLayer,
			color:       paint[colorKey],
			opacity:     paint[opacityKey],
			width:       paint["line-width"],
			outline:     paint["fill-outline-color"],
			filter:      layer["filter"],
			minZoom:     0,
			maxZoom:     24,
		}
		if v, ok := layer["minzoom"]; ok {
			lp.minZoom = asFloat(v, 0)
		}
		if v, ok := layer["maxzoom"]; ok {
			lp.maxZoom = asFloat(v, 24)
		}
		parsed.layers = append(parsed.layers, lp)
	}
	return parsed, nil
}

func paintOFMTile(parsed *ofmParsed, layers mvt.Layers, zoom int) *image.RGBA {
	byName := map[string]*mvt.Layer{}
	for _, layer := range layers {
		byName[layer.Name] = layer
	}
	dst := image.NewRGBA(image.Rect(0, 0, ofmTilePx, ofmTilePx))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: parsed.bg}, image.Point{}, draw.Src)
	ras := vector.NewRasterizer(ofmTilePx, ofmTilePx)
	z := float64(zoom)
	for _, layer := range parsed.layers {
		if z < layer.minZoom || z >= layer.maxZoom {
			continue
		}
		tileLayer := byName[layer.sourceLayer]
		if tileLayer == nil || len(tileLayer.Features) == 0 || tileLayer.Extent == 0 {
			continue
		}
		col, ok := evalColor(layer.color, z)
		if !ok {
			continue
		}
		col = withOpacity(col, evalOpacity(layer.opacity, z))
		if col.A == 0 {
			continue
		}
		width := 1.0
		if layer.kind == "line" {
			w, ok := evalNumber(layer.width, z)
			if !ok {
				w = 1
			}
			if w <= 0.2 {
				continue
			}
			width = w
		}
		outline, hasOutline := evalColor(layer.outline, z)
		if hasOutline {
			outline = withOpacity(outline, evalOpacity(layer.opacity, z))
			hasOutline = outline.A > 0
		}
		scale := float64(ofmTilePx) / float64(tileLayer.Extent)
		for _, feature := range tileLayer.Features {
			if feature == nil || feature.Geometry == nil {
				continue
			}
			if !featureMatches(layer.filter, feature.Properties, mvtGeomType(feature.Geometry)) {
				continue
			}
			paintGeom(dst, ras, feature.Geometry, scale, layer.kind, col, width, outline, hasOutline)
		}
	}
	return dst
}

func decodeMVT(data []byte) (mvt.Layers, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		return mvt.UnmarshalGzipped(data)
	}
	layers, err := mvt.Unmarshal(data)
	if err == mvt.ErrDataIsGZipped {
		return mvt.UnmarshalGzipped(data)
	}
	return layers, err
}

func paintGeom(dst *image.RGBA, ras *vector.Rasterizer, g orb.Geometry, scale float64, kind string, col color.RGBA, width float64, outline color.RGBA, hasOutline bool) {
	switch g := g.(type) {
	case orb.Polygon:
		if kind == "line" {
			for _, ring := range g {
				strokeRing(dst, ring, scale, width, col)
			}
			return
		}
		ofmFillPolygon(dst, ras, g, scale, col)
		if hasOutline {
			for _, ring := range g {
				strokeRing(dst, ring, scale, 1, outline)
			}
		}
	case orb.MultiPolygon:
		for _, poly := range g {
			paintGeom(dst, ras, poly, scale, kind, col, width, outline, hasOutline)
		}
	case orb.LineString:
		if kind == "line" {
			strokeRing(dst, orb.Ring(g), scale, width, col)
		}
	case orb.MultiLineString:
		for _, line := range g {
			paintGeom(dst, ras, line, scale, kind, col, width, outline, hasOutline)
		}
	}
}

func ofmFillPolygon(dst *image.RGBA, ras *vector.Rasterizer, poly orb.Polygon, scale float64, col color.RGBA) {
	ras.Reset(dst.Bounds().Dx(), dst.Bounds().Dy())
	anyRing := false
	for _, ring := range poly {
		if len(ring) < 3 {
			continue
		}
		ras.MoveTo(float32(ring[0][0]*scale), float32(ring[0][1]*scale))
		for _, p := range ring[1:] {
			ras.LineTo(float32(p[0]*scale), float32(p[1]*scale))
		}
		ras.ClosePath()
		anyRing = true
	}
	if anyRing {
		ras.Draw(dst, dst.Bounds(), &image.Uniform{C: col}, image.Point{})
	}
}

func strokeRing(dst *image.RGBA, ring orb.Ring, scale, width float64, col color.RGBA) {
	if len(ring) < 2 {
		return
	}
	for i := 1; i < len(ring); i++ {
		strokeSegment(dst, ring[i-1][0]*scale, ring[i-1][1]*scale, ring[i][0]*scale, ring[i][1]*scale, width, col)
	}
}

func strokeSegment(dst *image.RGBA, x0, y0, x1, y1, width float64, col color.RGBA) {
	x0i := int(math.Round(x0))
	y0i := int(math.Round(y0))
	x1i := int(math.Round(x1))
	y1i := int(math.Round(y1))
	dx := x1i - x0i
	dy := y1i - y0i
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	sx, sy := 1, 1
	if x0i > x1i {
		sx = -1
	}
	if y0i > y1i {
		sy = -1
	}
	err := dx - dy
	half := int(math.Round(width / 2))
	if half < 0 {
		half = 0
	}
	b := dst.Bounds()
	for {
		for oy := -half; oy <= half; oy++ {
			yy := y0i + oy
			if yy < b.Min.Y || yy >= b.Max.Y {
				continue
			}
			for ox := -half; ox <= half; ox++ {
				xx := x0i + ox
				if xx < b.Min.X || xx >= b.Max.X {
					continue
				}
				blendPixel(dst, xx, yy, col)
			}
		}
		if x0i == x1i && y0i == y1i {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0i += sx
		}
		if e2 < dx {
			err += dx
			y0i += sy
		}
	}
}

func blendPixel(dst *image.RGBA, x, y int, c color.RGBA) {
	i := dst.PixOffset(x, y)
	if c.A == 255 {
		dst.Pix[i] = c.R
		dst.Pix[i+1] = c.G
		dst.Pix[i+2] = c.B
		dst.Pix[i+3] = 255
		return
	}
	inv := uint32(255 - c.A)
	a := uint32(c.A)
	dst.Pix[i] = uint8((uint32(c.R)*a + uint32(dst.Pix[i])*inv) / 255)
	dst.Pix[i+1] = uint8((uint32(c.G)*a + uint32(dst.Pix[i+1])*inv) / 255)
	dst.Pix[i+2] = uint8((uint32(c.B)*a + uint32(dst.Pix[i+2])*inv) / 255)
	dst.Pix[i+3] = 255
}

func mvtGeomType(g orb.Geometry) string {
	switch g.(type) {
	case orb.Point, orb.MultiPoint:
		return "Point"
	case orb.LineString, orb.MultiLineString:
		return "LineString"
	case orb.Polygon, orb.MultiPolygon:
		return "Polygon"
	default:
		return ""
	}
}

func featureMatches(filter any, props geojson.Properties, geomType string) bool {
	if filter == nil {
		return true
	}
	list, ok := filter.([]any)
	if !ok || len(list) == 0 {
		return !ok
	}
	op, _ := list[0].(string)
	switch op {
	case "all":
		for _, item := range list[1:] {
			if !featureMatches(item, props, geomType) {
				return false
			}
		}
		return true
	case "any":
		for _, item := range list[1:] {
			if featureMatches(item, props, geomType) {
				return true
			}
		}
		return false
	case "!":
		if len(list) < 2 {
			return false
		}
		return !featureMatches(list[1], props, geomType)
	case "==":
		if len(list) < 3 {
			return false
		}
		return valuesEqual(resolveExpr(list[1], props, geomType), resolveExpr(list[2], props, geomType))
	case "!=":
		if len(list) < 3 {
			return false
		}
		return !valuesEqual(resolveExpr(list[1], props, geomType), resolveExpr(list[2], props, geomType))
	case "has":
		key := propertyName(list, 1)
		if key == "" || props == nil {
			return false
		}
		v, ok := props[key]
		return ok && v != nil
	case "in":
		if len(list) < 3 {
			return false
		}
		needle := resolveExpr(list[1], props, geomType)
		for _, item := range list[2:] {
			if valuesEqual(needle, resolveExpr(item, props, geomType)) {
				return true
			}
		}
		return false
	case "match":
		return matchExpr(list, props, geomType)
	default:
		return false
	}
}

func matchExpr(list []any, props geojson.Properties, geomType string) bool {
	if len(list) < 3 {
		return false
	}
	input := resolveExpr(list[1], props, geomType)
	last := len(list) - 1
	i := 2
	for i < last {
		if labelMatches(list[i], input, props, geomType) {
			return ofmTruthy(list[i+1], props, geomType)
		}
		i += 2
	}
	if i == last {
		return ofmTruthy(list[last], props, geomType)
	}
	return false
}

func labelMatches(label, input any, props geojson.Properties, geomType string) bool {
	if labels, ok := label.([]any); ok {
		for _, item := range labels {
			if valuesEqual(input, resolveExpr(item, props, geomType)) {
				return true
			}
		}
		return false
	}
	return valuesEqual(input, label)
}

func ofmTruthy(v any, props geojson.Properties, geomType string) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t != "" && !strings.EqualFold(t, "false")
	case []any:
		return featureMatches(t, props, geomType)
	default:
		return v != nil
	}
}

func resolveExpr(v any, props geojson.Properties, geomType string) any {
	list, ok := v.([]any)
	if !ok {
		return v
	}
	if len(list) == 0 {
		return nil
	}
	op, _ := list[0].(string)
	switch op {
	case "get":
		if len(list) < 2 || props == nil {
			return nil
		}
		key, _ := list[1].(string)
		return props[key]
	case "geometry-type":
		return geomType
	case "literal":
		if len(list) < 2 {
			return nil
		}
		return list[1]
	default:
		return nil
	}
}

func propertyName(list []any, index int) string {
	if len(list) <= index {
		return ""
	}
	if s, ok := list[index].(string); ok {
		return s
	}
	inner, ok := list[index].([]any)
	if ok && len(inner) > 1 && inner[0] == "get" {
		s, _ := inner[1].(string)
		return s
	}
	return ""
}

func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	af, aok := asNumber(a)
	bf, bok := asNumber(b)
	if aok && bok {
		return af == bf
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func evalColor(expr any, zoom float64) (color.RGBA, bool) {
	switch t := expr.(type) {
	case string:
		return parseCSSColor(t)
	case []any:
		if len(t) < 5 || t[0] != "interpolate" {
			return color.RGBA{}, false
		}
		var prev color.RGBA
		var prevZ float64
		have := false
		for i := 3; i+1 < len(t); i += 2 {
			z, ok := asNumber(t[i])
			if !ok {
				return color.RGBA{}, false
			}
			c, ok := evalColor(t[i+1], zoom)
			if !ok {
				return color.RGBA{}, false
			}
			if zoom <= z {
				if !have {
					return c, true
				}
				span := z - prevZ
				if span == 0 {
					return c, true
				}
				return lerpColor(prev, c, (zoom-prevZ)/span), true
			}
			prev, prevZ, have = c, z, true
		}
		if have {
			return prev, true
		}
		return color.RGBA{}, false
	default:
		return color.RGBA{}, false
	}
}

func evalNumber(expr any, zoom float64) (float64, bool) {
	if expr == nil {
		return 0, false
	}
	if n, ok := asNumber(expr); ok {
		return n, true
	}
	list, ok := expr.([]any)
	if !ok || len(list) < 5 || list[0] != "interpolate" {
		return 0, false
	}
	var prev, prevZ float64
	have := false
	for i := 3; i+1 < len(list); i += 2 {
		z, zok := asNumber(list[i])
		v, vok := asNumber(list[i+1])
		if !zok || !vok {
			return 0, false
		}
		if zoom <= z {
			if !have {
				return v, true
			}
			span := z - prevZ
			if span == 0 {
				return v, true
			}
			t := (zoom - prevZ) / span
			return prev + t*(v-prev), true
		}
		prev, prevZ, have = v, z, true
	}
	if have {
		return prev, true
	}
	return 0, false
}

func evalOpacity(expr any, zoom float64) float64 {
	if expr == nil {
		return 1
	}
	n, ok := evalNumber(expr, zoom)
	if !ok {
		return 1
	}
	if n < 0 {
		return 0
	}
	if n > 1 {
		return 1
	}
	return n
}

func withOpacity(c color.RGBA, opacity float64) color.RGBA {
	c.A = uint8(math.Round(float64(c.A) * opacity))
	return c
}

func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	mix := func(x, y uint8) uint8 {
		return uint8(math.Round(float64(x) + t*(float64(y)-float64(x))))
	}
	return color.RGBA{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), mix(a.A, b.A)}
}

func parseCSSColor(raw string) (color.RGBA, bool) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" || s == "transparent" {
		return color.RGBA{}, false
	}
	if strings.HasPrefix(s, "#") {
		h := strings.TrimPrefix(s, "#")
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) != 6 && len(h) != 8 {
			return color.RGBA{}, false
		}
		n, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return color.RGBA{}, false
		}
		if len(h) == 6 {
			return color.RGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 255}, true
		}
		return color.RGBA{uint8(n >> 24), uint8(n >> 16), uint8(n >> 8), uint8(n)}, true
	}
	if strings.HasPrefix(s, "rgb") {
		open := strings.IndexByte(s, '(')
		close := strings.LastIndexByte(s, ')')
		if open < 0 || close < open {
			return color.RGBA{}, false
		}
		parts := strings.FieldsFunc(s[open+1:close], func(r rune) bool {
			return r == ',' || r == '/' || r == ' '
		})
		parts = compact(parts)
		if len(parts) < 3 {
			return color.RGBA{}, false
		}
		r, ok1 := channel(parts[0])
		g, ok2 := channel(parts[1])
		b, ok3 := channel(parts[2])
		if !ok1 || !ok2 || !ok3 {
			return color.RGBA{}, false
		}
		a := uint8(255)
		if len(parts) > 3 {
			af, err := strconv.ParseFloat(strings.TrimSuffix(parts[3], "%"), 64)
			if err != nil {
				return color.RGBA{}, false
			}
			if strings.HasSuffix(parts[3], "%") {
				af = af / 100
			}
			if af <= 1 {
				a = uint8(math.Round(af * 255))
			} else {
				a = uint8(math.Round(af))
			}
		}
		return color.RGBA{r, g, b, a}, true
	}
	return color.RGBA{}, false
}

func channel(s string) (uint8, bool) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return 0, false
		}
		return uint8(math.Round(f / 100 * 255)), true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return uint8(math.Round(f)), true
}

func compact(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func ofmAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, openFreeMapHost) {
		return false
	}
	q := strings.ToLower(u.RawQuery)
	for _, secret := range []string{"api_key=", "apikey=", "access_token=", "token=", "key="} {
		if strings.Contains(q, secret) {
			return false
		}
	}
	return true
}

func ofmGet(ctx context.Context, rawURL, accept string) ([]byte, error) {
	if !ofmAllowed(rawURL) {
		return nil, fmt.Errorf("rejected basemap url")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AcuGIS-Report-Server/1.0 (embedded-map-basemap)")
	req.Header.Set("Accept", accept)
	client := &http.Client{Timeout: basemapHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openfreemap HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b && !strings.Contains(rawURL, ".pbf") {
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return body, nil
		}
		defer zr.Close()
		decoded, err := io.ReadAll(io.LimitReader(zr, 8<<20))
		if err != nil {
			return body, nil
		}
		return decoded, nil
	}
	return body, nil
}

func clampZoom(z int) int {
	if z < 0 {
		return 0
	}
	if z > 22 {
		return 22
	}
	return z
}
