package embeddedmap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/geoserver"
)

const geoserverHTTPTimeout = 60 * time.Second

type wmsBBox struct {
	MinX, MinY, MaxX, MaxY float64
}

type geoServerAuth struct {
	User string
	Pass string
}

// renderGeoServerPNG fetches a WMS GetMap image and overlays optional chrome.
func (s *Service) renderGeoServerPNG(ctx context.Context, emb Definition, reportName string, queryParams map[string]any) ([]byte, error) {
	workspace := strings.TrimSpace(emb.GeoServerWorkspace)
	layer := strings.TrimSpace(emb.GeoServerLayer)
	if workspace == "" {
		return nil, fmt.Errorf("GeoServer Embedded Map requires a workspace")
	}
	if layer == "" {
		return nil, fmt.Errorf("GeoServer Embedded Map requires a layer name")
	}
	qualified := layer
	if !strings.Contains(layer, ":") {
		qualified = workspace + ":" + layer
	}

	baseRoot, auth, err := s.resolveGeoServerEndpoint(ctx, emb)
	if err != nil {
		return nil, err
	}
	wmsBase := strings.TrimRight(baseRoot, "/") + "/" + url.PathEscape(workspace) + "/wms"

	cql := BuildCQLFilter(emb.GeoServerFilterMappings, queryParams)
	mapBBox, crs := resolveGeoServerExtent(ctx, emb, wmsBase, qualified, cql, auth)

	width := emb.Width
	height := emb.Height
	if width < 16 {
		width = 16
	}
	if height < 16 {
		height = 16
	}

	useBasemap := HasBasemap(emb.Basemap)
	getMapCRS := "EPSG:4326"
	extent4326 := mapBBox
	if useBasemap {
		// Align WMS with XYZ tiles in Web Mercator.
		if math.Abs(mapBBox.MinX) <= 180 && math.Abs(mapBBox.MaxX) <= 180 {
			mapBBox = bboxTo3857(mapBBox)
		}
		mapBBox = fitBBoxToAspect(mapBBox, width, height)
		getMapCRS = "EPSG:3857"
	} else {
		if strings.Contains(strings.ToUpper(crs), "3857") || strings.Contains(strings.ToUpper(crs), "900913") ||
			math.Abs(mapBBox.MinX) > 180 || math.Abs(mapBBox.MaxX) > 180 {
			if math.Abs(mapBBox.MinX) <= 180 && math.Abs(mapBBox.MaxX) <= 180 {
				mapBBox = bboxTo3857(mapBBox)
			}
			getMapCRS = "EPSG:3857"
		}
		mapBBox = fitBBoxToAspect(mapBBox, width, height)
	}

	transparent := useBasemap || strings.EqualFold(strings.TrimSpace(emb.Background), "transparent")
	params := url.Values{}
	params.Set("SERVICE", "WMS")
	params.Set("VERSION", "1.1.1")
	params.Set("REQUEST", "GetMap")
	params.Set("LAYERS", qualified)
	params.Set("STYLES", emb.GeoServerStyle)
	params.Set("SRS", getMapCRS)
	params.Set("BBOX", bboxCSV(mapBBox))
	params.Set("WIDTH", strconv.Itoa(width))
	params.Set("HEIGHT", strconv.Itoa(height))
	params.Set("FORMAT", "image/png")
	if transparent {
		params.Set("TRANSPARENT", "true")
	} else {
		params.Set("TRANSPARENT", "false")
	}
	params.Set("EXCEPTIONS", "application/vnd.ogc.se_inimage")
	if !transparent && isHexColor(emb.Background) {
		hex := strings.ToUpper(strings.TrimPrefix(emb.Background, "#"))
		params.Set("BGCOLOR", "0x"+hex)
	}
	if cql != "" {
		params.Set("CQL_FILTER", cql)
	}

	pngBytes, err := httpGetPNG(ctx, wmsBase+"?"+params.Encode(), auth)
	if err != nil {
		return nil, fmt.Errorf("GeoServer GetMap: %w", err)
	}

	if useBasemap {
		if getMapCRS == "EPSG:3857" {
			pngBytes, err = compositeBasemapUnderPNGMerc(ctx, emb, pngBytes, mapBBox, width, height)
		} else {
			ext := bbox{minX: extent4326.MinX, minY: extent4326.MinY, maxX: extent4326.MaxX, maxY: extent4326.MaxY}
			pngBytes, err = compositeBasemapUnderPNG(ctx, emb, pngBytes, ext, width, height)
		}
		if err != nil {
			return nil, err
		}
	}

	var legendPNG []byte
	if emb.IncludeLegend {
		legendPNG, _ = fetchLegendGraphic(ctx, wmsBase, qualified, emb.GeoServerStyle, auth)
	}
	title := ""
	if emb.ShowTitle {
		title = strings.TrimSpace(emb.Title)
		if title == "" {
			title = strings.TrimSpace(reportName)
		}
	}
	return overlayMapChrome(pngBytes, title, legendPNG)
}

func (s *Service) resolveGeoServerEndpoint(ctx context.Context, emb Definition) (string, geoServerAuth, error) {
	root := strings.TrimRight(strings.TrimSpace(emb.GeoServerBaseURL), "/")
	auth := geoServerAuth{}
	if s != nil && s.GeoServer != nil && emb.GeoServerConnection != "" {
		if c, ok := findGeoServerConn(ctx, s.GeoServer, emb.GeoServerConnection); ok {
			if root == "" {
				root = strings.TrimRight(strings.TrimSpace(c.URL), "/")
			}
			auth.User = c.Username
			auth.Pass = c.Password
		}
	}
	if root == "" {
		return "", auth, fmt.Errorf("GeoServer URL is missing. Select a GeoServer connection (URL is saved with the report)")
	}
	return root, auth, nil
}

func findGeoServerConn(ctx context.Context, store geoserver.Store, name string) (geoserver.Connection, bool) {
	if store == nil || strings.TrimSpace(name) == "" {
		return geoserver.Connection{}, false
	}
	list, err := store.ListEnabled(ctx)
	if err != nil {
		return geoserver.Connection{}, false
	}
	for _, c := range list {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return geoserver.Connection{}, false
}

// SnapshotGeoServerBaseURL copies the live connection URL onto the definition when missing.
func SnapshotGeoServerBaseURL(ctx context.Context, store geoserver.Store, emb *Definition) {
	if emb == nil || emb.Renderer != RendererGeoServer {
		return
	}
	if strings.TrimSpace(emb.GeoServerBaseURL) != "" || strings.TrimSpace(emb.GeoServerConnection) == "" {
		return
	}
	if c, ok := findGeoServerConn(ctx, store, emb.GeoServerConnection); ok {
		emb.GeoServerBaseURL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	}
}

func resolveGeoServerExtent(ctx context.Context, emb Definition, wmsBase, qualified, cql string, auth geoServerAuth) (wmsBBox, string) {
	pad := emb.Padding
	if emb.ZoomToFiltered && cql != "" {
		if ext, ok := wfsFetchFeatureExtent(ctx, wfsBaseFromWMS(wmsBase), qualified, cql, auth); ok {
			return padBBox(ext, pad), "EPSG:4326"
		}
	}
	if ext, crs, ok := wmsFetchLayerExtent(ctx, wmsBase, qualified, auth); ok {
		return padBBox(ext, pad), crs
	}
	return padBBox(wmsBBox{-180, -85, 180, 85}, pad), "EPSG:4326"
}

func wfsBaseFromWMS(wmsBase string) string {
	wmsBase = strings.TrimRight(strings.TrimSpace(wmsBase), "/")
	if strings.HasSuffix(strings.ToLower(wmsBase), "/wms") {
		return wmsBase[:len(wmsBase)-4] + "/wfs"
	}
	if strings.HasSuffix(strings.ToLower(wmsBase), "/ows") {
		return wmsBase
	}
	return wmsBase + "/wfs"
}

func padBBox(b wmsBBox, padding float64) wmsBBox {
	if padding < 0 {
		padding = 0
	}
	if padding > 0.4 {
		padding = 0.4
	}
	dx := (b.MaxX - b.MinX) * padding
	dy := (b.MaxY - b.MinY) * padding
	if dx == 0 {
		dx = 0.01
	}
	if dy == 0 {
		dy = 0.01
	}
	return wmsBBox{b.MinX - dx, b.MinY - dy, b.MaxX + dx, b.MaxY + dy}
}

func fitBBoxToAspect(b wmsBBox, width, height int) wmsBBox {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	dataW := b.MaxX - b.MinX
	dataH := b.MaxY - b.MinY
	if dataW <= 0 {
		dataW = 0.01
		b.MinX -= 0.005
		b.MaxX = b.MinX + dataW
	}
	if dataH <= 0 {
		dataH = 0.01
		b.MinY -= 0.005
		b.MaxY = b.MinY + dataH
	}
	imgAspect := float64(width) / float64(height)
	dataAspect := dataW / dataH
	if dataAspect < imgAspect {
		newW := dataH * imgAspect
		cx := (b.MinX + b.MaxX) / 2
		b.MinX = cx - newW/2
		b.MaxX = cx + newW/2
	} else if dataAspect > imgAspect {
		newH := dataW / imgAspect
		cy := (b.MinY + b.MaxY) / 2
		b.MinY = cy - newH/2
		b.MaxY = cy + newH/2
	}
	return b
}

func bboxCSV(b wmsBBox) string {
	return fmt.Sprintf("%.8f,%.8f,%.8f,%.8f", b.MinX, b.MinY, b.MaxX, b.MaxY)
}

func bboxTo3857(b wmsBBox) wmsBBox {
	x0, y0 := lonLatToMerc(b.MinX, b.MinY)
	x1, y1 := lonLatToMerc(b.MaxX, b.MaxY)
	return wmsBBox{
		MinX: math.Min(x0, x1),
		MinY: math.Min(y0, y1),
		MaxX: math.Max(x0, x1),
		MaxY: math.Max(y0, y1),
	}
}

func lonLatToMerc(lon, lat float64) (float64, float64) {
	if lat > 85.05112878 {
		lat = 85.05112878
	}
	if lat < -85.05112878 {
		lat = -85.05112878
	}
	x := lon * 20037508.342789244 / 180.0
	y := math.Log(math.Tan((90.0+lat)*math.Pi/360.0)) * 20037508.342789244 / math.Pi
	return x, y
}

func httpGetBytes(ctx context.Context, rawURL string, auth geoServerAuth) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if auth.User != "" {
		req.SetBasicAuth(auth.User, auth.Pass)
	}
	client := &http.Client{Timeout: geoserverHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateRunes(string(body), 200))
	}
	return body, nil
}

func httpGetPNG(ctx context.Context, rawURL string, auth geoServerAuth) ([]byte, error) {
	body, err := httpGetBytes(ctx, rawURL, auth)
	if err != nil {
		return nil, err
	}
	if len(body) < 8 || body[0] != 0x89 {
		return nil, fmt.Errorf("response was not a PNG image")
	}
	return body, nil
}

func fetchLegendGraphic(ctx context.Context, wmsBase, layer, style string, auth geoServerAuth) ([]byte, error) {
	params := url.Values{}
	params.Set("SERVICE", "WMS")
	params.Set("VERSION", "1.1.1")
	params.Set("REQUEST", "GetLegendGraphic")
	params.Set("LAYER", layer)
	params.Set("FORMAT", "image/png")
	params.Set("TRANSPARENT", "true")
	if strings.TrimSpace(style) != "" {
		params.Set("STYLE", style)
	}
	return httpGetPNG(ctx, wmsBase+"?"+params.Encode(), auth)
}

func wfsFetchFeatureExtent(ctx context.Context, wfsBase, typeName, cql string, auth geoServerAuth) (wmsBBox, bool) {
	params := url.Values{}
	params.Set("SERVICE", "WFS")
	params.Set("VERSION", "1.1.0")
	params.Set("REQUEST", "GetFeature")
	params.Set("TYPENAME", typeName)
	params.Set("OUTPUTFORMAT", "application/json")
	params.Set("SRSNAME", "EPSG:4326")
	params.Set("MAXFEATURES", "5000")
	if cql != "" {
		params.Set("CQL_FILTER", cql)
	}
	body, err := httpGetBytes(ctx, wfsBase+"?"+params.Encode(), auth)
	if err != nil {
		return wmsBBox{}, false
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return wmsBBox{}, false
	}
	var geojson map[string]any
	if err := json.Unmarshal(body, &geojson); err != nil {
		return wmsBBox{}, false
	}
	if feats, ok := geojson["features"].([]any); ok && len(feats) == 0 {
		return wmsBBox{}, false
	}
	b, ok := geoJSONBBox(geojson)
	return b, ok
}

func geoJSONBBox(geojson map[string]any) (wmsBBox, bool) {
	feats, _ := geojson["features"].([]any)
	hasFeatures := len(feats) > 0
	if !hasFeatures {
		if bb, ok := geojson["bbox"].([]any); ok && len(bb) >= 4 {
			return wmsBBox{asFloat(bb[0], 0), asFloat(bb[1], 0), asFloat(bb[2], 0), asFloat(bb[3], 0)}, true
		}
		return wmsBBox{}, false
	}
	var minX, minY, maxX, maxY float64
	found := false
	var visitCoords func(any)
	visitCoords = func(coords any) {
		arr, ok := coords.([]any)
		if !ok || len(arr) == 0 {
			return
		}
		if len(arr) >= 2 {
			if _, ok0 := asNumber(arr[0]); ok0 {
				if _, ok1 := asNumber(arr[1]); ok1 {
					x := asFloat(arr[0], 0)
					y := asFloat(arr[1], 0)
					if !found {
						minX, maxX, minY, maxY = x, x, y, y
						found = true
					} else {
						if x < minX {
							minX = x
						}
						if x > maxX {
							maxX = x
						}
						if y < minY {
							minY = y
						}
						if y > maxY {
							maxY = y
						}
					}
					return
				}
			}
		}
		for _, child := range arr {
			visitCoords(child)
		}
	}
	var visitGeom func(any)
	visitGeom = func(geom any) {
		m, ok := geom.(map[string]any)
		if !ok {
			return
		}
		if t, _ := m["type"].(string); t == "GeometryCollection" {
			if gs, ok := m["geometries"].([]any); ok {
				for _, g := range gs {
					visitGeom(g)
				}
			}
			return
		}
		if coords, ok := m["coordinates"]; ok {
			visitCoords(coords)
		}
	}
	for _, f := range feats {
		fm, ok := f.(map[string]any)
		if !ok {
			continue
		}
		visitGeom(fm["geometry"])
	}
	if !found {
		return wmsBBox{}, false
	}
	return wmsBBox{minX, minY, maxX, maxY}, true
}

func asNumber(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

var (
	reLatLonBox = regexp.MustCompile(`(?is)<LatLonBoundingBox[^>]*minx="([^"]+)"[^>]*miny="([^"]+)"[^>]*maxx="([^"]+)"[^>]*maxy="([^"]+)"`)
	reBBox4326  = regexp.MustCompile(`(?is)<BoundingBox[^>]*CRS="EPSG:4326"[^>]*minx="([^"]+)"[^>]*miny="([^"]+)"[^>]*maxx="([^"]+)"[^>]*maxy="([^"]+)"`)
)

func wmsFetchLayerExtent(ctx context.Context, wmsBase, layerName string, auth geoServerAuth) (wmsBBox, string, bool) {
	params := url.Values{}
	params.Set("SERVICE", "WMS")
	params.Set("VERSION", "1.3.0")
	params.Set("REQUEST", "GetCapabilities")
	body, err := httpGetBytes(ctx, wmsBase+"?"+params.Encode(), auth)
	if err != nil {
		return wmsBBox{}, "", false
	}
	return parseCapabilitiesExtent(string(body), layerName)
}

func parseCapabilitiesExtent(xmlBody, layerName string) (wmsBBox, string, bool) {
	layerName = strings.TrimSpace(layerName)
	// Prefer a LatLonBoundingBox near the matching layer name when possible.
	if layerName != "" {
		idx := strings.Index(strings.ToLower(xmlBody), ">"+strings.ToLower(layerName)+"<")
		if idx < 0 {
			// Try unqualified suffix match :layer
			if i := strings.LastIndex(layerName, ":"); i >= 0 {
				short := layerName[i+1:]
				idx = strings.Index(strings.ToLower(xmlBody), ">"+strings.ToLower(short)+"<")
			}
		}
		if idx >= 0 {
			window := xmlBody[idx:]
			if len(window) > 4000 {
				window = window[:4000]
			}
			if m := reLatLonBox.FindStringSubmatch(window); len(m) == 5 {
				return wmsBBox{parseF(m[1]), parseF(m[2]), parseF(m[3]), parseF(m[4])}, "EPSG:4326", true
			}
			if m := reBBox4326.FindStringSubmatch(window); len(m) == 5 {
				return wmsBBox{parseF(m[1]), parseF(m[2]), parseF(m[3]), parseF(m[4])}, "EPSG:4326", true
			}
		}
	}
	if m := reLatLonBox.FindStringSubmatch(xmlBody); len(m) == 5 {
		return wmsBBox{parseF(m[1]), parseF(m[2]), parseF(m[3]), parseF(m[4])}, "EPSG:4326", true
	}
	if m := reBBox4326.FindStringSubmatch(xmlBody); len(m) == 5 {
		return wmsBBox{parseF(m[1]), parseF(m[2]), parseF(m[3]), parseF(m[4])}, "EPSG:4326", true
	}
	return wmsBBox{}, "", false
}

func parseF(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func overlayMapChrome(mapPNG []byte, title string, legendPNG []byte) ([]byte, error) {
	if title == "" && len(legendPNG) == 0 {
		return mapPNG, nil
	}
	src, err := png.Decode(bytes.NewReader(mapPNG))
	if err != nil {
		return mapPNG, nil
	}
	bounds := src.Bounds()
	out := image.NewRGBA(bounds)
	draw.Draw(out, bounds, src, bounds.Min, draw.Src)

	if title != "" {
		// Simple title bar (top-left).
		fg := color.RGBA{R: 30, G: 30, B: 30, A: 255}
		bg := color.RGBA{R: 255, G: 255, B: 255, A: 220}
		pad := 6
		tw := len(title)*7 + pad*2
		th := 14 + pad*2
		for y := 8; y < 8+th && y < bounds.Max.Y; y++ {
			for x := 8; x < 8+tw && x < bounds.Max.X; x++ {
				out.Set(x, y, bg)
			}
		}
		drawString(out, 8+pad, 8+pad+10, title, fg)
	}

	if len(legendPNG) > 0 {
		leg, err := png.Decode(bytes.NewReader(legendPNG))
		if err == nil {
			lw := leg.Bounds().Dx()
			lh := leg.Bounds().Dy()
			maxW := int(math.Max(40, math.Min(float64(lw), float64(bounds.Dx())*0.35)))
			maxH := int(math.Max(40, math.Min(float64(lh), float64(bounds.Dy())*0.5)))
			scale := math.Min(float64(maxW)/math.Max(1, float64(lw)), math.Min(float64(maxH)/math.Max(1, float64(lh)), 1.0))
			dw := int(math.Max(1, math.Floor(float64(lw)*scale)))
			dh := int(math.Max(1, math.Floor(float64(lh)*scale)))
			dx := bounds.Dx() - dw - 10
			dy := bounds.Dy() - dh - 10
			if dx < 4 {
				dx = 4
			}
			if dy < 4 {
				dy = 4
			}
			draw.Draw(out, image.Rect(dx, dy, dx+dw, dy+dh), resizeNearest(leg, dw, dh), image.Point{}, draw.Over)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return mapPNG, nil
	}
	return buf.Bytes(), nil
}

func resizeNearest(src image.Image, dw, dh int) image.Image {
	sb := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		sy := sb.Min.Y + y*sb.Dy()/dh
		for x := 0; x < dw; x++ {
			sx := sb.Min.X + x*sb.Dx()/dw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
