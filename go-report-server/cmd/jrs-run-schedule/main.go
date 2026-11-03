package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/arcgis"
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/contentresource"
	"github.com/acugis/acugis-report-server-go/internal/database"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/embeddedmap"
	"github.com/acugis/acugis-report-server-go/internal/folders"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/internal/schedules"
	"github.com/acugis/acugis-report-server-go/internal/spatial"
)

// jrs-run-schedule is the Go replacement for scripts/run_schedule.php.
// Usage: jrs-run-schedule <schedule_id>
//
// Installed systemd timers may still call PHP until cutover; this binary is
// ready for ExecStart when operators approve switching units.
func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: jrs-run-schedule <schedule_id>")
		return 1
	}
	scheduleID, err := strconv.Atoi(os.Args[1])
	if err != nil || scheduleID <= 0 {
		fmt.Fprintln(os.Stderr, "Usage: jrs-run-schedule <schedule_id>")
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Jasper.Timeout+2*time.Minute)
	defer cancel()

	db, err := database.Open(ctx, cfg.Database, logger)
	if err != nil {
		logger.Error("postgres unavailable", "error", err)
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	defer db.Close()

	reportStore := reports.NewStore(cfg.Paths.ReportJSONDir, cfg.Paths.ThumbnailDir, cfg.Paths.JRXMLDir)
	reportStore.SetFileOwner(cfg.Paths.ReportsFileOwner, cfg.Paths.ReportsFileGroup)
	dsRepo := datasources.NewRepository(db.PGX(), cfg.Database.Schema)
	gsRepo := geoserver.NewRepository(db.PGX(), cfg.Database.Schema)
	foldersSvc := folders.NewService(folders.NewRepository(db.PGX(), cfg.Database.Schema))
	resourceRepo := contentresource.NewRepository(db.PGX(), cfg.Database.Schema)
	resourceFiles := contentresource.NewFileStore(cfg.Paths.ResourcesDir)
	resourceSvc := contentresource.NewService(resourceRepo, resourceFiles, contentresource.FolderAdapter{Svc: foldersSvc})
	embedSvc := &embeddedmap.Service{
		Spatial: &spatial.Resolver{
			Datasources: dsRepo,
			Connector:   datasources.PGConnector{},
			JRXMLDir:    cfg.Paths.JRXMLDir,
		},
		Paths: embeddedmap.Paths{
			JRXMLDir:                  cfg.Paths.JRXMLDir,
			JasperReportsContainerDir: cfg.Paths.JasperReportsContainerDir,
			FileOwner:                 cfg.Paths.ReportsFileOwner,
			FileGroup:                 cfg.Paths.ReportsFileGroup,
		},
		Edition:   scheduleEdition(cfg, logger),
		GeoServer: gsRepo,
	}
	_ = attachProfessionalEmbed(db.PGX(), cfg, embedSvc)
	schPaths := schedules.NewPaths(cfg.Paths.ScheduleDir, cfg.Paths.ScheduleOutputDir, cfg.Paths.EmailTmplDir, cfg.Paths.AppRoot)
	_, _, _, runner, _, _ := schedules.NewFromOptions(schedules.Options{
		Pool:        db.PGX(),
		Schema:      cfg.Database.Schema,
		Paths:       schPaths,
		Jasper:      schedules.DefaultRunnerConfig(cfg.Jasper.ReportURL, cfg.Jasper.Timeout),
		Mail:        schedules.MailConfigFromEnv(),
		Systemd:     schedules.DefaultSystemdOptions(cfg.Paths.AppRoot),
		AppRoot:     cfg.Paths.AppRoot,
		Reports:     reportStore,
		Datasources: dsRepo,
		EmbeddedMap: embedSvc,
	})
	if runner != nil {
		runner.ResourcePaths = cfg.Paths.ResourcePathsDir
		runner.ArcGIS = arcgis.NewRepository(db.PGX(), cfg.Database.Schema)
		runner.GeoServer = gsRepo
		runner.ResolveResourceBytes = func(ctx context.Context, resourceUUID string) ([]byte, error) {
			if resourceSvc == nil || !resourceSvc.Configured() {
				return nil, fmt.Errorf("content resources unavailable")
			}
			_, data, err := resourceSvc.ReadBytes(ctx, resourceUUID)
			if err != nil {
				return nil, err
			}
			if len(data) == 0 {
				return nil, fmt.Errorf("resource is empty")
			}
			return data, nil
		}
	}
	if runner == nil {
		fmt.Fprintln(os.Stderr, "schedule runner unavailable")
		return 1
	}

	result := runner.Execute(ctx, scheduleID, "scheduled")
	if !result.OK {
		msg := result.Message
		if msg == "" {
			msg = "run failed"
		}
		fmt.Fprintln(os.Stderr, msg)
		return 1
	}

	fmt.Printf("OK execution_id=%d file=%s email=%s\n",
		result.ExecutionID, result.OutputFile, result.EmailStatus)
	return 0
}
