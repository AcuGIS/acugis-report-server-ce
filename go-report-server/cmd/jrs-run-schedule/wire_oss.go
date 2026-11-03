//go:build oss

package main

import (
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/embeddedmap"
	"github.com/acugis/acugis-report-server-go/internal/qgis"
	"github.com/jackc/pgx/v5/pgxpool"
)

func attachProfessionalEmbed(*pgxpool.Pool, config.Config, *embeddedmap.Service) qgis.Store {
	return nil
}
