package datasources

import "errors"

var (
	// ErrNotFound means no enabled datasource matched the name.
	ErrNotFound = errors.New("datasource not found")
	// ErrNotPostgreSQL means the datasource is not PostgreSQL/PostGIS.
	ErrNotPostgreSQL = errors.New("datasource is not postgresql")
	// ErrNotMySQL means the datasource is not MySQL/MariaDB.
	ErrNotMySQL = errors.New("datasource is not mysql")
	// ErrNotOracle means the datasource is not Oracle.
	ErrNotOracle = errors.New("datasource is not oracle")
	// ErrNotSQLServer means the datasource is not Microsoft SQL Server.
	ErrNotSQLServer = errors.New("datasource is not sqlserver")
	// ErrNotSpatial means the datasource engine is not supported for Map Report.
	ErrNotSpatial = errors.New("datasource is not spatial-capable")
	// ErrUnavailable means the application DB pool is missing.
	ErrUnavailable = errors.New("datasource catalog unavailable")
	// ErrInvalidInput means required fields were missing or invalid.
	ErrInvalidInput = errors.New("invalid datasource input")
)
