// Package arcgis provides jasper_arcgis admin CRUD, properties export,
// and Feature Server collection fill. The same connection record is used
// for tabular reports and ArcGIS map rendering.
package arcgis

// Connection is one enabled row from jasper_arcgis.
type Connection struct {
	ID       int64  `json:"id,omitempty"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"-"`
}
