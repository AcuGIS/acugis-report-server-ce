package embeddedmap

import (
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// Jasper / JRXML contracts (parity with PHP).
const (
	ParamName   = "P_MAP_IMAGE"
	ReportDirParam = "REPORT_DIR" // set by Tomcat fill; used to resolve relative map PNGs
	ElementUUID = "a0000001-0000-4000-8000-0000000000a1"
	BandPadding = 8
	RendererID  = RendererPostGIS // default / legacy alias
)

// Bands supported for image placement.
var Bands = map[string]string{
	"summary":     "Summary",
	"detail":      "Detail",
	"groupHeader": "Group Header",
}

// MapTypes are thematic symbology ids.
var MapTypes = map[string]string{
	"locator":            "Locator",
	"choropleth":         "Choropleth",
	"categorized":        "Categorized",
	"graduated_symbols":  "Graduated Symbols",
	"selected_features":  "Selected Features",
	"overview_highlight": "Overview + Highlight",
}

// MapTypesReady are fully implemented for PNG output.
var MapTypesReady = []string{
	"locator", "choropleth", "categorized", "graduated_symbols",
	"selected_features", "overview_highlight",
}

// Classifications for choropleth / graduated.
var Classifications = map[string]string{
	"equal_interval": "Equal Interval",
	"quantile":       "Quantile",
	"natural_breaks": "Natural Breaks",
}

var ClassificationsReady = []string{"equal_interval", "quantile", "natural_breaks"}

// PointShapes for point layers.
var PointShapes = map[string]string{
	"circle":   "Circle",
	"square":   "Square",
	"triangle": "Triangle",
	"diamond":  "Diamond",
	"star":     "Star",
	"cross":    "Cross",
}

// ColorRamps sequential palettes (canonical ids).
var ColorRamps = map[string][]string{
	"Blue":   {"#eff3ff", "#bdd7e7", "#6baed6", "#3182bd", "#08519c"},
	"Green":  {"#edf8e9", "#bae4b3", "#74c476", "#31a354", "#006d2c"},
	"Red":    {"#fee5d9", "#fcae91", "#fb6a4a", "#de2d26", "#a50f15"},
	"Purple": {"#f2f0f7", "#cbc9e2", "#9e9ac8", "#756bb1", "#54278f"},
	"Orange": {"#feedde", "#fdbe85", "#fd8d3c", "#e6550d", "#a63603"},
}

// Definition is the report JSON embeddedMap object.
type Definition struct {
	Enabled            bool           `json:"enabled"`
	Band               string         `json:"band"`
	GroupName          string         `json:"groupName,omitempty"`
	Width              int            `json:"width"`
	Height             int            `json:"height"`
	X                  int            `json:"x"`
	Y                  int            `json:"y"`
	Padding            float64        `json:"padding"`
	Background         string         `json:"background"`
	DPI                int            `json:"dpi"`
	Renderer           string         `json:"renderer"`
	ParameterName      string         `json:"parameterName"`
	MapType            string         `json:"mapType"`
	Field              string         `json:"field,omitempty"`
	Classification     string         `json:"classification"`
	Classes            int            `json:"classes"`
	ColorRamp          string         `json:"colorRamp"`
	IncludeLegend      bool           `json:"includeLegend"`
	ShowTitle          bool           `json:"showTitle"`
	Title              string         `json:"title,omitempty"`
	HighlightSelected  bool           `json:"highlightSelected"`
	GrayContext        bool           `json:"grayContext"`
	ZoomToFiltered     bool           `json:"zoomToFiltered"`
	Basemap            string         `json:"basemap"`
	PointSymbol        map[string]any `json:"pointSymbol"`
	// GeoServer WMS fields (renderer == geoserver).
	GeoServerConnection     string                   `json:"geoserverConnection,omitempty"`
	GeoServerBaseURL        string                   `json:"geoserverBaseUrl,omitempty"`
	GeoServerWorkspace      string                   `json:"geoserverWorkspace,omitempty"`
	GeoServerLayer          string                   `json:"geoserverLayer,omitempty"`
	GeoServerStyle          string                   `json:"geoserverStyle,omitempty"`
	GeoServerFilterMappings []GeoServerFilterMapping `json:"geoserverFilterMappings,omitempty"`
	// QGIS Server WMS fields (renderer == qgis).
	QGISConnection string `json:"qgisConnection,omitempty"`
	QGISBaseURL    string `json:"qgisBaseUrl,omitempty"`
	QGISProject    string `json:"qgisProject,omitempty"`
	QGISLayer      string `json:"qgisLayer,omitempty"`
	QGISStyle      string `json:"qgisStyle,omitempty"`
}

// Default returns default embedded-map config.
func Default() Definition {
	return Definition{
		Enabled:           false,
		Band:              "summary",
		Width:             535,
		Height:            300,
		X:                 0,
		Y:                 10,
		Padding:           0.08,
		Background:        "#ffffff",
		DPI:               96,
		Renderer:          RendererID,
		ParameterName:     ParamName,
		MapType:           "locator",
		Classification:    "quantile",
		Classes:           5,
		ColorRamp:         "Blue",
		IncludeLegend:     true,
		ShowTitle:         false,
		HighlightSelected: true,
		GrayContext:       true,
		ZoomToFiltered:    true,
		Basemap:           BasemapNone,
		PointSymbol:       map[string]any{"shape": "circle"},
	}
}

// Normalize merges raw JSON into a complete Definition.
func Normalize(raw map[string]any) Definition {
	out := Default()
	if raw == nil {
		return out
	}
	out.Enabled = truthy(raw["enabled"])

	out.Band = normalizeBand(asString(raw["band"]))

	group := strings.TrimSpace(firstString(raw, "groupName", "group_name"))
	out.GroupName = group

	out.Width = clampInt(asInt(raw["width"], out.Width), 40, 2000)
	out.Height = clampInt(asInt(raw["height"], out.Height), 40, 2000)
	out.X = clampInt(asInt(raw["x"], out.X), 0, 2000)
	out.Y = clampInt(asInt(raw["y"], out.Y), 0, 2000)
	out.Padding = clampFloat(asFloat(raw["padding"], out.Padding), 0, 0.4)

	bg := strings.TrimSpace(asString(raw["background"]))
	if strings.EqualFold(bg, "transparent") {
		out.Background = "transparent"
	} else if isHexColor(bg) {
		out.Background = "#" + strings.TrimPrefix(bg, "#")
	}

	dpi := asInt(raw["dpi"], out.DPI)
	if dpi <= 0 {
		dpi = 96
	}
	out.DPI = clampInt(dpi, 72, 600)

	out.Renderer = NormalizeRenderer(firstString(raw, "renderer", "source"))

	param := strings.TrimSpace(firstString(raw, "parameterName", "parameter_name"))
	if param == "" || !isSafeIdent(param) {
		param = ParamName
	}
	out.ParameterName = param

	mapType := normalizeMapType(firstString(raw, "mapType", "map_type"))
	if _, ok := MapTypes[mapType]; !ok {
		mapType = "locator"
	}
	out.MapType = mapType

	field := strings.TrimSpace(asString(raw["field"]))
	if field != "" && isSafeFieldKey(field) {
		out.Field = field
	}

	class := normalizeClassification(asString(raw["classification"]))
	if _, ok := Classifications[class]; !ok {
		class = "quantile"
	}
	out.Classification = class

	classes := asInt(raw["classes"], out.Classes)
	out.Classes = clampInt(classes, 3, 7)

	out.ColorRamp = NormalizeColorRamp(firstString(raw, "colorRamp", "color_ramp"))

	if _, ok := raw["includeLegend"]; ok {
		out.IncludeLegend = truthy(raw["includeLegend"])
	} else if _, ok := raw["include_legend"]; ok {
		out.IncludeLegend = truthy(raw["include_legend"])
	}
	if _, ok := raw["showTitle"]; ok {
		out.ShowTitle = truthy(raw["showTitle"])
	} else if _, ok := raw["show_title"]; ok {
		out.ShowTitle = truthy(raw["show_title"])
	}
	title := strings.TrimSpace(asString(raw["title"]))
	out.Title = title

	if _, ok := raw["highlightSelected"]; ok {
		out.HighlightSelected = truthy(raw["highlightSelected"])
	} else if _, ok := raw["highlight_selected"]; ok {
		out.HighlightSelected = truthy(raw["highlight_selected"])
	}
	if _, ok := raw["grayContext"]; ok {
		out.GrayContext = truthy(raw["grayContext"])
	} else if _, ok := raw["gray_context"]; ok {
		out.GrayContext = truthy(raw["gray_context"])
	}
	if _, ok := raw["zoomToFiltered"]; ok {
		out.ZoomToFiltered = truthy(raw["zoomToFiltered"])
	} else if _, ok := raw["zoom_to_filtered"]; ok {
		out.ZoomToFiltered = truthy(raw["zoom_to_filtered"])
	}

	ps := asMap(raw["pointSymbol"])
	if ps == nil {
		ps = asMap(raw["point_symbol"])
	}
	shape := "circle"
	if ps != nil {
		shape = NormalizePointShape(asString(ps["shape"]))
	}
	out.PointSymbol = map[string]any{"shape": shape}

	out.Basemap = NormalizeBasemap(firstString(raw, "basemap", "baseMap", "base_map"))

	out.GeoServerConnection = strings.TrimSpace(firstString(raw, "geoserverConnection", "geoserver_connection"))
	out.GeoServerBaseURL = strings.TrimRight(strings.TrimSpace(firstString(raw, "geoserverBaseUrl", "geoserver_base_url")), "/")
	out.GeoServerWorkspace = strings.TrimSpace(firstString(raw, "geoserverWorkspace", "geoserver_workspace"))
	out.GeoServerLayer = strings.TrimSpace(firstString(raw, "geoserverLayer", "geoserver_layer"))
	out.GeoServerStyle = strings.TrimSpace(firstString(raw, "geoserverStyle", "geoserver_style"))
	out.GeoServerFilterMappings = parseFilterMappings(raw["geoserverFilterMappings"])
	if len(out.GeoServerFilterMappings) == 0 {
		out.GeoServerFilterMappings = parseFilterMappings(raw["geoserver_filter_mappings"])
	}

	out.QGISConnection = strings.TrimSpace(firstString(raw, "qgisConnection", "qgis_connection"))
	out.QGISBaseURL = strings.TrimRight(strings.TrimSpace(firstString(raw, "qgisBaseUrl", "qgis_base_url")), "/")
	out.QGISProject = strings.TrimSpace(firstString(raw, "qgisProject", "qgis_project"))
	out.QGISLayer = strings.TrimSpace(firstString(raw, "qgisLayer", "qgis_layer"))
	out.QGISStyle = strings.TrimSpace(firstString(raw, "qgisStyle", "qgis_style"))

	return out
}

func parseFilterMappings(v any) []GeoServerFilterMapping {
	switch t := v.(type) {
	case []GeoServerFilterMapping:
		return t
	case []any:
		var out []GeoServerFilterMapping
		for _, item := range t {
			m := asMap(item)
			if m == nil {
				continue
			}
			row := GeoServerFilterMapping{
				ReportParameter: strings.TrimSpace(firstString(m, "reportParameter", "report_parameter")),
				GeoServerField:  strings.TrimSpace(firstString(m, "geoserverField", "geoserver_field")),
				Operator:        strings.TrimSpace(asString(m["operator"])),
			}
			if row.ReportParameter == "" || row.GeoServerField == "" {
				continue
			}
			if row.Operator == "" {
				row.Operator = "="
			}
			out = append(out, row)
		}
		return out
	case []map[string]any:
		var out []GeoServerFilterMapping
		for _, m := range t {
			row := GeoServerFilterMapping{
				ReportParameter: strings.TrimSpace(firstString(m, "reportParameter", "report_parameter")),
				GeoServerField:  strings.TrimSpace(firstString(m, "geoserverField", "geoserver_field")),
				Operator:        strings.TrimSpace(asString(m["operator"])),
			}
			if row.ReportParameter == "" || row.GeoServerField == "" {
				continue
			}
			if row.Operator == "" {
				row.Operator = "="
			}
			out = append(out, row)
		}
		return out
	default:
		return nil
	}
}

// ToMap returns a JSON-ready map (canonical camelCase keys).
func (d Definition) ToMap() map[string]any {
	m := map[string]any{
		"enabled":           d.Enabled,
		"band":              d.Band,
		"width":             d.Width,
		"height":            d.Height,
		"x":                 d.X,
		"y":                 d.Y,
		"padding":           d.Padding,
		"background":        d.Background,
		"dpi":               d.DPI,
		"renderer":          d.Renderer,
		"parameterName":     d.ParameterName,
		"mapType":           d.MapType,
		"classification":    d.Classification,
		"classes":           d.Classes,
		"colorRamp":         d.ColorRamp,
		"includeLegend":     d.IncludeLegend,
		"showTitle":         d.ShowTitle,
		"highlightSelected": d.HighlightSelected,
		"grayContext":       d.GrayContext,
		"zoomToFiltered":    d.ZoomToFiltered,
		"basemap":           d.Basemap,
		"pointSymbol":       d.PointSymbol,
	}
	if d.GroupName != "" {
		m["groupName"] = d.GroupName
	} else {
		m["groupName"] = nil
	}
	if d.Field != "" {
		m["field"] = d.Field
	} else {
		m["field"] = nil
	}
	if d.Title != "" {
		m["title"] = d.Title
	} else {
		m["title"] = nil
	}
	if d.GeoServerConnection != "" {
		m["geoserverConnection"] = d.GeoServerConnection
	}
	if d.GeoServerBaseURL != "" {
		m["geoserverBaseUrl"] = d.GeoServerBaseURL
	}
	if d.GeoServerWorkspace != "" {
		m["geoserverWorkspace"] = d.GeoServerWorkspace
	}
	if d.GeoServerLayer != "" {
		m["geoserverLayer"] = d.GeoServerLayer
	}
	if d.GeoServerStyle != "" {
		m["geoserverStyle"] = d.GeoServerStyle
	}
	if d.QGISConnection != "" {
		m["qgisConnection"] = d.QGISConnection
	}
	if d.QGISBaseURL != "" {
		m["qgisBaseUrl"] = d.QGISBaseURL
	}
	if d.QGISProject != "" {
		m["qgisProject"] = d.QGISProject
	}
	if d.QGISLayer != "" {
		m["qgisLayer"] = d.QGISLayer
	}
	if d.QGISStyle != "" {
		m["qgisStyle"] = d.QGISStyle
	}
	if len(d.GeoServerFilterMappings) > 0 {
		rows := make([]map[string]any, 0, len(d.GeoServerFilterMappings))
		for _, row := range d.GeoServerFilterMappings {
			rows = append(rows, map[string]any{
				"reportParameter": row.ReportParameter,
				"geoserverField":  row.GeoServerField,
				"operator":        row.Operator,
			})
		}
		m["geoserverFilterMappings"] = rows
	}
	return m
}

// FromDefinition reads embeddedMap from a report definition.
func FromDefinition(def *reports.Definition) Definition {
	if def == nil || def.Raw == nil {
		return Default()
	}
	raw := asMap(def.Raw["embeddedMap"])
	if raw == nil {
		raw = asMap(def.Raw["embedded_map"])
	}
	return Normalize(raw)
}

// ReportEmbeddedMap returns normalized embed config when enabled, else nil.
func ReportEmbeddedMap(def *reports.Definition) *Definition {
	d := FromDefinition(def)
	if !d.Enabled {
		return nil
	}
	return &d
}

// NormalizeColorRamp maps aliases to canonical ramp ids.
func NormalizeColorRamp(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "Blue"
	}
	if _, ok := ColorRamps[v]; ok {
		return v
	}
	aliases := map[string]string{
		"blues": "Blue", "blue": "Blue",
		"greens": "Green", "green": "Green",
		"reds": "Red", "red": "Red",
		"purple": "Purple", "purples": "Purple",
		"orange": "Orange", "oranges": "Orange",
		"viridis": "Blue", "spectral": "Red",
	}
	if a, ok := aliases[strings.ToLower(v)]; ok {
		return a
	}
	return "Blue"
}

// NormalizePointShape returns a known shape or circle.
func NormalizePointShape(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if _, ok := PointShapes[v]; ok {
		return v
	}
	return "circle"
}

// ColorRampIDs returns ramp keys in stable order.
func ColorRampIDs() []string {
	return []string{"Blue", "Green", "Red", "Purple", "Orange"}
}

// MapTypeReady reports whether a map type is implemented.
func MapTypeReady(id string) bool {
	for _, r := range MapTypesReady {
		if r == id {
			return true
		}
	}
	return false
}

func normalizeMapType(v string) string {
	v = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(v), " ", "_"), "-", "_"))
	switch v {
	case "graduated":
		return "graduated_symbols"
	case "overview":
		return "overview_highlight"
	case "selected":
		return "selected_features"
	}
	return v
}

// normalizeBand maps free-form / legacy band ids to canonical keys.
// Do not blindly ToLower — "groupHeader" would become "groupheader" and fall back to summary.
func normalizeBand(v string) string {
	v = strings.TrimSpace(v)
	if _, ok := Bands[v]; ok {
		return v
	}
	switch strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(v, " ", ""), "-", "")) {
	case "summary":
		return "summary"
	case "detail":
		return "detail"
	case "groupheader", "group_header", "group":
		return "groupHeader"
	default:
		return "summary"
	}
}

func normalizeClassification(v string) string {
	v = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(v), " ", "_"), "-", "_"))
	switch v {
	case "equalinterval":
		return "equal_interval"
	case "naturalbreaks":
		return "natural_breaks"
	}
	return v
}
