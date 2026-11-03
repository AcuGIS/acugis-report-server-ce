package duckdb

import (
	"strings"
	"testing"
)

func TestMapJavaClass(t *testing.T) {
	cases := map[string]string{
		"VARCHAR":                  "java.lang.String",
		"INTEGER":                  "java.lang.Integer",
		"BIGINT":                   "java.lang.Long",
		"DOUBLE":                   "java.lang.Double",
		"DECIMAL(18,2)":            "java.math.BigDecimal",
		"BOOLEAN":                  "java.lang.Boolean",
		"DATE":                     "java.time.LocalDate",
		"TIMESTAMP":                "java.sql.Timestamp",
		"TIMESTAMP WITH TIME ZONE": "java.sql.Timestamp",
		"UUID":                     "java.util.UUID",
		"BLOB":                     "[B",
		"JSON":                     "java.lang.String",
		"LIST":                     "java.lang.String",
		"STRUCT":                   "java.lang.String",
	}
	for in, want := range cases {
		if got := MapJavaClass(in); got != want {
			t.Errorf("MapJavaClass(%q)=%q want %q", in, got, want)
		}
	}
}

func TestBindParameters(t *testing.T) {
	q, err := bindParameters("SELECT * FROM customers WHERE state = $P{p_state}", map[string]any{
		"p_state": "TX",
	})
	if err != nil {
		t.Fatal(err)
	}
	if q.SQL != "SELECT * FROM customers WHERE state = ?" {
		t.Fatalf("SQL=%q", q.SQL)
	}
	if len(q.Values) != 1 || q.Values[0] != "TX" {
		t.Fatalf("values=%v", q.Values)
	}
}

func TestSqlStringLiteral(t *testing.T) {
	got := sqlStringLiteral(`https://example.com/O'Brien.parquet`)
	want := `'https://example.com/O''Brien.parquet'`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFileEntriesAndMode(t *testing.T) {
	cfg := Config{
		Options: map[string]string{
			OptionMode: ModeFiles,
			OptionFiles: EncodeFileEntries([]FileEntry{{
				Alias:  "customers",
				Source: "https://example.com/data/customers.parquet",
				Format: FormatAuto,
			}}),
		},
	}
	entries := FileEntries(cfg)
	if len(entries) != 1 || entries[0].Alias != "customers" {
		t.Fatalf("entries=%v", entries)
	}
	if ConnectionMode(cfg) != ModeFiles {
		t.Fatal("expected files mode")
	}
	rel, err := ExternalFromRelation(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rel, "read_parquet(") || !strings.Contains(rel, "AS \"customers\"") {
		t.Fatalf("rel=%q", rel)
	}
}

func TestDetectFormatFromSource(t *testing.T) {
	f, err := DetectFormatFromSource("https://acugis.cloud/yellow_tripdata_2024-01.parquet")
	if err != nil || f != FormatParquet {
		t.Fatalf("got %q err=%v", f, err)
	}
	f, err = DetectFormatFromSource("https://example.com/data.csv.gz")
	if err != nil || f != FormatCSV {
		t.Fatalf("got %q err=%v", f, err)
	}
}

func TestListSubtitle(t *testing.T) {
	dbCfg := Config{Database: "/srv/data/analytics.duckdb", Options: map[string]string{OptionMode: ModeDatabase}}
	if got := ListSubtitle(dbCfg); got != "DuckDB · analytics.duckdb" {
		t.Fatalf("db subtitle=%q", got)
	}
	fileCfg := Config{Options: map[string]string{
		OptionMode: ModeFiles,
		OptionFiles: EncodeFileEntries([]FileEntry{{
			Alias: "taxi", Source: "https://example.com/taxi.parquet", Format: FormatParquet,
		}}),
	}}
	if got := ListSubtitle(fileCfg); got != "DuckDB · Remote Parquet" {
		t.Fatalf("files subtitle=%q", got)
	}
}
