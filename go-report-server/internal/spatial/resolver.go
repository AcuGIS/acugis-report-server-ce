package spatial

import (
	"context"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

const spatialRequiredMsg = "Map Report requires a spatial datasource (PostgreSQL/PostGIS, MySQL, SQL Server, or Oracle)."

// Resolver resolves map layers and opens spatial connections.
// Embedded maps and Map Reports share this; product HTTP APIs stay in mapreport.
type Resolver struct {
	Datasources datasources.Finder
	Connector   datasources.Connector
	JRXMLDir    string
}

// OpenSpatial opens a spatial connection and dialect for ds.
func (s *Resolver) OpenSpatial(ctx context.Context, ds *datasources.Datasource) (datasources.QueryConn, SpatialDialect, error) {
	d, err := DialectFor(ds)
	if err != nil {
		return nil, nil, err
	}
	if opener, ok := s.Connector.(datasources.SpatialOpener); ok {
		conn, err := opener.ConnectSpatial(ctx, ds)
		if err != nil {
			return nil, nil, err
		}
		return conn, d, nil
	}
	if ds.IsPostgreSQL() {
		conn, err := s.Connector.ConnectPG(ctx, ds)
		if err != nil {
			return nil, nil, err
		}
		return datasources.AdaptPG(conn), d, nil
	}
	return nil, nil, datasources.ErrNotSpatial
}
