// Package config loads typed settings from the same JRS_* environment variables
// used by the existing PHP application (see config.php and lib/auth.php).
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultListenAddr         = "127.0.0.1:9088"
	DefaultEnvironment        = "development"
	DefaultBasePath           = "/"
	DefaultDBHost             = "localhost"
	DefaultDBPort             = 5432
	DefaultDBName             = "jasper_reports"
	DefaultDBUser             = "postgres"
	DefaultDBSchema           = "public"
	DefaultDBSSLMode          = "disable"
	DefaultJasperReportURL    = "http://127.0.0.1:9099/report"
	DefaultJasperImageURL     = "http://127.0.0.1:9099/acugis_report_image"
	DefaultJasperTimeout      = 600 * time.Second
	DefaultSessionCookieName  = "jrs_session"
	DefaultDBConnectTimeout   = 5 * time.Second
	DefaultEdition            = "community"
	ProfileStandalone         = ""
	ProfileQCarta             = "qcarta"
	DefaultAuditRetentionDays = 90
	DefaultEmbedTokenTTL      = 15 * time.Minute
	ServiceName               = "acugis-report-server-go"
	ProductName               = "AcuGIS Report Server"
	PHPAppMount               = "/acugis-report-server"
)

// DefaultAppVersion is the fallback reported version when JRS_APP_VERSION is unset.
// Release packaging overwrites this via -ldflags at compile time.
var DefaultAppVersion = "1.0.0"

// Config is the typed runtime configuration for the Go development server.
type Config struct {
	Environment string
	ListenAddr  string
	BasePath    string
	AppVersion  string
	// Edition records ACUGIS_EDITION / JRS_EDITION for diagnostics only.
	// It does not grant Professional features. Entitlement comes from a
	// verified report-server-professional ACL1 token.
	Edition string
	// LicenseStateDir holds installation-id and license.lic.
	LicenseStateDir string
	// LicenseServerURL overrides the default https://license.acugis.com base.
	// Empty uses the production default. The client requires HTTPS.
	LicenseServerURL string
	// Profile selects a deployment cut (empty = standalone Community/Professional).
	// "qcarta" enables the QCarta runtime profile without changing edition products.
	Profile string

	Database     DatabaseConfig
	AuthDatabase DatabaseConfig
	Jasper       JasperConfig
	Paths        PathConfig

	// CentralLoginURL is the QCarta/Central login page. Empty keeps local /login.
	CentralLoginURL string
	// MySQLJDBCDriver overrides the MySQL Tomcat driver class (QCarta ships MariaDB Connector/J).
	MySQLJDBCDriver string
	// PATPrefixes are accepted Bearer prefixes. Empty defaults to jrs_ only.
	PATPrefixes []string

	// ReportEndpoint is the browser-facing execute URL used by the viewer iframe
	// (PHP config key report_endpoint / JRS_REPORT_ENDPOINT). Relative paths resolve
	// under /reports/{key}. Absolute or rooted paths (e.g. /acugis-report-server/…)
	// can point at PHP execute while Go owns the viewer shell.
	ReportEndpoint string

	SessionCookieName string
	AuthDisabled      bool

	// FolderAllowRecursiveDelete enables DELETE /api/v1/folders/{id}?recursive=1&confirm=1
	// for non-empty folders (JRS_FOLDER_ALLOW_RECURSIVE_DELETE).
	FolderAllowRecursiveDelete bool

	// EmbedAllowedOrigins is the startup environment allow-list
	// (ACUGIS_EMBED_ALLOWED_ORIGINS / JRS_EMBED_ALLOWED_ORIGINS).
	// Admin-configured origins live in embed_allowed_origin and are merged
	// at request time. This slice is not the full effective list.
	EmbedAllowedOrigins []string
	// EmbedTokenSecret signs short-lived emb_ tokens (required for embedding).
	EmbedTokenSecret string
	// EmbedTokenTTL is the default lifetime for minted embed tokens.
	EmbedTokenTTL time.Duration

	// Audit controls the append-only audit trail (JRS_AUDIT_* / AUDIT_*).
	Audit AuditConfig

	// MCP is the opt-in Professional MCP endpoint (JRS_MCP_ENABLED).
	// Disabled by default. An OSS build never mounts the endpoint.
	MCP MCPConfig

	// QGIS is the optional Docker/native QGIS Server reverse-proxy target.
	QGIS QGISConfig

	// Designer is the loopback Web Designer proxy. Empty Upstream leaves
	// Designer routes unavailable and does not affect the rest of Report Server.
	// Token is JRS_DESIGNER_INTERNAL_TOKEN. There is no production default.
	// Do not log Token or include it in RedactedSummary.
	Designer DesignerConfig
}

// DesignerConfig is the jrs-server → jrs-designer proxy.
// Upstream must be loopback. The token is a high-entropy shared secret.
type DesignerConfig struct {
	Upstream string
	Token    string
	Timeout  time.Duration
	MaxBody  int64
	// AssetsDir is the built Web Designer dist directory (npm run build:web).
	// jrs-server serves it at /designer-assets/. There is no Node process in production.
	// The installer should place the jrs-designer binary, this dist directory,
	// the Jasper helper, and the internal token beside the Report Server service.
	AssetsDir string
}

// MCPConfig enables the Professional MCP endpoint.
// Enabled is JRS_MCP_ENABLED. Unset, empty, and any value other than
// 1/true/yes/on leave the endpoint disabled.
// AllowedHosts is JRS_MCP_ALLOWED_HOSTS: comma-separated public hostnames
// accepted in addition to loopback when the MCP connection arrives on a
// loopback address. Empty keeps loopback Host values only.
type MCPConfig struct {
	Enabled      bool
	AllowedHosts []string
}

// AuditConfig controls audit logging, parameter/SQL capture, and retention.
type AuditConfig struct {
	Enabled       bool
	RetentionDays int
	LogParameters bool
	LogSQL        bool
}

// DatabaseConfig mirrors PHP config.php db + JRS_DB_* variables.
type DatabaseConfig struct {
	Host           string
	Port           int
	Name           string
	User           string
	Password       string
	Schema         string
	SSLMode        string
	ConnectTimeout time.Duration
	// URL, when set, is used as the DSN instead of the discrete fields.
	URL string
}

// QGISConfig is the optional QGIS Server companion (Docker profile) and MAP rewrite.
type QGISConfig struct {
	// Upstream is JRS_QGIS_UPSTREAM (e.g. http://qgis-server). Empty disables /qgis/ proxy.
	Upstream string
	// MapPrefix is JRS_QGIS_MAP_PREFIX (default /qgis/projects).
	MapPrefix string
}

// JasperConfig mirrors PHP jasper_internal_* URLs.
type JasperConfig struct {
	ReportURL string
	ImageURL  string
	Timeout   time.Duration
}

// PathConfig mirrors PHP filesystem locations under the install root.
type PathConfig struct {
	AppRoot                   string
	ReportJSONDir             string
	JRXMLDir                  string
	ThumbnailDir              string
	GeneratedReportDir        string
	ScheduleDir               string
	ScheduleOutputDir         string
	EmailTmplDir              string
	JasperContextPath         string
	GeoServerConnectionsPath  string
	QGISConnectionsPath       string
	ArcGISConnectionsPath     string
	JasperReportsContainerDir string // Tomcat-visible reports mount (P_MAP_IMAGE path translation)
	ReportsFileOwner          string // chown owner for JRXML/map cache (e.g. tomcat)
	ReportsFileGroup          string // chown group for JRXML/map cache
	ResourcesDir              string // UUID-based folder resources ({uuid}{ext}) — non-image types
	ResourceThumbnailsDir     string // UUID-based image resource thumbnails ({uuid}.png)
	ResourcePathsDir          string // Path-addressable Image Resources for Jasper (…/finance/images/x.png)
	DatasourceFilesDir        string // Designer catalog root for CSV, JSON, Excel, and XML uploads
}

// Load reads configuration from the process environment.
// Missing optional values fall back to the same defaults as the PHP application
// where those defaults are defined in config.php / lib/bootstrap.php.
func Load() (Config, error) {
	// Optional local .env (does not override already-exported variables).
	LoadDotEnvFiles("")

	appRoot, err := resolveAppRoot()
	if err != nil {
		return Config{}, err
	}
	// Re-scan after app root is known (PHP root /.env often holds shared JRS_*).
	LoadDotEnvFiles(appRoot)

	dbPort, err := envInt("JRS_DB_PORT", DefaultDBPort)
	if err != nil {
		return Config{}, fmt.Errorf("JRS_DB_PORT: %w", err)
	}

	connectTimeout, err := envDuration("JRS_DB_CONNECT_TIMEOUT", DefaultDBConnectTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("JRS_DB_CONNECT_TIMEOUT: %w", err)
	}

	jasperTimeout, err := envDuration("JRS_JASPER_TIMEOUT", DefaultJasperTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("JRS_JASPER_TIMEOUT: %w", err)
	}

	scheduleDir := envOr("JRS_JASPER_SCHEDULE_DIR", filepath.Join(appRoot, "schedules"))
	emailTmplDir := envOr("JRS_JASPER_EMAIL_TMPL_DIR", filepath.Join(appRoot, "email_tmpl"))
	jrxmlDir := resolveJRXMLDir(appRoot)
	contextPath := resolveJasperContextPath(appRoot)
	geoConnPath := envOr("JRS_GEOSERVER_CONNECTIONS_PATH", filepath.Join(appRoot, "docker", "report-server", "geoserver-connections.properties"))
	qgisConnPath := envOr("JRS_QGIS_CONNECTIONS_PATH", filepath.Join(appRoot, "docker", "report-server", "qgis-connections.properties"))
	arcgisConnPath := envOr("JRS_ARCGIS_CONNECTIONS_PATH", filepath.Join(appRoot, "docker", "report-server", "arcgis-connections.properties"))
	reportsContainerDir := resolveReportsContainerDir(jrxmlDir)

	cfg := Config{
		Environment:      envOr("JRS_ENVIRONMENT", DefaultEnvironment),
		ListenAddr:       envOr("JRS_LISTEN_ADDR", DefaultListenAddr),
		BasePath:         normalizeBasePath(envOr("JRS_BASE_PATH", DefaultBasePath)),
		AppVersion:       envOr("JRS_APP_VERSION", DefaultAppVersion),
		Edition:          resolveEditionEnv(),
		LicenseStateDir:  envOr("ACUGIS_LICENSE_STATE_DIR", "/var/lib/acugis-report-server"),
		LicenseServerURL: strings.TrimSpace(os.Getenv("ACUGIS_LICENSE_SERVER_URL")),
		Profile:          resolveProfileEnv(),
		AuthDatabase: DatabaseConfig{
			URL:            strings.TrimSpace(envOrFirst([]string{"JRS_AUTH_DATABASE_URL", "CENTRAL_DATABASE_URL"}, "")),
			ConnectTimeout: connectTimeout,
		},
		CentralLoginURL: strings.TrimSpace(envOrFirst([]string{"JRS_CENTRAL_LOGIN_URL", "CENTRAL_LOGIN_URL"}, "")),
		MySQLJDBCDriver: strings.TrimSpace(envOr("JRS_MYSQL_JDBC_DRIVER", "")),
		PATPrefixes:     parsePATPrefixes(os.Getenv("JRS_PAT_PREFIXES")),
		Database: DatabaseConfig{
			Host:           envOr("JRS_DB_HOST", DefaultDBHost),
			Port:           dbPort,
			Name:           envOr("JRS_DB_NAME", DefaultDBName),
			User:           envOr("JRS_DB_USER", DefaultDBUser),
			Password:       os.Getenv("JRS_DB_PASS"),
			Schema:         envOr("JRS_DB_SCHEMA", DefaultDBSchema),
			SSLMode:        envOr("JRS_DB_SSLMODE", DefaultDBSSLMode),
			ConnectTimeout: connectTimeout,
			URL:            strings.TrimSpace(envOrFirst([]string{"JRS_DATABASE_URL", "JRS_DB_URL"}, "")),
		},
		Jasper: JasperConfig{
			ReportURL: strings.TrimRight(envOr("JRS_JASPER_INTERNAL_REPORT_URL", DefaultJasperReportURL), "/"),
			ImageURL:  strings.TrimRight(envOr("JRS_JASPER_INTERNAL_IMAGE_URL", DefaultJasperImageURL), "/"),
			Timeout:   jasperTimeout,
		},
		Paths: PathConfig{
			AppRoot:                   appRoot,
			ReportJSONDir:             filepath.Join(appRoot, "reports", "json"),
			JRXMLDir:                  jrxmlDir,
			ThumbnailDir:              filepath.Join(appRoot, "assets", "reports"),
			GeneratedReportDir:        filepath.Join(scheduleDir, "output"),
			ScheduleDir:               scheduleDir,
			ScheduleOutputDir:         filepath.Join(scheduleDir, "output"),
			EmailTmplDir:              emailTmplDir,
			JasperContextPath:         contextPath,
			GeoServerConnectionsPath:  geoConnPath,
			QGISConnectionsPath:       qgisConnPath,
			ArcGISConnectionsPath:     arcgisConnPath,
			JasperReportsContainerDir: reportsContainerDir,
			ReportsFileOwner:          envOr("JRS_REPORTS_OWNER", "tomcat"),
			ReportsFileGroup:          envOr("JRS_REPORTS_GROUP", "tomcat"),
			ResourcesDir:              envOr("JRS_RESOURCES_DIR", filepath.Join(appRoot, "resources")),
			ResourceThumbnailsDir:     envOr("JRS_RESOURCE_THUMBNAILS_DIR", filepath.Join(appRoot, "thumbnails")),
			// Sibling of the Tomcat reports dir so Jasper can resolve /finance/images/….
			ResourcePathsDir:   envOr("JRS_RESOURCE_PATHS_DIR", filepath.Join(filepath.Dir(jrxmlDir), "resource-paths")),
			DatasourceFilesDir: resolveDatasourceFilesDir(),
		},
		SessionCookieName:          envOr("JRS_SESSION_COOKIE", DefaultSessionCookieName),
		AuthDisabled:               envBool("JRS_AUTH_DISABLED"),
		FolderAllowRecursiveDelete: envBool("JRS_FOLDER_ALLOW_RECURSIVE_DELETE"),
		ReportEndpoint:             strings.TrimSpace(envOr("JRS_REPORT_ENDPOINT", "execute_report.php")),
		EmbedAllowedOrigins: parseEmbedAllowedOrigins(envOrFirst(
			[]string{"ACUGIS_EMBED_ALLOWED_ORIGINS", "JRS_EMBED_ALLOWED_ORIGINS"},
			"",
		)),
		EmbedTokenSecret: strings.TrimSpace(envOrFirst(
			[]string{"ACUGIS_EMBED_TOKEN_SECRET", "JRS_EMBED_TOKEN_SECRET"},
			"",
		)),
		EmbedTokenTTL: DefaultEmbedTokenTTL,
		Audit:         loadAuditConfig(),
		MCP: MCPConfig{
			Enabled:      envBool("JRS_MCP_ENABLED"),
			AllowedHosts: parseMCPAllowedHosts(os.Getenv("JRS_MCP_ALLOWED_HOSTS")),
		},
		QGIS: QGISConfig{
			Upstream:  strings.TrimRight(strings.TrimSpace(os.Getenv("JRS_QGIS_UPSTREAM")), "/"),
			MapPrefix: envOr("JRS_QGIS_MAP_PREFIX", "/qgis/projects"),
		},
	}

	designerTimeout, err := envDuration("JRS_DESIGNER_PROXY_TIMEOUT", 120*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("JRS_DESIGNER_PROXY_TIMEOUT: %w", err)
	}
	designerMaxBody, err := envByteSize("JRS_DESIGNER_PROXY_MAX_BODY", 64<<20)
	if err != nil {
		return Config{}, fmt.Errorf("JRS_DESIGNER_PROXY_MAX_BODY: %w", err)
	}
	cfg.Designer = DesignerConfig{
		Upstream:  strings.TrimRight(strings.TrimSpace(os.Getenv("JRS_DESIGNER_UPSTREAM")), "/"),
		Token:     strings.TrimSpace(os.Getenv("JRS_DESIGNER_INTERNAL_TOKEN")),
		Timeout:   designerTimeout,
		MaxBody:   designerMaxBody,
		AssetsDir: envOr("JRS_DESIGNER_ASSETS_DIR", filepath.Join(appRoot, "designer-web")),
	}

	if ttl, err := envDurationFirst([]string{"ACUGIS_EMBED_TOKEN_TTL", "JRS_EMBED_TOKEN_TTL"}, DefaultEmbedTokenTTL); err != nil {
		return Config{}, fmt.Errorf("ACUGIS_EMBED_TOKEN_TTL: %w", err)
	} else {
		cfg.EmbedTokenTTL = ttl
	}

	// Development convenience: allow common localhost origins when unset.
	if len(cfg.EmbedAllowedOrigins) == 0 && strings.EqualFold(cfg.Environment, "development") {
		cfg.EmbedAllowedOrigins = []string{
			"http://localhost:3000",
			"http://localhost:5173",
			"http://127.0.0.1:3000",
			"http://127.0.0.1:5173",
			"http://localhost:8080",
			"http://127.0.0.1:8080",
		}
	}

	// Development fallback secret — production must set ACUGIS_EMBED_TOKEN_SECRET.
	if cfg.EmbedTokenSecret == "" && (strings.EqualFold(cfg.Environment, "development") || strings.EqualFold(cfg.Environment, "test")) {
		cfg.EmbedTokenSecret = "dev-only-embed-token-secret-change-me"
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// EmbedOriginAllowed reports whether origin may frame embed routes / call embed APIs.
func (c Config) EmbedOriginAllowed(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	for _, allowed := range c.EmbedAllowedOrigins {
		if strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

// FrameAncestorsCSP returns a Content-Security-Policy frame-ancestors value.
func (c Config) FrameAncestorsCSP() string {
	if len(c.EmbedAllowedOrigins) == 0 {
		return "'none'"
	}
	return strings.Join(c.EmbedAllowedOrigins, " ")
}

func parseMCPAllowedHosts(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key := strings.ToLower(p)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

func parseEmbedAllowedOrigins(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key := strings.ToLower(p)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

func envOrFirst(keys []string, fallback string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return fallback
}

func envDurationFirst(keys []string, fallback time.Duration) (time.Duration, error) {
	for _, key := range keys {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		return ParseDurationValue(raw, fallback)
	}
	return fallback, nil
}

// Validate returns clear errors for missing or invalid required values.
func (c Config) Validate() error {
	var missing []string
	if strings.TrimSpace(c.ListenAddr) == "" {
		missing = append(missing, "JRS_LISTEN_ADDR")
	}
	if strings.TrimSpace(c.Database.Host) == "" {
		missing = append(missing, "JRS_DB_HOST")
	}
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		return fmt.Errorf("invalid JRS_DB_PORT: %d", c.Database.Port)
	}
	if strings.TrimSpace(c.Database.Name) == "" {
		missing = append(missing, "JRS_DB_NAME")
	}
	if strings.TrimSpace(c.Database.User) == "" {
		missing = append(missing, "JRS_DB_USER")
	}
	if strings.TrimSpace(c.Paths.AppRoot) == "" {
		missing = append(missing, "JRS_JASPER_HOME (or sibling PHP install root)")
	}
	if strings.TrimSpace(c.Jasper.ReportURL) == "" {
		missing = append(missing, "JRS_JASPER_INTERNAL_REPORT_URL")
	}
	if strings.TrimSpace(c.Jasper.ImageURL) == "" {
		missing = append(missing, "JRS_JASPER_INTERNAL_IMAGE_URL")
	}
	if c.Database.ConnectTimeout <= 0 {
		return fmt.Errorf("JRS_DB_CONNECT_TIMEOUT must be positive")
	}
	if c.Jasper.Timeout <= 0 {
		return fmt.Errorf("JRS_JASPER_TIMEOUT must be positive")
	}
	if err := c.validateDesigner(); err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}
	return nil
}

// DSN builds a PostgreSQL connection string without embedding it in logs.
// Callers must not log the return value.
func (d DatabaseConfig) DSN() string {
	if u := strings.TrimSpace(d.URL); u != "" {
		return u
	}
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s connect_timeout=%d",
		d.Host,
		d.Port,
		d.Name,
		d.User,
		d.Password,
		d.SSLMode,
		int(d.ConnectTimeout.Seconds()),
	)
}

// HasOverride reports whether an explicit DSN or database name was supplied.
func (d DatabaseConfig) HasOverride() bool {
	return strings.TrimSpace(d.URL) != "" || strings.TrimSpace(d.Name) != ""
}

// IsQCartaProfile reports whether this process is the QCarta runtime cut.
func (c Config) IsQCartaProfile() bool {
	return strings.EqualFold(strings.TrimSpace(c.Profile), ProfileQCarta)
}

func resolveProfileEnv() string {
	raw := strings.ToLower(strings.TrimSpace(envOrFirst([]string{"JRS_PROFILE", "JRS_DEPLOYMENT_PROFILE"}, "")))
	switch raw {
	case ProfileQCarta:
		return ProfileQCarta
	default:
		return ProfileStandalone
	}
}

func parsePATPrefixes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// resolveEditionEnv records ACUGIS_EDITION, then JRS_EDITION, for diagnostics.
// The result is not Professional entitlement.
func resolveEditionEnv() string {
	raw := strings.TrimSpace(os.Getenv("ACUGIS_EDITION"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("JRS_EDITION"))
	}
	if raw == "" {
		return DefaultEdition
	}
	switch strings.ToLower(raw) {
	case "professional", "pro", "commercial":
		return "professional"
	case "community", "ce", "oss":
		return "community"
	default:
		return DefaultEdition
	}
}

// RedactedSummary returns non-secret fields suitable for diagnostics.
func (c Config) RedactedSummary() map[string]any {
	return map[string]any{
		"environment":                  c.Environment,
		"listen_addr":                  c.ListenAddr,
		"base_path":                    c.BasePath,
		"app_version":                  c.AppVersion,
		"edition":                      c.Edition,
		"db_host":                      c.Database.Host,
		"db_port":                      c.Database.Port,
		"db_name":                      c.Database.Name,
		"db_user":                      c.Database.User,
		"db_schema":                    c.Database.Schema,
		"db_sslmode":                   c.Database.SSLMode,
		"jasper_report_url":            c.Jasper.ReportURL,
		"jasper_image_url":             c.Jasper.ImageURL,
		"jasper_timeout":               c.Jasper.Timeout.String(),
		"app_root":                     c.Paths.AppRoot,
		"report_json_dir":              c.Paths.ReportJSONDir,
		"jrxml_dir":                    c.Paths.JRXMLDir,
		"thumbnail_dir":                c.Paths.ThumbnailDir,
		"schedule_dir":                 c.Paths.ScheduleDir,
		"schedule_output_dir":          c.Paths.ScheduleOutputDir,
		"email_tmpl_dir":               c.Paths.EmailTmplDir,
		"jasper_context_path":          c.Paths.JasperContextPath,
		"geoserver_connections_path":   c.Paths.GeoServerConnectionsPath,
		"qgis_connections_path":        c.Paths.QGISConnectionsPath,
		"qgis_upstream":                c.QGIS.Upstream,
		"qgis_map_prefix":              c.QGIS.MapPrefix,
		"arcgis_connections_path":      c.Paths.ArcGISConnectionsPath,
		"jasper_reports_container_dir": c.Paths.JasperReportsContainerDir,
		"reports_file_owner":           c.Paths.ReportsFileOwner,
		"reports_file_group":           c.Paths.ReportsFileGroup,
		"resources_dir":                c.Paths.ResourcesDir,
		"resource_thumbnails_dir":      c.Paths.ResourceThumbnailsDir,
		"resource_paths_dir":           c.Paths.ResourcePathsDir,
		"session_cookie_name":          c.SessionCookieName,
		"profile":                      c.Profile,
		"auth_disabled":                c.AuthDisabled,
		"audit_enabled":                c.Audit.Enabled,
		"audit_retention_days":         c.Audit.RetentionDays,
		"audit_log_parameters":         c.Audit.LogParameters,
		"audit_log_sql":                c.Audit.LogSQL,
		"mcp_enabled":                  c.MCP.Enabled,
		"mcp_allowed_hosts":            c.MCP.AllowedHosts,
		"designer_configured":          strings.TrimSpace(c.Designer.Upstream) != "" && strings.TrimSpace(c.Designer.Token) != "",
		"designer_proxy_timeout":       c.Designer.Timeout.String(),
	}
}

func (c Config) validateDesigner() error {
	up := strings.TrimSpace(c.Designer.Upstream)
	tok := strings.TrimSpace(c.Designer.Token)
	if up == "" && tok == "" {
		return nil
	}
	if up == "" || tok == "" {
		return fmt.Errorf("JRS_DESIGNER_UPSTREAM and JRS_DESIGNER_INTERNAL_TOKEN must both be set")
	}
	if _, err := ParseLoopbackURL(up); err != nil {
		return fmt.Errorf("JRS_DESIGNER_UPSTREAM must be a loopback http URL")
	}
	if c.Designer.Timeout <= 0 {
		return fmt.Errorf("JRS_DESIGNER_PROXY_TIMEOUT must be positive")
	}
	if c.Designer.MaxBody <= 0 {
		return fmt.Errorf("JRS_DESIGNER_PROXY_MAX_BODY must be positive")
	}
	return nil
}

// ParseLoopbackURL accepts only http(s) URLs on 127.0.0.1, localhost, or ::1
// with no userinfo and no path. The browser cannot choose this URL.
func ParseLoopbackURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("designer upstream must be a loopback http URL")
	}
	host := strings.Trim(strings.ToLower(u.Hostname()), "[]")
	switch host {
	case "127.0.0.1", "localhost", "::1":
	default:
		return nil, fmt.Errorf("designer upstream must be a loopback http URL")
	}
	if u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("designer upstream must not include a path")
	}
	return u, nil
}

func envByteSize(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	s := strings.ToUpper(raw)
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "MB"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		mult = 1 << 10
		s = strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "GB"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "GB")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid size %q", raw)
	}
	return n * mult, nil
}

// loadAuditConfig reads JRS_AUDIT_* with AUDIT_* aliases (product-facing names).
func loadAuditConfig() AuditConfig {
	retention, err := envIntFirst([]string{"JRS_AUDIT_RETENTION_DAYS", "AUDIT_RETENTION_DAYS"}, DefaultAuditRetentionDays)
	if err != nil || retention <= 0 {
		retention = DefaultAuditRetentionDays
	}
	return AuditConfig{
		Enabled:       envBoolDefaultFirst([]string{"JRS_AUDIT_ENABLED", "AUDIT_ENABLED"}, true),
		RetentionDays: retention,
		LogParameters: envBoolDefaultFirst([]string{"JRS_AUDIT_LOG_PARAMETERS", "AUDIT_LOG_PARAMETERS"}, true),
		LogSQL:        envBoolDefaultFirst([]string{"JRS_AUDIT_LOG_SQL", "AUDIT_LOG_SQL"}, false),
	}
}

func envBoolDefaultFirst(keys []string, def bool) bool {
	for _, key := range keys {
		raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if raw == "" {
			continue
		}
		switch raw {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

func envIntFirst(keys []string, fallback int) (int, error) {
	for _, key := range keys {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("%s: invalid integer %q", key, raw)
		}
		return n, nil
	}
	return fallback, nil
}

func resolveDatasourceFilesDir() string {
	if v := strings.TrimSpace(os.Getenv("JRS_DATASOURCE_FILES_DIR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("WEB_DESIGNER_DATA_DIR")); v != "" {
		return v
	}
	return "/var/lib/acugis-report-designer-web"
}

func resolveAppRoot() (string, error) {
	if home := strings.TrimSpace(os.Getenv("JRS_JASPER_HOME")); home != "" {
		return filepath.Clean(home), nil
	}
	// Development convenience: go-report-server sits beside the PHP install root.
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve app root: %w", err)
	}
	candidate := filepath.Clean(wd)
	for i := 0; i < 4; i++ {
		if looksLikePHPRoot(candidate) {
			return candidate, nil
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		// Prefer the parent of go-report-server when running from inside it.
		if filepath.Base(candidate) == "go-report-server" {
			parentCandidate := parent
			if looksLikePHPRoot(parentCandidate) {
				return parentCandidate, nil
			}
		}
		candidate = parent
	}
	if looksLikePHPRoot(filepath.Dir(wd)) {
		return filepath.Clean(filepath.Dir(wd)), nil
	}
	return "", fmt.Errorf("could not locate PHP install root; set JRS_JASPER_HOME to the AcuGIS Report Server directory")
}

func looksLikePHPRoot(dir string) bool {
	_, errConfig := os.Stat(filepath.Join(dir, "config.php"))
	_, errReports := os.Stat(filepath.Join(dir, "reports"))
	return errConfig == nil && errReports == nil
}

func normalizeBasePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// resolveJRXMLDir picks the Tomcat reports directory used for JRXML/.jasper uploads and map reads.
// Order: JRS_JRXML_DIR → existing $CATALINA_BASE|HOME/acugis/reports → docker/report-server/reports.
func resolveJRXMLDir(appRoot string) string {
	if v := strings.TrimSpace(os.Getenv("JRS_JRXML_DIR")); v != "" {
		return filepath.Clean(v)
	}
	if catalina := catalinaAcugisReportsDir(); catalina != "" {
		return catalina
	}
	return filepath.Join(appRoot, "docker", "report-server", "reports")
}

// resolveJasperContextPath picks where datasource admin writes Tomcat JNDI resources.
// Order: JRS_JASPER_CONTEXT_PATH → existing $CATALINA_*/conf/context.xml → docker/report-server/context.xml.
func resolveJasperContextPath(appRoot string) string {
	if v := strings.TrimSpace(os.Getenv("JRS_JASPER_CONTEXT_PATH")); v != "" {
		return filepath.Clean(v)
	}
	if catalina := catalinaContextXMLPath(); catalina != "" {
		return catalina
	}
	return filepath.Join(appRoot, "docker", "report-server", "context.xml")
}

// resolveReportsContainerDir is the path Tomcat uses inside P_MAP_IMAGE.
// Default: same absolute path as JRXMLDir (standalone / shared filesystem).
// Docker bind-mounts that differ host↔container must set JRS_JASPER_REPORTS_CONTAINER_DIR.
func resolveReportsContainerDir(jrxmlDir string) string {
	if v := strings.TrimSpace(os.Getenv("JRS_JASPER_REPORTS_CONTAINER_DIR")); v != "" {
		return filepath.ToSlash(filepath.Clean(v))
	}
	abs, err := filepath.Abs(jrxmlDir)
	if err != nil {
		return filepath.ToSlash(jrxmlDir)
	}
	return filepath.ToSlash(abs)
}

func catalinaBaseCandidates() []string {
	var out []string
	for _, key := range []string{"CATALINA_BASE", "CATALINA_HOME"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			out = append(out, filepath.Clean(v))
		}
	}
	return out
}

func catalinaAcugisReportsDir() string {
	for _, base := range catalinaBaseCandidates() {
		dir := filepath.Join(base, "acugis", "reports")
		if dirExists(dir) {
			return dir
		}
	}
	return ""
}

func catalinaContextXMLPath() string {
	for _, base := range catalinaBaseCandidates() {
		path := filepath.Join(base, "conf", "context.xml")
		// Prefer an existing file; otherwise use the path if conf/ exists (writable target).
		if fileExists(path) || dirExists(filepath.Join(base, "conf")) {
			return path
		}
	}
	return ""
}

func isCatalinaAcugisReports(dir string) bool {
	clean := filepath.Clean(dir)
	for _, base := range catalinaBaseCandidates() {
		if strings.EqualFold(clean, filepath.Join(base, "acugis", "reports")) {
			return true
		}
	}
	// Also treat .../acugis/reports as standalone even when CATALINA_* unset.
	return strings.HasSuffix(strings.ToLower(filepath.ToSlash(clean)), "/acugis/reports")
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", raw)
	}
	return n, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	return ParseDurationValue(os.Getenv(key), fallback)
}

// ParseBoolEnv matches PHP jrs_truthy_env semantics (1/true/yes/on).
func ParseBoolEnv(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// ParseDurationValue accepts Go durations (5s, 10m) or plain integer seconds.
func ParseDurationValue(raw string, fallback time.Duration) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d, nil
	}
	sec, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (use Go duration or seconds)", raw)
	}
	if sec <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return time.Duration(sec) * time.Second, nil
}
