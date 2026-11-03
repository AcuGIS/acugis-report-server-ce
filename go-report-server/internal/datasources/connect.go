package datasources

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	_ "github.com/microsoft/go-mssqldb"
	go_ora "github.com/sijms/go-ora/v2"
)

// Conn is a single PostGIS connection opened for a map query.
type Conn interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Close(ctx context.Context) error
}

// Connector opens ephemeral connections to report datasources.
type Connector interface {
	ConnectPG(ctx context.Context, ds *Datasource) (Conn, error)
}

// MySQLOpener opens MySQL connections for SQL-backed parameter option expansion
// (PHP jrs_mysql_connect_from_settings). Optional: connectors that only serve
// PostGIS map queries need not implement it.
type MySQLOpener interface {
	OpenMySQL(ctx context.Context, ds *Datasource) (*sql.DB, error)
}

// OracleOpener opens Oracle connections for SQL-backed parameter option expansion
// (PHP jrs_oracle_connect_from_settings). Optional: connectors that only serve
// PostGIS map queries need not implement it.
type OracleOpener interface {
	OpenOracle(ctx context.Context, ds *Datasource) (*sql.DB, error)
}

// SQLServerOpener opens Microsoft SQL Server connections for Map Report spatial queries.
type SQLServerOpener interface {
	OpenMSSQL(ctx context.Context, ds *Datasource) (*sql.DB, error)
}

// SpatialOpener opens a dialect-neutral connection for Map Report.
// PGConnector implements this for PostgreSQL, MySQL, Oracle, and SQL Server.
type SpatialOpener interface {
	ConnectSpatial(ctx context.Context, ds *Datasource) (QueryConn, error)
}

// PGConnector opens PostgreSQL connections via pgx (PHP jrs_pg_connect_from_settings)
// and MySQL/Oracle/SQL Server connections via database/sql for parameter options
// and Map Report spatial queries.
type PGConnector struct {
	Timeout time.Duration
}

// ConnectPG opens one connection to the datasource. Caller must Close it.
func (c PGConnector) ConnectPG(ctx context.Context, ds *Datasource) (Conn, error) {
	if ds == nil {
		return nil, ErrNotFound
	}
	if !ds.IsPostgreSQL() {
		return nil, ErrNotPostgreSQL
	}
	host := strings.TrimSpace(ds.Host)
	if host == "" {
		host = "localhost"
	}
	port := ds.Port
	if port <= 0 {
		port = 5432
	}
	dbname := strings.TrimSpace(ds.DBName)
	user := strings.TrimSpace(ds.Username)
	if dbname == "" || user == "" {
		return nil, fmt.Errorf("incomplete datasource connection settings")
	}

	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, ds.Password),
		Host:   host + ":" + strconv.Itoa(port),
		Path:   "/" + dbname,
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := pgx.Connect(connectCtx, u.String())
	if err != nil {
		return nil, fmt.Errorf("connect datasource %q: %w", ds.Name, err)
	}
	return conn, nil
}

// ConnectSpatial opens a QueryConn for Map Report against a spatial-capable datasource.
func (c PGConnector) ConnectSpatial(ctx context.Context, ds *Datasource) (QueryConn, error) {
	if ds == nil {
		return nil, ErrNotFound
	}
	if !ds.IsSpatialCapable() {
		return nil, ErrNotSpatial
	}
	switch {
	case ds.IsPostgreSQL():
		conn, err := c.ConnectPG(ctx, ds)
		if err != nil {
			return nil, err
		}
		return AdaptPG(conn), nil
	case ds.IsMySQL():
		db, err := c.OpenMySQL(ctx, ds)
		if err != nil {
			return nil, err
		}
		return AdaptSQL(db), nil
	case ds.IsOracle():
		db, err := c.OpenOracle(ctx, ds)
		if err != nil {
			return nil, err
		}
		return AdaptSQL(db), nil
	case ds.IsSQLServer():
		db, err := c.OpenMSSQL(ctx, ds)
		if err != nil {
			return nil, err
		}
		return AdaptSQL(db), nil
	default:
		return nil, ErrNotSpatial
	}
}

// OpenMySQL opens one MySQL/MariaDB *sql.DB for option queries. Caller must Close it.
func (c PGConnector) OpenMySQL(ctx context.Context, ds *Datasource) (*sql.DB, error) {
	if ds == nil {
		return nil, ErrNotFound
	}
	if !ds.IsMySQL() {
		return nil, ErrNotMySQL
	}
	dsn, err := mysqlDSN(ds, c.Timeout)
	if err != nil {
		return nil, err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("connect datasource %q: %w", ds.Name, err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(timeout)
	if err := db.PingContext(connectCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect datasource %q: %w", ds.Name, err)
	}
	return db, nil
}

func mysqlDSN(ds *Datasource, timeout time.Duration) (string, error) {
	host := strings.TrimSpace(ds.Host)
	if host == "" {
		host = "localhost"
	}
	port := ds.Port
	if port <= 0 {
		port = 3306
	}
	dbname := strings.TrimSpace(ds.DBName)
	user := strings.TrimSpace(ds.Username)
	if dbname == "" || user == "" {
		return "", fmt.Errorf("incomplete datasource connection settings")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	cfg := mysql.Config{
		User:                 user,
		Passwd:               ds.Password,
		Net:                  "tcp",
		Addr:                 host + ":" + strconv.Itoa(port),
		DBName:               dbname,
		ParseTime:            true,
		Loc:                  time.UTC,
		Timeout:              timeout,
		ReadTimeout:          60 * time.Second,
		WriteTimeout:         60 * time.Second,
		AllowNativePasswords: true,
	}
	return cfg.FormatDSN(), nil
}

// OpenOracle opens one Oracle *sql.DB for option queries. Caller must Close it.
// DBName is a service name, or "sid:ORCL" for SID (matches PHP / JDBC settings).
func (c PGConnector) OpenOracle(ctx context.Context, ds *Datasource) (*sql.DB, error) {
	if ds == nil {
		return nil, ErrNotFound
	}
	if !ds.IsOracle() {
		return nil, ErrNotOracle
	}
	dsn, err := oracleDSN(ds, c.Timeout)
	if err != nil {
		return nil, err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	db, err := sql.Open("oracle", dsn)
	if err != nil {
		return nil, fmt.Errorf("connect datasource %q: %w", ds.Name, err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(timeout)
	if err := db.PingContext(connectCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect datasource %q: %w", ds.Name, err)
	}
	return db, nil
}

func oracleDSN(ds *Datasource, timeout time.Duration) (string, error) {
	host := strings.TrimSpace(ds.Host)
	if host == "" {
		host = "localhost"
	}
	port := ds.Port
	if port <= 0 {
		port = 1521
	}
	dbname := strings.TrimSpace(ds.DBName)
	user := strings.TrimSpace(ds.Username)
	if dbname == "" || user == "" {
		return "", fmt.Errorf("incomplete datasource connection settings")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	opts := map[string]string{
		"TIMEOUT": strconv.Itoa(int(timeout.Seconds())),
	}
	service := dbname
	if strings.HasPrefix(strings.ToLower(dbname), "sid:") {
		sid := strings.TrimSpace(dbname[4:])
		if sid == "" {
			return "", fmt.Errorf("incomplete datasource connection settings")
		}
		opts["SID"] = sid
		service = ""
	}
	return go_ora.BuildUrl(host, port, service, user, ds.Password, opts), nil
}

// OpenMSSQL opens one SQL Server *sql.DB for Map Report. Caller must Close it.
func (c PGConnector) OpenMSSQL(ctx context.Context, ds *Datasource) (*sql.DB, error) {
	if ds == nil {
		return nil, ErrNotFound
	}
	if !ds.IsSQLServer() {
		return nil, ErrNotSQLServer
	}
	dsn, err := mssqlDSN(ds, c.Timeout)
	if err != nil {
		return nil, err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("connect datasource %q: %w", ds.Name, err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(timeout)
	if err := db.PingContext(connectCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect datasource %q: %w", ds.Name, err)
	}
	return db, nil
}

func mssqlDSN(ds *Datasource, timeout time.Duration) (string, error) {
	host := strings.TrimSpace(ds.Host)
	if host == "" {
		host = "localhost"
	}
	port := ds.Port
	if port <= 0 {
		port = 1433
	}
	dbname := strings.TrimSpace(ds.DBName)
	user := strings.TrimSpace(ds.Username)
	if dbname == "" || user == "" {
		return "", fmt.Errorf("incomplete datasource connection settings")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	q := url.Values{}
	q.Set("database", dbname)
	q.Set("connection timeout", strconv.Itoa(int(timeout.Seconds())))
	q.Set("dial timeout", strconv.Itoa(int(timeout.Seconds())))
	u := &url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(user, ds.Password),
		Host:     fmt.Sprintf("%s:%d", host, port),
		RawQuery: q.Encode(),
	}
	return u.String(), nil
}
