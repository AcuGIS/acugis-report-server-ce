package geoserver

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type fieldType struct {
	Name      string
	DataType  string
	UDT       string
	Nullable  bool
	Geometry  bool
	GeomType  string
	JavaClass string
}

func mapXSDType(xsd string) fieldType {
	raw := strings.TrimSpace(xsd)
	local := raw
	if i := strings.LastIndex(local, ":"); i >= 0 {
		local = local[i+1:]
	}
	low := strings.ToLower(local)
	ft := fieldType{UDT: raw, DataType: "text"}
	switch low {
	case "byte", "short", "int", "integer", "long", "nonnegativeinteger", "positiveinteger", "negativeinteger", "nonpositiveinteger":
		ft.DataType = "integer"
	case "decimal":
		ft.DataType = "decimal"
	case "double", "float":
		ft.DataType = "double"
	case "boolean":
		ft.DataType = "boolean"
	case "date":
		ft.DataType = "date"
	case "datetime", "time":
		ft.DataType = "timestamp"
	default:
		if isGeometryXSD(raw) {
			ft.DataType = "geometry"
			ft.Geometry = true
			ft.GeomType = geometryTypeFromXSD(low)
		}
	}
	return ft
}

func isGeometryXSD(xsd string) bool {
	low := strings.ToLower(xsd)
	if strings.Contains(low, "gml:") || strings.HasPrefix(low, "gml") {
		return strings.Contains(low, "geom") || strings.Contains(low, "point") ||
			strings.Contains(low, "line") || strings.Contains(low, "curve") ||
			strings.Contains(low, "polygon") || strings.Contains(low, "surface") ||
			strings.Contains(low, "propertytype")
	}
	return strings.HasSuffix(low, "propertytype") && (strings.Contains(low, "point") ||
		strings.Contains(low, "line") || strings.Contains(low, "polygon") ||
		strings.Contains(low, "curve") || strings.Contains(low, "surface") ||
		strings.Contains(low, "geom"))
}

func geometryTypeFromXSD(local string) string {
	switch {
	case strings.Contains(local, "multipoint"):
		return "MultiPoint"
	case strings.Contains(local, "point"):
		return "Point"
	case strings.Contains(local, "multilinestring"), strings.Contains(local, "multicurve"):
		return "MultiLineString"
	case strings.Contains(local, "linestring"), strings.Contains(local, "curve"):
		return "LineString"
	case strings.Contains(local, "multipolygon"), strings.Contains(local, "multisurface"):
		return "MultiPolygon"
	case strings.Contains(local, "polygon"), strings.Contains(local, "surface"):
		return "Polygon"
	default:
		return "Geometry"
	}
}

func coerceProperty(v any, ft fieldType) any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case json.Number:
		return coerceNumber(string(t), ft)
	case float64:
		return coerceNumber(strconv.FormatFloat(t, 'f', -1, 64), ft)
	case bool:
		if ft.DataType == "boolean" || ft.DataType == "" || ft.DataType == "text" {
			return t
		}
		return t
	case string:
		if t == "" && (ft.DataType == "integer" || ft.DataType == "double" || ft.DataType == "decimal" || ft.DataType == "date" || ft.DataType == "timestamp") {
			return nil
		}
		return coerceString(t, ft)
	default:
		return t
	}
}

func coerceNumber(s string, ft fieldType) any {
	switch ft.DataType {
	case "integer", "":
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && f == float64(int64(f)) {
			return int64(f)
		}
		if ft.DataType == "" {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f
			}
		}
	case "double", "decimal":
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	case "boolean":
		if s == "1" {
			return true
		}
		if s == "0" {
			return false
		}
	}
	if ft.DataType == "integer" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return s
}

func coerceString(s string, ft fieldType) any {
	switch ft.DataType {
	case "integer":
		if i, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
			return i
		}
		return s
	case "double", "decimal":
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return f
		}
		return s
	case "boolean":
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true", "t", "1":
			return true
		case "false", "f", "0":
			return false
		default:
			return s
		}
	case "date":
		if tm, ok := parseWFSTime(s, true); ok {
			return tm
		}
		return s
	case "timestamp":
		if tm, ok := parseWFSTime(s, false); ok {
			return tm
		}
		return s
	default:
		return s
	}
}

func parseWFSTime(s string, dateOnly bool) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	if dateOnly {
		layouts = []string{"2006-01-02", time.RFC3339Nano, time.RFC3339}
	}
	for _, layout := range layouts {
		if tm, err := time.Parse(layout, s); err == nil {
			return tm.UTC(), true
		}
	}
	return time.Time{}, false
}

func formatPreviewCell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case string:
		return t
	default:
		return strings.TrimSpace(strings.ReplaceAll(toString(t), "\n", " "))
	}
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return strings.Trim(string(b), `"`)
	}
}
