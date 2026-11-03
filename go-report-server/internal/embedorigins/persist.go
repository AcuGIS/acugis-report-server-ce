package embedorigins

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSystem      = errors.New("origin is system configured")
	ErrNotFound    = errors.New("origin not found")
	ErrUnavailable = errors.New("embed origins unavailable")
)

// Persister stores Admin-configured origins. Environment origins are not written.
type Persister interface {
	Configured() bool
	EnsureSchema(ctx context.Context) error
	List(ctx context.Context) ([]string, error)
	Insert(ctx context.Context, origin string, userID int64) error
	Delete(ctx context.Context, origin string) error
}

// Memory is an in-process persister for tests and for reload checks.
type Memory struct {
	mu      sync.Mutex
	origins []string
}

// NewMemory returns an empty memory persister.
func NewMemory() *Memory {
	return &Memory{}
}

func (m *Memory) Configured() bool { return m != nil }

func (m *Memory) EnsureSchema(context.Context) error { return nil }

func (m *Memory) List(context.Context) ([]string, error) {
	if m == nil {
		return nil, ErrUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.origins...), nil
}

func (m *Memory) Insert(_ context.Context, origin string, _ int64) error {
	if m == nil {
		return ErrUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.origins {
		if strings.EqualFold(existing, origin) {
			return nil
		}
	}
	m.origins = append(m.origins, origin)
	return nil
}

func (m *Memory) Delete(_ context.Context, origin string) error {
	if m == nil {
		return ErrUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	next := make([]string, 0, len(m.origins))
	found := false
	for _, existing := range m.origins {
		if strings.EqualFold(existing, origin) {
			found = true
			continue
		}
		next = append(next, existing)
	}
	if !found {
		return ErrNotFound
	}
	m.origins = next
	return nil
}

// Repository persists Admin origins in PostgreSQL.
type Repository struct {
	pool   *pgxpool.Pool
	schema string
}

// NewRepository constructs a repository. A nil pool is unconfigured.
func NewRepository(pool *pgxpool.Pool, schema string) *Repository {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		schema = "public"
	}
	return &Repository{pool: pool, schema: schema}
}

func (r *Repository) Configured() bool {
	return r != nil && r.pool != nil
}

func (r *Repository) q(table string) string {
	return quoteIdent(r.schema) + "." + quoteIdent(table)
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// EnsureSchema creates embed_allowed_origin if it is missing.
func (r *Repository) EnsureSchema(ctx context.Context) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	table := r.q("embed_allowed_origin")
	stmt := `CREATE TABLE IF NOT EXISTS ` + table + ` (
    origin text PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by integer NULL REFERENCES ` + r.q("dashboard_users") + ` (id) ON DELETE SET NULL
)`
	if _, err := r.pool.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("embed origins ensure schema: %w", err)
	}
	return nil
}

// List returns persisted origins in insertion order.
func (r *Repository) List(ctx context.Context) ([]string, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	rows, err := r.pool.Query(ctx, `SELECT origin FROM `+r.q("embed_allowed_origin")+` ORDER BY created_at, origin`)
	if err != nil {
		return nil, fmt.Errorf("embed origins list: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var origin string
		if err := rows.Scan(&origin); err != nil {
			return nil, fmt.Errorf("embed origins scan: %w", err)
		}
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		out = append(out, origin)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("embed origins list: %w", err)
	}
	return out, nil
}

// Insert stores one normalized origin. A duplicate is a no-op.
func (r *Repository) Insert(ctx context.Context, origin string, userID int64) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	var uid any
	if userID > 0 {
		uid = userID
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO `+r.q("embed_allowed_origin")+` (origin, created_by) VALUES ($1, $2) ON CONFLICT (origin) DO NOTHING`, origin, uid)
	if err != nil {
		return fmt.Errorf("embed origins insert: %w", err)
	}
	return nil
}

// Delete removes one persisted origin.
func (r *Repository) Delete(ctx context.Context, origin string) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM `+r.q("embed_allowed_origin")+` WHERE origin = $1`, origin)
	if err != nil {
		return fmt.Errorf("embed origins delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
