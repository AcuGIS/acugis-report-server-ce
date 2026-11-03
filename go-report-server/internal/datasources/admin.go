package datasources

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Store is the admin-capable datasource catalog (list/upsert/delete).
type Store interface {
	Finder
	Namer
	ListEnabled(ctx context.Context) ([]Datasource, error)
	Upsert(ctx context.Context, ds Datasource) error
	DeleteByID(ctx context.Context, id int64) error
	EnsureDBTypeColumn(ctx context.Context) error
	EnsureAuthColumns(ctx context.Context) error
	EnsureOptionsColumn(ctx context.Context) error
	TableAvailable(ctx context.Context) bool
}

// DBTypeMeta mirrors PHP jrs_db_type_meta.
type DBTypeMeta struct {
	Key         string
	Label       string
	Port        int
	Schema      string
	DBNameLabel string
}

// DBTypeMetas returns the engines offered in the datasources UI.
func DBTypeMetas() []DBTypeMeta {
	return []DBTypeMeta{
		{Key: "postgresql", Label: "PostgreSQL", Port: 5432, Schema: "public", DBNameLabel: "Database"},
		{Key: "mysql", Label: "MySQL", Port: 3306, Schema: "", DBNameLabel: "Database"},
		{Key: "oracle", Label: "Oracle", Port: 1521, Schema: "", DBNameLabel: "Service name / SID"},
		{Key: "sqlserver", Label: "Microsoft SQL Server", Port: 1433, Schema: "", DBNameLabel: "Database"},
		{Key: "snowflake", Label: "Snowflake", Port: 443, Schema: "PUBLIC", DBNameLabel: "Database"},
		{Key: "sqlite", Label: "SQLite / GeoPackage", Port: 0, Schema: "", DBNameLabel: "Database file path"},
		{Key: "cassandra", Label: "Apache Cassandra", Port: 9042, Schema: "", DBNameLabel: "Keyspace"},
		{Key: "bigquery", Label: "Google BigQuery", Port: 0, Schema: "", DBNameLabel: "Project ID"},
		{Key: "duckdb", Label: "DuckDB", Port: 0, Schema: "", DBNameLabel: "Database File"},
		{Key: "arcgis", Label: "ArcGIS Feature Server", Port: 0, Schema: "", DBNameLabel: "Service URL"},
		{Key: "geoserver", Label: "GeoServer", Port: 0, Schema: "", DBNameLabel: "GeoServer URL"},
		{Key: "csv", Label: "CSV", Port: 0, Schema: "", DBNameLabel: "Document"},
		{Key: "json", Label: "JSON", Port: 0, Schema: "", DBNameLabel: "Document"},
		{Key: "excel", Label: "Excel", Port: 0, Schema: "", DBNameLabel: "Workbook"},
		{Key: "xml", Label: "XML", Port: 0, Schema: "", DBNameLabel: "Document"},
	}
}

// QCartaDBTypeKeys is the Phase 1 QCarta runtime datasource set.
// Oracle remains listed so an operator-supplied ojdbc jar can be used.
var QCartaDBTypeKeys = []string{"postgresql", "mysql", "sqlserver", "sqlite", "oracle"}

// DBTypeMetasFor returns the UI engine list, optionally restricted to keys.
func DBTypeMetasFor(keys []string) []DBTypeMeta {
	if len(keys) == 0 {
		return DBTypeMetas()
	}
	allow := map[string]bool{}
	for _, k := range keys {
		allow[NormalizeDBType(k)] = true
	}
	var out []DBTypeMeta
	for _, m := range DBTypeMetas() {
		if allow[m.Key] {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return DBTypeMetas()
	}
	return out
}

// AllowedDBType reports whether dbType is in the optional allow-list (empty list = all).
func AllowedDBType(dbType string, keys []string) bool {
	if len(keys) == 0 {
		return true
	}
	want := NormalizeDBType(dbType)
	for _, k := range keys {
		if NormalizeDBType(k) == want {
			return true
		}
	}
	return false
}

// MetaForDBType returns UI defaults for a db_type.
func MetaForDBType(dbType string) DBTypeMeta {
	dbType = NormalizeDBType(dbType)
	for _, m := range DBTypeMetas() {
		if m.Key == dbType {
			return m
		}
	}
	return DBTypeMetas()[0]
}

// TableAvailable reports whether jasper_datasource (or pg_store fallback) is readable.
func (r *Repository) TableAvailable(ctx context.Context) bool {
	if !r.Configured() {
		return false
	}
	var one int
	err := r.pool.QueryRow(ctx, `SELECT 1 FROM `+r.qualified("jasper_datasource")+` LIMIT 1`).Scan(&one)
	if err == nil || err == pgx.ErrNoRows {
		return true
	}
	err = r.pool.QueryRow(ctx, `SELECT 1 FROM `+r.qualified("pg_store")+` LIMIT 1`).Scan(&one)
	return err == nil || err == pgx.ErrNoRows
}

// EnsureDBTypeColumn mirrors PHP jrs_ensure_datasource_db_type_column.
func (r *Repository) EnsureDBTypeColumn(ctx context.Context) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	_, err := r.pool.Exec(ctx, `
ALTER TABLE `+r.qualified("jasper_datasource")+`
ADD COLUMN IF NOT EXISTS db_type varchar(20) NOT NULL DEFAULT 'postgresql'`)
	if err != nil {
		// Table missing — leave ListEnabled to surface a clear error.
		return nil
	}
	return nil
}

// EnsureAuthColumns adds auth_type and key resource UUID columns when missing.
func (r *Repository) EnsureAuthColumns(ctx context.Context) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	stmts := []string{
		`ALTER TABLE ` + r.qualified("jasper_datasource") + ` ADD COLUMN IF NOT EXISTS auth_type varchar(40) NOT NULL DEFAULT 'password'`,
		`ALTER TABLE ` + r.qualified("jasper_datasource") + ` ADD COLUMN IF NOT EXISTS public_key_resource_uuid uuid NULL`,
		`ALTER TABLE ` + r.qualified("jasper_datasource") + ` ADD COLUMN IF NOT EXISTS private_key_resource_uuid uuid NULL`,
	}
	for _, sql := range stmts {
		if _, err := r.pool.Exec(ctx, sql); err != nil {
			return nil
		}
	}
	return nil
}

// ListEnabled returns all enabled datasources (PHP jrs_datasources).
func (r *Repository) ListEnabled(ctx context.Context) ([]Datasource, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	_ = r.EnsureDBTypeColumn(ctx)
	_ = r.EnsureAuthColumns(ctx)

	rows, err := r.pool.Query(ctx, `
SELECT id, name, COALESCE(db_type, 'postgresql'), host, port, dbname, username, password, COALESCE(schema_name, ''),
       COALESCE(auth_type, 'password'),
       COALESCE(public_key_resource_uuid::text, ''),
       COALESCE(private_key_resource_uuid::text, '')
FROM `+r.qualified("jasper_datasource")+`
WHERE enabled = true
ORDER BY name ASC`)
	if err != nil {
		// Pre-migration schema without auth columns / db_type.
		rows, err = r.pool.Query(ctx, `
SELECT id, name, COALESCE(db_type, 'postgresql'), host, port, dbname, username, password, COALESCE(schema_name, '')
FROM `+r.qualified("jasper_datasource")+`
WHERE enabled = true
ORDER BY name ASC`)
		if err != nil {
			rows, err = r.pool.Query(ctx, `
SELECT id, name, host, port, dbname, username, password, COALESCE(schema_name, '')
FROM `+r.qualified("jasper_datasource")+`
WHERE enabled = true
ORDER BY name ASC`)
			if err != nil {
				return r.listPGStore(ctx)
			}
			defer rows.Close()
			return scanLegacyRows(rows)
		}
		defer rows.Close()
		return scanDBTypeRows(rows)
	}
	defer rows.Close()

	var out []Datasource
	for rows.Next() {
		var ds Datasource
		if err := rows.Scan(&ds.ID, &ds.Name, &ds.DBType, &ds.Host, &ds.Port, &ds.DBName, &ds.Username, &ds.Password, &ds.SchemaName,
			&ds.AuthType, &ds.PublicKeyResourceUUID, &ds.PrivateKeyResourceUUID); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		ds.Name = strings.TrimSpace(ds.Name)
		ds.DBType = NormalizeDBType(ds.DBType)
		ds.SchemaName = strings.TrimSpace(ds.SchemaName)
		ds.AuthType = NormalizeAuthType(ds.AuthType)
		ds.PublicKeyResourceUUID = strings.TrimSpace(ds.PublicKeyResourceUUID)
		ds.PrivateKeyResourceUUID = strings.TrimSpace(ds.PrivateKeyResourceUUID)
		ds.Options = r.loadOptions(ctx, ds.Name)
		if ds.Name != "" {
			out = append(out, ds)
		}
	}
	return out, rows.Err()
}

func scanDBTypeRows(rows pgx.Rows) ([]Datasource, error) {
	var out []Datasource
	for rows.Next() {
		var ds Datasource
		if err := rows.Scan(&ds.ID, &ds.Name, &ds.DBType, &ds.Host, &ds.Port, &ds.DBName, &ds.Username, &ds.Password, &ds.SchemaName); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		ds.Name = strings.TrimSpace(ds.Name)
		ds.DBType = NormalizeDBType(ds.DBType)
		ds.SchemaName = strings.TrimSpace(ds.SchemaName)
		ds.AuthType = AuthPassword
		if ds.Name != "" {
			out = append(out, ds)
		}
	}
	return out, rows.Err()
}

func scanLegacyRows(rows pgx.Rows) ([]Datasource, error) {
	var out []Datasource
	for rows.Next() {
		var ds Datasource
		if err := rows.Scan(&ds.ID, &ds.Name, &ds.Host, &ds.Port, &ds.DBName, &ds.Username, &ds.Password, &ds.SchemaName); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		ds.DBType = "postgresql"
		ds.Name = strings.TrimSpace(ds.Name)
		ds.SchemaName = strings.TrimSpace(ds.SchemaName)
		ds.AuthType = AuthPassword
		if ds.Name != "" {
			out = append(out, ds)
		}
	}
	return out, rows.Err()
}

func (r *Repository) listPGStore(ctx context.Context) ([]Datasource, error) {
	rows, err := r.pool.Query(ctx, `
SELECT svc_name, host, port, dbname, username, password, COALESCE(schema, '')
FROM `+r.qualified("pg_store")+`
ORDER BY svc_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()
	var out []Datasource
	for rows.Next() {
		var ds Datasource
		if err := rows.Scan(&ds.Name, &ds.Host, &ds.Port, &ds.DBName, &ds.Username, &ds.Password, &ds.SchemaName); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		ds.DBType = "postgresql"
		ds.Name = strings.TrimSpace(ds.Name)
		ds.SchemaName = strings.TrimSpace(ds.SchemaName)
		if ds.Name != "" {
			out = append(out, ds)
		}
	}
	return out, rows.Err()
}

// Upsert inserts or updates a datasource by unique name (PHP datasources.php).
func (r *Repository) Upsert(ctx context.Context, ds Datasource) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	_ = r.EnsureDBTypeColumn(ctx)
	_ = r.EnsureAuthColumns(ctx)
	_ = r.EnsureOptionsColumn(ctx)

	ds.Name = strings.TrimSpace(ds.Name)
	ds.DBType = NormalizeDBType(ds.DBType)
	if ds.DBType == "arcgis" {
		return fmt.Errorf("%w: ArcGIS Feature Server is stored in jasper_arcgis", ErrInvalidInput)
	}
	if ds.DBType == "geoserver" {
		return fmt.Errorf("%w: GeoServer is stored in jasper_geoserver", ErrInvalidInput)
	}
	ds.Host = strings.TrimSpace(ds.Host)
	ds.DBName = strings.TrimSpace(ds.DBName)
	ds.Username = strings.TrimSpace(ds.Username)
	ds.SchemaName = strings.TrimSpace(ds.SchemaName)
	ds.AuthType = NormalizeAuthType(ds.AuthType)
	ds.PublicKeyResourceUUID = strings.TrimSpace(ds.PublicKeyResourceUUID)
	ds.PrivateKeyResourceUUID = strings.TrimSpace(ds.PrivateKeyResourceUUID)
	meta := MetaForDBType(ds.DBType)
	if ds.Port <= 0 {
		ds.Port = meta.Port
	}
	if ds.SchemaName == "" && ds.DBType == "postgresql" {
		ds.SchemaName = "public"
	}
	if ds.SchemaName == "" && ds.DBType == "snowflake" {
		ds.SchemaName = "PUBLIC"
	}
	if ds.Name == "" {
		return ErrInvalidInput
	}
	if !ds.IsDocument() && ds.DBType != "duckdb" && ds.DBName == "" {
		return ErrInvalidInput
	}
	if ds.IsDocument() {
		if err := NormalizeDocumentRecord(&ds); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
		}
	} else if ds.DBType == "bigquery" {
		ds.Host = ""
		ds.Port = 0
		ds.Username = ""
		ds.Password = ""
		ds.AuthType = AuthPassword
		ds.PublicKeyResourceUUID = ""
		ds.PrivateKeyResourceUUID = ""
		if ds.Options == nil {
			ds.Options = map[string]string{}
		}
		ds.Options[OptionProjectID] = strings.TrimSpace(ds.DBName)
		// Preserve existing credential resource UUID when the form omits it (edit save).
		credUUID := strings.TrimSpace(ds.Options[OptionCredentialsResourceUUID])
		if credUUID == "" {
			if existing := r.loadOptions(ctx, ds.Name); existing != nil {
				if prev := strings.TrimSpace(existing[OptionCredentialsResourceUUID]); prev != "" {
					ds.Options[OptionCredentialsResourceUUID] = prev
					credUUID = prev
				}
			}
		}
		if credUUID == "" {
			return ErrInvalidInput
		}
		// Default dataset is optional — store in Options and optionally schema_name for display.
		if defDS := strings.TrimSpace(ds.Options[OptionDefaultDataset]); defDS != "" {
			ds.SchemaName = defDS
		} else if strings.TrimSpace(ds.SchemaName) != "" {
			ds.Options[OptionDefaultDataset] = strings.TrimSpace(ds.SchemaName)
		} else {
			ds.SchemaName = ""
			delete(ds.Options, OptionDefaultDataset)
		}
		if loc := strings.TrimSpace(ds.Options[OptionLocation]); loc == "" {
			delete(ds.Options, OptionLocation)
		}
	} else if ds.DBType == "cassandra" {
		if ds.Host == "" {
			return ErrInvalidInput
		}
		if ds.IsAstra() {
			ds.Port = 0
			ds.Username = ""
			ds.AuthType = AuthPassword
			if ds.Options == nil {
				ds.Options = map[string]string{}
			}
			ds.Options[OptionConnectionMethod] = ConnectionMethodAstra
			delete(ds.Options, OptionLocalDatacenter)
		} else {
			if ds.Port <= 0 {
				ds.Port = 9042
			}
			if ds.Options == nil {
				ds.Options = map[string]string{}
			}
			ds.Options[OptionConnectionMethod] = ConnectionMethodStandard
		}
	} else if ds.DBType == "duckdb" {
		ds.Host = ""
		ds.Port = 0
		ds.AuthType = AuthPassword
		ds.PublicKeyResourceUUID = ""
		ds.PrivateKeyResourceUUID = ""
		if ds.Options == nil {
			ds.Options = map[string]string{}
		}
		mode := strings.ToLower(strings.TrimSpace(ds.Options[OptionMode]))
		if mode == "file" || mode == "datafiles" || mode == "data_files" {
			mode = DuckDBModeFiles
		}
		if mode != DuckDBModeFiles {
			mode = DuckDBModeDatabase
		}
		ds.Options[OptionMode] = mode
		if mode == DuckDBModeFiles {
			files := strings.TrimSpace(ds.Options[OptionFiles])
			if files == "" || files == "[]" {
				return ErrInvalidInput
			}
			if ds.DBName == "" {
				ds.DBName = "remote"
			}
			// Files mode: dbname holds logical alias for display; clear JDBC-ish fields.
			ds.Username = strings.TrimSpace(ds.Username) // keep for Basic Auth
			auth := strings.ToLower(strings.TrimSpace(ds.Options[OptionAuthType]))
			switch auth {
			case "basic", "basic_auth", "basicauth":
				ds.Options[OptionAuthType] = "basic"
			case "bearer", "bearer_token", "token":
				ds.Options[OptionAuthType] = "bearer"
				ds.Username = ""
			case "headers", "custom", "custom_headers", "http_headers":
				ds.Options[OptionAuthType] = "headers"
				ds.Username = ""
			default:
				ds.Options[OptionAuthType] = "none"
				ds.Username = ""
			}
			ds.Options[OptionSourceType] = "http"
			// Preserve header keys on edit when omitted.
			if existing := r.loadOptions(ctx, ds.Name); existing != nil {
				if strings.TrimSpace(ds.Options[OptionHTTPHeaderKeys]) == "" {
					if prev := strings.TrimSpace(existing[OptionHTTPHeaderKeys]); prev != "" {
						ds.Options[OptionHTTPHeaderKeys] = prev
					}
				}
			}
		} else {
			if strings.TrimSpace(ds.DBName) == "" {
				return ErrInvalidInput
			}
			delete(ds.Options, OptionFiles)
			delete(ds.Options, OptionAuthType)
			delete(ds.Options, OptionHTTPHeaderKeys)
			delete(ds.Options, OptionSourceType)
			ds.Username = ""
			ds.Password = ""
			ds.SchemaName = ""
		}
	} else if ds.DBType != "sqlite" && ds.DBType != "duckdb" && (ds.Host == "" || ds.Username == "") {
		return ErrInvalidInput
	}
	if ds.DBType != "bigquery" && ds.DBType != "duckdb" && ds.AuthType != AuthKeyPair {
		ds.PublicKeyResourceUUID = ""
		ds.PrivateKeyResourceUUID = ""
	}

	var pubKey any
	var privKey any
	if ds.PublicKeyResourceUUID != "" {
		pubKey = ds.PublicKeyResourceUUID
	}
	if ds.PrivateKeyResourceUUID != "" {
		privKey = ds.PrivateKeyResourceUUID
	}
	optsJSON := encodeOptions(ds.Options)

	// Blank password on update keeps the existing secret so the edit form can omit it.
	_, err := r.pool.Exec(ctx, `
INSERT INTO `+r.qualified("jasper_datasource")+`
    (name, db_type, host, port, dbname, username, password, schema_name,
     auth_type, public_key_resource_uuid, private_key_resource_uuid, options, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::uuid, $11::uuid, $12::jsonb, true)
ON CONFLICT (name) DO UPDATE SET
    db_type = EXCLUDED.db_type,
    host = EXCLUDED.host,
    port = EXCLUDED.port,
    dbname = EXCLUDED.dbname,
    username = EXCLUDED.username,
    password = CASE
        WHEN EXCLUDED.password = '' THEN `+r.qualified("jasper_datasource")+`.password
        ELSE EXCLUDED.password
    END,
    schema_name = EXCLUDED.schema_name,
    auth_type = EXCLUDED.auth_type,
    public_key_resource_uuid = EXCLUDED.public_key_resource_uuid,
    private_key_resource_uuid = EXCLUDED.private_key_resource_uuid,
    options = EXCLUDED.options,
    enabled = true`,
		ds.Name, ds.DBType, ds.Host, ds.Port, ds.DBName, ds.Username, ds.Password, ds.SchemaName,
		ds.AuthType, pubKey, privKey, optsJSON,
	)
	if err != nil {
		// Pre-options column: upsert without options then patch.
		if err2 := r.upsertWithoutOptions(ctx, ds, pubKey, privKey); err2 == nil {
			_ = r.EnsureOptionsColumn(ctx)
			_, _ = r.pool.Exec(ctx, `
UPDATE `+r.qualified("jasper_datasource")+` SET options = $2::jsonb WHERE name = $1`,
				ds.Name, optsJSON)
			return nil
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func (r *Repository) upsertWithoutOptions(ctx context.Context, ds Datasource, pubKey, privKey any) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO `+r.qualified("jasper_datasource")+`
    (name, db_type, host, port, dbname, username, password, schema_name,
     auth_type, public_key_resource_uuid, private_key_resource_uuid, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::uuid, $11::uuid, true)
ON CONFLICT (name) DO UPDATE SET
    db_type = EXCLUDED.db_type,
    host = EXCLUDED.host,
    port = EXCLUDED.port,
    dbname = EXCLUDED.dbname,
    username = EXCLUDED.username,
    password = CASE
        WHEN EXCLUDED.password = '' THEN `+r.qualified("jasper_datasource")+`.password
        ELSE EXCLUDED.password
    END,
    schema_name = EXCLUDED.schema_name,
    auth_type = EXCLUDED.auth_type,
    public_key_resource_uuid = EXCLUDED.public_key_resource_uuid,
    private_key_resource_uuid = EXCLUDED.private_key_resource_uuid,
    enabled = true`,
		ds.Name, ds.DBType, ds.Host, ds.Port, ds.DBName, ds.Username, ds.Password, ds.SchemaName,
		ds.AuthType, pubKey, privKey,
	)
	return err
}

// DeleteByID removes a datasource row by id.
func (r *Repository) DeleteByID(ctx context.Context, id int64) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	if id <= 0 {
		return ErrInvalidInput
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM `+r.qualified("jasper_datasource")+` WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}
