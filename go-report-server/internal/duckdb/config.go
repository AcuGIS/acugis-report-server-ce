// Package duckdb provides DuckDB datasource helpers for Report Server.
// Report fill uses Tomcat DuckDB JDBC + REPORT_CONNECTION (BuildJDBCSession).
// Test Connection / metadata may still use the Go duckdb-go engine.
package duckdb

import (
	"fmt"
	"strings"
)

const (
	ProviderID = "duckdb"

	OptionMode           = "mode"
	OptionFiles          = "files"
	OptionFileAliases    = "fileAliases"
	OptionAuthType       = "authType"
	OptionHTTPHeaderKeys = "httpHeaderKeys"
	OptionSourceType     = "sourceType"

	ModeDatabase = "database"
	ModeFiles    = "files"

	ObjectTypeExternalFile = "EXTERNAL_FILE"

	FormatAuto    = "auto"
	FormatParquet = "parquet"
	FormatCSV     = "csv"
	FormatJSON    = "json"

	AuthNone    = "none"
	AuthBasic   = "basic"
	AuthBearer  = "bearer"
	AuthHeaders = "headers"

	SourceTypeHTTP = "http"

	duckHTTPSecretName = "acugis_http"
)

// Config holds DuckDB connection settings for Server.
// Password holds runtime secrets (basic password, bearer token, or headers JSON)
// resolved from the datasource catalog — never returned to the browser or JRXML.
type Config struct {
	Database string // server filesystem .duckdb path in database mode
	Username string // Basic Auth username (non-secret)
	Password string // secret payload
	Options  map[string]string
}

// TestResult is returned by TestConnection.
type TestResult struct {
	Connected     bool
	Message       string
	Database      string
	Driver        string
	ServerVersion string
	SchemaCount   int
	TableCount    int
	CurrentSchema string
}

// SchemaInfo is a catalog/schema name.
type SchemaInfo struct {
	Name string
}

// TableInfo is a table, view, or EXTERNAL_FILE metadata object.
type TableInfo struct {
	Schema string
	Name   string
	Type   string // table | view | EXTERNAL_FILE
	Source string // remote URL for EXTERNAL_FILE
	Format string // parquet | csv | json
}

// ColumnInfo is a column descriptor.
type ColumnInfo struct {
	Name       string
	DataType   string
	UDTName    string
	IsNullable bool
}

// FileEntry is a remote analytical file registered on a DuckDB Data Files connection.
type FileEntry struct {
	Alias  string `json:"alias"`
	Source string `json:"source,omitempty"`
	Path   string `json:"path,omitempty"`
	Format string `json:"format,omitempty"`
}

// RemoteSource returns the remote URL for this entry (Source preferred over Path).
func (e FileEntry) RemoteSource() string {
	if s := strings.TrimSpace(e.Source); s != "" {
		return s
	}
	return strings.TrimSpace(e.Path)
}

// ResolvedFormat returns a normalized format (auto when empty).
func (e FileEntry) ResolvedFormat() string {
	return NormalizeFormat(e.Format)
}

// ConnectionMode returns database | files.
func ConnectionMode(cfg Config) string {
	if cfg.Options != nil {
		m := strings.ToLower(strings.TrimSpace(cfg.Options[OptionMode]))
		if m == ModeFiles || m == "file" || m == "datafiles" || m == "data_files" {
			return ModeFiles
		}
	}
	if len(FileEntries(cfg)) > 0 && strings.TrimSpace(cfg.Database) == "" {
		return ModeFiles
	}
	return ModeDatabase
}

// DatabasePath returns the .duckdb file path for database mode.
func DatabasePath(cfg Config) string {
	return strings.TrimSpace(cfg.Database)
}

// Option returns a trimmed options value.
func (c Config) Option(key string) string {
	if c.Options == nil {
		return ""
	}
	return strings.TrimSpace(c.Options[key])
}

// PreviewResult is a string-cell query preview (used by discovery helpers).
type PreviewResult struct {
	Columns   []string
	Rows      [][]string
	Truncated bool
}

func validateConfig(cfg Config) error {
	mode := ConnectionMode(cfg)
	if mode == ModeFiles {
		entries := FileEntries(cfg)
		if len(entries) == 0 {
			return fmt.Errorf("remote source URL is required (Parquet, CSV, or JSON over HTTP/HTTPS)")
		}
		for _, e := range entries {
			if err := ValidateRemoteSource(e.RemoteSource()); err != nil {
				return err
			}
		}
		return nil
	}
	if strings.TrimSpace(cfg.Database) == "" {
		return fmt.Errorf("DuckDB database file is required")
	}
	return nil
}
