package duckdb

import (
	"fmt"
	"strings"
)

// DetectFormatFromSource determines parquet|csv|json from a URL/path extension.
func DetectFormatFromSource(source string) (string, error) {
	lower := strings.ToLower(strings.TrimSpace(source))
	if i := strings.IndexAny(lower, "?#"); i >= 0 {
		lower = lower[:i]
	}
	switch {
	case strings.HasSuffix(lower, ".parquet"), strings.HasSuffix(lower, ".parq"):
		return FormatParquet, nil
	case strings.HasSuffix(lower, ".csv"), strings.HasSuffix(lower, ".csv.gz"),
		strings.HasSuffix(lower, ".tsv"), strings.HasSuffix(lower, ".tsv.gz"),
		strings.HasSuffix(lower, ".txt"), strings.HasSuffix(lower, ".txt.gz"):
		return FormatCSV, nil
	case strings.HasSuffix(lower, ".json"), strings.HasSuffix(lower, ".json.gz"),
		strings.HasSuffix(lower, ".jsonl"), strings.HasSuffix(lower, ".jsonl.gz"),
		strings.HasSuffix(lower, ".ndjson"), strings.HasSuffix(lower, ".ndjson.gz"):
		return FormatJSON, nil
	default:
		return "", fmt.Errorf(
			"cannot auto-detect format from URL %q — select Parquet, CSV, or JSON explicitly",
			RedactRemoteURL(source),
		)
	}
}

// ReaderFunctionForEntry returns the DuckDB table function name and resolved format.
func ReaderFunctionForEntry(e FileEntry) (reader string, resolvedFormat string, err error) {
	src := e.RemoteSource()
	if src == "" {
		return "", "", fmt.Errorf("remote source URL is required")
	}
	format := e.ResolvedFormat()
	if format == FormatAuto {
		detected, derr := DetectFormatFromSource(src)
		if derr != nil {
			return "", "", derr
		}
		format = detected
	}
	switch format {
	case FormatParquet:
		return "read_parquet", FormatParquet, nil
	case FormatCSV:
		return "read_csv_auto", FormatCSV, nil
	case FormatJSON:
		return "read_json_auto", FormatJSON, nil
	default:
		return "", "", fmt.Errorf(
			"unsupported format %q (use auto, parquet, csv, or json)",
			e.Format,
		)
	}
}

// FormatDisplayName returns a user-facing format label.
func FormatDisplayName(format string) string {
	switch NormalizeFormat(format) {
	case FormatParquet:
		return "Parquet"
	case FormatCSV:
		return "CSV"
	case FormatJSON:
		return "JSON"
	case FormatAuto:
		return "Auto Detect"
	default:
		return format
	}
}

// ExternalFromRelation builds a DuckDB FROM relation for an external file.
func ExternalFromRelation(e FileEntry) (string, error) {
	src := e.RemoteSource()
	if err := ValidateRemoteSource(src); err != nil {
		return "", err
	}
	reader, _, err := ReaderFunctionForEntry(e)
	if err != nil {
		return "", err
	}
	alias := strings.TrimSpace(e.Alias)
	if alias == "" {
		alias = defaultAlias(src)
	}
	return fmt.Sprintf("%s(%s) AS %s", reader, sqlStringLiteral(src), quoteIdent(alias)), nil
}

// TableFunctionSQL builds read_*(url) AS "alias" from query-table style fields.
func TableFunctionSQL(source, format, alias string) (string, error) {
	return ExternalFromRelation(FileEntry{
		Alias:  alias,
		Source: source,
		Format: format,
	})
}
