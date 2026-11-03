package arcgis

import (
	"context"
	"errors"
)

var (
	// ErrNotFound means no enabled connection matched the name.
	ErrNotFound = errors.New("arcgis connection not found")
	// ErrUnavailable means the application DB pool is missing or the table is unreadable.
	ErrUnavailable = errors.New("arcgis catalog unavailable")
	// ErrInvalidInput means required fields were missing or invalid.
	ErrInvalidInput = errors.New("invalid arcgis connection input")
)

// Credentials is an enabled jasper_arcgis connection used for feature queries.
// Password is held only in memory for the request. It is not written to JRXML.
type Credentials struct {
	Name     string
	URL      string
	Username string
	Password string `json:"-"`
}

// Catalog resolves an ArcGIS connection by the name stored in JRXML.
type Catalog interface {
	FindArcGIS(ctx context.Context, name string) (Credentials, error)
}
