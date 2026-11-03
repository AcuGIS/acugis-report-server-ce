package duckdb

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildJDBCSession builds the Tomcat _duckdbSession payload for direct DuckDB JDBC fill.
// Path resolution matches openDB / Test Connection (absolute existing .duckdb file).
// Credentials are included for files-mode auth; never log the returned map.
func BuildJDBCSession(cfg Config) (map[string]any, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	mode := ConnectionMode(cfg)
	session := map[string]any{
		"mode": mode,
	}
	if mode == ModeDatabase {
		path := DatabasePath(cfg)
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
		session["databasePath"] = filepath.ToSlash(abs)
		return session, nil
	}

	entries := FileEntries(cfg)
	files := make([]map[string]string, 0, len(entries))
	for _, e := range entries {
		files = append(files, map[string]string{
			"alias":  e.Alias,
			"source": e.RemoteSource(),
			"format": e.ResolvedFormat(),
		})
	}
	session["files"] = files
	session["authType"] = AuthType(cfg)
	session["username"] = cfg.Username
	session["password"] = cfg.Password
	if AuthType(cfg) == AuthHeaders {
		session["httpHeaderKeys"] = HTTPHeaderKeys(cfg)
	}
	if home, err := duckDBHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		session["homeDirectory"] = filepath.ToSlash(home)
	}
	if ext := strings.TrimSpace(os.Getenv("ACUGIS_DUCKDB_HTTPFS")); ext != "" {
		if st, err := os.Stat(ext); err == nil && !st.IsDir() {
			session["httpfsExtensionPath"] = filepath.ToSlash(ext)
		}
	}
	return session, nil
}
