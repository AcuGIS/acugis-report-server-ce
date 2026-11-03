package embeddedmap

import (
	"net/url"
	"strconv"
	"strings"
)

// FromForm parses embedded_map[...] fields from an edit form (url.Values).
func FromForm(form url.Values) Definition {
	get := func(key string) string {
		return strings.TrimSpace(form.Get("embedded_map[" + key + "]"))
	}
	has := func(key string) bool {
		_, ok := form["embedded_map["+key+"]"]
		return ok
	}

	raw := map[string]any{
		"enabled":              form.Get("embedded_map[enabled]") != "",
		"band":                 get("band"),
		"groupName":            get("groupName"),
		"width":                get("width"),
		"height":               get("height"),
		"x":                    get("x"),
		"y":                    get("y"),
		"padding":              get("padding"),
		"background":           get("background"),
		"dpi":                  get("dpi"),
		"renderer":             get("renderer"),
		"basemap":              get("basemap"),
		"parameterName":        get("parameterName"),
		"mapType":              get("mapType"),
		"field":                get("field"),
		"classification":       get("classification"),
		"classes":              get("classes"),
		"colorRamp":            get("colorRamp"),
		"title":                get("title"),
		"geoserverConnection":  get("geoserverConnection"),
		"geoserverBaseUrl":     get("geoserverBaseUrl"),
		"geoserverWorkspace":   get("geoserverWorkspace"),
		"geoserverLayer":       get("geoserverLayer"),
		"geoserverStyle":       get("geoserverStyle"),
		"geoserverFilterMappings": parseFormFilterMappings(form),
		"qgisConnection":       get("qgisConnection"),
		"qgisBaseUrl":          get("qgisBaseUrl"),
		"qgisProject":          get("qgisProject"),
		"qgisLayer":            get("qgisLayer"),
		"qgisStyle":            get("qgisStyle"),
	}

	shape := strings.TrimSpace(form.Get("embedded_map[pointSymbol][shape]"))
	raw["pointSymbol"] = map[string]any{"shape": shape}

	// Only set checkbox keys when present so Normalize keeps defaults when panel is disabled.
	if has("includeLegend") {
		raw["includeLegend"] = form.Get("embedded_map[includeLegend]") != ""
	}
	if has("showTitle") {
		raw["showTitle"] = form.Get("embedded_map[showTitle]") != ""
	}
	if has("highlightSelected") {
		raw["highlightSelected"] = form.Get("embedded_map[highlightSelected]") != ""
	}
	if has("grayContext") {
		raw["grayContext"] = form.Get("embedded_map[grayContext]") != ""
	}
	if has("zoomToFiltered") {
		raw["zoomToFiltered"] = form.Get("embedded_map[zoomToFiltered]") != ""
	}

	return Normalize(raw)
}

func parseFormFilterMappings(form url.Values) []map[string]any {
	var out []map[string]any
	for i := 0; i < 32; i++ {
		prefix := "embedded_map[geoserverFilterMappings][" + strconv.Itoa(i) + "]"
		rp := strings.TrimSpace(form.Get(prefix + "[reportParameter]"))
		field := strings.TrimSpace(form.Get(prefix + "[geoserverField]"))
		op := strings.TrimSpace(form.Get(prefix + "[operator]"))
		if rp == "" && field == "" {
			continue
		}
		if op == "" {
			op = "="
		}
		out = append(out, map[string]any{
			"reportParameter": rp,
			"geoserverField":  field,
			"operator":        op,
		})
	}
	return out
}
