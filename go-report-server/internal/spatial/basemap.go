package spatial

import "strings"

// Canonical Map Report basemap ids. OpenFreeMap styles are MapLibre style
// documents, not raster XYZ templates.
const (
	// DefaultMapReportBasemap is the single default for new Map Reports,
	// reports with no basemap, and legacy values that can no longer be used.
	DefaultMapReportBasemap = BasemapOFMPositron

	BasemapOFMPositron = "ofm_positron"
	BasemapOFMLiberty  = "ofm_liberty"
	BasemapOFMBright   = "ofm_bright"
	BasemapOFMDark     = "ofm_dark"
	BasemapEsriStreets = "esri_streets"
	BasemapEsriImagery = "esri_imagery"
)

// OpenFreeMapAttribution is the text OpenFreeMap requires for non-MapLibre clients.
const OpenFreeMapAttribution = "OpenFreeMap © OpenMapTiles Data from OpenStreetMap"

// mapReportBasemapAliases maps normalized legacy and display keys to a current id.
// Keys are lowercase with hyphens and spaces already folded to underscores.
var mapReportBasemapAliases = map[string]string{
	"osm":             DefaultMapReportBasemap,
	"openstreetmap":   DefaultMapReportBasemap,
	"open_street_map": DefaultMapReportBasemap,

	"carto_light":          BasemapOFMPositron,
	"carto":                BasemapOFMPositron,
	"light":                BasemapOFMPositron,
	"light_all":            BasemapOFMPositron,
	"positron":             BasemapOFMPositron,
	"ofm_positron":         BasemapOFMPositron,
	"openfreemap_positron": BasemapOFMPositron,
	"openfreemap_light":    BasemapOFMPositron,

	"carto_dark":        BasemapOFMDark,
	"dark":              BasemapOFMDark,
	"dark_all":          BasemapOFMDark,
	"dark_matter":       BasemapOFMDark,
	"carto_dark_matter": BasemapOFMDark,
	"ofm_dark":          BasemapOFMDark,
	"openfreemap_dark":  BasemapOFMDark,

	"carto_voyager":      BasemapOFMBright,
	"voyager":            BasemapOFMBright,
	"bright":             BasemapOFMBright,
	"ofm_bright":         BasemapOFMBright,
	"openfreemap_bright": BasemapOFMBright,

	"liberty":             BasemapOFMLiberty,
	"ofm_liberty":         BasemapOFMLiberty,
	"openfreemap_liberty": BasemapOFMLiberty,

	"esri_streets":   BasemapEsriStreets,
	"esri_topo":      BasemapEsriStreets,
	"streets":        BasemapEsriStreets,
	"terrain":        BasemapEsriStreets,
	"topo":           BasemapEsriStreets,
	"esri_imagery":   BasemapEsriImagery,
	"esri_satellite": BasemapEsriImagery,
	"satellite":      BasemapEsriImagery,
	"imagery":        BasemapEsriImagery,
}

// NormalizeMapReportBasemap resolves a saved Map Report basemap to a supported id.
// It does not rewrite stored report files; callers apply it when reading configuration.
func NormalizeMapReportBasemap(saved string) string {
	raw := strings.TrimSpace(saved)
	if raw == "" {
		return DefaultMapReportBasemap
	}
	lower := strings.ToLower(raw)
	if id, ok := openFreeMapStyleURL(lower); ok {
		return id
	}
	if strings.Contains(lower, "tile.openstreetmap.org") || strings.Contains(lower, "{s}.tile.openstreetmap.org") {
		return DefaultMapReportBasemap
	}
	if strings.Contains(lower, "basemaps.cartocdn.com") || strings.Contains(lower, "cartocdn.com") {
		switch {
		case strings.Contains(lower, "dark_all") || strings.Contains(lower, "dark-matter") || strings.Contains(lower, "dark_matter"):
			return BasemapOFMDark
		case strings.Contains(lower, "voyager"):
			return BasemapOFMBright
		default:
			return BasemapOFMPositron
		}
	}
	key := strings.ToLower(raw)
	key = strings.ReplaceAll(key, "-", " ")
	key = strings.ReplaceAll(key, "_", " ")
	key = strings.Join(strings.Fields(key), "_")
	if id, ok := mapReportBasemapAliases[key]; ok {
		return id
	}
	return DefaultMapReportBasemap
}

func openFreeMapStyleURL(lower string) (string, bool) {
	if !strings.Contains(lower, "tiles.openfreemap.org/styles/") {
		return "", false
	}
	switch {
	case strings.Contains(lower, "/styles/dark"):
		return BasemapOFMDark, true
	case strings.Contains(lower, "/styles/bright"):
		return BasemapOFMBright, true
	case strings.Contains(lower, "/styles/liberty"):
		return BasemapOFMLiberty, true
	case strings.Contains(lower, "/styles/positron"):
		return BasemapOFMPositron, true
	case strings.Contains(lower, "/styles/fiord"):
		return BasemapOFMDark, true
	default:
		return DefaultMapReportBasemap, true
	}
}

// MapReportViewerCSP is the Map Report Viewer Content-Security-Policy.
// OpenFreeMap resources are limited to https://tiles.openfreemap.org.
// The policy does not use a wildcard host source.
func MapReportViewerCSP() string {
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'",
		"style-src 'self' https://cdn.jsdelivr.net https://fonts.googleapis.com 'unsafe-inline'",
		"img-src 'self' data: blob: https://tiles.openfreemap.org https://server.arcgisonline.com",
		"font-src 'self' data: https://fonts.gstatic.com",
		"connect-src 'self' https://tiles.openfreemap.org",
		"worker-src 'self' blob:",
		"frame-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
	}, "; ")
}
