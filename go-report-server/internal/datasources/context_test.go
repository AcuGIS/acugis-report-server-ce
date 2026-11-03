package datasources_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

func TestJDBCSettingsAndContextXML(t *testing.T) {
	pg := datasources.Datasource{
		Name: "MainPG", DBType: "postgresql", Host: "db", Port: 5432,
		DBName: "reports", Username: "u", Password: "p&x",
	}
	jdbc := datasources.JDBCSettingsFor(pg)
	if jdbc == nil || jdbc.DriverClassName != "org.postgresql.Driver" {
		t.Fatalf("jdbc=%+v", jdbc)
	}
	if !strings.Contains(jdbc.URL, "jdbc:postgresql://db:5432/reports") {
		t.Fatalf("url=%s", jdbc.URL)
	}

	ora := datasources.Datasource{
		Name: "ORA", DBType: "oracle", Host: "ora", Port: 1521,
		DBName: "sid:ORCL", Username: "system", Password: "secret",
	}
	oj := datasources.JDBCSettingsFor(ora)
	if oj == nil || !strings.Contains(oj.URL, "@ora:1521:ORCL") {
		t.Fatalf("oracle=%+v", oj)
	}

	xml := datasources.BuildContextXML([]datasources.Datasource{pg, ora})
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<Context>`,
		`name="jdbc/MainPG"`,
		`password="p&amp;x"`,
		`name="jdbc/ORA"`,
		`initialSize="0"`,
		`</Context>`,
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("xml missing %q:\n%s", want, xml)
		}
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "context.xml")
	if err := datasources.WriteContextXML(path, []datasources.Datasource{pg}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `name="jdbc/MainPG"`) {
		t.Fatalf("file=%s", data)
	}
}

func TestMetaForDBType(t *testing.T) {
	if datasources.MetaForDBType("mysql").Port != 3306 {
		t.Fatal("mysql port")
	}
	if datasources.MetaForDBType("bogus").Key != "postgresql" {
		t.Fatal("default")
	}
	metas := datasources.DBTypeMetas()
	if len(metas) != 15 {
		t.Fatalf("expected 15 db types, got %d", len(metas))
	}
	wantLabels := []string{
		"PostgreSQL", "MySQL", "Oracle", "Microsoft SQL Server",
		"Snowflake", "SQLite / GeoPackage", "Apache Cassandra", "Google BigQuery", "DuckDB",
		"ArcGIS Feature Server", "GeoServer",
		"CSV", "JSON", "Excel", "XML",
	}
	for i, want := range wantLabels {
		if metas[i].Label != want {
			t.Fatalf("meta[%d]=%q want %q", i, metas[i].Label, want)
		}
	}
	if datasources.MetaForDBType("cassandra").Port != 9042 {
		t.Fatal("cassandra port")
	}
	if datasources.MetaForDBType("bigquery").DBNameLabel != "Project ID" {
		t.Fatal("bigquery label")
	}
	for _, key := range []string{"csv", "json", "excel", "xml"} {
		if datasources.MetaForDBType(key).Key != key {
			t.Fatalf("%s missing from catalog", key)
		}
		if datasources.JDBCSettingsFor(datasources.Datasource{DBType: key, Host: "db", Port: 5432, DBName: "datasources/abc/file.csv"}) != nil {
			t.Fatalf("%s must not emit JDBC settings", key)
		}
	}
}

func TestJDBCSettingsForNewEngines(t *testing.T) {
	mssql := datasources.JDBCSettingsFor(datasources.Datasource{
		DBType: "sqlserver", Host: "sql", Port: 1433, DBName: "app",
	})
	if mssql == nil || mssql.DriverClassName != "com.microsoft.sqlserver.jdbc.SQLServerDriver" {
		t.Fatalf("mssql=%+v", mssql)
	}
	if !strings.Contains(mssql.URL, "jdbc:sqlserver://sql:1433;databaseName=app;") {
		t.Fatalf("mssql url=%s", mssql.URL)
	}

	db2 := datasources.JDBCSettingsFor(datasources.Datasource{
		DBType: "db2", Host: "db2host", Port: 50000, DBName: "SAMPLE",
	})
	if db2 != nil {
		t.Fatalf("db2 should be skipped for Tomcat JNDI, got %+v", db2)
	}
	xml := datasources.BuildContextXML([]datasources.Datasource{{
		Name: "mydb2", DBType: "db2", Host: "db2host", Port: 50000, DBName: "SAMPLE",
		Username: "u", Password: "p",
	}})
	if strings.Contains(xml, "jdbc/mydb2") || strings.Contains(xml, "db2") {
		t.Fatalf("db2 must not appear in context.xml:\n%s", xml)
	}

	sf := datasources.JDBCSettingsFor(datasources.Datasource{
		DBType: "snowflake", Host: "xy12345", Port: 443, DBName: "ANALYTICS", SchemaName: "PUBLIC",
	})
	if sf == nil || sf.DriverClassName != "net.snowflake.client.jdbc.SnowflakeDriver" {
		t.Fatalf("snowflake=%+v", sf)
	}
	if !strings.Contains(sf.URL, "jdbc:snowflake://xy12345.snowflakecomputing.com/") {
		t.Fatalf("snowflake url=%s", sf.URL)
	}
	if !strings.Contains(sf.URL, "JDBC_QUERY_RESULT_FORMAT=JSON") {
		t.Fatalf("snowflake url missing JSON result format: %s", sf.URL)
	}
	if strings.Contains(sf.URL, "private_key_base64") || strings.Contains(sf.URL, "authenticator") {
		t.Fatalf("password auth should not include key pair params: %s", sf.URL)
	}

	sfKey := datasources.JDBCSettingsFor(datasources.Datasource{
		DBType: "snowflake", Host: "xy12345", Port: 443, DBName: "ANALYTICS", SchemaName: "PUBLIC",
		AuthType: datasources.AuthKeyPair, Username: "svc", Password: "keypass",
		PrivateKeyPEM: []byte("-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"),
	})
	if sfKey == nil || !sfKey.OmitPassword {
		t.Fatalf("snowflake key pair=%+v", sfKey)
	}
	for _, want := range []string{"authenticator=snowflake_jwt", "private_key_base64=", "private_key_pwd=keypass", "db=ANALYTICS"} {
		if !strings.Contains(sfKey.URL, want) {
			t.Fatalf("key pair url missing %q: %s", want, sfKey.URL)
		}
	}
	xmlKey := datasources.BuildContextXML([]datasources.Datasource{{
		Name: "SFKey", DBType: "snowflake", Host: "xy12345", Port: 443, DBName: "ANALYTICS",
		Username: "svc", AuthType: datasources.AuthKeyPair,
		PrivateKeyPEM: []byte("pem-bytes"),
	}})
	if !strings.Contains(xmlKey, `name="jdbc/SFKey"`) || strings.Contains(xmlKey, `password="`) {
		t.Fatalf("key pair context should omit password attribute:\n%s", xmlKey)
	}
	if !strings.Contains(xmlKey, "private_key_base64") {
		t.Fatalf("key pair context missing base64 key:\n%s", xmlKey)
	}
	// key_pair without resolved PEM must be omitted from context (avoid silent password fallback).
	xmlSkip := datasources.BuildContextXML([]datasources.Datasource{{
		Name: "SFMissing", DBType: "snowflake", Host: "xy12345", Port: 443, DBName: "ANALYTICS",
		Username: "svc", AuthType: datasources.AuthKeyPair, PrivateKeyResourceUUID: "11111111-1111-1111-1111-111111111111",
	}})
	if strings.Contains(xmlSkip, "jdbc/SFMissing") {
		t.Fatalf("unresolved key pair must not appear in context.xml:\n%s", xmlSkip)
	}

	sqlitePath := `C:\data\maps.gpkg`
	sqlite := datasources.JDBCSettingsFor(datasources.Datasource{
		DBType: "sqlite", DBName: sqlitePath,
	})
	if sqlite == nil || sqlite.DriverClassName != "org.sqlite.JDBC" {
		t.Fatalf("sqlite=%+v", sqlite)
	}
	// Production uses filepath.ToSlash only. On Windows that yields C:/...;
	// on Linux backslashes are left unchanged. Do not hard-code a Windows URL.
	wantSQLite := "jdbc:sqlite:" + filepath.ToSlash(sqlitePath)
	if sqlite.URL != wantSQLite {
		t.Fatalf("sqlite url=%s want %s", sqlite.URL, wantSQLite)
	}

	cass := datasources.JDBCSettingsFor(datasources.Datasource{
		Name: "Cass1", DBType: "cassandra", Host: "127.0.0.1", Port: 9042, DBName: "ks",
		Username: "u", Password: "p",
	})
	if cass != nil {
		t.Fatalf("cassandra must not produce JDBC settings, got %+v", cass)
	}
	astra := datasources.JDBCSettingsFor(datasources.Datasource{
		Name: "Astra1", DBType: "cassandra", Host: "https://example.apps.astra.datastax.com",
		DBName: "default_keyspace", Password: "AstraCS:token",
		Options: map[string]string{datasources.OptionConnectionMethod: datasources.ConnectionMethodAstra},
	})
	if astra != nil {
		t.Fatalf("astra must not produce JDBC settings, got %+v", astra)
	}

	notes := datasources.ContextSkipNotes([]datasources.Datasource{
		{Name: "MainPG", DBType: "postgresql", Host: "localhost", Port: 5432, DBName: "reports", Username: "u"},
		{Name: "Astra1", DBType: "cassandra", Host: "https://example.apps.astra.datastax.com",
			DBName: "default_keyspace", Options: map[string]string{datasources.OptionConnectionMethod: "astra"}},
	})
	if len(notes) != 1 || !strings.Contains(notes[0], "Astra1") || !strings.Contains(notes[0], "Astra") {
		t.Fatalf("skip notes=%v", notes)
	}
	xmlCass := datasources.BuildContextXML([]datasources.Datasource{
		{Name: "MainPG", DBType: "postgresql", Host: "localhost", Port: 5432, DBName: "reports", Username: "u", Password: "x"},
		{Name: "Astra1", DBType: "cassandra", Host: "https://example.apps.astra.datastax.com",
			DBName: "default_keyspace", Options: map[string]string{datasources.OptionConnectionMethod: "astra"}},
	})
	if !strings.Contains(xmlCass, `name="jdbc/MainPG"`) {
		t.Fatalf("expected MainPG in context, got %s", xmlCass)
	}
	if strings.Contains(xmlCass, "Astra1") {
		t.Fatalf("Astra must not appear in context.xml: %s", xmlCass)
	}
	if datasources.CountJDBCContextResources([]datasources.Datasource{
		{Name: "MainPG", DBType: "postgresql", Host: "localhost", Port: 5432, DBName: "reports", Username: "u"},
		{Name: "Astra1", DBType: "cassandra", Host: "https://x", DBName: "ks"},
		{Name: "BQ1", DBType: "bigquery", DBName: "proj"},
	}) != 1 {
		t.Fatal("expected 1 JDBC resource")
	}
	bq := datasources.JDBCSettingsFor(datasources.Datasource{
		Name: "BQ1", DBType: "bigquery", DBName: "flawless-spirit-386523",
		Options: map[string]string{datasources.OptionCredentialsResourceUUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
	})
	if bq != nil {
		t.Fatalf("bigquery must not produce JDBC settings, got %+v", bq)
	}
	duck := datasources.JDBCSettingsFor(datasources.Datasource{
		Name: "Duck1", DBType: "duckdb", DBName: "/srv/data/analytics.duckdb",
		Options: map[string]string{datasources.OptionMode: datasources.DuckDBModeDatabase},
	})
	if duck != nil {
		t.Fatalf("duckdb must not produce JDBC settings, got %+v", duck)
	}
}
