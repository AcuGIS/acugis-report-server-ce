//go:build oss

package qgis

import "context"

type Connection struct {
	ID      int64
	Name    string
	URL     string
	Project string
}

type Store interface {
	ListEnabled(ctx context.Context) ([]Connection, error)
	Upsert(ctx context.Context, c Connection) error
	DeleteByID(ctx context.Context, id int64) error
	TableAvailable(ctx context.Context) bool
}
