// Package geoserver provides jasper_geoserver admin CRUD and properties export
// for Java GeoServerAuth (Designer-published maps).
package geoserver

import (
	"context"
	"errors"
)

var (
	// ErrNotFound means no enabled connection matched the name.
	ErrNotFound = errors.New("geoserver connection not found")
	// ErrUnavailable means the application DB pool is missing or the table is unreadable.
	ErrUnavailable = errors.New("geoserver catalog unavailable")
	// ErrInvalidInput means required fields were missing or invalid.
	ErrInvalidInput = errors.New("invalid geoserver connection input")
)

// Connection is one enabled row from jasper_geoserver.
type Connection struct {
	ID       int64  `json:"id,omitempty"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"-"`
}

// Credentials is an enabled jasper_geoserver connection used for WFS queries.
// Password is held only in memory for the request. It is not written to JRXML.
type Credentials struct {
	Name     string
	URL      string
	Username string
	Password string `json:"-"`
}

// Catalog resolves a GeoServer connection by the name stored in JRXML.
type Catalog interface {
	FindGeoServer(ctx context.Context, name string) (Credentials, error)
}
