package datasources

import (
	"context"
	"fmt"
	"strings"
)

// RunBoundParameterLookup executes pre-bound lookup SQL and returns [{label,value}, …]
// using display/value column names (case-insensitive; falls back to column 0).
func RunBoundParameterLookup(ctx context.Context, ds *Datasource, connector Connector, sqlText string, args []any, displayField, valueField string) ([]map[string]any, error) {
	if ds == nil {
		return nil, fmt.Errorf("datasource required")
	}
	if connector == nil {
		return nil, fmt.Errorf("connector required")
	}
	sqlText = strings.TrimSpace(sqlText)
	sqlText = strings.TrimRight(sqlText, " \t\r\n;")
	if sqlText == "" {
		return nil, fmt.Errorf("lookup SQL required")
	}

	conn, err := openLookupConn(ctx, ds, connector)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	return fetchLookupOptions(ctx, conn, sqlText, args, displayField, valueField)
}

func openLookupConn(ctx context.Context, ds *Datasource, connector Connector) (QueryConn, error) {
	if opener, ok := connector.(SpatialOpener); ok && ds.IsSpatialCapable() {
		return opener.ConnectSpatial(ctx, ds)
	}
	switch {
	case ds.IsPostgreSQL():
		conn, err := connector.ConnectPG(ctx, ds)
		if err != nil {
			return nil, err
		}
		return AdaptPG(conn), nil
	case ds.IsMySQL():
		opener, ok := connector.(MySQLOpener)
		if !ok {
			return nil, fmt.Errorf("mysql connector unavailable")
		}
		db, err := opener.OpenMySQL(ctx, ds)
		if err != nil {
			return nil, err
		}
		return AdaptSQL(db), nil
	case ds.IsOracle():
		opener, ok := connector.(OracleOpener)
		if !ok {
			return nil, fmt.Errorf("oracle connector unavailable")
		}
		db, err := opener.OpenOracle(ctx, ds)
		if err != nil {
			return nil, err
		}
		return AdaptSQL(db), nil
	case ds.IsSQLServer():
		opener, ok := connector.(SQLServerOpener)
		if !ok {
			return nil, fmt.Errorf("sqlserver connector unavailable")
		}
		db, err := opener.OpenMSSQL(ctx, ds)
		if err != nil {
			return nil, err
		}
		return AdaptSQL(db), nil
	default:
		return nil, fmt.Errorf("unsupported datasource type %q for parameter lookup", ds.DBType)
	}
}

func fetchLookupOptions(ctx context.Context, conn QueryConn, query string, args []any, displayField, valueField string) ([]map[string]any, error) {
	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return []map[string]any{}, nil
	}

	dIdx := colIndexCI(cols, displayField)
	vIdx := colIndexCI(cols, valueField)
	if dIdx < 0 {
		dIdx = 0
	}
	if vIdx < 0 {
		vIdx = dIdx
	}

	seen := map[string]bool{}
	options := make([]map[string]any, 0)
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		vals := make([]any, len(raw))
		for i, v := range raw {
			vals[i] = normalizeOptionValue(v)
		}
		label := optionCellString(vals[dIdx])
		value := optionCellString(vals[vIdx])
		if value == "" {
			continue
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		if label == "" {
			label = value
		}
		options = append(options, map[string]any{"label": label, "value": value})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return options, nil
}

func colIndexCI(cols []string, name string) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return -1
	}
	lower := strings.ToLower(name)
	for i, c := range cols {
		if strings.ToLower(strings.TrimSpace(c)) == lower {
			return i
		}
	}
	return -1
}

func optionCellString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
