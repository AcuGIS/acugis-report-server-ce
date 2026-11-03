package mapreport

import (
	"fmt"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

const spatialRequiredMsg = "Map Report requires a spatial datasource (PostgreSQL/PostGIS, MySQL, SQL Server, or Oracle)."

// Service orchestrates map features / extent using report JSON + spatial datasources.
type Service struct {
	Datasources datasources.Finder
	Connector   datasources.Connector
	JRXMLDir    string
}

// FeaturesRequest is the input for Features().
type FeaturesRequest struct {
	Definition    *reports.Definition
	RawQuery      string
	FilterContext map[string]any // optional precomputed; if nil, parsed from RawQuery
}

// ExtentRequest is the input for Extent().
type ExtentRequest struct {
	Definition      *reports.Definition
	XMin            float64
	YMin            float64
	XMax            float64
	YMax            float64
	SRID            int
	GeometryGeoJSON string
}

// ExtentResult mirrors PHP map_extent.php JSON.
type ExtentResult struct {
	PGeometry string `json:"p_geometry"`
	PSRID     int    `json:"p_srid"`
}

func nullIfEmpty(s string) string {
	return s
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
	return strings.TrimSpace(fmt.Sprint(v))
}
