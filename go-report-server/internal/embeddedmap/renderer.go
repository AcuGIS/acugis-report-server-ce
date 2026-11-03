package embeddedmap

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"
)

// RenderPNG draws styled GeoJSON features to a PNG.
func RenderPNG(thematic ThematicResult, emb Definition) ([]byte, error) {
	return RenderPNGContext(context.Background(), thematic, emb)
}

// RenderPNGContext is RenderPNG with a cancellable context for basemap tile fetches.
func RenderPNGContext(ctx context.Context, thematic ThematicResult, emb Definition) ([]byte, error) {
	w, h := emb.Width, emb.Height
	if w < 40 {
		w = 40
	}
	if h < 40 {
		h = 40
	}
	legendW := 0
	if emb.IncludeLegend && len(thematic.Legend) > 0 {
		legendW = 140
	}
	titleH := 0
	if emb.ShowTitle && thematic.Title != "" {
		titleH = 22
	}

	mapW := w - legendW
	mapH := h - titleH
	if mapW < 40 {
		mapW = 40
	}
	if mapH < 40 {
		mapH = 40
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := parseColor(emb.Background)
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	zoomFeats := thematic.ZoomFeatures
	if !emb.ZoomToFiltered || len(zoomFeats) == 0 {
		zoomFeats = append(featureMaps(thematic.ContextFeatures), featureMaps(thematic.Features)...)
	}
	extent := computeExtent(zoomFeats)
	if extent.empty() {
		extent = bbox{-180, -90, 180, 90}
	}
	pad := emb.Padding
	if pad < 0 {
		pad = 0
	}
	if pad > 0.4 {
		pad = 0.4
	}
	extent = extent.pad(pad)

	var proj projector
	if HasBasemap(emb.Basemap) {
		drawBasemapTiles(ctx, img, extent, mapW, mapH, 0, titleH, emb.Basemap)
		proj = newMercatorProjector(extent, mapW, mapH, 0, titleH)
	} else {
		proj = newProjector(extent, mapW, mapH, 0, titleH)
	}

	// Context (gray) under selection.
	for _, sf := range thematic.ContextFeatures {
		drawFeature(img, sf, proj)
	}
	for _, sf := range thematic.Features {
		drawFeature(img, sf, proj)
	}

	if titleH > 0 {
		drawString(img, 8, 14, thematic.Title, color.RGBA{R: 30, G: 30, B: 30, A: 255})
	}
	if legendW > 0 {
		drawLegend(img, mapW+8, titleH+8, thematic.Legend)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type bbox struct{ minX, minY, maxX, maxY float64 }

func (b bbox) empty() bool {
	return b.minX > b.maxX || b.minY > b.maxY
}

func (b bbox) pad(p float64) bbox {
	dx := (b.maxX - b.minX) * p
	dy := (b.maxY - b.minY) * p
	if dx == 0 {
		dx = 0.01
	}
	if dy == 0 {
		dy = 0.01
	}
	return bbox{b.minX - dx, b.minY - dy, b.maxX + dx, b.maxY + dy}
}

type projector struct {
	extent        bbox
	w, h          int
	offX, offY    int
	scale, scaleX float64
	scaleY        float64
	// mercator: when true, xy() converts lon/lat to Web Mercator before projecting.
	mercator bool
}

func newProjector(ext bbox, w, h, offX, offY int) projector {
	dx := ext.maxX - ext.minX
	dy := ext.maxY - ext.minY
	if dx <= 0 {
		dx = 1
	}
	if dy <= 0 {
		dy = 1
	}
	sx := float64(w) / dx
	sy := float64(h) / dy
	scale := math.Min(sx, sy)
	return projector{
		extent: ext, w: w, h: h, offX: offX, offY: offY,
		scale: scale, scaleX: sx, scaleY: sy, mercator: false,
	}
}

func (p projector) xy(lon, lat float64) (int, int) {
	xVal, yVal := lon, lat
	if p.mercator {
		xVal = lonToMercX(lon)
		yVal = latToMercY(lat)
	}
	// Center content when aspect differs.
	dx := p.extent.maxX - p.extent.minX
	dy := p.extent.maxY - p.extent.minY
	usedW := dx * p.scale
	usedH := dy * p.scale
	padX := (float64(p.w) - usedW) / 2
	padY := (float64(p.h) - usedH) / 2
	x := p.offX + int(padX+(xVal-p.extent.minX)*p.scale)
	y := p.offY + int(padY+(p.extent.maxY-yVal)*p.scale)
	return x, y
}

func featureMaps(sfs []StyledFeature) []map[string]any {
	out := make([]map[string]any, 0, len(sfs))
	for _, sf := range sfs {
		out = append(out, sf.Feature)
	}
	return out
}

func computeExtent(features []map[string]any) bbox {
	b := bbox{minX: 1e9, minY: 1e9, maxX: -1e9, maxY: -1e9}
	found := false
	for _, f := range features {
		geom, _ := f["geometry"].(map[string]any)
		if geom == nil {
			continue
		}
		walkCoords(geom["coordinates"], func(lon, lat float64) {
			found = true
			if lon < b.minX {
				b.minX = lon
			}
			if lat < b.minY {
				b.minY = lat
			}
			if lon > b.maxX {
				b.maxX = lon
			}
			if lat > b.maxY {
				b.maxY = lat
			}
		})
	}
	if !found {
		return bbox{1, 1, -1, -1}
	}
	return b
}

func walkCoords(v any, fn func(lon, lat float64)) {
	switch t := v.(type) {
	case []any:
		if len(t) >= 2 {
			if _, ok := t[0].(float64); ok {
				fn(asFloat(t[0], 0), asFloat(t[1], 0))
				return
			}
		}
		for _, c := range t {
			walkCoords(c, fn)
		}
	}
}

func drawFeature(img *image.RGBA, sf StyledFeature, proj projector) {
	geom, _ := sf.Feature["geometry"].(map[string]any)
	if geom == nil {
		return
	}
	gtype := strings.ToLower(asString(geom["type"]))
	fill := parseColor(sf.Fill)
	stroke := parseColor(sf.Stroke)
	coords := geom["coordinates"]

	switch gtype {
	case "point":
		drawPoint(img, coords, sf.Size, fill, stroke, proj, NormalizePointShape(""))
	case "multipoint":
		if arr, ok := coords.([]any); ok {
			for _, c := range arr {
				drawPoint(img, c, sf.Size, fill, stroke, proj, "circle")
			}
		}
	case "linestring":
		drawLine(img, coords, stroke, proj)
	case "multilinestring":
		if arr, ok := coords.([]any); ok {
			for _, c := range arr {
				drawLine(img, c, stroke, proj)
			}
		}
	case "polygon":
		drawPolygon(img, coords, fill, stroke, proj)
	case "multipolygon":
		if arr, ok := coords.([]any); ok {
			for _, c := range arr {
				drawPolygon(img, c, fill, stroke, proj)
			}
		}
	}
}

func drawPoint(img *image.RGBA, coords any, size float64, fill, stroke color.RGBA, proj projector, shape string) {
	arr, ok := coords.([]any)
	if !ok || len(arr) < 2 {
		return
	}
	x, y := proj.xy(asFloat(arr[0], 0), asFloat(arr[1], 0))
	r := int(size)
	if r < 2 {
		r = 2
	}
	_ = shape
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				set(img, x+dx, y+dy, fill)
			} else if dx*dx+dy*dy <= (r+1)*(r+1) {
				set(img, x+dx, y+dy, stroke)
			}
		}
	}
}

func drawLine(img *image.RGBA, coords any, stroke color.RGBA, proj projector) {
	pts := ringPoints(coords, proj)
	for i := 1; i < len(pts); i++ {
		line(img, pts[i-1][0], pts[i-1][1], pts[i][0], pts[i][1], stroke)
	}
}

func drawPolygon(img *image.RGBA, coords any, fill, stroke color.RGBA, proj projector) {
	rings, ok := coords.([]any)
	if !ok || len(rings) == 0 {
		return
	}
	outer := ringPoints(rings[0], proj)
	if len(outer) < 3 {
		return
	}
	fillPolygon(img, outer, fill)
	for i := 1; i < len(outer); i++ {
		line(img, outer[i-1][0], outer[i-1][1], outer[i][0], outer[i][1], stroke)
	}
	line(img, outer[len(outer)-1][0], outer[len(outer)-1][1], outer[0][0], outer[0][1], stroke)
}

func ringPoints(coords any, proj projector) [][2]int {
	arr, ok := coords.([]any)
	if !ok {
		return nil
	}
	var pts [][2]int
	for _, c := range arr {
		pair, ok := c.([]any)
		if !ok || len(pair) < 2 {
			continue
		}
		x, y := proj.xy(asFloat(pair[0], 0), asFloat(pair[1], 0))
		pts = append(pts, [2]int{x, y})
	}
	return pts
}

func fillPolygon(img *image.RGBA, pts [][2]int, col color.RGBA) {
	if len(pts) < 3 {
		return
	}
	minY, maxY := pts[0][1], pts[0][1]
	for _, p := range pts {
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	b := img.Bounds()
	if minY < b.Min.Y {
		minY = b.Min.Y
	}
	if maxY >= b.Max.Y {
		maxY = b.Max.Y - 1
	}
	n := len(pts)
	for y := minY; y <= maxY; y++ {
		var xs []int
		j := n - 1
		for i := 0; i < n; i++ {
			yi, yj := pts[i][1], pts[j][1]
			xi, xj := pts[i][0], pts[j][0]
			if (yi > y) != (yj > y) {
				x := xi + (y-yi)*(xj-xi)/(yj-yi)
				xs = append(xs, x)
			}
			j = i
		}
		// sort xs
		for i := 1; i < len(xs); i++ {
			for k := i; k > 0 && xs[k] < xs[k-1]; k-- {
				xs[k], xs[k-1] = xs[k-1], xs[k]
			}
		}
		for i := 0; i+1 < len(xs); i += 2 {
			for x := xs[i]; x <= xs[i+1]; x++ {
				set(img, x, y, col)
			}
		}
	}
}

func line(img *image.RGBA, x0, y0, x1, y1 int, col color.RGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		set(img, x0, y0, col)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func set(img *image.RGBA, x, y int, c color.RGBA) {
	if !image.Pt(x, y).In(img.Bounds()) {
		return
	}
	img.SetRGBA(x, y, c)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func parseColor(s string) color.RGBA {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "transparent") {
		return color.RGBA{R: 255, G: 255, B: 255, A: 0}
	}
	r, g, b := parseHex(s)
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
}

func drawLegend(img *image.RGBA, x, y int, legend []LegendEntry) {
	row := y
	for _, e := range legend {
		col := parseColor(e.Color)
		for dy := 0; dy < 12; dy++ {
			for dx := 0; dx < 12; dx++ {
				set(img, x+dx, row+dy, col)
			}
		}
		drawString(img, x+18, row+10, e.Label, color.RGBA{R: 40, G: 40, B: 40, A: 255})
		row += 18
		if row > img.Bounds().Max.Y-10 {
			break
		}
	}
}

// Minimal 5x7 bitmap font for ASCII labels (title/legend).
func drawString(img *image.RGBA, x, y int, s string, col color.RGBA) {
	cx := x
	for _, ch := range s {
		glyph := fontGlyph(ch)
		for row := 0; row < 7; row++ {
			bits := glyph[row]
			for colBit := 0; colBit < 5; colBit++ {
				if bits&(1<<uint(4-colBit)) != 0 {
					set(img, cx+colBit, y-6+row, col)
				}
			}
		}
		cx += 6
		if cx > img.Bounds().Max.X-6 {
			break
		}
	}
}

func fontGlyph(ch rune) [7]byte {
	// Extremely small subset; unknown → blank.
	switch ch {
	case 'A', 'a':
		return [7]byte{0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11}
	case 'B', 'b':
		return [7]byte{0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E}
	case 'C', 'c':
		return [7]byte{0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E}
	case 'D', 'd':
		return [7]byte{0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E}
	case 'E', 'e':
		return [7]byte{0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F}
	case 'F', 'f':
		return [7]byte{0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10}
	case 'G', 'g':
		return [7]byte{0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0E}
	case 'H', 'h':
		return [7]byte{0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11}
	case 'I', 'i':
		return [7]byte{0x0E, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E}
	case 'L', 'l':
		return [7]byte{0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F}
	case 'M', 'm':
		return [7]byte{0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11}
	case 'N', 'n':
		return [7]byte{0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11}
	case 'O', 'o':
		return [7]byte{0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E}
	case 'P', 'p':
		return [7]byte{0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10}
	case 'R', 'r':
		return [7]byte{0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11}
	case 'S', 's':
		return [7]byte{0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E}
	case 'T', 't':
		return [7]byte{0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04}
	case 'U', 'u':
		return [7]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E}
	case 'V', 'v':
		return [7]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04}
	case 'W', 'w':
		return [7]byte{0x11, 0x11, 0x11, 0x15, 0x15, 0x1B, 0x11}
	case 'Y', 'y':
		return [7]byte{0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04}
	case '0':
		return [7]byte{0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E}
	case '1':
		return [7]byte{0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E}
	case '2':
		return [7]byte{0x0E, 0x11, 0x01, 0x06, 0x08, 0x10, 0x1F}
	case '3':
		return [7]byte{0x1F, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0E}
	case '4':
		return [7]byte{0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02}
	case '5':
		return [7]byte{0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E}
	case '6':
		return [7]byte{0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E}
	case '7':
		return [7]byte{0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08}
	case '8':
		return [7]byte{0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E}
	case '9':
		return [7]byte{0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C}
	case '.':
		return [7]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x0C, 0x0C}
	case '-':
		return [7]byte{0x00, 0x00, 0x00, 0x1F, 0x00, 0x00, 0x00}
	case ' ':
		return [7]byte{}
	case '_':
		return [7]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1F}
	default:
		return [7]byte{0x0E, 0x11, 0x01, 0x06, 0x04, 0x00, 0x04} // '?'
	}
}
