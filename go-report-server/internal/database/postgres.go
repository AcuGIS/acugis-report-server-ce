// Package database provides a read-only PostgreSQL pool for diagnostics.
// It must not create, alter, seed, or mutate application data.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/acugis/acugis-report-server-go/internal/config"
)

// ExpectedTables are tables defined in database/schema.sql that diagnostics may probe via metadata only.
var ExpectedTables = []string{
	"jasper_datasource",
	"jasper_geoserver",
	"jasper_qgis",
	"jasper_arcgis",
	"jasper_access_group",
	"jasper_schedule",
	"jasper_recipient",
	"jasper_schedule_execution",
	"dashboard_users",
	"dashboard_sessions",
	"personal_access_tokens",
	"audit_log",
	"auth_groups",
	"auth_group_members",
	"resources",
	"resource_permissions",
	"content_folders",
	"content_items",
	"content_favorites",
}

// Pool wraps pgxpool with read-only diagnostic helpers.
type Pool struct {
	pool   *pgxpool.Pool
	schema string
	logger *slog.Logger
}

// Open creates a connection pool and validates connectivity with SELECT 1.
func Open(ctx context.Context, cfg config.DatabaseConfig, logger *slog.Logger) (*Pool, error) {
	if logger == nil {
		logger = slog.Default()
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	poolCfg.MaxConns = 4
	poolCfg.MinConns = 0
	poolCfg.MaxConnLifetime = 30 * time.Minute

	connectCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	p := &Pool{pool: pool, schema: cfg.Schema, logger: logger}
	if err := p.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, err
	}

	logger.Info("postgres pool ready",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.Name,
		"user", cfg.User,
		"schema", cfg.Schema,
		"sslmode", cfg.SSLMode,
	)
	return p, nil
}

// Ping runs SELECT 1.
func (p *Pool) Ping(ctx context.Context) error {
	if p == nil || p.pool == nil {
		return fmt.Errorf("postgres pool is nil")
	}
	var one int
	if err := p.pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("postgres ping failed: %w", err)
	}
	if one != 1 {
		return fmt.Errorf("postgres ping unexpected result: %d", one)
	}
	return nil
}

// TablePresence returns whether expected application tables exist (metadata only).
func (p *Pool) TablePresence(ctx context.Context) (map[string]bool, error) {
	out := make(map[string]bool, len(ExpectedTables))
	for _, name := range ExpectedTables {
		out[name] = false
	}
	if p == nil || p.pool == nil {
		return out, fmt.Errorf("postgres pool is nil")
	}

	schema := p.schema
	if schema == "" {
		schema = "public"
	}

	rows, err := p.pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = $1
		  AND table_type = 'BASE TABLE'
		  AND table_name = ANY($2)
	`, schema, ExpectedTables)
	if err != nil {
		return out, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return out, fmt.Errorf("scan table name: %w", err)
		}
		out[name] = true
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("iterate tables: %w", err)
	}
	return out, nil
}

// PGX returns the underlying pgx pool for read-only package repositories.
// Callers must not mutate application data through this pool.
func (p *Pool) PGX() *pgxpool.Pool {
	if p == nil {
		return nil
	}
	return p.pool
}

// Close shuts down the pool.
func (p *Pool) Close() {
	if p == nil || p.pool == nil {
		return
	}
	p.pool.Close()
	p.logger.Info("postgres pool closed")
}
