package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/admin"
	"github.com/acugis/acugis-report-server-go/internal/arcgis"
	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auditexport"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/contentresource"
	"github.com/acugis/acugis-report-server-go/internal/database"
	"github.com/acugis/acugis-report-server-go/internal/datasetintel"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/embeddedmap"
	"github.com/acugis/acugis-report-server-go/internal/embedorigins"
	"github.com/acugis/acugis-report-server-go/internal/explorer"
	"github.com/acugis/acugis-report-server-go/internal/filesystem"
	"github.com/acugis/acugis-report-server-go/internal/folders"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/governance"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/license"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
	"github.com/acugis/acugis-report-server-go/internal/oidcauth"
	"github.com/acugis/acugis-report-server-go/internal/qgis"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/internal/samlauth"
	"github.com/acugis/acugis-report-server-go/internal/schedules"
	"github.com/acugis/acugis-report-server-go/internal/usage"
)

// Dependencies are optional collaborators used by readiness, diagnostics, and gateway proxies.
type Dependencies struct {
	DB           *database.Pool
	Paths        *filesystem.Paths
	Jasper       *jasper.Client
	Gateway      jasper.Gateway
	ACL          acl.Service
	Auth         *auth.Service
	Reports      *reports.Store
	Map          *mapreport.Service
	EmbeddedMap  *embeddedmap.Service
	Datasources  datasources.Finder
	GeoServer    geoserver.Store
	QGIS         qgis.Store
	ArcGIS       arcgis.Store
	DSConnector  datasources.Connector
	Explorer     *explorer.Service
	DatasetIntel *datasetintel.Service
	Folders      *folders.Service
	Resources    *contentresource.Service
	ACLWriter    acl.ResourceWriter
	Admin        *admin.Repository
	Schedules    *schedules.Repository
	Recipients   *schedules.RecipientRepository
	Executions   *schedules.ExecutionRepository
	ScheduleRun  *schedules.Runner
	ScheduleSys  *schedules.SystemdManager
	EmailTmpls   *schedules.EmailTemplateStore
	Audit        *audit.Service
	AuditExport  *auditexport.Service
	Usage        *usage.Service
	Governance   *governance.Service
	OIDC         *oidcauth.Service
	SAML         *samlauth.Service
	EmbedOrigins *embedorigins.Policy
}

// Server is the development HTTP server.
type Server struct {
	cfg             config.Config
	deps            Dependencies
	edition         *edition.Service
	license         *license.Manager
	designerLicense *license.Manager
	logger          *slog.Logger
	http            *http.Server
	// Test hooks for Designer create/Save As. Production leaves them nil.
	testDesignerFolder func(folderID int64) error
	testPlaceReport    func(folderID int64, key, name string, createdBy int64) error
	// Test hooks for resource-base execute and move. Production leaves them nil.
	testReportFolder func(reportKey string) (folderID int64, found bool)
	testSlashFolder  func(folderID int64) (string, error)

	// mcpOnce builds the Professional MCP handler on first use.
	// The handler stays nil on OSS builds, which never call it.
	mcpOnce    sync.Once
	mcpHandler http.Handler
}

// New constructs an HTTP server that is not yet listening.
func New(cfg config.Config, deps Dependencies, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		cfg:     cfg,
		deps:    deps,
		edition: edition.New(edition.Normalize(cfg.Edition)),
		logger:  logger,
	}
	handler := s.routes()

	// WriteTimeout must cover long-running Jasper exports (default 600s).
	writeTimeout := cfg.Jasper.Timeout + 30*time.Second
	if writeTimeout < 90*time.Second {
		writeTimeout = 90 * time.Second
	}

	s.http = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}
	return s
}

// SetEdition replaces the entitlement service. Production binds the ACL1 resolver.
func (s *Server) SetEdition(ed *edition.Service) {
	if s == nil || ed == nil {
		return
	}
	s.edition = ed
}

// SetLicenseManager attaches the process license manager used by admin activation.
func (s *Server) SetLicenseManager(m *license.Manager) {
	if s == nil {
		return
	}
	s.license = m
}

// SetDesignerLicense attaches the Report Designer Web entitlement.
// It does not change Report Server Professional.
func (s *Server) SetDesignerLicense(m *license.Manager) {
	if s == nil {
		return
	}
	s.designerLicense = m
}

// Start begins serving until the context is cancelled or ListenAndServe fails.
func (s *Server) Start() error {
	s.logger.Info("starting http server",
		"addr", s.cfg.ListenAddr,
		"base_path", s.cfg.BasePath,
		"environment", s.cfg.Environment,
		"edition", s.edition.CurrentEdition(),
		"app_version", s.cfg.AppVersion,
	)
	err := s.http.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down http server")
	return s.http.Shutdown(ctx)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(payload); err != nil {
		s.logger.Error("write json response", "error", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": config.ServiceName,
	})
}

type readyCheck struct {
	Postgres   checkResult              `json:"postgres"`
	Filesystem filesystemReady          `json:"filesystem"`
	Jasper     jasper.HealthCheckResult `json:"jasper"`
}

type checkResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

type filesystemReady struct {
	OK       bool                    `json:"ok"`
	Required []filesystem.PathStatus `json:"required_paths"`
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result := readyCheck{}
	ready := true

	if s.deps.DB == nil {
		result.Postgres = checkResult{OK: false, Message: "postgres pool not initialized"}
		ready = false
	} else if err := s.deps.DB.Ping(ctx); err != nil {
		result.Postgres = checkResult{OK: false, Message: "unreachable"}
		ready = false
		s.logger.Warn("ready: postgres ping failed", "error", err)
	} else {
		result.Postgres = checkResult{OK: true, Message: "ok"}
	}

	var required []filesystem.PathStatus
	fsOK := true
	if s.deps.Paths == nil {
		fsOK = false
		required = nil
	} else {
		for _, st := range s.deps.Paths.ValidateRoots() {
			if st.Required {
				required = append(required, st)
				if !st.Exists || st.Error != "" {
					fsOK = false
				}
			}
		}
	}
	result.Filesystem = filesystemReady{OK: fsOK, Required: required}
	if !fsOK {
		ready = false
	}

	if s.deps.Jasper == nil {
		result.Jasper = jasper.HealthCheckResult{
			Configured:   false,
			Reachability: jasper.ReachabilityUnknown,
			Message:      "jasper client not initialized",
		}
	} else {
		result.Jasper = s.deps.Jasper.CheckHealth(ctx)
	}

	status := http.StatusOK
	body := map[string]any{
		"status":  "ready",
		"service": config.ServiceName,
		"checks":  result,
	}
	if !ready {
		status = http.StatusServiceUnavailable
		body["status"] = "not_ready"
	}
	s.writeJSON(w, status, body)
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	pgOK := false
	tables := map[string]bool{}
	if s.deps.DB != nil {
		if err := s.deps.DB.Ping(ctx); err == nil {
			pgOK = true
			if presence, err := s.deps.DB.TablePresence(ctx); err == nil {
				tables = presence
			} else {
				s.logger.Warn("diagnostics: table presence failed", "error", err)
			}
		}
	}

	fsPresent := false
	if s.deps.Paths != nil {
		fsPresent = s.deps.Paths.AllRequiredPresent()
	}

	jasperConfigured := false
	jasperReachable := "unknown"
	if s.deps.Jasper != nil {
		h := s.deps.Jasper.CheckHealth(ctx)
		jasperConfigured = h.Configured
		switch h.Reachability {
		case jasper.ReachabilityReachable:
			jasperReachable = "true"
		case jasper.ReachabilityUnreachable:
			jasperReachable = "false"
		case jasper.ReachabilityNotChecked:
			jasperReachable = "unknown"
		default:
			jasperReachable = "unknown"
		}
	}

	authRepoConfigured := s.deps.Auth != nil && s.deps.Auth.RepositoryConfigured()
	aclRepoConfigured := s.deps.ACL != nil && s.deps.ACL.Configured()
	authDisabled := s.cfg.AuthDisabled
	if s.deps.Auth != nil {
		authDisabled = s.deps.Auth.AuthDisabled()
	}

	payload := map[string]any{
		"service":                              config.ServiceName,
		"product":                              config.ProductName,
		"go_version":                           runtime.Version(),
		"app_version":                          s.cfg.AppVersion,
		"edition":                              string(s.edition.CurrentEdition()),
		"edition_display":                      s.edition.DisplayName(),
		"capabilities":                         s.edition.Capabilities(),
		"environment":                          s.cfg.Environment,
		"base_path":                            s.cfg.BasePath,
		"listen_addr":                          s.cfg.ListenAddr,
		"postgres_reachable":                   pgOK,
		"filesystem_paths_present":             fsPresent,
		"jasper_endpoint_configured":           jasperConfigured,
		"jasper_reachable":                     jasperReachable,
		"expected_tables_present":              tables,
		"authentication_repository_configured": authRepoConfigured,
		"acl_repository_configured":            aclRepoConfigured,
		"auth_disabled":                        authDisabled,
		"timestamp":                            time.Now().UTC().Format(time.RFC3339),
		// Non-sensitive config summary only.
		"config": s.cfg.RedactedSummary(),
	}
	s.writeJSON(w, http.StatusOK, payload)
}

// Handler exposes the chi router for tests.
func (s *Server) Handler() http.Handler {
	return s.http.Handler
}

// MountPath returns the effective URL prefix for public routes.
func (s *Server) MountPath() string {
	if s.cfg.BasePath == "/" {
		return ""
	}
	return s.cfg.BasePath
}
