package spatial

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// LayerMeta is the resolved map layer (PHP jrs_resolve_map_layer subset).
type LayerMeta struct {
	Type            string           `json:"type"`
	SpatialCapable  bool             `json:"spatial_capable"`
	Table           string           `json:"table"`
	Schema          string           `json:"schema"`
	GeometryColumn  string           `json:"geometry_column"`
	SRID            int              `json:"srid"`
	GeometryType    string           `json:"geometry_type,omitempty"`
	ReportFields    []map[string]any `json:"report_fields"`
	GeometryColumns []string         `json:"geometry_columns"`
	Error           string           `json:"error,omitempty"`
}

// DiscoverGeometry finds the first geometry/geography column for a table.
func DiscoverGeometry(ctx context.Context, conn datasources.QueryConn, d SpatialDialect, schema, table string) (LayerMeta, error) {
	out := LayerMeta{
		Schema: schema,
		Table:  table,
	}
	if d == nil {
		d = pgSpatial{}
	}
	if schema == "" {
		out.Schema = d.DefaultSchema(nil)
		if out.Schema == "" {
			out.Schema = "public"
		}
	}
	if !IsSafeSQLIdent(table) || (out.Schema != "" && !IsSafeSQLIdent(out.Schema)) {
		out.Error = "Unsafe table identifier."
		return out, ErrLayerUnresolved
	}

	var geomCol, geomType string
	var sridVal any
	sql, argsFn := d.DiscoverGeometrySQL()
	err := conn.QueryRow(ctx, sql, argsFn(out.Schema, table)...).Scan(&geomCol, &sridVal, &geomType)
	if err == nil && strings.TrimSpace(geomCol) != "" {
		out.GeometryColumn = strings.TrimSpace(geomCol)
		out.GeometryType = strings.TrimSpace(geomType)
		if s := intFromAny(sridVal, 0); s > 0 {
			out.SRID = s
		}
	}

	// PostGIS geography_columns fallback
	if out.GeometryColumn == "" && d.Engine() == EnginePostgreSQL {
		_ = conn.QueryRow(ctx, `
SELECT f_geography_column, 4326
FROM public.geography_columns
WHERE f_table_schema = $1 AND f_table_name = $2
LIMIT 1`, out.Schema, table).Scan(&geomCol, &out.SRID)
		if strings.TrimSpace(geomCol) != "" {
			out.GeometryColumn = strings.TrimSpace(geomCol)
			out.SRID = 4326
		}
	}

	// information_schema / name heuristics when catalog missed
	if out.GeometryColumn == "" {
		attrSQL, attrArgs := d.ListAttributeColumnsSQL()
		// Re-query all columns including geometry via list geometry, then heuristics.
		listSQL, listArgs := d.ListGeometryColumnsSQL()
		rows, qerr := conn.Query(ctx, listSQL, listArgs(out.Schema, table)...)
		if qerr == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				if rows.Scan(&name) == nil && strings.TrimSpace(name) != "" {
					out.GeometryColumn = strings.TrimSpace(name)
					break
				}
			}
		}
		_ = attrSQL
		_ = attrArgs
	}

	if out.GeometryColumn == "" && d.Engine() == EnginePostgreSQL {
		_ = conn.QueryRow(ctx, `
SELECT column_name
FROM information_schema.columns
WHERE table_schema = $1
  AND table_name = $2
  AND udt_name IN ('geometry', 'geography')
ORDER BY ordinal_position
LIMIT 1`, out.Schema, table).Scan(&geomCol)
		out.GeometryColumn = strings.TrimSpace(geomCol)
	}

	if out.GeometryColumn == "" && d.Engine() == EngineMySQL {
		// Fall back: known spatial names + columns ending in _geom from information_schema.
		candidates := append([]string{}, geometryColumnNames...)
		ph := d.Placeholder(1)
		ph2 := d.Placeholder(2)
		if rows, qerr := conn.Query(ctx,
			`SELECT column_name FROM information_schema.columns WHERE table_schema = `+ph+` AND table_name = `+ph2+` ORDER BY ordinal_position`,
			out.Schema, table); qerr == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				if rows.Scan(&name) != nil {
					continue
				}
				lower := strings.ToLower(strings.TrimSpace(name))
				if isGeometryName(lower) || strings.HasSuffix(lower, "_geom") || strings.HasSuffix(lower, "_geometry") {
					candidates = append(candidates, strings.TrimSpace(name))
				}
			}
		}
		seen := map[string]bool{}
		for _, candidate := range candidates {
			if !IsSafeSQLIdent(candidate) || seen[strings.ToLower(candidate)] {
				continue
			}
			seen[strings.ToLower(candidate)] = true
			probe := "SELECT 1 FROM " + qualifyTable(d, out.Schema, table) +
				" WHERE " + d.QuoteIdent(candidate) + " IS NOT NULL" + d.LimitOne()
			var one any
			if qerr := conn.QueryRow(ctx, probe).Scan(&one); qerr == nil {
				out.GeometryColumn = candidate
				break
			}
		}
	}

	if out.GeometryColumn == "" && d.Engine() == EngineSQLServer {
		for _, candidate := range geometryColumnNames {
			if !IsSafeSQLIdent(candidate) {
				continue
			}
			probe := "SELECT TOP 1 1 FROM " + qualifyTable(d, out.Schema, table) +
				" WHERE " + d.QuoteIdent(candidate) + " IS NOT NULL"
			var one any
			if qerr := conn.QueryRow(ctx, probe).Scan(&one); qerr == nil {
				out.GeometryColumn = candidate
				break
			}
		}
	}

	if out.GeometryColumn == "" {
		out.Error = fmt.Sprintf("No geometry column found for table %s.%s.", out.Schema, table)
		return out, ErrLayerUnresolved
	}
	if !IsSafeSQLIdent(out.GeometryColumn) {
		out.Error = "Unsafe geometry column identifier."
		out.GeometryColumn = ""
		return out, ErrLayerUnresolved
	}

	if out.SRID <= 0 {
		sridSQL := d.ProbeSRIDSQL(out.Schema, table, out.GeometryColumn)
		var discovered any
		if err := conn.QueryRow(ctx, sridSQL).Scan(&discovered); err == nil {
			if s := intFromAny(discovered, 0); s > 0 {
				out.SRID = s
			} else {
				out.SRID = 4326
			}
		} else {
			out.SRID = 4326
		}
	}
	return out, nil
}

// ListGeometryColumns lists geometry column names for a table.
func ListGeometryColumns(ctx context.Context, conn datasources.QueryConn, d SpatialDialect, schema, table string) []string {
	if d == nil {
		d = pgSpatial{}
	}
	sql, argsFn := d.ListGeometryColumnsSQL()
	rows, err := conn.Query(ctx, sql, argsFn(schema, table)...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// ResolveLayer resolves schema/table/geometry for a map report definition.
func (s *Resolver) ResolveLayer(ctx context.Context, def *reports.Definition) (LayerMeta, error) {
	out := LayerMeta{
		Type:         "jasper",
		ReportFields: []map[string]any{},
	}
	if def == nil {
		out.Error = "Missing report key."
		return out, ErrInvalidReport
	}
	out.Type = def.Type()
	jrxmlPath := filepath.Join(s.JRXMLDir, def.Key+".jrxml")
	out.SpatialCapable = JRXMLHasSpatialParameters(jrxmlPath)
	out.ReportFields = ReportDatasetFields(jrxmlPath)

	mapDef := ReportMapDefinition(def)
	if out.Type != "map" && mapDef == nil {
		return out, nil
	}

	sql, err := JRXMLSQLQuery(jrxmlPath)
	if err != nil {
		out.Error = "Could not read JRXML."
		return out, err
	}
	tableInfo := SQLPrimaryTable(sql)
	if tableInfo.Table == "" {
		out.Error = "Could not determine report table from JRXML query."
		return out, ErrLayerUnresolved
	}

	dsName := def.Datasource()
	if dsName == "" {
		out.Error = "Report datasource is not configured."
		return out, ErrLayerUnresolved
	}
	ds, err := s.Datasources.Find(ctx, dsName)
	if err != nil {
		out.Error = "Failed to connect to report datasource."
		return out, err
	}
	if !ds.IsSpatialCapable() {
		out.Error = spatialRequiredMsg
		return out, datasources.ErrNotSpatial
	}
	conn, dialect, err := s.OpenSpatial(ctx, ds)
	if err != nil {
		out.Error = "Failed to connect to report datasource."
		return out, err
	}
	defer conn.Close(ctx)

	schema := tableInfo.Schema
	if schema == "" {
		schema = dialect.DefaultSchema(ds)
	}
	if schema == "" && ds.SchemaName != "" && IsSafeSQLIdent(ds.SchemaName) {
		schema = ds.SchemaName
	}
	if schema == "" && dialect.Engine() == EnginePostgreSQL {
		schema = "public"
	}

	preferred := ""
	if mapDef != nil {
		preferred = stringField(mapDef, "geometryColumn")
	}

	// Try primary FROM table first, then JOIN tables (geometry often lives on a location join).
	candidates := SQLRelationList(sql)
	if len(candidates) == 0 {
		candidates = []TableRef{{Schema: schema, Table: tableInfo.Table}}
	}
	for i := range candidates {
		if candidates[i].Schema == "" {
			candidates[i].Schema = schema
		}
	}

	var geom LayerMeta
	var errGeom error
	for _, cand := range candidates {
		if cand.Table == "" || !IsSafeSQLIdent(cand.Table) {
			continue
		}
		sch := cand.Schema
		if sch == "" {
			sch = schema
		}
		cols := ListGeometryColumns(ctx, conn, dialect, sch, cand.Table)
		out.GeometryColumns = appendUniqueStrings(out.GeometryColumns, cols...)

		discovered, derr := DiscoverGeometry(ctx, conn, dialect, sch, cand.Table)
		if preferred != "" && IsSafeSQLIdent(preferred) {
			inList := false
			for _, c := range cols {
				if c == preferred {
					inList = true
					break
				}
			}
			if inList || discovered.GeometryColumn == preferred || (discovered.GeometryColumn == "" && len(cols) == 0 && cand.Table == tableInfo.Table) {
				// Accept preferred on this table when catalog lists it, or as last-resort on primary.
				if inList || discovered.GeometryColumn == preferred {
					discovered.GeometryColumn = preferred
					discovered.Schema = sch
					discovered.Table = cand.Table
					discovered.Error = ""
					derr = nil
					if discovered.SRID <= 0 {
						sridSQL := dialect.ProbeSRIDSQL(sch, cand.Table, preferred)
						var srid int
						if qerr := conn.QueryRow(ctx, sridSQL).Scan(&srid); qerr == nil && srid > 0 {
							discovered.SRID = srid
						} else {
							discovered.SRID = 4326
						}
					}
				}
			}
		}
		if derr == nil && discovered.GeometryColumn != "" && discovered.Error == "" {
			geom = discovered
			errGeom = nil
			break
		}
		if geom.GeometryColumn == "" {
			geom = discovered
			errGeom = derr
		}
	}

	// If still unresolved but preferred was set, bind it to the first table that lists it.
	if (geom.GeometryColumn == "" || geom.Error != "") && preferred != "" && IsSafeSQLIdent(preferred) {
		for _, cand := range candidates {
			sch := cand.Schema
			if sch == "" {
				sch = schema
			}
			cols := ListGeometryColumns(ctx, conn, dialect, sch, cand.Table)
			for _, c := range cols {
				if c != preferred {
					continue
				}
				geom = LayerMeta{Schema: sch, Table: cand.Table, GeometryColumn: preferred, SRID: 4326}
				sridSQL := dialect.ProbeSRIDSQL(sch, cand.Table, preferred)
				var srid int
				if qerr := conn.QueryRow(ctx, sridSQL).Scan(&srid); qerr == nil && srid > 0 {
					geom.SRID = srid
				}
				errGeom = nil
				break
			}
			if geom.GeometryColumn != "" && geom.Error == "" && errGeom == nil {
				break
			}
		}
	}

	out.Schema = geom.Schema
	out.Table = geom.Table
	out.GeometryColumn = geom.GeometryColumn
	out.SRID = geom.SRID
	out.GeometryType = geom.GeometryType
	out.Error = geom.Error
	if errGeom != nil || out.GeometryColumn == "" || out.Error != "" {
		if out.Error == "" {
			out.Error = "Map layer could not be resolved."
		}
		return out, ErrLayerUnresolved
	}
	return out, nil
}

func appendUniqueStrings(dst []string, add ...string) []string {
	seen := map[string]bool{}
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range add {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		dst = append(dst, s)
	}
	return dst
}
