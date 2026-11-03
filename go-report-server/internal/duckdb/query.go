package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

func executeTypedRows(ctx context.Context, db *sql.DB, sqlText string, params map[string]any, limit int) ([]map[string]any, error) {
	if err := ensureReadOnly(sqlText); err != nil {
		return nil, err
	}
	bound, err := bindParameters(sqlText, params)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	log.Printf("[acugis-duckdb] fill query start limit=%d", limit)

	rows, err := db.QueryContext(ctx, bound.SQL, bound.Values...)
	if err != nil {
		return nil, fmt.Errorf("DuckDB query failed: %s", SanitizeErrorMessage(err.Error()))
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		if limit > 0 && len(out) >= limit {
			break
		}
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, name := range cols {
			m[name] = jasperValue(raw[i])
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	log.Printf("[acugis-duckdb] fill query done duration=%s rows=%d", time.Since(start), len(out))
	return out, nil
}

func inspectFileColumns(ctx context.Context, db *sql.DB, entry FileEntry) ([]ColumnInfo, error) {
	src := entry.RemoteSource()
	if err := ValidateRemoteSource(src); err != nil {
		return nil, err
	}
	reader, _, err := ReaderFunctionForEntry(entry)
	if err != nil {
		return nil, err
	}
	// DESCRIBE read_parquet('https://...') uses Parquet footer / CSV header — does not download the full file.
	q := fmt.Sprintf("DESCRIBE SELECT * FROM %s(%s)", reader, sqlStringLiteral(src))
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("inspect remote file schema failed: %s", SanitizeErrorMessage(err.Error()))
	}
	defer rows.Close()
	var out []ColumnInfo
	for rows.Next() {
		var name, typ string
		var nullability, key, def, extra sql.NullString
		if err := rows.Scan(&name, &typ, &nullability, &key, &def, &extra); err != nil {
			var n, t string
			if err2 := rows.Scan(&n, &t); err2 != nil {
				continue
			}
			name, typ = n, t
		}
		out = append(out, ColumnInfo{
			Name:       name,
			DataType:   typ,
			UDTName:    typ,
			IsNullable: true,
		})
	}
	return out, rows.Err()
}
