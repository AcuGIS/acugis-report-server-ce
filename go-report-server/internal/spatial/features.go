package spatial

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

var reSelectPrefix = regexp.MustCompile(`(?i)^\s*SELECT\s+`)

// FeatureCollection is a GeoJSON FeatureCollection plus map meta.
type FeatureCollection struct {
	Type          string           `json:"type"`
	Features      []map[string]any `json:"features"`
	CRS           map[string]any   `json:"crs,omitempty"`
	Meta          map[string]any   `json:"meta,omitempty"`
	MapDefinition map[string]any   `json:"mapDefinition,omitempty"`
	Error         string           `json:"error,omitempty"`
	Detail        string           `json:"detail,omitempty"`
	FilterDebug   []map[string]any `json:"-"`
}

// LayerGeoJSONFromReport builds features from report SQL + geometry injection.
func LayerGeoJSONFromReport(
	ctx context.Context,
	conn datasources.QueryConn,
	d SpatialDialect,
	reportSQL string,
	schema, table, geometryColumn string,
	filterContext map[string]any,
	jrxmlParams map[string]ParamMeta,
) (*FeatureCollection, error) {
	if conn == nil {
		return &FeatureCollection{Error: "Database connection unavailable."}, ErrQueryFailed
	}
	if d == nil {
		d = pgSpatial{}
	}
	if reportSQL == "" {
		return &FeatureCollection{Error: "Report SQL is empty."}, ErrQueryFailed
	}
	if !IsSafeSQLIdent(table) || !IsSafeSQLIdent(geometryColumn) {
		return &FeatureCollection{Error: "Unsafe SQL identifier."}, ErrQueryFailed
	}

	neutral := NeutralizeJasperSQL(reportSQL)
	geomRef := SQLGeometryRefDialect(neutral, schema, table, geometryColumn, d)
	if geomRef == "" {
		return &FeatureCollection{Error: "Could not resolve geometry column reference."}, ErrQueryFailed
	}

	// When no map selection is active, drop the Jasper spatial OR-gate so engines
	// like MySQL never evaluate ST_GeomFromText(NULL, …).
	sqlForBind := stripInactiveSpatialFilter(reportSQL, filterContext)

	bound, err := BindJasperSQLDialect(sqlForBind, filterContext, jrxmlParams, d)
	if err != nil {
		return &FeatureCollection{Error: err.Error()}, ErrQueryFailed
	}
	preparedSQL := bound.SQL
	if !reSelectPrefix.MatchString(preparedSQL) {
		return &FeatureCollection{Error: "Report SQL must start with SELECT."}, ErrQueryFailed
	}
	loc := reSelectPrefix.FindStringIndex(preparedSQL)
	selectBody := preparedSQL[loc[1]:]
	selectBody = stripGeometryColumnFromSelect(selectBody, geometryColumn, d)
	injected := "SELECT " + d.AsGeoJSON4326(geomRef) + " AS " + geoJSONSelectAlias(d) + ", " + selectBody

	rows, err := conn.Query(ctx, injected, bound.Params...)
	if err != nil {
		return &FeatureCollection{
			Error:       "Failed to load map features from report dataset.",
			Detail:      err.Error() + " | sql=" + truncateSQL(injected),
			FilterDebug: bound.Debug,
		}, ErrQueryFailed
	}
	defer rows.Close()

	features, ferr := scanGeoJSONFeatures(rows, geometryColumn)
	if ferr != nil {
		return &FeatureCollection{
			Error:       "Failed to load map features from report dataset.",
			Detail:      ferr.Error() + " | sql=" + truncateSQL(injected),
			FilterDebug: bound.Debug,
		}, ErrQueryFailed
	}
	return &FeatureCollection{
		Type:        "FeatureCollection",
		Features:    features,
		FilterDebug: bound.Debug,
	}, nil
}

// LayerGeoJSON is the geometry-table fallback (PHP jrs_map_layer_geojson).
func LayerGeoJSON(
	ctx context.Context,
	conn datasources.QueryConn,
	d SpatialDialect,
	schema, table, geometryColumn string,
	propertyColumns []string,
) (*FeatureCollection, error) {
	if conn == nil {
		return &FeatureCollection{Error: "Database connection unavailable."}, ErrQueryFailed
	}
	if d == nil {
		d = pgSpatial{}
	}
	if !IsSafeSQLIdent(table) || !IsSafeSQLIdent(geometryColumn) {
		return &FeatureCollection{Error: "Unsafe SQL identifier."}, ErrQueryFailed
	}
	if schema != "" && !IsSafeSQLIdent(schema) {
		return &FeatureCollection{Error: "Unsafe SQL identifier."}, ErrQueryFailed
	}

	safeProps := make([]string, 0, len(propertyColumns))
	for _, col := range propertyColumns {
		col = strings.TrimSpace(col)
		if i := strings.LastIndex(col, "."); i >= 0 {
			col = col[i+1:]
		}
		if col != "" && IsSafeSQLIdent(col) && col != geometryColumn {
			dup := false
			for _, existing := range safeProps {
				if existing == col {
					dup = true
					break
				}
			}
			if !dup {
				safeProps = append(safeProps, col)
			}
		}
	}

	qualified := qualifyTable(d, schema, table)
	geom := d.QuoteIdent(geometryColumn)
	alias := geoJSONSelectAlias(d)
	selectParts := []string{d.AsGeoJSON4326("t."+geom) + " AS " + alias}
	for _, col := range safeProps {
		selectParts = append(selectParts, "t."+d.QuoteIdent(col))
	}

	// Oracle rejects AS for table aliases (ORA-00911). SQL Server prefers bare aliases.
	fromAlias := " AS t"
	switch d.Engine() {
	case EngineSQLServer, EngineOracle:
		fromAlias = " t"
	}
	sql := "SELECT " + strings.Join(selectParts, ", ") +
		" FROM " + qualified + fromAlias +
		" WHERE t." + geom + " IS NOT NULL"
	if d.Engine() == EngineSQLServer {
		// Inject TOP after SELECT for a safety cap (no OFFSET/FETCH without ORDER BY).
		sql = "SELECT TOP (5000) " + strings.Join(selectParts, ", ") +
			" FROM " + qualified + fromAlias +
			" WHERE t." + geom + " IS NOT NULL"
	}

	rows, err := conn.Query(ctx, sql)
	if err != nil {
		// Fallbacks for engines where AsGeoJSON4326 may use unavailable functions.
		fallbacks := layerGeoJSONFallbacks(d, "t."+geom)
		var lastDetail string
		lastDetail = err.Error()
		for _, expr := range fallbacks {
			parts := []string{expr + " AS " + alias}
			parts = append(parts, selectParts[1:]...)
			trySQL := "SELECT " + strings.Join(parts, ", ") +
				" FROM " + qualified + fromAlias +
				" WHERE t." + geom + " IS NOT NULL"
			if d.Engine() == EngineSQLServer {
				trySQL = "SELECT TOP (5000) " + strings.Join(parts, ", ") +
					" FROM " + qualified + fromAlias +
					" WHERE t." + geom + " IS NOT NULL"
			}
			rows2, err2 := conn.Query(ctx, trySQL)
			if err2 != nil {
				lastDetail = err2.Error()
				continue
			}
			features, ferr := scanGeoJSONFeatures(rows2, geometryColumn)
			_ = rows2.Close()
			if ferr != nil {
				lastDetail = ferr.Error()
				continue
			}
			return &FeatureCollection{Type: "FeatureCollection", Features: features}, nil
		}
		return &FeatureCollection{Error: "Failed to load map features.", Detail: lastDetail}, ErrQueryFailed
	}
	defer rows.Close()

	features, ferr := scanGeoJSONFeatures(rows, geometryColumn)
	if ferr != nil {
		return &FeatureCollection{Error: "Failed to load map features.", Detail: ferr.Error()}, ErrQueryFailed
	}
	return &FeatureCollection{
		Type:     "FeatureCollection",
		Features: features,
	}, nil
}

func layerGeoJSONFallbacks(d SpatialDialect, geomExpr string) []string {
	if d == nil {
		return nil
	}
	switch d.Engine() {
	case EngineSQLServer:
		return []string{
			geomExpr + ".STAsText()",
			geomExpr + ".ToString()",
		}
	case EngineMySQL:
		return []string{
			"ST_AsGeoJSON(" + geomExpr + ")",
			"ST_AsText(" + geomExpr + ")",
		}
	case EngineOracle:
		return []string{
			"SDO_UTIL.TO_GEOJSON(" + geomExpr + ")",
			"SDO_UTIL.TO_WKTGEOMETRY(" + geomExpr + ")",
		}
	case EnginePostgreSQL:
		return []string{
			"ST_AsGeoJSON(" + geomExpr + ")",
			"ST_AsText(" + geomExpr + ")",
		}
	default:
		return nil
	}
}

// geoJSONSelectAlias is the SELECT alias for the geometry expression.
// Oracle rejects unquoted identifiers that start with "_" (ORA-00911).
func geoJSONSelectAlias(d SpatialDialect) string {
	if d != nil && d.Engine() == EngineOracle {
		return "jrs_geojson"
	}
	return "_jrs_geojson"
}

func isGeoJSONAliasCol(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "_jrs_geojson", "jrs_geojson", "_jrs_geometry", "jrs_geometry":
		return true
	default:
		return false
	}
}

func scanGeoJSONFeatures(rows datasources.QueryRows, geometryColumn string) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	skip := map[string]bool{
		"_jrs_geojson":  true,
		"jrs_geojson":   true,
		"_jrs_geometry": true,
		"jrs_geometry":  true,
		strings.ToLower(geometryColumn): true,
	}
	for _, g := range geometryColumnNames {
		skip[g] = true
	}

	var features []map[string]any
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		var geom any
		props := map[string]any{}
		for i, name := range cols {
			key := strings.TrimSpace(name)
			lower := strings.ToLower(key)
			val := normalizeScanValue(raw[i])
			if isGeoJSONAliasCol(lower) {
				geom = decodeGeometryValue(val)
				continue
			}
			if skip[lower] {
				continue
			}
			switch val.(type) {
			case string, float64, int64, int, bool, nil:
				props[key] = val
			default:
				// skip bytes / complex driver types in properties
			}
		}
		if geom == nil {
			continue
		}
		features = append(features, map[string]any{
			"type":       "Feature",
			"geometry":   geom,
			"properties": props,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if features == nil {
		features = []map[string]any{}
	}
	return features, nil
}

func normalizeScanValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(t)
	case int32:
		return int64(t)
	case int:
		return int64(t)
	case float32:
		return float64(t)
	default:
		return t
	}
}

func decodeGeometryValue(v any) any {
	if v == nil {
		return nil
	}
	var s string
	switch t := v.(type) {
	case string:
		s = strings.TrimSpace(t)
	case []byte:
		s = strings.TrimSpace(string(t))
	default:
		s = strings.TrimSpace(fmt.Sprint(t))
	}
	if s == "" || strings.EqualFold(s, "null") {
		return nil
	}
	// GeoJSON object / Feature
	if strings.HasPrefix(s, "{") {
		var geom any
		if err := json.Unmarshal([]byte(s), &geom); err == nil {
			if m, ok := geom.(map[string]any); ok {
				if g, ok := m["geometry"].(map[string]any); ok {
					if typ, _ := g["type"].(string); typ != "" {
						return g
					}
				}
				if typ, _ := m["type"].(string); typ != "" && !strings.EqualFold(typ, "Feature") {
					return m
				}
			}
			return geom
		}
	}
	// WKT (SQL Server STAsText, Oracle TO_WKTGEOMETRY, etc.)
	if g, ok := WKTToGeoJSON(s); ok {
		return g
	}
	return nil
}

var reInactiveSpatialFilter = regexp.MustCompile(`(?is)\(\s*\$P\{p_geometry\}\s+IS\s+NULL\s+OR\s+ST_Intersects\s*\((?:[^()]|\([^()]*\))*\)\s*\)`)

// stripInactiveSpatialFilter replaces the common Jasper spatial gate with (1=1)
// when p_geometry is not in the active filter context.
func stripInactiveSpatialFilter(sql string, filterContext map[string]any) string {
	if hasActiveGeometryFilter(filterContext) {
		return sql
	}
	return reInactiveSpatialFilter.ReplaceAllString(sql, "(1=1)")
}

func hasActiveGeometryFilter(filterContext map[string]any) bool {
	if filterContext == nil {
		return false
	}
	raw, ok := filterContext["p_geometry"]
	if !ok || raw == nil {
		return false
	}
	switch t := raw.(type) {
	case string:
		return strings.TrimSpace(t) != ""
	case []byte:
		return strings.TrimSpace(string(t)) != ""
	default:
		s := strings.TrimSpace(fmt.Sprint(t))
		return s != "" && s != "<nil>"
	}
}

func truncateSQL(sql string) string {
	sql = strings.Join(strings.Fields(sql), " ")
	if len(sql) > 500 {
		return sql[:500] + "…"
	}
	return sql
}

// stripGeometryColumnFromSelect removes the layer geometry column from the SELECT
// list so MySQL does not return raw WKB alongside ST_AsGeoJSON.
func stripGeometryColumnFromSelect(afterSelectKeyword string, geometryColumn string, d SpatialDialect) string {
	if d == nil {
		d = pgSpatial{}
	}
	geometryColumn = strings.TrimSpace(geometryColumn)
	if geometryColumn == "" || !IsSafeSQLIdent(geometryColumn) {
		return afterSelectKeyword
	}
	fromIdx := regexp.MustCompile(`(?is)\s+FROM\s+`).FindStringIndex(afterSelectKeyword)
	if fromIdx == nil {
		return afterSelectKeyword
	}
	selectList := afterSelectKeyword[:fromIdx[0]]
	rest := afterSelectKeyword[fromIdx[0]:]
	parts := splitSelectList(selectList)
	kept := make([]string, 0, len(parts))
	geomLower := strings.ToLower(geometryColumn)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if selectExprIsGeometryColumn(part, geomLower) {
			continue
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 {
		return afterSelectKeyword
	}
	return " " + strings.Join(kept, ", ") + rest
}

func selectExprIsGeometryColumn(expr, geomLower string) bool {
	e := strings.TrimSpace(expr)
	if am := regexp.MustCompile(`(?is)^(.*)\s+AS\s+(?:"([^"]+)"|` + "`([^`]+)`" + `|\[([^\]]+)\]|([A-Za-z_][\w]*))\s*$`).FindStringSubmatch(e); am != nil {
		e = strings.TrimSpace(am[1])
	}
	idents := regexp.MustCompile(`(?i)^(?:(?:"[^"]+"|` + "`[^`]+`" + `|\[[^\]]+\]|[A-Za-z_][\w]*)\.){0,2}(?:"([^"]+)"|` + "`([^`]+)`" + `|\[([^\]]+)\]|([A-Za-z_][\w]*))$`)
	m := idents.FindStringSubmatch(strings.TrimSpace(e))
	if m == nil {
		return false
	}
	name := m[1]
	if name == "" {
		name = m[2]
	}
	if name == "" {
		name = m[3]
	}
	if name == "" {
		name = m[4]
	}
	return strings.EqualFold(strings.TrimSpace(name), geomLower)
}

// TransformExtentToWKT mirrors PHP jrs_transform_extent_to_wkt.
func TransformExtentToWKT(ctx context.Context, conn datasources.QueryConn, d SpatialDialect, xmin, ymin, xmax, ymax float64, targetSRID int) (string, error) {
	if conn == nil {
		return "", fmt.Errorf("Database connection unavailable.")
	}
	if d == nil {
		d = pgSpatial{}
	}
	if targetSRID <= 0 {
		return "", fmt.Errorf("Invalid target SRID.")
	}
	var wkt string
	err := conn.QueryRow(ctx, d.ExtentToWKTSQL(), xmin, ymin, xmax, ymax, targetSRID).Scan(&wkt)
	if err != nil {
		return "", fmt.Errorf("Failed to transform map extent.")
	}
	wkt = strings.TrimSpace(wkt)
	if wkt == "" {
		return "", fmt.Errorf("Empty WKT from extent transform.")
	}
	return wkt, nil
}

// TransformGeoJSONToWKT transforms a GeoJSON geometry assumed to be EPSG:4326
// into WKT in targetSRID.
func TransformGeoJSONToWKT(ctx context.Context, conn datasources.QueryConn, d SpatialDialect, geoJSON string, targetSRID int) (string, error) {
	if conn == nil {
		return "", fmt.Errorf("Database connection unavailable.")
	}
	if d == nil {
		d = pgSpatial{}
	}
	if targetSRID <= 0 {
		return "", fmt.Errorf("Invalid target SRID.")
	}
	geoJSON = strings.TrimSpace(geoJSON)
	if geoJSON == "" {
		return "", fmt.Errorf("geometry is required")
	}

	if sql := d.GeoJSONToWKTSQL(); sql != "" {
		var wkt string
		err := conn.QueryRow(ctx, sql, geoJSON, targetSRID).Scan(&wkt)
		if err == nil {
			wkt = strings.TrimSpace(wkt)
			if wkt != "" {
				return wkt, nil
			}
		}
		// Fall through to WKT path on dialect/driver gaps.
	}

	srcWKT, err := GeoJSONToWKT(geoJSON)
	if err != nil {
		return "", err
	}
	if targetSRID == 4326 {
		return srcWKT, nil
	}
	var wkt string
	if d.Engine() == EngineMySQL {
		err = conn.QueryRow(ctx, d.WKTToWKTSQL(), srcWKT, targetSRID).Scan(&wkt)
	} else {
		err = conn.QueryRow(ctx, d.WKTToWKTSQL(), srcWKT, 4326, targetSRID).Scan(&wkt)
	}
	if err != nil {
		return "", fmt.Errorf("Failed to transform selection geometry.")
	}
	wkt = strings.TrimSpace(wkt)
	if wkt == "" {
		return "", fmt.Errorf("Empty WKT from geometry transform.")
	}
	return wkt, nil
}
