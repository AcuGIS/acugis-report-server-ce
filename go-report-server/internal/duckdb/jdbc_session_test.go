package duckdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/duckdb"
)

func TestBuildJDBCSessionDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "analytics.duckdb")
	if err := os.WriteFile(dbPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := duckdb.Config{
		Database: dbPath,
		Options:  map[string]string{duckdb.OptionMode: duckdb.ModeDatabase},
	}
	session, err := duckdb.BuildJDBCSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if session["mode"] != duckdb.ModeDatabase {
		t.Fatalf("mode=%v", session["mode"])
	}
	got, _ := session["databasePath"].(string)
	if got == "" || filepath.Base(got) != "analytics.duckdb" {
		t.Fatalf("databasePath=%q", got)
	}
	if _, ok := session["files"]; ok {
		t.Fatal("database mode must not include files")
	}
}

func TestBuildJDBCSessionFiles(t *testing.T) {
	cfg := duckdb.Config{
		Options: map[string]string{
			duckdb.OptionMode:     duckdb.ModeFiles,
			duckdb.OptionFiles:    duckdb.EncodeFileEntries([]duckdb.FileEntry{{Alias: "trips", Source: "https://example.com/t.parquet", Format: duckdb.FormatParquet}}),
			duckdb.OptionAuthType: duckdb.AuthNone,
		},
	}
	session, err := duckdb.BuildJDBCSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if session["mode"] != duckdb.ModeFiles {
		t.Fatalf("mode=%v", session["mode"])
	}
	files, ok := session["files"].([]map[string]string)
	if !ok || len(files) != 1 || files[0]["alias"] != "trips" {
		t.Fatalf("files=%v", session["files"])
	}
}

func TestBuildJDBCSessionMissingDatabase(t *testing.T) {
	cfg := duckdb.Config{
		Database: filepath.Join(t.TempDir(), "missing.duckdb"),
		Options:  map[string]string{duckdb.OptionMode: duckdb.ModeDatabase},
	}
	if _, err := duckdb.BuildJDBCSession(cfg); err == nil {
		t.Fatal("expected missing database error")
	}
}
