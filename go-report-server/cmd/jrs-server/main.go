package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/admin"
	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/contentresource"
	"github.com/acugis/acugis-report-server-go/internal/database"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/datasources/sfdriver"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/embeddedmap"
	"github.com/acugis/acugis-report-server-go/internal/embedorigins"
	"github.com/acugis/acugis-report-server-go/internal/embedtoken"
	"github.com/acugis/acugis-report-server-go/internal/filesystem"
	"github.com/acugis/acugis-report-server-go/internal/folders"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/internal/schedules"
	"github.com/acugis/acugis-report-server-go/internal/server"
	"github.com/acugis/acugis-report-server-go/internal/spatial"
)

// embedAuthAdapter validates emb_ Bearer tokens for API calls inside embedded viewers.
type embedAuthAdapter struct {
	secret string
}

func (a embedAuthAdapter) ResolveEmbedToken(_ context.Context, raw string) (*auth.User, error) {
	claims, err := embedtoken.ParseAndValidate(a.secret, raw)
	if err != nil {
		return nil, err
	}
	return auth.NewEmbedUser(claims.UserID, claims.Username, string(claims.ContentType), claims.ContentID, claims.Origin, claims.Parameters), nil
}

func main() {
	os.Exit(run())
}

func run() int {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		config.LoadDotEnvFiles("")
		ver := os.Getenv("JRS_APP_VERSION")
		if ver == "" {
			ver = config.DefaultAppVersion
		}
		fmt.Printf("%s\nVersion: %s\nEdition: %s\n", config.ProductName, ver, versionEdition().DisplayName())
		return 0
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		return 1
	}

	mailCfg := schedules.MailConfigFromEnv()
	licMgr, liveEdition := openLicense(cfg, logger)
	logger.Info("configuration loaded",
		"environment", cfg.Environment,
		"listen_addr", cfg.ListenAddr,
		"base_path", cfg.BasePath,
		"app_root", cfg.Paths.AppRoot,
		"app_version", cfg.AppVersion,
		"edition_env", cfg.Edition,
		"edition", string(liveEdition.CurrentEdition()),
		"auth_disabled", cfg.AuthDisabled,
		"smtp_configured", mailCfg.Host != "",
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.Database, logger)
	if err != nil {
		logger.Error("postgres startup failed", "error", err)
		return 1
	}
	defer db.Close()

	paths := filesystem.New(cfg.Paths)
	for _, st := range paths.ValidateRoots() {
		logger.Info("filesystem path check",
			"name", st.Name,
			"path", st.Path,
			"exists", st.Exists,
			"required", st.Required,
			"readable", st.Readable,
		)
	}

	authPool := db.PGX()
	var authDB *database.Pool
	if cfg.AuthDatabase.HasOverride() {
		opened, authErr := database.Open(ctx, cfg.AuthDatabase, logger)
		if authErr != nil {
			logger.Error("auth postgres startup failed", "error", authErr)
			return 1
		}
		authDB = opened
		authPool = authDB.PGX()
		defer authDB.Close()
		logger.Info("auth/acl pool attached", "url_set", cfg.AuthDatabase.URL != "")
	}
	authRepo := auth.NewRepository(authPool)
	authOpts := auth.Options{
		CookieName:   cfg.SessionCookieName,
		AuthDisabled: cfg.AuthDisabled,
		Logger:       logger,
		PATPrefixes:  cfg.PATPrefixes,
	}
	if cfg.IsQCartaProfile() && len(authOpts.PATPrefixes) == 0 {
		authOpts.PATPrefixes = []string{"agp_", "jrs_"}
	}
	authSvc := auth.NewService(authRepo, authOpts)
	if cfg.EmbedTokenSecret != "" && !cfg.IsQCartaProfile() {
		authSvc.SetEmbedTokenResolver(embedAuthAdapter{secret: cfg.EmbedTokenSecret})
	}
	aclRepo := acl.NewRepository(authPool)
	if cfg.IsQCartaProfile() {
		aclRepo.SetPlatformService(acl.ServiceKeyReportRuntime, "QCarta Reports", "report-runtime")
	}
	aclSvc := acl.NewService(aclRepo, acl.Options{
		AuthDisabled: cfg.AuthDisabled,
		Logger:       logger,
		ServiceKey:   reportServiceKey(cfg.IsQCartaProfile()),
	})
	foldersRepo := folders.NewRepository(db.PGX(), cfg.Database.Schema)
	foldersSvc := folders.NewService(foldersRepo)
	aclSvc.SetInheritance(foldersRepo)
	if !cfg.IsQCartaProfile() {
		if _, err := foldersSvc.EnsureHome(ctx); err != nil {
			logger.Warn("ensure home folder", "error", err)
		} else {
			logger.Info("content folders ready")
		}
	}
	reportStore := reports.NewStore(cfg.Paths.ReportJSONDir, cfg.Paths.ThumbnailDir, cfg.Paths.JRXMLDir)
	reportStore.SetFileOwner(cfg.Paths.ReportsFileOwner, cfg.Paths.ReportsFileGroup)
	resourceRepo := contentresource.NewRepository(db.PGX(), cfg.Database.Schema)
	resourceFiles := contentresource.NewFileStore(cfg.Paths.ResourcesDir)
	resourceThumbs := contentresource.NewThumbnailStore(cfg.Paths.ResourceThumbnailsDir)
	resourcePaths := contentresource.NewPathStore(cfg.Paths.ResourcePathsDir)
	resourcePaths.SetFileOwner(cfg.Paths.ReportsFileOwner, cfg.Paths.ReportsFileGroup)
	resourceSvc := contentresource.NewService(resourceRepo, resourceFiles, contentresource.FolderAdapter{Svc: foldersSvc})
	resourceSvc.Thumbnails = resourceThumbs
	resourceSvc.Paths = resourcePaths
	if err := resourceFiles.EnsureDir(); err != nil {
		logger.Warn("ensure resources dir", "error", err, "path", cfg.Paths.ResourcesDir)
	} else {
		logger.Info("content resources ready", "path", cfg.Paths.ResourcesDir)
	}
	if err := resourceThumbs.EnsureDir(); err != nil {
		logger.Warn("ensure resource thumbnails dir", "error", err, "path", cfg.Paths.ResourceThumbnailsDir)
	} else {
		logger.Info("resource thumbnails ready", "path", cfg.Paths.ResourceThumbnailsDir)
	}
	if err := resourcePaths.EnsureDir(); err != nil {
		logger.Warn("ensure resource paths dir", "error", err, "path", cfg.Paths.ResourcePathsDir)
	} else {
		logger.Info("image resource paths ready",
			"path", cfg.Paths.ResourcePathsDir,
			"owner", cfg.Paths.ReportsFileOwner,
			"group", cfg.Paths.ReportsFileGroup)
	}
	dsRepo := datasources.NewRepository(db.PGX(), cfg.Database.Schema)
	gsRepo := geoserver.NewRepository(db.PGX(), cfg.Database.Schema)
	adminRepo := admin.NewRepository(db.PGX())
	sfdriver.Register()
	dsConnector := datasources.PGConnector{}
	spatialRes := &spatial.Resolver{
		Datasources: dsRepo,
		Connector:   dsConnector,
		JRXMLDir:    cfg.Paths.JRXMLDir,
	}
	mapSvc := &mapreport.Service{
		Datasources: dsRepo,
		Connector:   dsConnector,
		JRXMLDir:    cfg.Paths.JRXMLDir,
	}
	embedSvc := &embeddedmap.Service{
		Spatial: spatialRes,
		Paths: embeddedmap.Paths{
			JRXMLDir:                  cfg.Paths.JRXMLDir,
			JasperReportsContainerDir: cfg.Paths.JasperReportsContainerDir,
			FileOwner:                 cfg.Paths.ReportsFileOwner,
			FileGroup:                 cfg.Paths.ReportsFileGroup,
		},
		Edition:   liveEdition,
		GeoServer: gsRepo,
	}
	gateway := jasper.NewGateway(cfg.Jasper, jasper.Options{
		ReportJSONDir: cfg.Paths.ReportJSONDir,
		ReportExists:  reportStore.Exists,
	})

	auditRepo := audit.NewRepository(db.PGX(), cfg.Database.Schema)
	auditSvc := audit.NewService(auditRepo, audit.Options{
		Enabled:       cfg.Audit.Enabled,
		LogParameters: cfg.Audit.LogParameters,
		LogSQL:        cfg.Audit.LogSQL,
		RetentionDays: cfg.Audit.RetentionDays,
		Logger:        logger,
		CookieName:    cfg.SessionCookieName,
	})
	if cfg.Audit.Enabled {
		if err := auditRepo.EnsureSchema(ctx); err != nil {
			logger.Warn("audit schema", "error", err)
		} else {
			logger.Info("audit log ready",
				"retention_days", cfg.Audit.RetentionDays,
				"log_parameters", cfg.Audit.LogParameters,
				"log_sql", cfg.Audit.LogSQL,
			)
		}
	}

	auditRetention := audit.NewRetentionRunner(auditSvc, 24*time.Hour, logger)
	auditRetention.Start()
	defer auditRetention.Stop()

	jasperClient := gateway.Client()
	var schRepo *schedules.Repository
	var schRecipients *schedules.RecipientRepository
	var schExecutions *schedules.ExecutionRepository
	var schRunner *schedules.Runner
	var schSystemd *schedules.SystemdManager
	var schTmpls *schedules.EmailTemplateStore
	if !cfg.IsQCartaProfile() {
		schPaths := schedules.NewPaths(cfg.Paths.ScheduleDir, cfg.Paths.ScheduleOutputDir, cfg.Paths.EmailTmplDir, cfg.Paths.AppRoot)
		schRepo, schRecipients, schExecutions, schRunner, schSystemd, schTmpls = schedules.NewFromOptions(schedules.Options{
			Pool:        db.PGX(),
			Schema:      cfg.Database.Schema,
			Paths:       schPaths,
			Jasper:      schedules.DefaultRunnerConfig(cfg.Jasper.ReportURL, cfg.Jasper.Timeout),
			Mail:        mailCfg,
			Systemd:     schedules.DefaultSystemdOptions(cfg.Paths.AppRoot),
			AppRoot:     cfg.Paths.AppRoot,
			Reports:     reportStore,
			Datasources: dsRepo,
			EmbeddedMap: embedSvc,
		})
		schRunner.ResourcePaths = cfg.Paths.ResourcePathsDir
		schRunner.ResolveResourceBytes = func(ctx context.Context, resourceUUID string) ([]byte, error) {
			return resourceKeyLoader{svc: resourceSvc}.LoadPrivateKeyPEM(ctx, resourceUUID)
		}
	}
	health := jasperClient.CheckHealth(ctx)
	logger.Info("jasper gateway configured",
		"configured", health.Configured,
		"reachability", string(health.Reachability),
		"message", health.Message,
		"timeout", gateway.Timeout().String(),
	)

	embedRepo := embedorigins.NewRepository(db.PGX(), cfg.Database.Schema)
	if err := embedRepo.EnsureSchema(ctx); err != nil {
		logger.Warn("embed allowed origins schema", "error", err)
	}
	embedPolicy := embedorigins.New(embedRepo)
	if err := embedPolicy.Reload(ctx); err != nil {
		logger.Warn("embed allowed origins load", "error", err)
	}

	deps := server.Dependencies{
		DB:           db,
		Paths:        paths,
		Jasper:       jasperClient,
		Gateway:      gateway,
		ACL:          aclSvc,
		Auth:         authSvc,
		Reports:      reportStore,
		Map:          mapSvc,
		EmbeddedMap:  embedSvc,
		Datasources:  dsRepo,
		GeoServer:    gsRepo,
		DSConnector:  dsConnector,
		Folders:      foldersSvc,
		Resources:    resourceSvc,
		ACLWriter:    aclRepo,
		Admin:        adminRepo,
		Schedules:    schRepo,
		Recipients:   schRecipients,
		Executions:   schExecutions,
		ScheduleRun:  schRunner,
		ScheduleSys:  schSystemd,
		EmailTmpls:   schTmpls,
		Audit:        auditSvc,
		EmbedOrigins: embedPolicy,
	}
	stopProfessional := func() {}
	if !cfg.IsQCartaProfile() {
		stopProfessional = applyProfessional(
			ctx, cfg, logger, db.PGX(), cfg.Database.Schema,
			dsRepo, gsRepo, dsConnector, foldersSvc, reportStore, resourceSvc,
			aclSvc, authSvc, authRepo, auditSvc, gateway, embedSvc, &deps,
		)
	}
	defer stopProfessional()

	httpCfg := cfg
	httpCfg.Edition = string(edition.EditionCommunity)
	srv := server.New(httpCfg, deps, logger)
	srv.SetEdition(liveEdition)
	srv.SetLicenseManager(licMgr)
	if licMgr != nil {
		go licMgr.RunRefreshLoop(ctx)
	}
	designerLic := openDesignerLicense(cfg, logger)
	srv.SetDesignerLicense(designerLic)
	if designerLic != nil {
		go designerLic.RunRefreshLoop(ctx)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			logger.Error("http server failed", "error", err)
			return 1
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown error", "error", err)
		return 1
	}
	logger.Info("shutdown complete")
	return 0
}

// resourceKeyLoader resolves Snowflake private key PEM from content resources.
type resourceKeyLoader struct {
	svc *contentresource.Service
}

func (r resourceKeyLoader) LoadPrivateKeyPEM(ctx context.Context, resourceUUID string) ([]byte, error) {
	if r.svc == nil || !r.svc.Configured() {
		return nil, fmt.Errorf("content resources unavailable")
	}
	_, data, err := r.svc.ReadBytes(ctx, resourceUUID)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("private key resource is empty")
	}
	return data, nil
}

type authUserLookup struct {
	repo *auth.Repository
}

func (a authUserLookup) FindUserByID(ctx context.Context, id int64) (*auth.User, error) {
	if a.repo == nil {
		return nil, auth.ErrInvalidSession
	}
	return a.repo.FindUserByID(ctx, id)
}

type auditGovernanceAdapter struct {
	svc *audit.Service
}

func reportServiceKey(qcarta bool) string {
	if qcarta {
		return acl.ServiceKeyReportRuntime
	}
	return acl.ServiceKeyJasperReports
}

func (a auditGovernanceAdapter) Record(ctx context.Context, e audit.Event) {
	if a.svc == nil {
		return
	}
	a.svc.Record(ctx, e)
}
