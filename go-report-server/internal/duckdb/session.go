package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func openDB(cfg Config) (*sql.DB, error) {
	mode := ConnectionMode(cfg)
	dsn := ""
	if mode == ModeDatabase {
		path := DatabasePath(cfg)
		if path == "" {
			return nil, fmt.Errorf("DuckDB database file is required")
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("DuckDB database not found")
		}
		if _, err := os.Stat(abs); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("DuckDB database not found: %s", abs)
			}
			return nil, fmt.Errorf("cannot open DuckDB database: %w", err)
		}
		dsn = filepath.ToSlash(abs)
	} else {
		// In-memory database for remote Data Files mode — DuckDB reads remote objects directly.
		dsn = ""
	}

	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to open DuckDB: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(time.Minute)

	ctxTimeout := 20 * time.Second
	if mode == ModeFiles {
		ctxTimeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("unable to open DuckDB: %w", err)
	}

	if mode == ModeFiles {
		if err := ensureHTTPFS(ctx, db); err != nil {
			_ = db.Close()
			return nil, err
		}
		if err := configureHTTPAuth(ctx, db, cfg); err != nil {
			_ = db.Close()
			return nil, SanitizeRemoteError(err)
		}
		if err := registerFileViews(ctx, db, cfg); err != nil {
			_ = db.Close()
			return nil, SanitizeRemoteError(err)
		}
	}
	return db, nil
}

// ensureHTTPFS installs/loads the httpfs extension so DuckDB can read HTTP(S) sources.
// Server processes (Windows services, containers) often have no HOME — DuckDB INSTALL
// requires home_directory, so we set one before INSTALL/LOAD.
func ensureHTTPFS(ctx context.Context, db *sql.DB) error {
	if err := ensureDuckDBHome(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "LOAD httpfs"); err == nil {
		return nil
	}
	if _, err := db.ExecContext(ctx, "INSTALL httpfs"); err != nil {
		if _, loadErr := db.ExecContext(ctx, "LOAD httpfs"); loadErr == nil {
			return nil
		}
		return fmt.Errorf("failed to enable DuckDB HTTP/HTTPS support (httpfs): %w", err)
	}
	if _, err := db.ExecContext(ctx, "LOAD httpfs"); err != nil {
		return fmt.Errorf("failed to load DuckDB httpfs extension: %w", err)
	}
	return nil
}

// ensureDuckDBHome sets home_directory so INSTALL/LOAD can write extension cache.
func ensureDuckDBHome(ctx context.Context, db *sql.DB) error {
	home, err := duckDBHomeDir()
	if err != nil {
		return fmt.Errorf("failed to resolve DuckDB home directory: %w", err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return fmt.Errorf("failed to create DuckDB home directory: %w", err)
	}
	_, err = db.ExecContext(ctx, "SET home_directory="+sqlStringLiteral(filepath.ToSlash(home)))
	if err != nil {
		return fmt.Errorf("failed to set DuckDB home_directory: %w", err)
	}
	return nil
}

// duckDBHomeDir picks a writable directory for DuckDB extension installs.
// Order: ACUGIS_DUCKDB_HOME / DUCKDB_HOME → user home → user cache → temp.
func duckDBHomeDir() (string, error) {
	for _, key := range []string{"ACUGIS_DUCKDB_HOME", "DUCKDB_HOME"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return filepath.Abs(v)
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".acugis", "duckdb"), nil
	}
	if cache, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cache) != "" {
		return filepath.Join(cache, "acugis", "duckdb"), nil
	}
	return filepath.Join(os.TempDir(), "acugis-duckdb"), nil
}

func registerFileViews(ctx context.Context, db *sql.DB, cfg Config) error {
	entries := FileEntries(cfg)
	if len(entries) == 0 {
		return fmt.Errorf("remote source URL is required (Parquet, CSV, or JSON over HTTP/HTTPS)")
	}
	for _, e := range entries {
		if err := ValidateRemoteSource(e.RemoteSource()); err != nil {
			return err
		}
		viewSQL, err := createViewSQL(e)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, viewSQL); err != nil {
			return fmt.Errorf(
				"failed to register remote file %s as %s: %w",
				RedactRemoteURL(e.RemoteSource()), e.Alias, err,
			)
		}
	}
	return nil
}

func createViewSQL(e FileEntry) (string, error) {
	src := e.RemoteSource()
	if err := ValidateRemoteSource(src); err != nil {
		return "", err
	}
	reader, _, err := ReaderFunctionForEntry(e)
	if err != nil {
		return "", err
	}
	alias := quoteIdent(e.Alias)
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT * FROM %s(%s)",
		alias, reader, sqlStringLiteral(src),
	), nil
}

func quoteIdent(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
