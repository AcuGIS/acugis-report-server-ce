package geoserver

import (
	"net/url"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/spatial"
)

// ResolveConnectionName chooses the jasper_geoserver name for a report.
// The JRXML property acugis.datasource.connection wins. Viewer _dataSource
// and the catalog datasource name are fallbacks for the same connection name.
func ResolveConnectionName(jrxmlConnection, rawQuery, catalogName string) string {
	if v := strings.TrimSpace(jrxmlConnection); v != "" {
		return v
	}
	vals, err := url.ParseQuery(rawQuery)
	if err == nil {
		if v := strings.TrimSpace(vals.Get("_dataSource")); v != "" {
			return v
		}
		if v := strings.TrimSpace(vals.Get("datasource")); v != "" {
			return v
		}
	}
	return strings.TrimSpace(catalogName)
}

// FillParams builds the parameter map WFS CQL translation consumes.
// It uses the same collector as the other collection providers so repeated
// query keys stay slices and JRXML defaults are not flattened into one string.
func FillParams(rawQuery, jrxmlPath string, reportParams []map[string]any) map[string]any {
	query := spatial.ParseQueryMulti(rawQuery)
	meta := map[string]spatial.ParamMeta{}
	if strings.TrimSpace(jrxmlPath) != "" {
		if parsed, err := spatial.JRXMLParameters(jrxmlPath); err == nil && parsed != nil {
			meta = parsed
		}
	}
	return spatial.CollectFilterContext(query, meta, reportParams)
}
