package spatial

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// GeoJSONToWKT converts a simple GeoJSON geometry object to WKT (EPSG:4326).
// Supports Point, MultiPoint, LineString, MultiLineString, Polygon, MultiPolygon.
func GeoJSONToWKT(geoJSON string) (string, error) {
	geoJSON = strings.TrimSpace(geoJSON)
	if geoJSON == "" {
		return "", fmt.Errorf("geometry is required")
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(geoJSON), &raw); err != nil {
		return "", fmt.Errorf("invalid GeoJSON: %w", err)
	}
	typ, _ := raw["type"].(string)
	coords := raw["coordinates"]
	switch strings.ToUpper(strings.TrimSpace(typ)) {
	case "POINT":
		return "POINT(" + formatCoordPair(coords) + ")", nil
	case "MULTIPOINT":
		return "MULTIPOINT(" + formatCoordList(coords, false) + ")", nil
	case "LINESTRING":
		return "LINESTRING(" + formatCoordList(coords, false) + ")", nil
	case "MULTILINESTRING":
		return "MULTILINESTRING(" + formatRingList(coords) + ")", nil
	case "POLYGON":
		return "POLYGON(" + formatRingList(coords) + ")", nil
	case "MULTIPOLYGON":
		parts, ok := coords.([]any)
		if !ok {
			return "", fmt.Errorf("invalid MultiPolygon coordinates")
		}
		rings := make([]string, 0, len(parts))
		for _, p := range parts {
			rings = append(rings, "("+formatRingList(p)+")")
		}
		return "MULTIPOLYGON(" + strings.Join(rings, ",") + ")", nil
	default:
		return "", fmt.Errorf("unsupported GeoJSON type %q", typ)
	}
}

func formatCoordPair(v any) string {
	arr, ok := v.([]any)
	if !ok || len(arr) < 2 {
		return "0 0"
	}
	return formatNum(arr[0]) + " " + formatNum(arr[1])
}

func formatCoordList(v any, paren bool) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(arr))
	for _, item := range arr {
		pair := formatCoordPair(item)
		if paren {
			parts = append(parts, "("+pair+")")
		} else {
			parts = append(parts, pair)
		}
	}
	return strings.Join(parts, ",")
}

func formatRingList(v any) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(arr))
	for _, ring := range arr {
		parts = append(parts, "("+formatCoordList(ring, false)+")")
	}
	return strings.Join(parts, ",")
}

func formatNum(v any) string {
	switch t := v.(type) {
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}

// WKTToGeoJSON converts common WKT geometry text to a GeoJSON geometry object.
func WKTToGeoJSON(s string) (map[string]any, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToUpper(s), "SRID=") {
		if i := strings.IndexByte(s, ';'); i >= 0 {
			s = strings.TrimSpace(s[i+1:])
		}
	}
	u := strings.ToUpper(s)
	switch {
	case strings.HasPrefix(u, "POINT"):
		coords, ok := parseWKTCoords(s[len("POINT"):])
		if !ok || len(coords) == 0 {
			return nil, false
		}
		return map[string]any{"type": "Point", "coordinates": coords[0]}, true
	case strings.HasPrefix(u, "MULTIPOINT"):
		coords, ok := parseWKTCoords(s[len("MULTIPOINT"):])
		if !ok {
			return nil, false
		}
		return map[string]any{"type": "MultiPoint", "coordinates": coords}, true
	case strings.HasPrefix(u, "LINESTRING"):
		coords, ok := parseWKTCoords(s[len("LINESTRING"):])
		if !ok {
			return nil, false
		}
		return map[string]any{"type": "LineString", "coordinates": coords}, true
	case strings.HasPrefix(u, "MULTILINESTRING"):
		parts, ok := parseWKTNested(s[len("MULTILINESTRING"):], 1)
		if !ok {
			return nil, false
		}
		return map[string]any{"type": "MultiLineString", "coordinates": parts}, true
	case strings.HasPrefix(u, "POLYGON"):
		parts, ok := parseWKTNested(s[len("POLYGON"):], 1)
		if !ok {
			return nil, false
		}
		return map[string]any{"type": "Polygon", "coordinates": parts}, true
	case strings.HasPrefix(u, "MULTIPOLYGON"):
		parts, ok := parseWKTNested(s[len("MULTIPOLYGON"):], 2)
		if !ok {
			return nil, false
		}
		return map[string]any{"type": "MultiPolygon", "coordinates": parts}, true
	default:
		return nil, false
	}
}

func parseWKTCoords(body string) ([][]float64, bool) {
	body = strings.TrimSpace(body)
	// Strip optional Z/M/ZM dimension markers before the coordinate list.
	for {
		u := strings.ToUpper(body)
		switch {
		case strings.HasPrefix(u, "ZM"):
			body = strings.TrimSpace(body[2:])
		case strings.HasPrefix(u, "Z"), strings.HasPrefix(u, "M"):
			body = strings.TrimSpace(body[1:])
		default:
			goto coords
		}
	}
coords:
	body = strings.TrimPrefix(body, "(")
	body = strings.TrimSuffix(body, ")")
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, false
	}
	// MULTIPOINT may use (x y), (x y) form
	if strings.Contains(body, "(") {
		var out [][]float64
		depth := 0
		start := -1
		for i, r := range body {
			switch r {
			case '(':
				if depth == 0 {
					start = i + 1
				}
				depth++
			case ')':
				depth--
				if depth == 0 && start >= 0 {
					pair := strings.Fields(body[start:i])
					if len(pair) >= 2 {
						x, err1 := strconv.ParseFloat(pair[0], 64)
						y, err2 := strconv.ParseFloat(pair[1], 64)
						if err1 == nil && err2 == nil {
							out = append(out, []float64{x, y})
						}
					}
					start = -1
				}
			}
		}
		return out, len(out) > 0
	}
	parts := strings.Split(body, ",")
	out := make([][]float64, 0, len(parts))
	for _, p := range parts {
		fields := strings.Fields(strings.TrimSpace(p))
		if len(fields) < 2 {
			return nil, false
		}
		x, err1 := strconv.ParseFloat(fields[0], 64)
		y, err2 := strconv.ParseFloat(fields[1], 64)
		if err1 != nil || err2 != nil {
			return nil, false
		}
		out = append(out, []float64{x, y})
	}
	return out, len(out) > 0
}

func parseWKTNested(body string, depth int) (any, bool) {
	body = strings.TrimSpace(body)
	body = strings.TrimPrefix(body, "(")
	body = strings.TrimSuffix(body, ")")
	body = strings.TrimSpace(body)
	if depth <= 0 {
		return parseWKTCoords("(" + body + ")")
	}
	var chunks []string
	level := 0
	start := 0
	for i, r := range body {
		switch r {
		case '(':
			level++
		case ')':
			level--
		case ',':
			if level == 0 {
				chunks = append(chunks, strings.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}
	chunks = append(chunks, strings.TrimSpace(body[start:]))
	out := make([]any, 0, len(chunks))
	for _, ch := range chunks {
		if depth == 1 {
			coords, ok := parseWKTCoords(ch)
			if !ok {
				return nil, false
			}
			out = append(out, coords)
			continue
		}
		inner, ok := parseWKTNested(ch, depth-1)
		if !ok {
			return nil, false
		}
		out = append(out, inner)
	}
	return out, len(out) > 0
}
