package geoserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the admin-capable GeoServer connection catalog.
type Store interface {
	ListEnabled(ctx context.Context) ([]Connection, error)
	// FindGeoServer returns one enabled connection, including its password, for
	// server-side WFS queries. The password is never written to JRXML.
	FindGeoServer(ctx context.Context, name string) (Credentials, error)
	Upsert(ctx context.Context, c Connection) error
	DeleteByID(ctx context.Context, id int64) error
	TableAvailable(ctx context.Context) bool
}

// Repository reads/writes jasper_geoserver from the app DB.
type Repository struct {
	pool   *pgxpool.Pool
	schema string
}

// NewRepository constructs a GeoServer connection catalog.
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

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// TableAvailable reports whether jasper_geoserver is readable.
func (r *Repository) TableAvailable(ctx context.Context) bool {
	if !r.Configured() {
		return false
	}
	var one int
	err := r.pool.QueryRow(ctx, `SELECT 1 FROM `+r.qualified("jasper_geoserver")+` LIMIT 1`).Scan(&one)
	return err == nil || err == pgx.ErrNoRows
}

// ListEnabled returns all enabled GeoServer connections.
func (r *Repository) ListEnabled(ctx context.Context) ([]Connection, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	sql := `
SELECT id, name, url, COALESCE(username, ''), COALESCE(password, '')
FROM ` + r.qualified("jasper_geoserver") + `
WHERE enabled = true
ORDER BY name ASC`
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var out []Connection
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.ID, &c.Name, &c.URL, &c.Username, &c.Password); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		c.Name = strings.TrimSpace(c.Name)
		c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
		c.Username = strings.TrimSpace(c.Username)
		if c.Name == "" {
			continue
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FindGeoServer loads one enabled connection by its catalog name.
// The name is the JRXML value acugis.datasource.connection.
func (r *Repository) FindGeoServer(ctx context.Context, name string) (Credentials, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Credentials{}, ErrInvalidInput
	}
	if !r.Configured() {
		return Credentials{}, ErrUnavailable
	}
	var c Credentials
	err := r.pool.QueryRow(ctx, `
SELECT name, url, COALESCE(username, ''), COALESCE(password, '')
FROM `+r.qualified("jasper_geoserver")+`
WHERE enabled = true AND name = $1`, name).Scan(&c.Name, &c.URL, &c.Username, &c.Password)
	if err == pgx.ErrNoRows {
		return Credentials{}, ErrNotFound
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	c.Name = strings.TrimSpace(c.Name)
	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	c.Username = strings.TrimSpace(c.Username)
	return c, nil
}

// blankPasswordKeepsExisting is the upsert clause that leaves a stored password
// in place when an edit submits a blank password field.
const blankPasswordKeepsExisting = `WHEN EXCLUDED.password = '' THEN `

// Upsert inserts or updates a connection by unique name.
// A blank password on update keeps the stored password.
func (r *Repository) Upsert(ctx context.Context, c Connection) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	c.Name = strings.TrimSpace(c.Name)
	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	c.Username = strings.TrimSpace(c.Username)
	if c.Name == "" || c.URL == "" {
		return ErrInvalidInput
	}
	if strings.Contains(c.Name, ".") {
		return fmt.Errorf("%w: name must not contain a period", ErrInvalidInput)
	}

	_, err := r.pool.Exec(ctx, `
INSERT INTO `+r.qualified("jasper_geoserver")+`
    (name, url, username, password, enabled)
VALUES ($1, $2, $3, $4, true)
ON CONFLICT (name) DO UPDATE SET
    url = EXCLUDED.url,
    username = EXCLUDED.username,
    password = CASE
        `+blankPasswordKeepsExisting+r.qualified("jasper_geoserver")+`.password
        ELSE EXCLUDED.password
    END,
    enabled = true`,
		c.Name, c.URL, c.Username, c.Password,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// DeleteByID removes a connection row by id.
func (r *Repository) DeleteByID(ctx context.Context, id int64) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	if id <= 0 {
		return ErrInvalidInput
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM `+r.qualified("jasper_geoserver")+` WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}
