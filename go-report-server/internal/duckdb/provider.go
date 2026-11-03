package duckdb

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"
)

// TestConnection opens DuckDB and verifies database readability or remote source schema.
func TestConnection(cfg Config) (*TestResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := openDB(cfg)
	if err != nil {
		return nil, SanitizeRemoteError(err)
	}
	defer db.Close()

	result := &TestResult{
		Connected: true,
		Driver:    "duckdb/duckdb-go",
		Message:   "Connected",
	}

	var version string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&version); err == nil {
		result.ServerVersion = version
	}

	mode := ConnectionMode(cfg)
	if mode == ModeFiles {
		entries := FileEntries(cfg)
		if len(entries) == 0 {
			return nil, fmt.Errorf("remote source URL is required")
		}
		e := entries[0]
		cols, err := inspectFileColumns(ctx, db, e)
		if err != nil {
			return nil, SanitizeRemoteError(fmt.Errorf("remote source is not readable: %w", err))
		}
		_, resolved, rerr := ReaderFunctionForEntry(e)
		if rerr != nil {
			resolved = e.ResolvedFormat()
		}
		displaySrc := RedactRemoteURL(e.RemoteSource())
		result.Database = displaySrc
		result.SchemaCount = 1
		result.TableCount = 1
		result.CurrentSchema = "files"
		result.Message = fmt.Sprintf(
			"Connected\nSource: %s\nFormat: %s\nFields Found: %d",
			displaySrc,
			FormatDisplayName(resolved),
			len(cols),
		)
		if result.ServerVersion != "" {
			result.Message += "\nDuckDB " + result.ServerVersion
		}
	} else {
		path := DatabasePath(cfg)
		result.Database = path
		schemas, err := listSchemasDB(ctx, db, cfg)
		if err == nil {
			result.SchemaCount = len(schemas)
			tables := 0
			for _, s := range schemas {
				t, tErr := listTablesDB(ctx, db, cfg, s.Name)
				if tErr == nil {
					tables += len(t)
				}
			}
			result.TableCount = tables
		}
		result.Message = fmt.Sprintf("Connected · %s", filepath.Base(path))
		if result.ServerVersion != "" {
			result.Message += " · DuckDB " + result.ServerVersion
		}
		if result.SchemaCount > 0 {
			result.Message += fmt.Sprintf("\nSchemas Found: %d\nTables/Views Found: %d", result.SchemaCount, result.TableCount)
		}
	}
	log.Printf("[acugis-duckdb] test ok mode=%s auth=%s schemas=%d tables=%d", mode, AuthType(cfg), result.SchemaCount, result.TableCount)
	return result, nil
}

// ListSchemas returns schemas (or synthetic "files" for remote mode).
func ListSchemas(cfg Config) ([]SchemaInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := openDB(cfg)
	if err != nil {
		return nil, SanitizeRemoteError(err)
	}
	defer db.Close()
	return listSchemasDB(ctx, db, cfg)
}

// ListTables returns tables/views or EXTERNAL_FILE logical objects.
func ListTables(cfg Config, schema string) ([]TableInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := openDB(cfg)
	if err != nil {
		return nil, SanitizeRemoteError(err)
	}
	defer db.Close()
	return listTablesDB(ctx, db, cfg, schema)
}

// ListColumns returns column metadata for a table or remote file alias.
func ListColumns(cfg Config, schema, table string) ([]ColumnInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := openDB(cfg)
	if err != nil {
		return nil, SanitizeRemoteError(err)
	}
	defer db.Close()
	cols, err := listColumnsDB(ctx, db, cfg, schema, table)
	if err != nil {
		return nil, SanitizeRemoteError(err)
	}
	return cols, nil
}

// FetchTypedRows executes DuckDB SQL for Test Connection / metadata helpers.
// Report fill no longer uses this path — Tomcat opens DuckDB JDBC via BuildJDBCSession.
func FetchTypedRows(cfg Config, sqlText string, params map[string]any, limit int) ([]map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	db, err := openDB(cfg)
	if err != nil {
		return nil, SanitizeRemoteError(err)
	}
	defer db.Close()
	if limit <= 0 {
		limit = 5000
	}
	rows, err := executeTypedRows(ctx, db, sqlText, params, limit)
	if err != nil {
		return nil, SanitizeRemoteError(err)
	}
	return rows, nil
}
