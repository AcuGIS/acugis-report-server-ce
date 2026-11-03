package database

import (
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/config"
)

func TestDatabaseConfigDSNConstruction(t *testing.T) {
	t.Parallel()
	cfg := config.DatabaseConfig{
		Host:           "db.internal",
		Port:           5432,
		Name:           "jasper_reports",
		User:           "jrs",
		Password:       "s3cret",
		SSLMode:        "require",
		ConnectTimeout: 5 * time.Second,
	}
	dsn := cfg.DSN()
	for _, part := range []string{
		"host=db.internal",
		"port=5432",
		"dbname=jasper_reports",
		"user=jrs",
		"password=s3cret",
		"sslmode=require",
		"connect_timeout=5",
	} {
		if !strings.Contains(dsn, part) {
			t.Fatalf("DSN missing %q: %s", part, dsn)
		}
	}
}

func TestExpectedTablesMatchSchemaContract(t *testing.T) {
	t.Parallel()
	required := []string{
		"jasper_datasource",
		"jasper_geoserver",
		"jasper_qgis",
		"jasper_arcgis",
		"dashboard_users",
		"dashboard_sessions",
		"resources",
		"resource_permissions",
	}
	set := map[string]bool{}
	for _, name := range ExpectedTables {
		set[name] = true
	}
	for _, name := range required {
		if !set[name] {
			t.Fatalf("ExpectedTables missing %s", name)
		}
	}
}
