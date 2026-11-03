package datasources

import (
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/duckdb"
)

// DuckDBConfig maps a catalog Datasource onto the DuckDB client Config.
// Password must already be resolved (blank form field → reload from catalog).
func DuckDBConfig(ds Datasource) duckdb.Config {
	opts := map[string]string{}
	for k, v := range ds.Options {
		opts[k] = v
	}
	mode := strings.ToLower(strings.TrimSpace(opts[duckdb.OptionMode]))
	if mode == "file" || mode == "datafiles" || mode == "data_files" {
		mode = duckdb.ModeFiles
	}
	if mode != duckdb.ModeFiles {
		// Infer files when options.files is present and dbname is not a .duckdb path.
		if strings.TrimSpace(opts[duckdb.OptionFiles]) != "" && opts[duckdb.OptionFiles] != "[]" &&
			!looksLikeDuckDBPath(ds.DBName) {
			mode = duckdb.ModeFiles
		} else {
			mode = duckdb.ModeDatabase
		}
	}
	opts[duckdb.OptionMode] = mode

	database := ""
	if mode == duckdb.ModeDatabase {
		database = strings.TrimSpace(ds.DBName)
	}

	return duckdb.Config{
		Database: database,
		Username: ds.Username,
		Password: ds.Password,
		Options:  opts,
	}
}

func looksLikeDuckDBPath(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if duckdb.IsRemoteHTTPSource(v) {
		return false
	}
	lower := strings.ToLower(v)
	return strings.HasSuffix(lower, ".duckdb") || strings.HasSuffix(lower, ".db") ||
		strings.ContainsAny(v, `/\`) || filepath.IsAbs(v)
}

// DuckDBMode returns database | files for a DuckDB datasource.
func (d Datasource) DuckDBMode() string {
	if !d.IsDuckDB() {
		return ""
	}
	return duckdb.ConnectionMode(DuckDBConfig(d))
}

// DuckDBSubtitle returns a non-sensitive list subtitle for DuckDB datasources.
func (d Datasource) DuckDBSubtitle() string {
	if !d.IsDuckDB() {
		return ""
	}
	cfg := DuckDBConfig(d)
	if duckdb.ConnectionMode(cfg) == duckdb.ModeFiles {
		entries := duckdb.FileEntries(cfg)
		if len(entries) == 0 {
			return "DuckDB · Remote"
		}
		_, format, err := duckdb.ReaderFunctionForEntry(entries[0])
		if err != nil {
			format = entries[0].ResolvedFormat()
		}
		return "DuckDB · Remote " + duckdb.FormatDisplayName(format)
	}
	base := filepath.Base(strings.TrimSpace(d.DBName))
	if base == "" || base == "." {
		base = "database"
	}
	return "DuckDB · " + base
}
