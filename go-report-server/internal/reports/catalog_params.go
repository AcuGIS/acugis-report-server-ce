package reports

import "strings"

// CatalogParametersFromJRXML returns report.json parameter entries for a
// Designer-native save. Extraction uses ExtractMainDataset (main dataset only).
// The UI type mapping matches first-time publish: name and type, no lookup SQL.
// Jasper built-ins and map-image parameters are omitted. Map-driven parameters
// stay in the list but are hidden.
func CatalogParametersFromJRXML(data []byte) []map[string]any {
	md, err := ExtractMainDataset(data)
	if err != nil || len(md.Parameters) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(md.Parameters))
	for _, p := range md.Parameters {
		name := strings.TrimSpace(p.Name)
		if name == "" || skipCatalogParameter(name) || !safeParamIdent(name) {
			continue
		}
		out = append(out, catalogParamEntry(p))
	}
	if out == nil {
		return []map[string]any{}
	}
	return out
}

// skipCatalogParameter mirrors the publish importer's built-in skip list
// (spatial.skipPublishParams / REPORT_* / P_MAP_IMAGE).
func skipCatalogParameter(name string) bool {
	switch name {
	case "REPORT_PARAMETERS_MAP",
		"REPORT_CONNECTION",
		"REPORT_DATA_SOURCE",
		"REPORT_MAX_COUNT",
		"REPORT_LOCALE",
		"REPORT_RESOURCE_BUNDLE",
		"REPORT_TIME_ZONE",
		"REPORT_FORMAT_FACTORY",
		"REPORT_CLASS_LOADER",
		"REPORT_URL_HANDLER_FACTORY",
		"REPORT_FILE_RESOLVER",
		"REPORT_SCRIPTLET",
		"REPORT_VIRTUALIZER",
		"REPORT_TEMPLATES",
		"SORT_FIELDS",
		"FILTER",
		"REPORT_DIR",
		"SUBREPORT_DIR",
		"P_MAP_IMAGE":
		return true
	default:
		return strings.HasPrefix(name, "REPORT_")
	}
}

func safeParamIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func catalogParamEntry(p MainParameter) map[string]any {
	entry := map[string]any{
		"name":     p.Name,
		"label":    p.Name,
		"required": true,
		"hidden":   false,
	}
	kind := javaClassKind(p.Class)
	if javaClassKind(p.NestedType) == "collection" {
		kind = "collection"
	}
	switch kind {
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
	if isMapDrivenParameter(p.Name) {
		entry["required"] = false
		entry["hidden"] = true
	}
	return entry
}

func javaClassKind(class string) string {
	c := strings.ToLower(strings.TrimSpace(class))
	if c == "" {
		return "string"
	}
	switch {
	case strings.Contains(c, "collection"), strings.Contains(c, "list"), strings.Contains(c, "set"):
		return "collection"
	case strings.Contains(c, "integer"), strings.Contains(c, "long"), strings.Contains(c, "short"), strings.Contains(c, "biginteger"):
		return "integer"
	case strings.Contains(c, "double"), strings.Contains(c, "float"), strings.Contains(c, "decimal"),
		strings.Contains(c, "bigdecimal"), strings.Contains(c, "number"):
		return "double"
	case strings.Contains(c, "bool"):
		return "boolean"
	case strings.Contains(c, "date"), strings.Contains(c, "time"):
		return "date"
	default:
		return "string"
	}
}

func isMapDrivenParameter(name string) bool {
	switch name {
	case "p_geometry", "p_srid", "selected_ids":
		return true
	default:
		return false
	}
}
