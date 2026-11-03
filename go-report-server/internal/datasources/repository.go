package datasources

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Finder looks up enabled datasources by name.
type Finder interface {
	Find(ctx context.Context, name string) (*Datasource, error)
}

// Namer lists enabled datasource names for publish/edit UIs.
type Namer interface {
	ListNames(ctx context.Context) ([]string, error)
}

// Repository reads jasper_datasource (with pg_store fallback) from the app DB.
type Repository struct {
	pool   *pgxpool.Pool
	schema string
}

// NewRepository constructs a read-only datasource catalog.
func NewRepository(pool *pgxpool.Pool, schema string) *Repository {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		schema = "public"
	}
	return &Repository{pool: pool, schema: schema}
}

// Configured reports whether a pool is available.
func (r *Repository) Configured() bool {
	return r != nil && r.pool != nil
}

func (r *Repository) qualified(table string) string {
	return quoteIdent(r.schema) + "." + quoteIdent(table)
}

// Find returns an enabled datasource by exact name (PHP jrs_find_datasource).
func (r *Repository) Find(ctx context.Context, name string) (*Datasource, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNotFound
	}
	if !r.Configured() {
		return nil, ErrUnavailable
	}

	ds, err := r.findJasperDatasource(ctx, name)
	if err == nil {
		return ds, nil
	}
	if err != ErrNotFound {
		// Table missing or pre-migration schema — try fallback / alternate select.
		if alt, altErr := r.findJasperDatasourceLegacy(ctx, name); altErr == nil {
			return alt, nil
		}
		if fb, fbErr := r.findPGStore(ctx, name); fbErr == nil {
			return fb, nil
		}
		return nil, err
	}
	if fb, fbErr := r.findPGStore(ctx, name); fbErr == nil {
		return fb, nil
	}
	return nil, ErrNotFound
}

func (r *Repository) findJasperDatasource(ctx context.Context, name string) (*Datasource, error) {
	_ = r.EnsureDBTypeColumn(ctx)
	_ = r.EnsureAuthColumns(ctx)

	sql := `
SELECT id, name, COALESCE(db_type, 'postgresql'), host, port, dbname, username, password, COALESCE(schema_name, ''),
       COALESCE(auth_type, 'password'),
       COALESCE(public_key_resource_uuid::text, ''),
       COALESCE(private_key_resource_uuid::text, '')
FROM ` + r.qualified("jasper_datasource") + `
WHERE enabled = true AND name = $1
LIMIT 1`
	var ds Datasource
	err := r.pool.QueryRow(ctx, sql, name).Scan(
		&ds.ID, &ds.Name, &ds.DBType, &ds.Host, &ds.Port, &ds.DBName, &ds.Username, &ds.Password, &ds.SchemaName,
		&ds.AuthType, &ds.PublicKeyResourceUUID, &ds.PrivateKeyResourceUUID,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		// Pre-auth-column schema: fall back without auth fields.
		return r.findJasperDatasourceNoAuth(ctx, name)
	}
	ds.DBType = NormalizeDBType(ds.DBType)
	ds.Name = strings.TrimSpace(ds.Name)
	ds.SchemaName = strings.TrimSpace(ds.SchemaName)
	ds.AuthType = NormalizeAuthType(ds.AuthType)
	ds.PublicKeyResourceUUID = strings.TrimSpace(ds.PublicKeyResourceUUID)
	ds.PrivateKeyResourceUUID = strings.TrimSpace(ds.PrivateKeyResourceUUID)
	ds.Options = r.loadOptions(ctx, ds.Name)
	return &ds, nil
}

func (r *Repository) findJasperDatasourceNoAuth(ctx context.Context, name string) (*Datasource, error) {
	sql := `
SELECT id, name, COALESCE(db_type, 'postgresql'), host, port, dbname, username, password, COALESCE(schema_name, '')
FROM ` + r.qualified("jasper_datasource") + `
WHERE enabled = true AND name = $1
LIMIT 1`
	var ds Datasource
	err := r.pool.QueryRow(ctx, sql, name).Scan(
		&ds.ID, &ds.Name, &ds.DBType, &ds.Host, &ds.Port, &ds.DBName, &ds.Username, &ds.Password, &ds.SchemaName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	ds.DBType = NormalizeDBType(ds.DBType)
	ds.Name = strings.TrimSpace(ds.Name)
	ds.SchemaName = strings.TrimSpace(ds.SchemaName)
	ds.AuthType = AuthPassword
	return &ds, nil
}

func (r *Repository) findJasperDatasourceLegacy(ctx context.Context, name string) (*Datasource, error) {
	sql := `
SELECT id, name, host, port, dbname, username, password, COALESCE(schema_name, '')
FROM ` + r.qualified("jasper_datasource") + `
WHERE enabled = true AND name = $1
LIMIT 1`
	var ds Datasource
	err := r.pool.QueryRow(ctx, sql, name).Scan(
		&ds.ID, &ds.Name, &ds.Host, &ds.Port, &ds.DBName, &ds.Username, &ds.Password, &ds.SchemaName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	ds.DBType = "postgresql"
	ds.Name = strings.TrimSpace(ds.Name)
	ds.SchemaName = strings.TrimSpace(ds.SchemaName)
	return &ds, nil
}

func (r *Repository) findPGStore(ctx context.Context, name string) (*Datasource, error) {
	sql := `
SELECT svc_name, host, port, dbname, username, password, COALESCE(schema, '')
FROM ` + r.qualified("pg_store") + `
WHERE svc_name = $1
LIMIT 1`
	var ds Datasource
	err := r.pool.QueryRow(ctx, sql, name).Scan(
		&ds.Name, &ds.Host, &ds.Port, &ds.DBName, &ds.Username, &ds.Password, &ds.SchemaName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	ds.DBType = "postgresql"
	ds.Name = strings.TrimSpace(ds.Name)
	ds.SchemaName = strings.TrimSpace(ds.SchemaName)
	return &ds, nil
}

// ListNames returns enabled datasource names (PHP jrs_datasource_names).
func (r *Repository) ListNames(ctx context.Context) ([]string, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	sql := `SELECT name FROM ` + r.qualified("jasper_datasource") + ` WHERE enabled = true ORDER BY name ASC`
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		// Fall back to legacy pg_store names when jasper_datasource is missing.
		return r.listPGStoreNames(ctx)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if len(out) == 0 {
		if fb, fbErr := r.listPGStoreNames(ctx); fbErr == nil && len(fb) > 0 {
			return fb, nil
		}
	}
	return out, nil
}

func (r *Repository) listPGStoreNames(ctx context.Context) ([]string, error) {
	sql := `SELECT svc_name FROM ` + r.qualified("pg_store") + ` ORDER BY svc_name ASC`
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
