package spatial

import (
	"fmt"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

// Engine identifiers for Map Report spatial dialects.
const (
	EnginePostgreSQL = "postgresql"
	EngineMySQL      = "mysql"
	EngineSQLServer  = "sqlserver"
	EngineOracle     = "oracle"
	EngineDb2        = "db2"
)

// Known geometry / spatial column name heuristics (case-insensitive).
var geometryColumnNames = []string{
	"geom", "geometry", "the_geom", "wkb_geometry", "shape", "way", "geog", "geography", "location",
}

// mysqlSpatialDataTypes matches INFORMATION_SCHEMA data_type / column_type values
// for MySQL/MariaDB spatial columns (POINT, etc. — not only "geometry").
const mysqlSpatialTypePred = `(
  LOWER(data_type) IN (
    'geometry', 'point', 'linestring', 'polygon',
    'multipoint', 'multilinestring', 'multipolygon',
    'geomcollection', 'geometrycollection'
  )
  OR LOWER(column_type) LIKE 'geometry%'
  OR LOWER(column_type) LIKE 'point%'
  OR LOWER(column_type) LIKE 'linestring%'
  OR LOWER(column_type) LIKE 'polygon%'
  OR LOWER(column_type) LIKE 'multipoint%'
  OR LOWER(column_type) LIKE 'multilinestring%'
  OR LOWER(column_type) LIKE 'multipolygon%'
  OR LOWER(column_type) LIKE 'geomcollection%'
  OR LOWER(column_type) LIKE 'geometrycollection%'
)`

// SpatialDialect abstracts quoting, placeholders, and spatial SQL per engine.
type SpatialDialect interface {
	Engine() string
	QuoteIdent(name string) string
	Placeholder(n int) string // 1-based
	DefaultSchema(ds *datasources.Datasource) string
	// CastInteger / CastFloat wrap a placeholder for typed Jasper binds.
	CastInteger(placeholder string) string
	CastFloat(placeholder string) string
	// AsGeoJSON4326 returns a SQL expression yielding GeoJSON text in EPSG:4326.
	AsGeoJSON4326(geomExpr string) string
	// ExtentToWKTSQL returns SQL with 5 placeholders (xmin,ymin,xmax,ymax,targetSRID)
	// yielding WKT text in the target SRID. Source envelope is EPSG:4326.
	ExtentToWKTSQL() string
	// GeoJSONToWKTSQL returns SQL with 2 placeholders (geojson text, targetSRID)
	// yielding WKT. Returns empty if the dialect cannot ingest GeoJSON natively
	// (caller should convert GeoJSON→WKT in Go then use WKTToWKTSQL).
	GeoJSONToWKTSQL() string
	// WKTToWKTSQL returns SQL with 3 placeholders (wkt, sourceSRID, targetSRID)
	// transforming WKT between SRIDs. Used when GeoJSONToWKTSQL is unavailable
	// or as a fallback after in-process GeoJSON→WKT conversion.
	WKTToWKTSQL() string
	// DiscoverGeometrySQL / ListGeometryColumnsSQL / ListAttributeColumnsSQL
	// and ProbeSRIDSQL are engine-specific catalog queries.
	DiscoverGeometrySQL() (sql string, argsFn func(schema, table string) []any)
	ListGeometryColumnsSQL() (sql string, argsFn func(schema, table string) []any)
	ListAttributeColumnsSQL() (sql string, argsFn func(schema, table string) []any)
	ProbeSRIDSQL(schema, table, geomCol string) string
	LimitOne() string
}

// DialectFor returns the spatial dialect for a datasource.
func DialectFor(ds *datasources.Datasource) (SpatialDialect, error) {
	if ds == nil {
		return nil, datasources.ErrNotFound
	}
	switch datasources.NormalizeDBType(ds.DBType) {
	case EnginePostgreSQL:
		return pgSpatial{}, nil
	case EngineMySQL:
		return mysqlSpatial{}, nil
	case EngineSQLServer:
		return mssqlSpatial{}, nil
	case EngineOracle:
		return oracleSpatial{}, nil
	case EngineDb2:
		return db2Spatial{}, nil
	default:
		return nil, datasources.ErrNotSpatial
	}
}

func isGeometryName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, g := range geometryColumnNames {
		if lower == g {
			return true
		}
	}
	return false
}

func qualifyTable(d SpatialDialect, schema, table string) string {
	if schema == "" {
		return d.QuoteIdent(table)
	}
	return d.QuoteIdent(schema) + "." + d.QuoteIdent(table)
}

// --- PostgreSQL / PostGIS ---

type pgSpatial struct{}

func (pgSpatial) Engine() string { return EnginePostgreSQL }
func (pgSpatial) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
func (pgSpatial) Placeholder(n int) string { return fmt.Sprintf("$%d", n) }
func (pgSpatial) DefaultSchema(ds *datasources.Datasource) string {
	if ds != nil && strings.TrimSpace(ds.SchemaName) != "" {
		return strings.TrimSpace(ds.SchemaName)
	}
	return "public"
}
func (pgSpatial) CastInteger(ph string) string { return "(" + ph + ")::int" }
func (pgSpatial) CastFloat(ph string) string   { return "(" + ph + ")::float8" }
func (pgSpatial) AsGeoJSON4326(geomExpr string) string {
	return "ST_AsGeoJSON(ST_Transform(" + geomExpr + ", 4326))"
}
func (pgSpatial) ExtentToWKTSQL() string {
	return `SELECT ST_AsText(ST_Transform(ST_MakeEnvelope($1::float8, $2::float8, $3::float8, $4::float8, 4326), $5::int)) AS wkt`
}
func (pgSpatial) GeoJSONToWKTSQL() string {
	return `SELECT ST_AsText(ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($1::text), 4326), $2::int)) AS wkt`
}
func (pgSpatial) WKTToWKTSQL() string {
	return `SELECT ST_AsText(ST_Transform(ST_GeomFromText($1::text, $2::int), $3::int)) AS wkt`
}
func (pgSpatial) DiscoverGeometrySQL() (string, func(schema, table string) []any) {
	return `
SELECT f_geometry_column, srid, type
FROM public.geometry_columns
WHERE f_table_schema = $1 AND f_table_name = $2
LIMIT 1`, func(schema, table string) []any { return []any{schema, table} }
}
func (pgSpatial) ListGeometryColumnsSQL() (string, func(schema, table string) []any) {
	return `
SELECT f_geometry_column FROM public.geometry_columns
WHERE f_table_schema = $1 AND f_table_name = $2
ORDER BY f_geometry_column`, func(schema, table string) []any { return []any{schema, table} }
}
func (pgSpatial) ListAttributeColumnsSQL() (string, func(schema, table string) []any) {
	return `
SELECT column_name, udt_name
FROM information_schema.columns
WHERE table_schema = $1
  AND table_name = $2
  AND udt_name NOT IN ('geometry', 'geography')
ORDER BY ordinal_position`, func(schema, table string) []any { return []any{schema, table} }
}
func (d pgSpatial) ProbeSRIDSQL(schema, table, geomCol string) string {
	return "SELECT ST_SRID(" + d.QuoteIdent(geomCol) + ") AS srid FROM " +
		qualifyTable(d, schema, table) + " WHERE " + d.QuoteIdent(geomCol) + " IS NOT NULL LIMIT 1"
}
func (pgSpatial) LimitOne() string { return " LIMIT 1" }

// --- MySQL / MariaDB spatial ---

type mysqlSpatial struct{}

func (mysqlSpatial) Engine() string { return EngineMySQL }
func (mysqlSpatial) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
func (mysqlSpatial) Placeholder(n int) string { return "?" }
func (mysqlSpatial) DefaultSchema(ds *datasources.Datasource) string {
	if ds == nil {
		return ""
	}
	if s := strings.TrimSpace(ds.SchemaName); s != "" {
		return s
	}
	return strings.TrimSpace(ds.DBName)
}
func (mysqlSpatial) CastInteger(ph string) string { return "CAST(" + ph + " AS SIGNED)" }
func (mysqlSpatial) CastFloat(ph string) string   { return "CAST(" + ph + " AS DOUBLE)" }
func (mysqlSpatial) AsGeoJSON4326(geomExpr string) string {
	// Use ST_AsGeoJSON only — ST_Transform is MySQL 8.0+ and fails for SRID 0
	// (common for lon/lat POINT columns such as classicmodels.location).
	return "ST_AsGeoJSON(" + geomExpr + ")"
}
func (mysqlSpatial) ExtentToWKTSQL() string {
	// Args: xmin, ymin, xmax, ymax, targetSRID (SRID consumed for arity; envelope is WGS84).
	// Avoid ST_SRID(g, srid) / ST_Transform — not portable across MySQL 5.7 / MariaDB.
	return `SELECT ST_AsText(ST_MakeEnvelope(?, ?, ?, ?)) AS wkt FROM (SELECT ? AS _jrs_srid) AS _jrs_srid_wrap`
}
func (mysqlSpatial) GeoJSONToWKTSQL() string {
	// Args: geojson, targetSRID
	return `SELECT ST_AsText(ST_GeomFromGeoJSON(?)) AS wkt FROM (SELECT ? AS _jrs_srid) AS _jrs_srid_wrap`
}
func (mysqlSpatial) WKTToWKTSQL() string {
	// Args: wkt, targetSRID
	return `SELECT ST_AsText(ST_GeomFromText(?, IFNULL(NULLIF(?, 0), 4326))) AS wkt`
}
func (mysqlSpatial) DiscoverGeometrySQL() (string, func(schema, table string) []any) {
	return `
SELECT column_name, 0, UPPER(IFNULL(column_type, data_type))
FROM information_schema.columns
WHERE table_schema = ? AND table_name = ?
  AND ` + mysqlSpatialTypePred + `
ORDER BY ordinal_position
LIMIT 1`, func(schema, table string) []any { return []any{schema, table} }
}
func (mysqlSpatial) ListGeometryColumnsSQL() (string, func(schema, table string) []any) {
	return `
SELECT column_name
FROM information_schema.columns
WHERE table_schema = ? AND table_name = ?
  AND ` + mysqlSpatialTypePred + `
ORDER BY ordinal_position`, func(schema, table string) []any { return []any{schema, table} }
}
func (mysqlSpatial) ListAttributeColumnsSQL() (string, func(schema, table string) []any) {
	return `
SELECT column_name, data_type
FROM information_schema.columns
WHERE table_schema = ? AND table_name = ?
  AND NOT (` + mysqlSpatialTypePred + `)
ORDER BY ordinal_position`, func(schema, table string) []any { return []any{schema, table} }
}
func (d mysqlSpatial) ProbeSRIDSQL(schema, table, geomCol string) string {
	return "SELECT ST_SRID(" + d.QuoteIdent(geomCol) + ") AS srid FROM " +
		qualifyTable(d, schema, table) + " WHERE " + d.QuoteIdent(geomCol) + " IS NOT NULL LIMIT 1"
}
func (mysqlSpatial) LimitOne() string { return " LIMIT 1" }

// --- Microsoft SQL Server ---

type mssqlSpatial struct{}

func (mssqlSpatial) Engine() string { return EngineSQLServer }
func (mssqlSpatial) QuoteIdent(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}
func (mssqlSpatial) Placeholder(n int) string { return fmt.Sprintf("@p%d", n) }
func (mssqlSpatial) DefaultSchema(ds *datasources.Datasource) string {
	if ds != nil && strings.TrimSpace(ds.SchemaName) != "" {
		return strings.TrimSpace(ds.SchemaName)
	}
	return "dbo"
}
func (mssqlSpatial) CastInteger(ph string) string { return "CAST(" + ph + " AS INT)" }
func (mssqlSpatial) CastFloat(ph string) string   { return "CAST(" + ph + " AS FLOAT)" }
func (mssqlSpatial) AsGeoJSON4326(geomExpr string) string {
	// STAsGeoJSON() is SQL Server 2022 / Azure only; STTransform does not exist on
	// geometry/geography. Emit WKT via STAsText — decodeGeometryValue converts to GeoJSON.
	return geomExpr + ".STAsText()"
}
func (mssqlSpatial) ExtentToWKTSQL() string {
	poly := `'POLYGON((' +
    CAST(@p1 AS varchar(32)) + ' ' + CAST(@p2 AS varchar(32)) + ',' +
    CAST(@p3 AS varchar(32)) + ' ' + CAST(@p2 AS varchar(32)) + ',' +
    CAST(@p3 AS varchar(32)) + ' ' + CAST(@p4 AS varchar(32)) + ',' +
    CAST(@p1 AS varchar(32)) + ' ' + CAST(@p4 AS varchar(32)) + ',' +
    CAST(@p1 AS varchar(32)) + ' ' + CAST(@p2 AS varchar(32)) +
  '))'`
	return `SELECT CASE
  WHEN @p5 = 4326 THEN geography::STGeomFromText(` + poly + `, 4326).STAsText()
  ELSE geometry::STGeomFromText(` + poly + `, 4326).STTransform(@p5).STAsText()
END AS wkt`
}
func (mssqlSpatial) GeoJSONToWKTSQL() string {
	// No native GeoJSON→geometry in older SQL Server; use Go conversion + WKTToWKTSQL.
	return ""
}
func (mssqlSpatial) WKTToWKTSQL() string {
	// When target SRID is 4326, reproject via geography; otherwise return as-is with SetSRID.
	return `SELECT CASE
  WHEN @p3 = 4326 THEN geography::STGeomFromText(@p1, @p2).STAsText()
  WHEN @p2 = @p3 THEN geometry::STGeomFromText(@p1, @p2).STAsText()
  ELSE geometry::STGeomFromText(@p1, @p2).STTransform(@p3).STAsText()
END AS wkt`
}
func (mssqlSpatial) DiscoverGeometrySQL() (string, func(schema, table string) []any) {
	return `
SELECT c.name,
  CASE WHEN t.name = 'geography' THEN 4326 ELSE ISNULL(c.column_id * 0, 0) END,
  UPPER(t.name)
FROM sys.columns c
JOIN sys.types t ON c.user_type_id = t.user_type_id
JOIN sys.tables tb ON c.object_id = tb.object_id
JOIN sys.schemas s ON tb.schema_id = s.schema_id
WHERE s.name = @p1 AND tb.name = @p2
  AND t.name IN ('geometry', 'geography')
ORDER BY c.column_id
OFFSET 0 ROWS FETCH NEXT 1 ROWS ONLY`, func(schema, table string) []any { return []any{schema, table} }
}
func (mssqlSpatial) ListGeometryColumnsSQL() (string, func(schema, table string) []any) {
	return `
SELECT c.name
FROM sys.columns c
JOIN sys.types t ON c.user_type_id = t.user_type_id
JOIN sys.tables tb ON c.object_id = tb.object_id
JOIN sys.schemas s ON tb.schema_id = s.schema_id
WHERE s.name = @p1 AND tb.name = @p2
  AND t.name IN ('geometry', 'geography')
ORDER BY c.name`, func(schema, table string) []any { return []any{schema, table} }
}
func (mssqlSpatial) ListAttributeColumnsSQL() (string, func(schema, table string) []any) {
	return `
SELECT c.name, t.name
FROM sys.columns c
JOIN sys.types t ON c.user_type_id = t.user_type_id
JOIN sys.tables tb ON c.object_id = tb.object_id
JOIN sys.schemas s ON tb.schema_id = s.schema_id
WHERE s.name = @p1 AND tb.name = @p2
  AND t.name NOT IN ('geometry', 'geography')
ORDER BY c.column_id`, func(schema, table string) []any { return []any{schema, table} }
}
func (d mssqlSpatial) ProbeSRIDSQL(schema, table, geomCol string) string {
	q := qualifyTable(d, schema, table)
	g := d.QuoteIdent(geomCol)
	return "SELECT TOP 1 " + g + ".STSrid AS srid FROM " + q + " WHERE " + g + " IS NOT NULL"
}
func (mssqlSpatial) LimitOne() string { return "" } // use TOP in ProbeSRIDSQL

// --- Oracle Spatial ---

type oracleSpatial struct{}

func (oracleSpatial) Engine() string { return EngineOracle }
func (oracleSpatial) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(strings.ToUpper(name), `"`, `""`) + `"`
}
func (oracleSpatial) Placeholder(n int) string { return fmt.Sprintf(":%d", n) }
func (oracleSpatial) DefaultSchema(ds *datasources.Datasource) string {
	if ds != nil && strings.TrimSpace(ds.SchemaName) != "" {
		return strings.ToUpper(strings.TrimSpace(ds.SchemaName))
	}
	if ds != nil && strings.TrimSpace(ds.Username) != "" {
		return strings.ToUpper(strings.TrimSpace(ds.Username))
	}
	return ""
}
func (oracleSpatial) CastInteger(ph string) string { return "CAST(" + ph + " AS NUMBER)" }
func (oracleSpatial) CastFloat(ph string) string   { return "CAST(" + ph + " AS BINARY_DOUBLE)" }
func (oracleSpatial) AsGeoJSON4326(geomExpr string) string {
	// Prefer WKT — TO_GEOJSON requires newer Oracle Spatial; WKT is widely available.
	// decodeGeometryValue converts WKT → GeoJSON.
	return "SDO_UTIL.TO_WKTGEOMETRY(" + geomExpr + ")"
}
func (oracleSpatial) ExtentToWKTSQL() string {
	return `SELECT SDO_UTIL.TO_WKTGEOMETRY(
  SDO_CS.TRANSFORM(
    SDO_GEOMETRY(2003, 4326, NULL,
      SDO_ELEM_INFO_ARRAY(1, 1003, 3),
      SDO_ORDINATE_ARRAY(:1, :2, :3, :4)
    ), :5
  )
) AS wkt FROM DUAL`
}
func (oracleSpatial) GeoJSONToWKTSQL() string {
	return `SELECT SDO_UTIL.TO_WKTGEOMETRY(
  SDO_CS.TRANSFORM(SDO_UTIL.FROM_GEOJSON(:1, NULL, 4326), :2)
) AS wkt FROM DUAL`
}
func (oracleSpatial) WKTToWKTSQL() string {
	return `SELECT SDO_UTIL.TO_WKTGEOMETRY(
  SDO_CS.TRANSFORM(SDO_GEOMETRY(:1, :2), :3)
) AS wkt FROM DUAL`
}
func (oracleSpatial) DiscoverGeometrySQL() (string, func(schema, table string) []any) {
	return `
SELECT COLUMN_NAME, NVL(SRID, 0), 'SDO_GEOMETRY'
FROM ALL_SDO_GEOM_METADATA
WHERE OWNER = :1 AND TABLE_NAME = :2
  AND ROWNUM = 1`, func(schema, table string) []any {
		return []any{strings.ToUpper(schema), strings.ToUpper(table)}
	}
}
func (oracleSpatial) ListGeometryColumnsSQL() (string, func(schema, table string) []any) {
	return `
SELECT COLUMN_NAME
FROM ALL_SDO_GEOM_METADATA
WHERE OWNER = :1 AND TABLE_NAME = :2
ORDER BY COLUMN_NAME`, func(schema, table string) []any {
		return []any{strings.ToUpper(schema), strings.ToUpper(table)}
	}
}
func (oracleSpatial) ListAttributeColumnsSQL() (string, func(schema, table string) []any) {
	return `
SELECT COLUMN_NAME, DATA_TYPE
FROM ALL_TAB_COLUMNS
WHERE OWNER = :1 AND TABLE_NAME = :2
  AND DATA_TYPE <> 'SDO_GEOMETRY'
ORDER BY COLUMN_ID`, func(schema, table string) []any {
		return []any{strings.ToUpper(schema), strings.ToUpper(table)}
	}
}
func (d oracleSpatial) ProbeSRIDSQL(schema, table, geomCol string) string {
	return "SELECT SRID FROM ALL_SDO_GEOM_METADATA WHERE OWNER = '" +
		strings.ReplaceAll(strings.ToUpper(schema), "'", "''") +
		"' AND TABLE_NAME = '" +
		strings.ReplaceAll(strings.ToUpper(table), "'", "''") +
		"' AND COLUMN_NAME = '" +
		strings.ReplaceAll(strings.ToUpper(geomCol), "'", "''") +
		"' AND ROWNUM = 1"
}
func (oracleSpatial) LimitOne() string { return "" }

// --- IBM Db2 Spatial (DB2GSE.ST_GEOMETRY) ---

type db2Spatial struct{}

func (db2Spatial) Engine() string { return EngineDb2 }
func (db2Spatial) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
func (db2Spatial) Placeholder(n int) string { return "?" }
func (db2Spatial) DefaultSchema(ds *datasources.Datasource) string {
	if ds != nil && strings.TrimSpace(ds.SchemaName) != "" {
		return strings.ToUpper(strings.TrimSpace(ds.SchemaName))
	}
	if ds != nil && strings.TrimSpace(ds.Username) != "" {
		return strings.ToUpper(strings.TrimSpace(ds.Username))
	}
	return "DB2INST1"
}
func (db2Spatial) CastInteger(ph string) string { return "CAST(" + ph + " AS INTEGER)" }
func (db2Spatial) CastFloat(ph string) string   { return "CAST(" + ph + " AS DOUBLE)" }
func (db2Spatial) AsGeoJSON4326(geomExpr string) string {
	return "DB2GSE.ST_AsGeoJSON(DB2GSE.ST_Transform(" + geomExpr + ", 4326))"
}
func (db2Spatial) ExtentToWKTSQL() string {
	// Placeholders: xmin, ymin, xmax, ymax, targetSRID (arity matches SpatialDialect contract).
	return `SELECT CAST(NULL AS CLOB) AS wkt FROM SYSIBM.SYSDUMMY1 WHERE CAST(? AS DOUBLE) IS NOT NULL AND CAST(? AS DOUBLE) IS NOT NULL AND CAST(? AS DOUBLE) IS NOT NULL AND CAST(? AS DOUBLE) IS NOT NULL AND CAST(? AS INTEGER) IS NOT NULL`
}
func (db2Spatial) GeoJSONToWKTSQL() string { return "" }
func (db2Spatial) WKTToWKTSQL() string {
	return `SELECT DB2GSE.ST_AsText(DB2GSE.ST_Transform(DB2GSE.ST_GeomFromText(?, ?), ?)) AS wkt FROM SYSIBM.SYSDUMMY1`
}
func (db2Spatial) DiscoverGeometrySQL() (string, func(schema, table string) []any) {
	return `
SELECT COLNAME, 0, TYPENAME
FROM SYSCAT.COLUMNS
WHERE TABSCHEMA = ? AND TABNAME = ?
  AND (TYPENAME LIKE 'ST_%' OR TYPENAME = 'ST_GEOMETRY')
ORDER BY COLNO
FETCH FIRST 1 ROW ONLY`, func(schema, table string) []any {
		return []any{strings.ToUpper(schema), strings.ToUpper(table)}
	}
}
func (db2Spatial) ListGeometryColumnsSQL() (string, func(schema, table string) []any) {
	return `
SELECT COLNAME
FROM SYSCAT.COLUMNS
WHERE TABSCHEMA = ? AND TABNAME = ?
  AND (TYPENAME LIKE 'ST_%' OR TYPENAME = 'ST_GEOMETRY')
ORDER BY COLNAME`, func(schema, table string) []any {
		return []any{strings.ToUpper(schema), strings.ToUpper(table)}
	}
}
func (db2Spatial) ListAttributeColumnsSQL() (string, func(schema, table string) []any) {
	return `
SELECT COLNAME, TYPENAME
FROM SYSCAT.COLUMNS
WHERE TABSCHEMA = ? AND TABNAME = ?
  AND NOT (TYPENAME LIKE 'ST_%' OR TYPENAME = 'ST_GEOMETRY')
ORDER BY COLNO`, func(schema, table string) []any {
		return []any{strings.ToUpper(schema), strings.ToUpper(table)}
	}
}
func (d db2Spatial) ProbeSRIDSQL(schema, table, geomCol string) string {
	return "SELECT DB2GSE.ST_SRID(" + d.QuoteIdent(geomCol) + ") AS srid FROM " +
		qualifyTable(d, schema, table) + " WHERE " + d.QuoteIdent(geomCol) + " IS NOT NULL FETCH FIRST 1 ROW ONLY"
}
func (db2Spatial) LimitOne() string { return " FETCH FIRST 1 ROW ONLY" }
