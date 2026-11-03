// Package datasourcefs is the public read facade for Report Server datasource
// and map-connection catalogs.
//
// Designer cannot import internal datasource packages. This package delegates
// to those stores, normalizers, and the content-resource reader. It does not
// create tables, copy SQL into Designer, or write a second connection registry.
//
// Secret fields use json:"-" so a Record cannot be accidentally serialized to
// a browser. Callers that build API responses must still copy only safe fields.
package datasourcefs

import "errors"

const (
	// RegistryDatasource is public.jasper_datasource (and the pg_store fallback
	// the existing repository already applies).
	RegistryDatasource = "datasource"
	// RegistryGeoServer is public.jasper_geoserver.
	RegistryGeoServer = "geoserver"
	// RegistryQGIS is public.jasper_qgis. The table is part of the non-OSS build.
	RegistryQGIS = "qgis"
	// RegistryArcGIS is public.jasper_arcgis. The same rows serve Feature Server
	// reports and ArcGIS map rendering in both editions.
	RegistryArcGIS = "arcgis"
)

var (
	// ErrUnavailable means the application database or a required store cannot be read.
	ErrUnavailable = errors.New("report server datasource catalog unavailable")
	// ErrNotFound means the requested connection was not in the catalog.
	ErrNotFound = errors.New("report server datasource not found")
)

// Record is one enabled Report Server connection after the existing stores
// have loaded it. Password, PEM, and service-account JSON stay in this process.
type Record struct {
	ID       int64
	Registry string
	Name     string
	// DBType is the Report Server type (postgresql, mysql, jasper_geoserver, …).
	// Designer maps it to a provider id. This package does not invent that mapping.
	DBType   string
	Host     string
	Port     int
	Database string
	Username string
	Schema   string
	AuthType string

	Password        string `json:"-"`
	PrivateKeyPEM   []byte `json:"-"`
	CredentialsJSON []byte `json:"-"`

	// Resource UUIDs are server-side pointers. They are not datasource secrets,
	// but they are not needed by the browser.
	PrivateKeyResourceUUID  string `json:"-"`
	CredentialsResourceUUID string `json:"-"`

	// Options are non-secret engine settings (mode, files, header names, project).
	Options map[string]string
}

// PasswordSet reports whether a password, token, or header secret is present.
func (r Record) PasswordSet() bool {
	return r.Password != ""
}

// PrivateKeySet reports whether PEM bytes were loaded.
func (r Record) PrivateKeySet() bool {
	return len(r.PrivateKeyPEM) > 0
}

// CredentialsSet reports whether service-account JSON was loaded.
func (r Record) CredentialsSet() bool {
	return len(r.CredentialsJSON) > 0
}
