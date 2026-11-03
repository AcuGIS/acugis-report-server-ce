package embeddedmap

import (
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

// Supported embedded map renderers.
const (
	RendererPostGIS   = "postgis"
	RendererMySQL     = "mysql"
	RendererGeoServer = "geoserver"
	RendererQGIS      = "qgis"
)

// NormalizeRenderer maps aliases to a canonical renderer id.
// Unknown values default to postgis.
func NormalizeRenderer(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "postgis", "postgresql", "postgres", "pgsql", "pg":
		return RendererPostGIS
	case "mysql", "mariadb":
		return RendererMySQL
	case "geoserver", "gs":
		return RendererGeoServer
	case "qgis", "qgisserver", "qgis_server", "qgis-server":
		return RendererQGIS
	default:
		return RendererPostGIS
	}
}

// IsSpatialRenderer reports whether the renderer loads features from a JDBC spatial datasource.
func IsSpatialRenderer(renderer string) bool {
	switch NormalizeRenderer(renderer) {
	case RendererPostGIS, RendererMySQL:
		return true
	default:
		return false
	}
}

// ResolveProvider picks the entitlement / prepare provider id.
// GeoServer is never overwritten by the JDBC datasource type.
// For spatial renderers, a matching datasource type wins when available.
func ResolveProvider(renderer string, ds *datasources.Datasource) string {
	r := NormalizeRenderer(renderer)
	if r == RendererGeoServer {
		return RendererGeoServer
	}
	if r == RendererQGIS {
		return RendererQGIS
	}
	if ds != nil {
		switch {
		case ds.IsPostgreSQL():
			return RendererPostGIS
		case ds.IsMySQL():
			return RendererMySQL
		}
	}
	return r
}
