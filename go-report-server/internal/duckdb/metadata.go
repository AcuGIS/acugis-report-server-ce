package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

func listSchemasDB(ctx context.Context, db *sql.DB, cfg Config) ([]SchemaInfo, error) {
	if ConnectionMode(cfg) == ModeFiles {
		return []SchemaInfo{{Name: "files"}}, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT schema_name
		FROM information_schema.schemata
		WHERE schema_name NOT IN ('information_schema', 'pg_catalog')
		ORDER BY schema_name`)
	if err != nil {
		return []SchemaInfo{{Name: "main"}}, nil
	}
	defer rows.Close()
	var out []SchemaInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}
		out = append(out, SchemaInfo{Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		out = []SchemaInfo{{Name: "main"}}
	}
	return out, nil
}

func listTablesDB(ctx context.Context, db *sql.DB, cfg Config, schema string) ([]TableInfo, error) {
	if ConnectionMode(cfg) == ModeFiles {
		entries := FileEntries(cfg)
		out := make([]TableInfo, 0, len(entries))
		for _, e := range entries {
			_, resolved, err := ReaderFunctionForEntry(e)
			if err != nil {
				resolved = e.ResolvedFormat()
			}
			out = append(out, TableInfo{
				Schema: "files",
				Name:   e.Alias,
				Type:   ObjectTypeExternalFile,
				Source: e.RemoteSource(),
				Format: resolved,
			})
		}
		return out, nil
	}
	if schema == "" {
		schema = "main"
	}
	rows, err := db.QueryContext(ctx, `
		SELECT table_name, table_type
		FROM information_schema.tables
		WHERE table_schema = ?
		ORDER BY table_name`, schema)
	if err != nil {
		return nil, fmt.Errorf("list DuckDB tables: %w", err)
	}
	defer rows.Close()
	var out []TableInfo
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			return nil, err
		}
		t := "table"
		if strings.Contains(strings.ToUpper(typ), "VIEW") {
			t = "view"
		}
		out = append(out, TableInfo{Schema: schema, Name: name, Type: t})
	}
	log.Printf("[acugis-duckdb] schema=%s tables=%d", schema, len(out))
	return out, rows.Err()
}

func listColumnsDB(ctx context.Context, db *sql.DB, cfg Config, schema, table string) ([]ColumnInfo, error) {
	if ConnectionMode(cfg) == ModeFiles {
		for _, e := range FileEntries(cfg) {
			if strings.EqualFold(e.Alias, table) || e.Alias == table {
				cols, err := inspectFileColumns(ctx, db, e)
				if err != nil {
					break
				}
				log.Printf("[acugis-duckdb] file columns alias=%s count=%d", e.Alias, len(cols))
				return cols, nil
			}
		}
	}
	if schema == "" {
		schema = "main"
	}
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		ORDER BY ordinal_position`, schema, table)
	if err != nil {
		q := fmt.Sprintf("DESCRIBE %s", quoteIdent(table))
		if schema != "" && schema != "main" && schema != "files" {
			q = fmt.Sprintf("DESCRIBE %s.%s", quoteIdent(schema), quoteIdent(table))
		}
		drows, dErr := db.QueryContext(ctx, q)
		if dErr != nil {
			return nil, fmt.Errorf("list DuckDB columns: %w", err)
		}
		defer drows.Close()
		var out []ColumnInfo
		for drows.Next() {
			var name, typ string
			var nullability, key, def, extra sql.NullString
			if scanErr := drows.Scan(&name, &typ, &nullability, &key, &def, &extra); scanErr != nil {
				continue
			}
			out = append(out, ColumnInfo{
				Name: name, DataType: typ, UDTName: typ, IsNullable: true,
			})
		}
		return out, drows.Err()
	}
	defer rows.Close()
	var out []ColumnInfo
	for rows.Next() {
		var name, typ, nullable string
		if err := rows.Scan(&name, &typ, &nullable); err != nil {
			return nil, err
		}
		out = append(out, ColumnInfo{
			Name:       name,
			DataType:   typ,
			UDTName:    typ,
			IsNullable: !strings.EqualFold(nullable, "NO"),
		})
	}
	return out, rows.Err()
}
