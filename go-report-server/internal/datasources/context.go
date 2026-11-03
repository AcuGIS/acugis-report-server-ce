package datasources

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// JDBCSettings are Tomcat Resource attributes (PHP jrs_jdbc_settings_for_datasource).
type JDBCSettings struct {
	DriverClassName string
	URL             string
	ValidationQuery string
	// OmitPassword suppresses the Tomcat password= attribute (Snowflake key-pair).
	OmitPassword bool
}

// JDBCSettingsFor returns JDBC settings for a datasource row, or nil when incomplete.
func JDBCSettingsFor(ds Datasource) *JDBCSettings {
	host := strings.TrimSpace(ds.Host)
	dbName := strings.TrimSpace(ds.DBName)
	port := ds.Port
	dbType := NormalizeDBType(ds.DBType)
	if ds.IsDocument() {
		return nil
	}

	if dbName == "" {
		return nil
	}
	if dbType == "sqlite" {
		path := dbName
		if path == "" {
			path = host
		}
		if path == "" {
			return nil
		}
		return &JDBCSettings{
			DriverClassName: "org.sqlite.JDBC",
			URL:             "jdbc:sqlite:" + filepath.ToSlash(path),
			ValidationQuery: "SELECT 1",
		}
	}
	if host == "" || port <= 0 {
		return nil
	}

	switch dbType {
	case "mysql":
		driver := strings.TrimSpace(os.Getenv("JRS_MYSQL_JDBC_DRIVER"))
		if driver == "" {
			driver = "com.mysql.cj.jdbc.Driver"
		}
		return &JDBCSettings{
			DriverClassName: driver,
			URL: fmt.Sprintf(
				"jdbc:mysql://%s:%d/%s?useSSL=false&allowPublicKeyRetrieval=true&serverTimezone=UTC",
				host, port, dbName,
			),
			ValidationQuery: "SELECT 1",
		}
	case "oracle":
		url := ""
		if strings.HasPrefix(strings.ToLower(dbName), "sid:") {
			sid := strings.TrimSpace(dbName[4:])
			url = fmt.Sprintf("jdbc:oracle:thin:@%s:%d:%s", host, port, sid)
		} else {
			url = fmt.Sprintf("jdbc:oracle:thin:@//%s:%d/%s", host, port, dbName)
		}
		return &JDBCSettings{
			DriverClassName: "oracle.jdbc.OracleDriver",
			URL:             url,
			ValidationQuery: "SELECT 1 FROM DUAL",
		}
	case "sqlserver":
		return &JDBCSettings{
			DriverClassName: "com.microsoft.sqlserver.jdbc.SQLServerDriver",
			URL: fmt.Sprintf(
				"jdbc:sqlserver://%s:%d;databaseName=%s;encrypt=true;trustServerCertificate=false;",
				host, port, dbName,
			),
			ValidationQuery: "SELECT 1",
		}
	case "db2":
		// Db2 is not packaged / not offered for Tomcat JNDI — skip context.xml generation.
		return nil
	case "cassandra":
		// Cassandra / Astra use Go collection fill (gocql / Data API), not JDBC JNDI.
		return nil
	case "bigquery":
		// Google BigQuery uses Go collection fill (cloud.google.com/go/bigquery), not JDBC JNDI.
		return nil
	case "duckdb":
		// DuckDB uses Tomcat DuckDB JDBC + REPORT_CONNECTION (not context.xml pool /
		// not Go collection fill). Session init (httpfs/secrets/views) is per-connection.
		return nil
	case "arcgis":
		// ArcGIS Feature Server uses Go collection fill from jasper_arcgis, not JDBC JNDI.
		return nil
	case "geoserver":
		// GeoServer uses Go collection fill from jasper_geoserver, not JDBC JNDI.
		return nil
	case "csv", "json", "excel", "xml":
		return nil
	case "snowflake":
		return snowflakeJDBCSettings(ds, host, dbName)
	default:
		return &JDBCSettings{
			DriverClassName: "org.postgresql.Driver",
			URL:             fmt.Sprintf("jdbc:postgresql://%s:%d/%s", host, port, dbName),
			ValidationQuery: "SELECT 1",
		}
	}
}

func snowflakeJDBCSettings(ds Datasource, host, dbName string) *JDBCSettings {
	accountHost := snowflakeAccountHost(host)
	q := url.Values{}
	q.Set("db", dbName)
	if schema := strings.TrimSpace(ds.SchemaName); schema != "" {
		q.Set("schema", schema)
	}
	// Avoid Apache Arrow result format (fails on JDK 16+ without --add-opens).
	q.Set("JDBC_QUERY_RESULT_FORMAT", "JSON")

	omitPassword := false
	if NormalizeAuthType(ds.AuthType) == AuthKeyPair {
		if len(ds.PrivateKeyPEM) == 0 {
			// Key-pair selected but PEM not resolved — skip so we don't emit broken password auth.
			return nil
		}
		q.Set("authenticator", "snowflake_jwt")
		q.Set("private_key_base64", base64.StdEncoding.EncodeToString(ds.PrivateKeyPEM))
		if pwd := strings.TrimSpace(ds.Password); pwd != "" {
			q.Set("private_key_pwd", pwd)
		}
		omitPassword = true
	}

	return &JDBCSettings{
		DriverClassName: "net.snowflake.client.jdbc.SnowflakeDriver",
		URL:             fmt.Sprintf("jdbc:snowflake://%s/?%s", accountHost, q.Encode()),
		ValidationQuery: "SELECT 1",
		OmitPassword:    omitPassword,
	}
}

func snowflakeAccountHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(strings.ToLower(host), "https://")
	host = strings.TrimPrefix(host, "http://")
	if strings.Contains(strings.ToLower(host), "snowflakecomputing.com") {
		return host
	}
	return host + ".snowflakecomputing.com"
}

// ContextSkipNotes explains why catalog datasources were omitted from context.xml
// (Cassandra/Astra collection fill, Db2, incomplete JDBC settings).
func ContextSkipNotes(rows []Datasource) []string {
	var notes []string
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		if name == "" {
			continue
		}
		if row.IsCassandra() {
			method := row.CassandraConnectionMethod()
			if method == ConnectionMethodAstra {
				notes = append(notes, fmt.Sprintf(
					"%s: Apache Cassandra (DataStax Astra) uses Go Data API collection fill — not written to context.xml (no JDBC JNDI).",
					name,
				))
			} else {
				notes = append(notes, fmt.Sprintf(
					"%s: Apache Cassandra (standard/gocql) uses Go collection fill — not written to context.xml (no JDBC JNDI).",
					name,
				))
			}
			continue
		}
		if row.IsBigQuery() {
			notes = append(notes, fmt.Sprintf(
				"%s: Google BigQuery uses Go collection fill (cloud.google.com/go/bigquery) — not written to context.xml (no JDBC JNDI).",
				name,
			))
			continue
		}
		if row.IsDuckDB() {
			notes = append(notes, fmt.Sprintf(
				"%s: DuckDB uses Tomcat DuckDB JDBC REPORT_CONNECTION (not context.xml / not Go collection fill).",
				name,
			))
			continue
		}
		if row.IsArcGIS() {
			notes = append(notes, fmt.Sprintf(
				"%s: ArcGIS Feature Server uses Go collection fill from jasper_arcgis — not written to context.xml (no JDBC JNDI).",
				name,
			))
			continue
		}
		if row.IsGeoServer() {
			notes = append(notes, fmt.Sprintf(
				"%s: GeoServer uses Go collection fill from jasper_geoserver — not written to context.xml (no JDBC JNDI).",
				name,
			))
			continue
		}
		if row.IsDocument() {
			notes = append(notes, fmt.Sprintf(
				"%s: %s is a document datasource — not written to context.xml.",
				name, MetaForDBType(row.DBType).Label,
			))
			continue
		}
		if JDBCSettingsFor(row) == nil {
			dbType := NormalizeDBType(row.DBType)
			if dbType == "db2" {
				notes = append(notes, fmt.Sprintf("%s: IBM Db2 is not packaged for Tomcat JNDI; omitted from context.xml.", name))
			} else {
				notes = append(notes, fmt.Sprintf("%s: incomplete JDBC settings (host/port/database); omitted from context.xml.", name))
			}
		}
	}
	return notes
}

// CountJDBCContextResources returns how many datasources will appear as jdbc/* Resources.
func CountJDBCContextResources(rows []Datasource) int {
	n := 0
	for _, row := range rows {
		if strings.TrimSpace(row.Name) == "" {
			continue
		}
		if JDBCSettingsFor(row) != nil {
			n++
		}
	}
	return n
}

// BuildContextXML mirrors reports/generate_jasper_context.php output.
func BuildContextXML(rows []Datasource) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteByte('\n')
	b.WriteString("<Context>\n")
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		jdbc := JDBCSettingsFor(row)
		if name == "" || jdbc == nil {
			continue
		}
		b.WriteString("  <Resource name=\"jdbc/")
		b.WriteString(xmlEscape(name))
		b.WriteString("\"\n")
		b.WriteString("            auth=\"Container\"\n")
		b.WriteString("            type=\"javax.sql.DataSource\"\n")
		b.WriteString("            factory=\"org.apache.tomcat.jdbc.pool.DataSourceFactory\"\n")
		b.WriteString("            driverClassName=\"")
		b.WriteString(xmlEscape(jdbc.DriverClassName))
		b.WriteString("\"\n")
		b.WriteString("            url=\"")
		b.WriteString(xmlEscape(jdbc.URL))
		b.WriteString("\"\n")
		b.WriteString("            username=\"")
		b.WriteString(xmlEscape(row.Username))
		b.WriteString("\"\n")
		if !jdbc.OmitPassword {
			b.WriteString("            password=\"")
			b.WriteString(xmlEscape(row.Password))
			b.WriteString("\"\n")
		}
		b.WriteString("            initialSize=\"0\"\n")
		b.WriteString("            maxTotal=\"20\"\n")
		b.WriteString("            maxIdle=\"10\"\n")
		b.WriteString("            maxWaitMillis=\"10000\"\n")
		b.WriteString("            validationQuery=\"")
		b.WriteString(xmlEscape(jdbc.ValidationQuery))
		b.WriteString("\"\n")
		b.WriteString("            testOnBorrow=\"true\"/>\n")
	}
	b.WriteString("</Context>\n")
	return b.String()
}

// WriteContextXML writes Tomcat context.xml (PHP generate_jasper_context.php).
func WriteContextXML(path string, rows []Datasource) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("context path not configured")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o775); err != nil {
		return fmt.Errorf("output directory is not writable: %s", dir)
	}
	// Probe writability like PHP is_writable.
	tmp, err := os.CreateTemp(dir, ".ctx-probe-*")
	if err != nil {
		return fmt.Errorf("output directory is not writable: %s", dir)
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)

	xml := BuildContextXML(rows)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(xml), 0o644); err != nil {
		return fmt.Errorf("failed to write: %s", path)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write: %s", path)
	}
	return nil
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
