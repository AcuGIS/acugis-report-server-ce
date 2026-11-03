//go:build oss

package datasourcefs

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/acugis/acugis-report-server-go/internal/arcgis"
)

// listEditionMapConnections reads jasper_arcgis. Feature Server connections
// are shared by tabular reports and ArcGIS map rendering. jasper_qgis stays
// in the Professional build. A missing table is skipped.
func listEditionMapConnections(ctx context.Context, pool *pgxpool.Pool, schema string) ([]Record, error) {
	var out []Record
	if pool == nil {
		return out, nil
	}
	arepo := arcgis.NewRepository(pool, schema)
	if rows, err := arepo.ListEnabled(ctx); err == nil {
		for _, c := range rows {
			if c.ID <= 0 || strings.TrimSpace(c.Name) == "" {
				continue
			}
			out = append(out, MapArcGIS(c.ID, c.Name, c.URL, c.Username, c.Password))
		}
	}
	return out, nil
}
