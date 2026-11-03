package spatial

import "strings"

// System / built-in parameters that should not become report prompts.
var skipPublishParams = map[string]bool{
	"REPORT_PARAMETERS_MAP":      true,
	"REPORT_CONNECTION":          true,
	"REPORT_DATA_SOURCE":         true,
	"REPORT_MAX_COUNT":           true,
	"REPORT_LOCALE":              true,
	"REPORT_RESOURCE_BUNDLE":     true,
	"REPORT_TIME_ZONE":           true,
	"REPORT_FORMAT_FACTORY":      true,
	"REPORT_CLASS_LOADER":        true,
	"REPORT_URL_HANDLER_FACTORY": true,
	"REPORT_FILE_RESOLVER":       true,
	"REPORT_SCRIPTLET":           true,
	"REPORT_VIRTUALIZER":         true,
	"REPORT_TEMPLATES":           true,
	"SORT_FIELDS":                true,
	"FILTER":                     true,
	"REPORT_DIR":                 true,
	"SUBREPORT_DIR":              true,
	"P_MAP_IMAGE":                true,
}

// PublishParametersFromJRXML returns minimal report-JSON parameter definitions
// (name + UI type only) for first-time publish. Lookup/SQL is not imported —
// cascading lookups continue to come from the JRXML at config-read time.
func PublishParametersFromJRXML(data []byte) []map[string]any {
	if len(data) == 0 {
		return nil
	}
	meta := extractJRXMLParameters(data)
	if len(meta) == 0 {
		return nil
	}
	order := parameterNamesInOrder(data)
	out := make([]map[string]any, 0, len(meta))
	seen := map[string]bool{}
	appendOne := func(name string) {
		m, ok := meta[name]
		if !ok || seen[name] {
			return
		}
		if skipPublishParams[name] || strings.HasPrefix(name, "REPORT_") {
			return
		}
		seen[name] = true
		out = append(out, publishParamFromMeta(m))
	}
	for _, name := range order {
		appendOne(name)
	}
	for name := range meta {
		appendOne(name)
	}
	return out
}

func publishParamFromMeta(m ParamMeta) map[string]any {
	entry := map[string]any{
		"name":     m.Name,
		"label":    m.Name,
		"required": true,
		"hidden":   false,
	}
	switch m.Kind {
	case "integer", "double":
		entry["type"] = "number"
	case "date":
		entry["type"] = "date"
	case "boolean":
		entry["type"] = "string"
	case "collection":
		entry["type"] = "select"
		entry["multiple"] = true
		entry["options"] = []any{}
	default:
		entry["type"] = "string"
	}
	if IsMapDrivenParameter(m.Name) {
		entry["required"] = false
		entry["hidden"] = true
	}
	return entry
}

// IsMapDrivenParameter reports parameters the map viewer supplies from a
// spatial selection. An unfiltered Map Report run omits them, which leaves
// the JRXML null-gate ($P{p_geometry} IS NULL OR …) inactive.
// Names are exact. A user parameter such as p_customer is not map-driven.
func IsMapDrivenParameter(name string) bool {
	switch name {
	case "p_geometry", "p_srid", "selected_ids":
		return true
	default:
		return false
	}
}

// IsPromptExcludedParameter is a parameter Dataset Intelligence must not ask
// the user to type. It covers Jasper built-ins, embedded-map P_MAP_IMAGE,
// and map-driven parameters. The JRXML definition is unchanged.
func IsPromptExcludedParameter(name string) bool {
	if skipPublishParams[name] || strings.HasPrefix(name, "REPORT_") {
		return true
	}
	return IsMapDrivenParameter(name)
}

func parameterNamesInOrder(data []byte) []string {
	var order []string
	seen := map[string]bool{}
	for _, m := range reParamTag.FindAllStringSubmatch(string(data), -1) {
		if len(m) < 2 {
			continue
		}
		nm := reAttrName.FindStringSubmatch(m[1])
		if len(nm) < 2 {
			continue
		}
		name := strings.TrimSpace(nm[1])
		if name == "" || !IsSafeSQLIdent(name) || seen[name] {
			continue
		}
		seen[name] = true
		order = append(order, name)
	}
	return order
}
