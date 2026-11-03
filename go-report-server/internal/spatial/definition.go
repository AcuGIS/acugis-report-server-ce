package spatial

import (
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// DefaultMapDefinition mirrors PHP jrs_default_map_definition.
func DefaultMapDefinition(geometryColumn string) map[string]any {
	if geometryColumn == "" {
		geometryColumn = "geom"
	}
	return map[string]any{
		"enabled":        true,
		"basemap":        DefaultMapReportBasemap,
		"geometryColumn": geometryColumn,
		"initialZoom":    "fit",
		"renderer":       "single",
		"field":          nil,
		"classification": "quantile",
		"classes":        5,
		"colorRamp":      "Blues",
		"singleSymbol": map[string]any{
			"fillColor":    "#26a69a",
			"outlineColor": "#00796b",
			"outlineWidth": 1.5,
			"opacity":      0.35,
			"shape":        "circle",
		},
		"categories": []any{},
		"labels": map[string]any{
			"enabled":   false,
			"field":     nil,
			"fontSize":  12,
			"fillColor": "#212121",
			"halo":      true,
		},
		"popups": map[string]any{
			"enabled": false,
			"fields":  []any{},
		},
		"tooltips": map[string]any{
			"enabled": false,
			"fields":  []any{},
		},
		"sync": map[string]any{
			"filterMapFromReport":       true,
			"filterReportFromMap":       true,
			"zoomToReportResults":       true,
			"highlightReturnedFeatures": true,
		},
	}
}

// NormalizeMapDefinition mirrors PHP jrs_normalize_map_definition (subset for viewer).
func NormalizeMapDefinition(raw map[string]any, fallbackGeometryColumn string) map[string]any {
	out := DefaultMapDefinition(fallbackGeometryColumn)
	if raw == nil {
		return out
	}
	if _, ok := raw["enabled"]; ok {
		out["enabled"] = truthy(raw["enabled"])
	}
	out["basemap"] = NormalizeMapReportBasemap(stringField(raw, "basemap"))

	geom := strings.TrimSpace(stringField(raw, "geometryColumn"))
	if geom == "" {
		geom = strings.TrimSpace(stringField(raw, "geometry_column"))
	}
	if geom != "" && IsSafeSQLIdent(geom) {
		out["geometryColumn"] = geom
	} else if fallbackGeometryColumn != "" && IsSafeSQLIdent(fallbackGeometryColumn) {
		out["geometryColumn"] = fallbackGeometryColumn
	}

	if renderer := strings.ToLower(strings.TrimSpace(stringField(raw, "renderer"))); renderer != "" {
		out["renderer"] = renderer
	}
	if field := stringField(raw, "field"); field != "" {
		out["field"] = field
	}
	if ss, ok := raw["singleSymbol"].(map[string]any); ok {
		merged := out["singleSymbol"].(map[string]any)
		for k, v := range ss {
			merged[k] = v
		}
		out["singleSymbol"] = merged
	}
	if sync, ok := raw["sync"].(map[string]any); ok {
		merged := out["sync"].(map[string]any)
		for k, v := range sync {
			merged[k] = v
		}
		out["sync"] = merged
	}
	if labels, ok := raw["labels"].(map[string]any); ok {
		merged := out["labels"].(map[string]any)
		for k, v := range labels {
			merged[k] = v
		}
		out["labels"] = merged
	}
	if popups, ok := raw["popups"].(map[string]any); ok {
		merged := out["popups"].(map[string]any)
		for k, v := range popups {
			merged[k] = v
		}
		out["popups"] = merged
	}
	if tooltips, ok := raw["tooltips"].(map[string]any); ok {
		merged := out["tooltips"].(map[string]any)
		for k, v := range tooltips {
			merged[k] = v
		}
		out["tooltips"] = merged
	}
	if class := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(stringField(raw, "classification"), " ", "_"), "-", "_")); class != "" {
		out["classification"] = class
	}
	if classes := intFromAny(raw["classes"], 5); classes > 0 {
		if classes < 2 {
			classes = 2
		}
		if classes > 12 {
			classes = 12
		}
		out["classes"] = classes
	}
	if ramp := stringField(raw, "colorRamp"); ramp == "" {
		ramp = stringField(raw, "color_ramp")
		if ramp != "" {
			out["colorRamp"] = ramp
		}
	} else {
		out["colorRamp"] = ramp
	}
	if zoom := stringField(raw, "initialZoom"); zoom == "" {
		zoom = stringField(raw, "initial_zoom")
		if zoom != "" {
			out["initialZoom"] = zoom
		}
	} else {
		out["initialZoom"] = zoom
	}
	if cats, ok := raw["categories"].([]any); ok {
		out["categories"] = cats
	} else if cats, ok := raw["categories"].([]map[string]any); ok {
		arr := make([]any, len(cats))
		for i, c := range cats {
			arr[i] = c
		}
		out["categories"] = arr
	}
	return out
}

func intFromAny(v any, fallback int) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case uint32:
		return int(t)
	case uint64:
		return int(t)
	case float64:
		return int(t)
	case []byte:
		s := strings.TrimSpace(string(t))
		if s == "" {
			return fallback
		}
		n, err := strconv.Atoi(s)
		if err == nil {
			return n
		}
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return fallback
		}
		n, err := strconv.Atoi(s)
		if err == nil {
			return n
		}
	}
	return fallback
}

// ReportMapDefinition returns the effective map definition or nil when map is off.
func ReportMapDefinition(def *reports.Definition) map[string]any {
	if def == nil || def.Raw == nil {
		return nil
	}
	isMap := def.Type() == "map"
	raw, _ := def.Raw["mapDefinition"].(map[string]any)
	if raw == nil {
		raw, _ = def.Raw["map_definition"].(map[string]any)
	}
	if !isMap && (raw == nil || !truthy(raw["enabled"])) {
		return nil
	}
	fallback := ""
	if raw != nil {
		fallback = stringField(raw, "geometryColumn")
	}
	normalized := NormalizeMapDefinition(raw, fallback)
	if isMap {
		normalized["enabled"] = true
	}
	if !truthy(normalized["enabled"]) {
		return nil
	}
	return normalized
}

// IsMapReport mirrors PHP jrs_is_map_report: only type "map" (not jasper + mapDefinition).
// Viewer chrome and map_features calls from view.php use this gate; map_features.php itself
// still accepts an enabled mapDefinition on jasper reports when called directly.
func IsMapReport(def *reports.Definition) bool {
	if def == nil {
		return false
	}
	return def.Type() == "map"
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(toString(v))
}
