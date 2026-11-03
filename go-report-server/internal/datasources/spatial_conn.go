package datasources

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
)

// QueryConn is a dialect-neutral connection for Map Report spatial queries.
// It wraps either a pgx connection or database/sql.
type QueryConn interface {
	Query(ctx context.Context, query string, args ...any) (QueryRows, error)
	QueryRow(ctx context.Context, query string, args ...any) QueryRow
	Close(ctx context.Context) error
}

// QueryRows is a result set with column names (for portable GeoJSON assembly).
type QueryRows interface {
	Next() bool
	Scan(dest ...any) error
	Columns() ([]string, error)
	Close() error
	Err() error
}

// QueryRow scans a single row.
type QueryRow interface {
	Scan(dest ...any) error
}

// AdaptPG wraps a pgx Conn as QueryConn.
func AdaptPG(conn Conn) QueryConn {
	if conn == nil {
		return nil
	}
	return pgQueryConn{conn: conn}
}

// AdaptSQL wraps *sql.DB as QueryConn. Caller must Close the returned handle
// (which closes the DB).
func AdaptSQL(db *sql.DB) QueryConn {
	if db == nil {
		return nil
	}
	return sqlQueryConn{db: db}
}

type pgQueryConn struct {
	conn Conn
}

func (c pgQueryConn) Query(ctx context.Context, query string, args ...any) (QueryRows, error) {
	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgQueryRows{rows: rows}, nil
}

func (c pgQueryConn) QueryRow(ctx context.Context, query string, args ...any) QueryRow {
	return pgQueryRow{row: c.conn.QueryRow(ctx, query, args...)}
}

func (c pgQueryConn) Close(ctx context.Context) error {
	return c.conn.Close(ctx)
}

type pgQueryRows struct {
	rows pgx.Rows
}

func (r pgQueryRows) Next() bool                  { return r.rows.Next() }
func (r pgQueryRows) Scan(dest ...any) error      { return r.rows.Scan(dest...) }
func (r pgQueryRows) Close() error                { r.rows.Close(); return nil }
func (r pgQueryRows) Err() error                  { return r.rows.Err() }
func (r pgQueryRows) Columns() ([]string, error) {
	fds := r.rows.FieldDescriptions()
	out := make([]string, len(fds))
	for i, fd := range fds {
		out[i] = string(fd.Name)
	}
	return out, nil
}

type pgQueryRow struct {
	row pgx.Row
}

func (r pgQueryRow) Scan(dest ...any) error { return r.row.Scan(dest...) }

type sqlQueryConn struct {
	db *sql.DB
}

func (c sqlQueryConn) Query(ctx context.Context, query string, args ...any) (QueryRows, error) {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return sqlQueryRows{rows: rows}, nil
}

func (c sqlQueryConn) QueryRow(ctx context.Context, query string, args ...any) QueryRow {
	return c.db.QueryRowContext(ctx, query, args...)
}

func (c sqlQueryConn) Close(ctx context.Context) error {
	_ = ctx
	return c.db.Close()
}

type sqlQueryRows struct {
	rows *sql.Rows
}

func (r sqlQueryRows) Next() bool             { return r.rows.Next() }
func (r sqlQueryRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r sqlQueryRows) Close() error           { return r.rows.Close() }
func (r sqlQueryRows) Err() error             { return r.rows.Err() }
func (r sqlQueryRows) Columns() ([]string, error) {
	return r.rows.Columns()
}
