package duckdb_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/duckdb"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestRemoteParquetHTTPEndToEnd(t *testing.T) {
	dir := t.TempDir()
	parquetPath := filepath.Join(dir, "customers.parquet")
	if err := writeCustomersParquet(t, parquetPath); err != nil {
		t.Fatalf("write parquet: %v", err)
	}

	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()
	remoteURL := srv.URL + "/customers.parquet"

	cfg := duckdb.Config{
		Options: map[string]string{
			duckdb.OptionMode: duckdb.ModeFiles,
			duckdb.OptionFiles: duckdb.EncodeFileEntries([]duckdb.FileEntry{
				{Alias: "customers", Source: remoteURL, Format: duckdb.FormatAuto},
			}),
			duckdb.OptionAuthType: duckdb.AuthNone,
		},
	}

	res, err := duckdb.TestConnection(cfg)
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if !res.Connected || !strings.Contains(res.Message, "Fields Found:") {
		t.Fatalf("message=%q", res.Message)
	}

	tables, err := duckdb.ListTables(cfg, "files")
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	if len(tables) != 1 || tables[0].Type != duckdb.ObjectTypeExternalFile {
		t.Fatalf("tables=%#v", tables)
	}

	rows, err := duckdb.FetchTypedRows(cfg, `
SELECT customer_id, state, sales
FROM read_parquet('`+strings.ReplaceAll(remoteURL, `'`, `''`)+`') AS "customers"
WHERE state = 'CA'
LIMIT 10
`, nil, 10)
	if err != nil {
		t.Fatalf("FetchTypedRows: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected filtered rows")
	}
	if _, ok := rows[0]["customer_id"]; !ok {
		t.Fatalf("row=%#v", rows[0])
	}
}

func TestDuckDBDatabaseModeEndToEnd(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "analytics.duckdb")
	if err := seedDatabase(t, dbPath); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg := duckdb.Config{
		Database: dbPath,
		Options:  map[string]string{duckdb.OptionMode: duckdb.ModeDatabase},
	}
	res, err := duckdb.TestConnection(cfg)
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if !res.Connected {
		t.Fatal("not connected")
	}
	tables, err := duckdb.ListTables(cfg, "main")
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	found := false
	for _, tb := range tables {
		if tb.Name == "sales" && tb.Type != duckdb.ObjectTypeExternalFile {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected physical sales table, got %#v", tables)
	}
	rows, err := duckdb.FetchTypedRows(cfg, `SELECT * FROM "main"."sales" LIMIT 5`, nil, 5)
	if err != nil {
		t.Fatalf("FetchTypedRows: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected rows")
	}
}

func writeCustomersParquet(t *testing.T, path string) error {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, `
CREATE TABLE customers AS
SELECT * FROM (VALUES
  (1, 'Ada Lovelace', 'CA', 1200.50),
  (2, 'Alan Turing', 'TX', 980.00),
  (3, 'Grace Hopper', 'CA', 1500.75)
) AS t(customer_id, customer_name, state, sales)`)
	if err != nil {
		return err
	}
	lit := "'" + filepath.ToSlash(path) + "'"
	_, err = db.ExecContext(ctx, "COPY customers TO "+lit+" (FORMAT PARQUET)")
	return err
}

func seedDatabase(t *testing.T, path string) error {
	t.Helper()
	db, err := sql.Open("duckdb", filepath.ToSlash(path))
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, `
CREATE TABLE sales AS
SELECT * FROM (VALUES
  (1, 'CA', 100.0),
  (2, 'TX', 200.0)
) AS t(id, state, amount)`)
	return err
}
