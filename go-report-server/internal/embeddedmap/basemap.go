package embeddedmap

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Basemap ids for the embedded-map editor and compositor.
// Legacy stored ids carto_light and carto_dark stay readable and render as
// OpenFreeMap. NormalizeBasemap does not rewrite those stored ids.
const (
	BasemapNone             = "none"
	BasemapOSM              = "osm"
	BasemapOpenFreeMapLight = "openfreemap_light"
	BasemapOpenFreeMapDark  = "openfreemap_dark"
	BasemapEsriStreets      = "esri_streets"
	BasemapEsriImagery      = "esri_imagery"

	storedCartoLight = "carto_light"
	storedCartoDark  = "carto_dark"
)

// BasemapOptions are labels for the embedded-map edit UI.
var BasemapOptions = []struct {
	Key   string
	Label string
}{
	{BasemapNone, "None"},
	{BasemapOSM, "OpenStreetMap"},
	{BasemapOpenFreeMapLight, "OpenFreeMap Light"},
	{BasemapOpenFreeMapDark, "OpenFreeMap Dark"},
	{BasemapEsriStreets, "Esri Streets"},
	{BasemapEsriImagery, "Esri Imagery"},
}

const (
	basemapTileSize    = 256
	basemapMaxTiles    = 48
	basemapHTTPTimeout = 20 * time.Second
	mercatorMax        = 20037508.342789244
)

type basemapProvider struct {
	URLTemplate string
	VectorStyle string
}

var basemapProviders = map[string]basemapProvider{
	BasemapOSM: {
		URLTemplate: "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
	},
	BasemapOpenFreeMapLight: {
		VectorStyle: openFreeMapLightStyle,
	},
	BasemapOpenFreeMapDark: {
		VectorStyle: openFreeMapDarkStyle,
	},
	BasemapEsriStreets: {
		URLTemplate: "https://server.arcgisonline.com/ArcGIS/rest/services/World_Topo_Map/MapServer/tile/{z}/{y}/{x}",
	},
	BasemapEsriImagery: {
		URLTemplate: "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}",
	},
}

func foldBasemap(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ReplaceAll(v, " ", "_")
	return v
}

// NormalizeBasemap maps aliases onto the id stored for a definition.
// Legacy CARTO ids are preserved so reading a saved map does not rewrite it.
func NormalizeBasemap(v string) string {
	switch foldBasemap(v) {
	case "", BasemapNone, "off", "solid", "false":
		return BasemapNone
	case BasemapOSM, "openstreetmap", "open_street_map":
		return BasemapOSM
	case storedCartoLight, "carto", "light", "light_all":
		return storedCartoLight
	case BasemapOpenFreeMapLight, "openfreemap_positron", "positron":
		return BasemapOpenFreeMapLight
	case storedCartoDark, "dark", "dark_all", "dark_matter":
		return storedCartoDark
	case BasemapOpenFreeMapDark:
		return BasemapOpenFreeMapDark
	case BasemapEsriStreets, "streets", "terrain", "topo", "esri_topo":
		return BasemapEsriStreets
	case BasemapEsriImagery, "satellite", "imagery", "esri_sat":
		return BasemapEsriImagery
	default:
		return BasemapNone
	}
}

// RenderBasemap is the provider key used when painting.
// carto_light and carto_dark resolve to OpenFreeMap without changing stored data.
func RenderBasemap(v string) string {
	switch NormalizeBasemap(v) {
	case storedCartoLight, BasemapOpenFreeMapLight:
		return BasemapOpenFreeMapLight
	case storedCartoDark, BasemapOpenFreeMapDark:
		return BasemapOpenFreeMapDark
	default:
		return NormalizeBasemap(v)
	}
}

// SameBasemap reports whether two ids paint the same basemap.
// Used by the editor so a stored carto_light value shows OpenFreeMap Light.
func SameBasemap(a, b string) bool {
	return RenderBasemap(a) == RenderBasemap(b)
}

// HasBasemap reports whether tiles should be fetched.
func HasBasemap(id string) bool {
	_, ok := basemapProviders[RenderBasemap(id)]
	return ok
}

// compositeBasemapUnderPNG draws basemap tiles then overlays a PNG (e.g. transparent WMS).
func compositeBasemapUnderPNG(ctx context.Context, emb Definition, overlayPNG []byte, extent4326 bbox, width, height int) ([]byte, error) {
	if !HasBasemap(emb.Basemap) {
		return overlayPNG, nil
	}
	merc := bbox{
		minX: lonToMercX(extent4326.minX),
		minY: latToMercY(extent4326.minY),
		maxX: lonToMercX(extent4326.maxX),
		maxY: latToMercY(extent4326.maxY),
	}
	if merc.minX > merc.maxX {
		merc.minX, merc.maxX = merc.maxX, merc.minX
	}
	if merc.minY > merc.maxY {
		merc.minY, merc.maxY = merc.maxY, merc.minY
	}
	return compositeBasemapUnderPNGMerc(ctx, emb, overlayPNG, wmsBBox{merc.minX, merc.minY, merc.maxX, merc.maxY}, width, height)
}

// compositeBasemapUnderPNGMerc composites XYZ tiles for a Web Mercator extent under overlayPNG.
func compositeBasemapUnderPNGMerc(ctx context.Context, emb Definition, overlayPNG []byte, merc wmsBBox, width, height int) ([]byte, error) {
	if !HasBasemap(emb.Basemap) {
		return overlayPNG, nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	bg := parseColor(emb.Background)
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)
	drawBasemapTilesMerc(ctx, dst, bbox{merc.MinX, merc.MinY, merc.MaxX, merc.MaxY}, width, height, 0, 0, emb.Basemap)

	overlay, err := png.Decode(bytes.NewReader(overlayPNG))
	if err != nil {
		return overlayPNG, nil
	}
	if overlay.Bounds().Dx() != width || overlay.Bounds().Dy() != height {
		overlay = resizeNearest(overlay, width, height)
	}
	draw.Draw(dst, dst.Bounds(), overlay, image.Point{}, draw.Over)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return overlayPNG, nil
	}
	return buf.Bytes(), nil
}

// drawBasemapTiles paints XYZ tiles for a lon/lat extent into the map region.
func drawBasemapTiles(ctx context.Context, dst *image.RGBA, ext bbox, mapW, mapH, offX, offY int, basemap string) {
	merc := bbox{
		minX: lonToMercX(ext.minX),
		minY: latToMercY(ext.minY),
		maxX: lonToMercX(ext.maxX),
		maxY: latToMercY(ext.maxY),
	}
	if merc.minX > merc.maxX {
		merc.minX, merc.maxX = merc.maxX, merc.minX
	}
	if merc.minY > merc.maxY {
		merc.minY, merc.maxY = merc.maxY, merc.minY
	}
	drawBasemapTilesMerc(ctx, dst, merc, mapW, mapH, offX, offY, basemap)
}

// drawBasemapTilesMerc paints XYZ tiles for a Web Mercator extent.
func drawBasemapTilesMerc(ctx context.Context, dst *image.RGBA, merc bbox, mapW, mapH, offX, offY int, basemap string) {
	basemap = RenderBasemap(basemap)
	prov, ok := basemapProviders[basemap]
	if !ok || dst == nil || mapW < 1 || mapH < 1 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dx := merc.maxX - merc.minX
	dy := merc.maxY - merc.minY
	if dx <= 0 {
		dx = 1
	}
	if dy <= 0 {
		dy = 1
	}
	scale := math.Min(float64(mapW)/dx, float64(mapH)/dy)
	usedW := dx * scale
	usedH := dy * scale
	padX := (float64(mapW) - usedW) / 2
	padY := (float64(mapH) - usedH) / 2

	z := basemapZoom(dx, usedW)
	if prov.VectorStyle != "" {
		prepareOpenFreeMap(ctx, prov.VectorStyle)
		if mz := openFreeMapMaxZoom(prov.VectorStyle); z > mz {
			z = mz
		}
	}
	n := 1 << z
	tileSpan := (2 * mercatorMax) / float64(n)

	x0 := int(math.Floor((merc.minX + mercatorMax) / tileSpan))
	x1 := int(math.Floor((merc.maxX + mercatorMax) / tileSpan))
	y0 := int(math.Floor((mercatorMax - merc.maxY) / tileSpan))
	y1 := int(math.Floor((mercatorMax - merc.minY) / tileSpan))
	x0, x1, y0, y1 = clampTileRange(x0, x1, y0, y1, n)
	if x1 < x0 || y1 < y0 {
		return
	}
	for (x1-x0+1)*(y1-y0+1) > basemapMaxTiles && z > 0 {
		z--
		n = 1 << z
		tileSpan = (2 * mercatorMax) / float64(n)
		x0 = int(math.Floor((merc.minX + mercatorMax) / tileSpan))
		x1 = int(math.Floor((merc.maxX + mercatorMax) / tileSpan))
		y0 = int(math.Floor((mercatorMax - merc.maxY) / tileSpan))
		y1 = int(math.Floor((mercatorMax - merc.minY) / tileSpan))
		x0, x1, y0, y1 = clampTileRange(x0, x1, y0, y1, n)
	}

	type job struct{ x, y int }
	var jobs []job
	for ty := y0; ty <= y1; ty++ {
		for tx := x0; tx <= x1; tx++ {
			jobs = append(jobs, job{tx, ty})
		}
	}

	var mu sync.Mutex
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, j := range jobs {
		j := j
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var img image.Image
			var err error
			if prov.VectorStyle != "" {
				img, err = renderOpenFreeMapTile(ctx, prov.VectorStyle, z, j.x, j.y)
			} else {
				img, err = fetchBasemapTile(ctx, basemapTileURL(prov, z, j.x, j.y))
			}
			if err != nil || img == nil {
				return
			}
			tileMinX := -mercatorMax + float64(j.x)*tileSpan
			tileMaxY := mercatorMax - float64(j.y)*tileSpan
			tileMaxX := tileMinX + tileSpan
			tileMinY := tileMaxY - tileSpan

			dx0 := offX + int(padX+(tileMinX-merc.minX)*scale)
			dx1 := offX + int(padX+(tileMaxX-merc.minX)*scale)
			dy0 := offY + int(padY+(merc.maxY-tileMaxY)*scale)
			dy1 := offY + int(padY+(merc.maxY-tileMinY)*scale)
			if dx1 <= dx0 || dy1 <= dy0 {
				return
			}
			resized := resizeNearest(img, dx1-dx0, dy1-dy0)
			mu.Lock()
			draw.Draw(dst, image.Rect(dx0, dy0, dx1, dy1), resized, image.Point{}, draw.Over)
			mu.Unlock()
		}()
	}
	wg.Wait()
}

func clampTileRange(x0, x1, y0, y1, n int) (int, int, int, int) {
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 >= n {
		x1 = n - 1
	}
	if y1 >= n {
		y1 = n - 1
	}
	return x0, x1, y0, y1
}

func basemapZoom(mercWidth, pixelWidth float64) int {
	if mercWidth <= 0 || pixelWidth <= 0 {
		return 2
	}
	z := int(math.Floor(math.Log2((pixelWidth * 2 * mercatorMax) / (mercWidth * float64(basemapTileSize)))))
	if z < 0 {
		z = 0
	}
	if z > 18 {
		z = 18
	}
	return z
}

func basemapTileURL(p basemapProvider, z, x, y int) string {
	u := p.URLTemplate
	u = strings.ReplaceAll(u, "{z}", fmt.Sprintf("%d", z))
	u = strings.ReplaceAll(u, "{x}", fmt.Sprintf("%d", x))
	u = strings.ReplaceAll(u, "{y}", fmt.Sprintf("%d", y))
	return u
}

func fetchBasemapTile(ctx context.Context, tileURL string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AcuGIS-Report-Server/1.0 (embedded-map-basemap)")
	client := &http.Client{Timeout: basemapHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tile HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return img, nil
}

func lonToMercX(lon float64) float64 {
	return lon * mercatorMax / 180.0
}

func latToMercY(lat float64) float64 {
	if lat > 85.05112878 {
		lat = 85.05112878
	}
	if lat < -85.05112878 {
		lat = -85.05112878
	}
	return math.Log(math.Tan((90.0+lat)*math.Pi/360.0)) * mercatorMax / math.Pi
}

// newMercatorProjector projects lon/lat features onto a Web Mercator viewport
// (same transform as drawBasemapTiles).
func newMercatorProjector(ext4326 bbox, w, h, offX, offY int) projector {
	merc := bbox{
		minX: lonToMercX(ext4326.minX),
		minY: latToMercY(ext4326.minY),
		maxX: lonToMercX(ext4326.maxX),
		maxY: latToMercY(ext4326.maxY),
	}
	if merc.minX > merc.maxX {
		merc.minX, merc.maxX = merc.maxX, merc.minX
	}
	if merc.minY > merc.maxY {
		merc.minY, merc.maxY = merc.maxY, merc.minY
	}
	dx := merc.maxX - merc.minX
	dy := merc.maxY - merc.minY
	if dx <= 0 {
		dx = 1
	}
	if dy <= 0 {
		dy = 1
	}
	scale := math.Min(float64(w)/dx, float64(h)/dy)
	return projector{
		extent:   merc,
		w:        w,
		h:        h,
		offX:     offX,
		offY:     offY,
		scale:    scale,
		mercator: true,
	}
}
