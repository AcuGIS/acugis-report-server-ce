package datasources

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ExpandSQLParameterOptions mirrors PHP jrs_expand_sql_parameter_options:
// for each parameter with source "sql:…", run the query on the report datasource
// and set options to [{label,value}, …]. Failures leave the definition unchanged
// for that parameter (empty options), matching PHP's silent empty return.
func ExpandSQLParameterOptions(ctx context.Context, definition map[string]any, finder Finder, connector Connector) map[string]any {
	if definition == nil || finder == nil || connector == nil {
		return definition
	}

	params, key := parameterList(definition)
	if params == nil {
		return definition
	}

	dsName := strings.TrimSpace(stringAny(definition["datasource"]))
	if dsName == "" {
		dsName = strings.TrimSpace(stringAny(definition["dataSource"]))
	}
	if dsName == "" {
		return definition
	}

	ds, err := finder.Find(ctx, dsName)
	if err != nil || ds == nil {
		return definition
	}

	switch {
	case ds.IsPostgreSQL():
		return expandSQLParameterOptionsPG(ctx, definition, params, key, ds, connector)
	case ds.IsMySQL():
		return expandSQLParameterOptionsMySQL(ctx, definition, params, key, ds, connector)
	case ds.IsOracle():
		return expandSQLParameterOptionsOracle(ctx, definition, params, key, ds, connector)
	default:
		return definition
	}
}

func expandSQLParameterOptionsPG(ctx context.Context, definition map[string]any, params []any, key string, ds *Datasource, connector Connector) map[string]any {
	conn, err := connector.ConnectPG(ctx, ds)
	if err != nil {
		return definition
	}
	defer conn.Close(ctx)

	changed := false
	for i, raw := range params {
		param, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		sqlText, ok := sqlSourceQuery(param)
		if !ok {
			continue
		}
		opts, ferr := fetchSQLOptionRows(ctx, conn, sqlText)
		if ferr != nil {
			continue
		}
		param["options"] = opts
		params[i] = param
		changed = true
	}
	if !changed {
		return definition
	}
	return withExpandedParameters(definition, params, key)
}

func expandSQLParameterOptionsMySQL(ctx context.Context, definition map[string]any, params []any, key string, ds *Datasource, connector Connector) map[string]any {
	opener, ok := connector.(MySQLOpener)
	if !ok {
		return definition
	}
	db, err := opener.OpenMySQL(ctx, ds)
	if err != nil {
		return definition
	}
	defer db.Close()

	changed := false
	for i, raw := range params {
		param, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		sqlText, ok := sqlSourceQuery(param)
		if !ok {
			continue
		}
		opts, ferr := fetchSQLOptionRowsDB(ctx, db, sqlText)
		if ferr != nil {
			continue
		}
		param["options"] = opts
		params[i] = param
		changed = true
	}
	if !changed {
		return definition
	}
	return withExpandedParameters(definition, params, key)
}

func expandSQLParameterOptionsOracle(ctx context.Context, definition map[string]any, params []any, key string, ds *Datasource, connector Connector) map[string]any {
	opener, ok := connector.(OracleOpener)
	if !ok {
		return definition
	}
	db, err := opener.OpenOracle(ctx, ds)
	if err != nil {
		return definition
	}
	defer db.Close()

	changed := false
	for i, raw := range params {
		param, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		sqlText, ok := sqlSourceQuery(param)
		if !ok {
			continue
		}
		opts, ferr := fetchSQLOptionRowsDB(ctx, db, sqlText)
		if ferr != nil {
			continue
		}
		param["options"] = opts
		params[i] = param
		changed = true
	}
	if !changed {
		return definition
	}
	return withExpandedParameters(definition, params, key)
}

func sqlSourceQuery(param map[string]any) (string, bool) {
	source := strings.TrimSpace(stringAny(param["source"]))
	if len(source) < 4 || !strings.EqualFold(source[:4], "sql:") {
		return "", false
	}
	sqlText := strings.TrimSpace(source[4:])
	sqlText = strings.TrimRight(sqlText, " \t\r\n;")
	if sqlText == "" {
		return "", false
	}
	return sqlText, true
}

func withExpandedParameters(definition map[string]any, params []any, key string) map[string]any {
	out := make(map[string]any, len(definition)+1)
	for k, v := range definition {
		out[k] = v
	}
	out["parameters"] = params
	if key == "params" {
		delete(out, "params")
	}
	return out
}

func parameterList(definition map[string]any) ([]any, string) {
	for _, key := range []string{"parameters", "params"} {
		v, ok := definition[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case []any:
			return t, key
		case []map[string]any:
			out := make([]any, len(t))
			for i, m := range t {
				out[i] = m
			}
			return out, key
		}
	}
	return nil, ""
}

func fetchSQLOptionRows(ctx context.Context, conn Conn, sql string) ([]map[string]any, error) {
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var options []map[string]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		if opt, ok := optionFromValues(vals); ok {
			options = append(options, opt)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if options == nil {
		options = []map[string]any{}
	}
	return options, nil
}

func fetchSQLOptionRowsDB(ctx context.Context, db *sql.DB, query string) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query)
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

	var options []map[string]any
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
		if opt, ok := optionFromValues(vals); ok {
			options = append(options, opt)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if options == nil {
		options = []map[string]any{}
	}
	return options, nil
}

func optionFromValues(vals []any) (map[string]any, bool) {
	if len(vals) == 0 {
		return nil, false
	}
	if len(vals) == 1 {
		return map[string]any{"label": vals[0], "value": vals[0]}, true
	}
	return map[string]any{"label": vals[0], "value": vals[1]}, true
}

func normalizeOptionValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(t)
	default:
		return v
	}
}

func stringAny(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}
