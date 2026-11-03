package datasourcefs

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/acugis/acugis-report-server-go/internal/contentresource"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
)

// Config opens the existing application database. No schema is created.
type Config struct {
	DatabaseURL  string
	Schema       string
	ResourcesDir string
}

// Store reads datasource and map-connection catalogs through the existing
// repositories. It does not write connections or credential files.
type Store struct {
	pool      *pgxpool.Pool
	ds        *datasources.Repository
	geo       *geoserver.Repository
	resources *contentresource.Repository
	files     *contentresource.FileStore
	schema    string
}

// Open connects to the Report Server application database.
// An empty URL returns ErrUnavailable. Connection errors are returned as
// ErrUnavailable so callers do not log a DSN.
func Open(ctx context.Context, cfg Config) (*Store, error) {
	url := strings.TrimSpace(cfg.DatabaseURL)
	if url == "" {
		return nil, ErrUnavailable
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, ErrUnavailable
	}
	schema := strings.TrimSpace(cfg.Schema)
	if schema == "" {
		schema = "public"
	}
	return &Store{
		pool:      pool,
		ds:        datasources.NewRepository(pool, schema),
		geo:       geoserver.NewRepository(pool, schema),
		resources: contentresource.NewRepository(pool, schema),
		files:     contentresource.NewFileStore(cfg.ResourcesDir),
		schema:    schema,
	}, nil
}

// Close releases the pool.
func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

// List returns enabled datasource and map connections.
// A missing optional map table does not hide jasper_datasource.
// jasper_datasource itself failing returns ErrUnavailable.
func (s *Store) List(ctx context.Context) ([]Record, error) {
	if s == nil || s.ds == nil || !s.ds.Configured() {
		return nil, ErrUnavailable
	}
	rows, err := s.ds.ListEnabled(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	out := make([]Record, 0, len(rows))
	for _, ds := range rows {
		if ds.ID <= 0 || strings.TrimSpace(ds.Name) == "" {
			// pg_store fallback rows have no integer id. They are not exposed
			// under an invented identifier.
			continue
		}
		var pem, creds []byte
		if datasources.NormalizeAuthType(ds.AuthType) == datasources.AuthKeyPair || strings.TrimSpace(ds.PrivateKeyResourceUUID) != "" {
			pem, _ = s.readResource(ctx, ds.PrivateKeyResourceUUID)
		}
		if id := ds.CredentialsResourceUUID(); id != "" {
			creds, _ = s.readResource(ctx, id)
		}
		out = append(out, MapDatasource(ds, pem, creds))
	}
	if s.geo != nil {
		if geos, err := s.geo.ListEnabled(ctx); err == nil {
			for _, c := range geos {
				if c.ID <= 0 || strings.TrimSpace(c.Name) == "" {
					continue
				}
				out = append(out, MapGeoServer(c.ID, c.Name, c.URL, c.Username, c.Password))
			}
		}
	}
	maps, err := listEditionMapConnections(ctx, s.pool, s.schema)
	if err != nil {
		return nil, err
	}
	out = append(out, maps...)
	return out, nil
}

func (s *Store) readResource(ctx context.Context, resourceUUID string) ([]byte, error) {
	resourceUUID = strings.TrimSpace(resourceUUID)
	if resourceUUID == "" || s.resources == nil || s.files == nil || strings.TrimSpace(s.files.Dir) == "" {
		return nil, ErrUnavailable
	}
	res, err := s.resources.GetByUUID(ctx, resourceUUID)
	if err != nil || res == nil {
		return nil, ErrNotFound
	}
	return s.files.Read(res.StorageFilename)
}
