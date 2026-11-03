package duckdb

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestRemoteParquetHTTPFS(t *testing.T) {
	dir := t.TempDir()
	parquetPath := filepath.Join(dir, "yellow_tripdata_2024-01.parquet")
	if err := writeSampleParquet(parquetPath); err != nil {
		t.Fatalf("write parquet: %v", err)
	}

	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()
	remoteURL := srv.URL + "/yellow_tripdata_2024-01.parquet"

	cfg := Config{
		Options: map[string]string{
			OptionMode:     ModeFiles,
			OptionAuthType: AuthNone,
			OptionFiles: EncodeFileEntries([]FileEntry{{
				Alias:  "yellow_tripdata_2024_01",
				Source: remoteURL,
				Format: FormatParquet,
			}}),
		},
	}

	res, err := TestConnection(cfg)
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if res == nil || !res.Connected {
		t.Fatal("expected connected")
	}
	if !strings.Contains(res.Message, "Fields Found:") {
		t.Fatalf("message=%q", res.Message)
	}
	if !strings.Contains(res.Message, "Parquet") {
		t.Fatalf("expected Parquet in message: %q", res.Message)
	}

	tables, err := ListTables(cfg, "files")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0].Type != ObjectTypeExternalFile {
		t.Fatalf("tables=%#v", tables)
	}
	if tables[0].Source != remoteURL {
		t.Fatalf("source=%q", tables[0].Source)
	}

	cols, err := ListColumns(cfg, "files", "yellow_tripdata_2024_01")
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) < 2 {
		t.Fatalf("cols=%#v", cols)
	}

	sqlText := "SELECT VendorID, total_amount FROM " + mustRelation(remoteURL) + " WHERE VendorID = 2 LIMIT 5"
	rows, err := FetchTypedRows(cfg, sqlText, nil, 5)
	if err != nil {
		t.Fatalf("FetchTypedRows: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected rows")
	}
	if _, ok := rows[0]["VendorID"]; !ok {
		t.Fatalf("missing VendorID: %#v", rows[0])
	}

	// Aggregate / filter / LIMIT path (no full-file download into Go).
	aggSQL := "SELECT VendorID, COUNT(*) AS trip_count, SUM(total_amount) AS total_amount FROM " +
		mustRelation(remoteURL) + " GROUP BY VendorID ORDER BY VendorID"
	agg, err := FetchTypedRows(cfg, aggSQL, nil, 100)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(agg) == 0 {
		t.Fatal("expected aggregate rows")
	}
}

func TestPublicRemoteParquetOptional(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	cfg := Config{
		Options: map[string]string{
			OptionMode:     ModeFiles,
			OptionAuthType: AuthNone,
			OptionFiles: EncodeFileEntries([]FileEntry{{
				Alias:  "yellow_tripdata_2024_01",
				Source: "https://acugis.cloud/yellow_tripdata_2024-01.parquet",
				Format: FormatParquet,
			}}),
		},
	}
	res, err := TestConnection(cfg)
	if err != nil {
		t.Skipf("public sample unavailable (%v) — httptest remote parquet test covers the path", err)
	}
	if res == nil || !res.Connected {
		t.Skip("public sample not connected")
	}
}

func TestEnsureHTTPFSWithoutHomeEnv(t *testing.T) {
	// Simulate a service process with no HOME/USERPROFILE — the bug that
	// produced: Can't find the home directory at ''.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	// Use a stable temp path (not t.TempDir) so Windows can unlock the extension DLL
	// before the directory is removed.
	dir := filepath.Join(os.TempDir(), "acugis-duckdb-httpfs-test")
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACUGIS_DUCKDB_HOME", dir)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := ensureHTTPFS(ctx, db); err != nil {
		_ = db.Close()
		t.Fatalf("ensureHTTPFS: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDuckDBDatabaseMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "analytics.duckdb")
	dsn := filepath.ToSlash(path)
	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(time.Minute)
	_, err = db.Exec(`CREATE TABLE sales (id INTEGER, sale_date DATE, amount DOUBLE);
INSERT INTO sales VALUES (1, DATE '2024-01-15', 12.5), (2, DATE '2024-01-16', 20.0);`)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}

	cfg := Config{Database: path, Options: map[string]string{OptionMode: ModeDatabase}}
	res, err := TestConnection(cfg)
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if res == nil || !res.Connected {
		t.Fatal("expected connected")
	}
	schemas, err := ListSchemas(cfg)
	if err != nil || len(schemas) == 0 {
		t.Fatalf("schemas=%v err=%v", schemas, err)
	}
	tables, err := ListTables(cfg, "main")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tb := range tables {
		if tb.Name == "sales" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tables=%v", tables)
	}
	rows, err := FetchTypedRows(cfg, `SELECT id, amount FROM sales ORDER BY id`, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
}

func writeSampleParquet(path string) error {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	q := `COPY (
		SELECT * FROM (VALUES
			(1, 10.5),
			(2, 20.0),
			(2, 15.25),
			(1, 8.0)
		) AS t(VendorID, total_amount)
	) TO ` + sqlStringLiteral(filepath.ToSlash(path)) + ` (FORMAT PARQUET)`
	_, err = db.Exec(q)
	return err
}

func mustRelation(url string) string {
	rel, err := ExternalFromRelation(FileEntry{
		Alias:  "yellow_tripdata_2024_01",
		Source: url,
		Format: FormatParquet,
	})
	if err != nil {
		panic(err)
	}
	return rel
}
