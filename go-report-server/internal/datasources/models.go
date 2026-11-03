// Package datasources provides Jasper datasource catalog access and admin CRUD.
package datasources

import "strings"

// Datasource is one enabled row from jasper_datasource (PHP jrs_find_datasource).
type Datasource struct {
	ID                     int64  `json:"id,omitempty"`
	Name                   string `json:"name"`
	DBType                 string `json:"db_type"`
	Host                   string `json:"host"`
	Port                   int    `json:"port"`
	DBName                 string `json:"dbname"`
	Username               string `json:"username"`
	Password               string `json:"-"`
	SchemaName             string `json:"schema_name,omitempty"`
	AuthType               string `json:"auth_type,omitempty"`
	PublicKeyResourceUUID  string `json:"public_key_resource_uuid,omitempty"`
	PrivateKeyResourceUUID string `json:"private_key_resource_uuid,omitempty"`
	// PrivateKeyPEM is filled only when generating Jasper context for key_pair auth.
	// It is never persisted to the database.
	PrivateKeyPEM []byte `json:"-"`
	// Options holds engine-specific settings (Designer-compatible camelCase keys).
	// For Cassandra: connectionMethod (standard|astra), localDatacenter.
	// Secrets never belong here — password/token stay in Password.
	Options map[string]string `json:"options,omitempty"`
}

// Auth type constants for jasper_datasource.auth_type.
const (
	AuthPassword = "password"
	AuthOAuth    = "oauth"
	AuthKeyPair  = "key_pair"
)

// NormalizeAuthType returns a supported auth_type (default password).
func NormalizeAuthType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case AuthOAuth:
		return AuthOAuth
	case AuthKeyPair, "keypair", "key-pair":
		return AuthKeyPair
	default:
		return AuthPassword
	}
}

// NormalizeDBType mirrors PHP jrs_normalize_db_type.
func NormalizeDBType(dbType string) string {
	normalized := strings.ToLower(strings.TrimSpace(dbType))
	switch normalized {
	case "postgres", "pgsql", "pg":
		return "postgresql"
	case "mariadb":
		return "mysql"
	case "mssql", "sql server", "microsoft sql server":
		return "sqlserver"
	case "geopackage", "gpkg":
		return "sqlite"
	case "ibm db2", "ibmdb2":
		return "db2"
	case "apache cassandra", "datastax", "datastax astra", "astra":
		return "cassandra"
	case "google bigquery", "googlebigquery", "bq":
		return "bigquery"
	case "duck db", "duck":
		return "duckdb"
	case "arcgis", "arcgis feature server", "feature server", "featureserver", "jasper_arcgis", "arcgis_feature_server":
		return "arcgis"
	case "geoserver", "geo server", "jasper_geoserver":
		return "geoserver"
	case "csv", "tsv":
		return "csv"
	case "json":
		return "json"
	case "excel", "xlsx":
		return "excel"
	case "xml":
		return "xml"
	case "postgresql", "mysql", "oracle", "sqlserver", "db2", "snowflake", "sqlite", "cassandra", "bigquery", "duckdb":
		return normalized
	default:
		return "postgresql"
	}
}

// IsPostgreSQL reports whether the datasource is PostgreSQL/PostGIS.
func (d Datasource) IsPostgreSQL() bool {
	return NormalizeDBType(d.DBType) == "postgresql"
}

// IsMySQL reports whether the datasource is MySQL/MariaDB.
func (d Datasource) IsMySQL() bool {
	return NormalizeDBType(d.DBType) == "mysql"
}

// IsOracle reports whether the datasource is Oracle.
func (d Datasource) IsOracle() bool {
	return NormalizeDBType(d.DBType) == "oracle"
}

// IsSQLServer reports whether the datasource is Microsoft SQL Server.
func (d Datasource) IsSQLServer() bool {
	return NormalizeDBType(d.DBType) == "sqlserver"
}

// IsSpatialCapable reports whether Map Report can query geometry from this
// datasource engine (PostGIS, MySQL spatial, SQL Server, or Oracle Spatial).
func (d Datasource) IsSpatialCapable() bool {
	switch NormalizeDBType(d.DBType) {
	case "postgresql", "mysql", "sqlserver", "oracle":
		return true
	default:
		return false
	}
}

// IsSnowflake reports whether the datasource is Snowflake.
func (d Datasource) IsSnowflake() bool {
	return NormalizeDBType(d.DBType) == "snowflake"
}

// IsSQLite reports whether the datasource is SQLite / GeoPackage.
func (d Datasource) IsSQLite() bool {
	return NormalizeDBType(d.DBType) == "sqlite"
}

// IsCassandra reports whether the datasource is Apache Cassandra / DataStax Astra.
func (d Datasource) IsCassandra() bool {
	return NormalizeDBType(d.DBType) == "cassandra"
}

// IsBigQuery reports whether the datasource is Google BigQuery.
func (d Datasource) IsBigQuery() bool {
	return NormalizeDBType(d.DBType) == "bigquery"
}

// IsDuckDB reports whether the datasource is DuckDB.
func (d Datasource) IsDuckDB() bool {
	return NormalizeDBType(d.DBType) == "duckdb"
}

// IsArcGIS reports whether the datasource is an ArcGIS Feature Server connection.
// Those rows are stored in jasper_arcgis, not jasper_datasource.
func (d Datasource) IsArcGIS() bool {
	return NormalizeDBType(d.DBType) == "arcgis"
}

// IsGeoServer reports whether the datasource is a GeoServer connection.
// Those rows are stored in jasper_geoserver, not jasper_datasource.
func (d Datasource) IsGeoServer() bool {
	return NormalizeDBType(d.DBType) == "geoserver"
}

// IsCSV reports whether the datasource is a CSV document.
func (d Datasource) IsCSV() bool {
	return NormalizeDBType(d.DBType) == "csv"
}

// IsJSON reports whether the datasource is a JSON document.
func (d Datasource) IsJSON() bool {
	return NormalizeDBType(d.DBType) == "json"
}

// IsExcel reports whether the datasource is an Excel workbook.
func (d Datasource) IsExcel() bool {
	return NormalizeDBType(d.DBType) == "excel"
}

// IsXML reports whether the datasource is an XML document.
func (d Datasource) IsXML() bool {
	return NormalizeDBType(d.DBType) == "xml"
}

// IsDocument reports whether the datasource is CSV, JSON, Excel, or XML.
func (d Datasource) IsDocument() bool {
	switch NormalizeDBType(d.DBType) {
	case "csv", "json", "excel", "xml":
		return true
	default:
		return false
	}
}

// IsJSONURL reports whether a JSON datasource fetches one HTTP(S) document.
func (d Datasource) IsJSONURL() bool {
	return d.IsJSON() && strings.EqualFold(d.Option(OptionJSONSource), JSONSourceURL)
}

// IsDocumentType reports whether dbType is a document provider.
func IsDocumentType(dbType string) bool {
	return Datasource{DBType: dbType}.IsDocument()
}

// IsCollectionFill reports whether Jasper fill uses JRMapCollectionDataSource
// (no JDBC / Tomcat JNDI resource). DuckDB uses direct DuckDB JDBC and is excluded.
func (d Datasource) IsCollectionFill() bool {
	return d.IsCassandra() || d.IsBigQuery()
}

// Option keys mirrored from Report Designer Cassandra / BigQuery / DuckDB connections.
const (
	OptionConnectionMethod        = "connectionMethod"
	OptionLocalDatacenter         = "localDatacenter"
	OptionCredentialsResourceUUID = "credentialsResourceUuid"
	OptionDefaultDataset          = "defaultDataset"
	OptionLocation                = "location"
	OptionProjectID               = "projectId"
	OptionMode                    = "mode"
	OptionFiles                   = "files"
	OptionAuthType                = "authType"
	OptionHTTPHeaderKeys          = "httpHeaderKeys"
	OptionSourceType              = "sourceType"
	// Document provider options match Designer Connection Manager.
	OptionResourceFile       = "resourceFile"
	OptionDelimiter          = "delimiter"
	OptionJSONSource         = "source"
	OptionJSONURL            = "jsonUrl"
	OptionCollection         = "collection"
	OptionSheet              = "sheet"
	JSONSourceFile           = "file"
	JSONSourceURL            = "url"
	ConnectionMethodStandard = "standard"
	ConnectionMethodAstra    = "astra"
	DuckDBModeDatabase       = "database"
	DuckDBModeFiles          = "files"
)

// CredentialsResourceUUID returns the Service Account JSON resource UUID for BigQuery.
func (d Datasource) CredentialsResourceUUID() string {
	return d.Option(OptionCredentialsResourceUUID)
}

// DefaultDataset returns the optional BigQuery default dataset.
func (d Datasource) DefaultDataset() string {
	if v := d.Option(OptionDefaultDataset); v != "" {
		return v
	}
	return strings.TrimSpace(d.SchemaName)
}

// BigQueryLocation returns the optional BigQuery job location.
func (d Datasource) BigQueryLocation() string {
	return d.Option(OptionLocation)
}

// CassandraConnectionMethod returns "standard" or "astra".
func (d Datasource) CassandraConnectionMethod() string {
	if d.Options == nil {
		return ConnectionMethodStandard
	}
	m := strings.ToLower(strings.TrimSpace(d.Options[OptionConnectionMethod]))
	switch m {
	case ConnectionMethodAstra, "datastax", "datastax_astra", "astra_db":
		return ConnectionMethodAstra
	default:
		return ConnectionMethodStandard
	}
}

// IsAstra reports whether this Cassandra datasource uses the Astra Data API.
func (d Datasource) IsAstra() bool {
	return d.IsCassandra() && d.CassandraConnectionMethod() == ConnectionMethodAstra
}

// Option returns a trimmed options value.
func (d Datasource) Option(key string) string {
	if d.Options == nil {
		return ""
	}
	return strings.TrimSpace(d.Options[key])
}

// DuckDBAuthType returns none|basic|bearer|headers for Data Files mode.
func (d Datasource) DuckDBAuthType() string {
	switch strings.ToLower(d.Option(OptionAuthType)) {
	case "basic", "basic_auth", "basicauth":
		return "basic"
	case "bearer", "bearer_token", "token":
		return "bearer"
	case "headers", "custom", "custom_headers", "header", "http_headers":
		return "headers"
	default:
		return "none"
	}
}
