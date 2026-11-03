package datasources_test

import (
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

func TestNormalizeDBType(t *testing.T) {
	cases := map[string]string{
		"postgres":              "postgresql",
		"pgsql":                 "postgresql",
		"MariaDB":               "mysql",
		"mysql":                 "mysql",
		"oracle":                "oracle",
		"mssql":                 "sqlserver",
		"SQL Server":            "sqlserver",
		"sqlserver":             "sqlserver",
		"db2":                   "db2",
		"IBM Db2":               "db2",
		"snowflake":             "snowflake",
		"sqlite":                "sqlite",
		"GeoPackage":            "sqlite",
		"gpkg":                  "sqlite",
		"cassandra":             "cassandra",
		"Apache Cassandra":      "cassandra",
		"astra":                 "cassandra",
		"DataStax Astra":        "cassandra",
		"bigquery":              "bigquery",
		"Google BigQuery":       "bigquery",
		"bq":                    "bigquery",
		"duckdb":                "duckdb",
		"DuckDB":                "duckdb",
		"duck":                  "duckdb",
		"arcgis":                "arcgis",
		"ArcGIS Feature Server": "arcgis",
		"jasper_arcgis":         "arcgis",
		"geoserver":             "geoserver",
		"GeoServer":             "geoserver",
		"jasper_geoserver":      "geoserver",
		"csv":                   "csv",
		"CSV":                   "csv",
		"tsv":                   "csv",
		"json":                  "json",
		"JSON":                  "json",
		"excel":                 "excel",
		"xlsx":                  "excel",
		"xml":                   "xml",
		"XML":                   "xml",
		"":                      "postgresql",
		"weird":                 "postgresql",
	}
	for in, want := range cases {
		if got := datasources.NormalizeDBType(in); got != want {
			t.Fatalf("%q => %q want %q", in, got, want)
		}
	}
}

func TestNormalizeAuthType(t *testing.T) {
	cases := map[string]string{
		"":         datasources.AuthPassword,
		"password": datasources.AuthPassword,
		"OAuth":    datasources.AuthOAuth,
		"key_pair": datasources.AuthKeyPair,
		"keypair":  datasources.AuthKeyPair,
	}
	for in, want := range cases {
		if got := datasources.NormalizeAuthType(in); got != want {
			t.Fatalf("%q => %q want %q", in, got, want)
		}
	}
}

func TestIsPostgreSQL(t *testing.T) {
	ds := datasources.Datasource{DBType: "mysql"}
	if ds.IsPostgreSQL() {
		t.Fatal("mysql should not be pg")
	}
	ds.DBType = "pgsql"
	if !ds.IsPostgreSQL() {
		t.Fatal("pgsql should be pg")
	}
}

func TestIsMySQL(t *testing.T) {
	ds := datasources.Datasource{DBType: "MariaDB"}
	if !ds.IsMySQL() {
		t.Fatal("mariadb should be mysql")
	}
	ds.DBType = "postgresql"
	if ds.IsMySQL() {
		t.Fatal("postgresql should not be mysql")
	}
}

func TestIsOracle(t *testing.T) {
	ds := datasources.Datasource{DBType: "oracle"}
	if !ds.IsOracle() {
		t.Fatal("oracle should be oracle")
	}
	ds.DBType = "mysql"
	if ds.IsOracle() {
		t.Fatal("mysql should not be oracle")
	}
}

func TestIsSQLServerAndSpatial(t *testing.T) {
	ds := datasources.Datasource{DBType: "mssql"}
	if !ds.IsSQLServer() {
		t.Fatal("mssql should be sqlserver")
	}
	if !ds.IsSpatialCapable() {
		t.Fatal("sqlserver should be spatial-capable")
	}
	for _, typ := range []string{"postgresql", "mysql", "oracle", "sqlserver"} {
		d := datasources.Datasource{DBType: typ}
		if !d.IsSpatialCapable() {
			t.Fatalf("%s should be spatial-capable", typ)
		}
	}
	ds.DBType = "sqlite"
	if ds.IsSpatialCapable() {
		t.Fatal("sqlite should not be spatial-capable for Map Report")
	}
}

func TestCassandraHelpers(t *testing.T) {
	ds := datasources.Datasource{DBType: "cassandra", Options: map[string]string{
		datasources.OptionConnectionMethod: datasources.ConnectionMethodAstra,
	}}
	if !ds.IsCassandra() || !ds.IsAstra() || !ds.IsCollectionFill() {
		t.Fatalf("astra flags: cassandra=%v astra=%v collection=%v", ds.IsCassandra(), ds.IsAstra(), ds.IsCollectionFill())
	}
	if ds.CassandraConnectionMethod() != datasources.ConnectionMethodAstra {
		t.Fatalf("method=%s", ds.CassandraConnectionMethod())
	}
	std := datasources.Datasource{DBType: "cassandra"}
	if std.IsAstra() || std.CassandraConnectionMethod() != datasources.ConnectionMethodStandard {
		t.Fatalf("standard defaults failed")
	}
}

func TestBigQueryHelpers(t *testing.T) {
	ds := datasources.Datasource{
		DBType: "bigquery",
		DBName: "flawless-spirit-386523",
		Options: map[string]string{
			datasources.OptionCredentialsResourceUUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			datasources.OptionDefaultDataset:          "demo",
			datasources.OptionLocation:                "US",
		},
	}
	if !ds.IsBigQuery() || !ds.IsCollectionFill() || ds.IsCassandra() {
		t.Fatalf("bigquery flags failed")
	}
	if ds.CredentialsResourceUUID() == "" || ds.DefaultDataset() != "demo" || ds.BigQueryLocation() != "US" {
		t.Fatalf("options: cred=%q ds=%q loc=%q", ds.CredentialsResourceUUID(), ds.DefaultDataset(), ds.BigQueryLocation())
	}
}

func TestDuckDBHelpers(t *testing.T) {
	db := datasources.Datasource{
		DBType: "duckdb",
		DBName: "/srv/data/analytics.duckdb",
		Options: map[string]string{
			datasources.OptionMode: datasources.DuckDBModeDatabase,
		},
	}
	if !db.IsDuckDB() || db.IsCollectionFill() || db.IsBigQuery() {
		t.Fatalf("duckdb database flags failed")
	}
	if db.DuckDBMode() != datasources.DuckDBModeDatabase {
		t.Fatalf("mode=%s", db.DuckDBMode())
	}
	if sub := db.DuckDBSubtitle(); sub != "DuckDB · analytics.duckdb" {
		t.Fatalf("subtitle=%q", sub)
	}
	files := datasources.Datasource{
		DBType: "duckdb",
		DBName: "taxi",
		Options: map[string]string{
			datasources.OptionMode:     datasources.DuckDBModeFiles,
			datasources.OptionFiles:    `[{"alias":"taxi","source":"https://example.com/taxi.parquet","format":"parquet"}]`,
			datasources.OptionAuthType: "bearer",
		},
	}
	if files.DuckDBMode() != datasources.DuckDBModeFiles || files.DuckDBAuthType() != "bearer" {
		t.Fatalf("files mode/auth failed: %s %s", files.DuckDBMode(), files.DuckDBAuthType())
	}
	if sub := files.DuckDBSubtitle(); sub != "DuckDB · Remote Parquet" {
		t.Fatalf("files subtitle=%q", sub)
	}
	cfg := datasources.DuckDBConfig(files)
	if cfg.Database != "" {
		t.Fatalf("files mode must not set Database path: %q", cfg.Database)
	}
}
