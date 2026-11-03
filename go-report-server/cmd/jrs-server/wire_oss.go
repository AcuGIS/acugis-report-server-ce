//go:build oss

package main

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/arcgis"
	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/contentresource"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/embeddedmap"
	"github.com/acugis/acugis-report-server-go/internal/folders"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/internal/server"
)

func applyProfessional(
	_ context.Context,
	_ config.Config,
	_ *slog.Logger,
	pool *pgxpool.Pool,
	schema string,
	_ *datasources.Repository,
	gsRepo geoserver.Store,
	_ datasources.PGConnector,
	_ *folders.Service,
	_ *reports.Store,
	_ *contentresource.Service,
	_ acl.Service,
	_ *auth.Service,
	_ *auth.Repository,
	_ *audit.Service,
	_ jasper.Gateway,
	_ *embeddedmap.Service,
	deps *server.Dependencies,
) func() {
	// ArcGIS Feature Server reporting uses the same jasper_arcgis catalog in
	// both editions. QGIS, Data Explorer, and the other Professional modules
	// stay unwired here.
	agRepo := arcgis.NewRepository(pool, schema)
	deps.ArcGIS = agRepo
	if deps.ScheduleRun != nil {
		deps.ScheduleRun.ArcGIS = agRepo
		deps.ScheduleRun.GeoServer = gsRepo
	}
	return func() {}
}
