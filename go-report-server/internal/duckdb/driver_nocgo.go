//go:build !cgo

package duckdb

// CGO_ENABLED=0 builds do not link the DuckDB Go engine.
// JDBC fill and catalog reads do not need it. TestConnection and
// schema discovery through this package require a cgo build.
