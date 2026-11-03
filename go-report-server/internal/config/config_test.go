package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseBoolEnv(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"1":     true,
		"true":  true,
		"TRUE":  true,
		"yes":   true,
		"on":    true,
		"0":     false,
		"false": false,
		"":      false,
		"no":    false,
	}
	for raw, want := range cases {
		if got := ParseBoolEnv(raw); got != want {
			t.Errorf("ParseBoolEnv(%q)=%v want %v", raw, got, want)
		}
	}
}

func TestParseDurationValue(t *testing.T) {
	t.Parallel()
	fallback := 5 * time.Second

	d, err := ParseDurationValue("", fallback)
	if err != nil || d != fallback {
		t.Fatalf("empty: got %v %v", d, err)
	}
	d, err = ParseDurationValue("10s", fallback)
	if err != nil || d != 10*time.Second {
		t.Fatalf("10s: got %v %v", d, err)
	}
	d, err = ParseDurationValue("30", fallback)
	if err != nil || d != 30*time.Second {
		t.Fatalf("30: got %v %v", d, err)
	}
	if _, err = ParseDurationValue("nope", fallback); err == nil {
		t.Fatal("expected error for invalid duration")
	}
	if _, err = ParseDurationValue("-1", fallback); err == nil {
		t.Fatal("expected error for negative duration")
	}
}

func TestNormalizeBasePath(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                       "/",
		"/":                      "/",
		"acugis-report-server":   "/acugis-report-server",
		"/acugis-report-server/": "/acugis-report-server",
		"/acugis-report-server":  "/acugis-report-server",
	}
	for in, want := range cases {
		if got := normalizeBasePath(in); got != want {
			t.Errorf("normalizeBasePath(%q)=%q want %q", in, got, want)
		}
	}
}

func TestValidateRequiresDBFields(t *testing.T) {
	t.Parallel()
	cfg := Config{
		ListenAddr: "127.0.0.1:9088",
		Database: DatabaseConfig{
			Host:           "localhost",
			Port:           5432,
			Name:           "jasper_reports",
			User:           "postgres",
			ConnectTimeout: 5 * time.Second,
		},
		Jasper: JasperConfig{
			ReportURL: "http://127.0.0.1:9099/report",
			ImageURL:  "http://127.0.0.1:9099/acugis_report_image",
			Timeout:   time.Minute,
		},
		Paths: PathConfig{AppRoot: "/tmp"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
	cfg.Database.Name = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing JRS_DB_NAME")
	}
}

func TestDatabaseDSNDoesNotLogPasswordInSummary(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Environment: "development",
		ListenAddr:  "127.0.0.1:9088",
		BasePath:    "/",
		Database: DatabaseConfig{
			Host:     "db.example",
			Port:     5432,
			Name:     "jasper_reports",
			User:     "postgres",
			Password: "super-secret-password",
			SSLMode:  "disable",
		},
		Jasper: JasperConfig{
			ReportURL: "http://127.0.0.1:9099/report",
			ImageURL:  "http://127.0.0.1:9099/acugis_report_image",
		},
		Paths: PathConfig{AppRoot: "/opt/acugis-report-server"},
	}
	summary := cfg.RedactedSummary()
	for _, v := range summary {
		if s, ok := v.(string); ok && s == "super-secret-password" {
			t.Fatal("password leaked into redacted summary")
		}
	}
	dsn := cfg.Database.DSN()
	if dsn == "" || !contains(dsn, "super-secret-password") {
		t.Fatal("DSN should contain password for connection use only")
	}
}

func TestLoadUsesEnvAndPHPRoot(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "config.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("JRS_JASPER_HOME", tmp)
	t.Setenv("JRS_DB_HOST", "pghost")
	t.Setenv("JRS_DB_PORT", "5433")
	t.Setenv("JRS_DB_NAME", "testdb")
	t.Setenv("JRS_DB_USER", "testuser")
	t.Setenv("JRS_DB_PASS", "testpass")
	t.Setenv("JRS_AUTH_DISABLED", "1")
	t.Setenv("JRS_MCP_ENABLED", "")
	t.Setenv("JRS_LISTEN_ADDR", "127.0.0.1:9090")
	t.Setenv("JRS_BASE_PATH", "/acugis-report-server")
	t.Setenv("JRS_ENVIRONMENT", "test")
	t.Setenv("JRS_JRXML_DIR", "")
	t.Setenv("JRS_JASPER_CONTEXT_PATH", "")
	t.Setenv("JRS_JASPER_REPORTS_CONTAINER_DIR", "")
	t.Setenv("CATALINA_BASE", "")
	t.Setenv("CATALINA_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.Host != "pghost" || cfg.Database.Port != 5433 {
		t.Fatalf("db config: %+v", cfg.Database)
	}
	if cfg.Paths.ReportJSONDir != filepath.Join(tmp, "reports", "json") {
		t.Fatalf("json dir: %s", cfg.Paths.ReportJSONDir)
	}
	if cfg.Paths.JRXMLDir != filepath.Join(tmp, "docker", "report-server", "reports") {
		t.Fatalf("jrxml dir: %s", cfg.Paths.JRXMLDir)
	}
	if cfg.Paths.JasperContextPath != filepath.Join(tmp, "docker", "report-server", "context.xml") {
		t.Fatalf("context path: %s", cfg.Paths.JasperContextPath)
	}
	if !cfg.AuthDisabled {
		t.Fatal("expected AuthDisabled")
	}
	if cfg.MCP.Enabled {
		t.Fatal("MCP must stay disabled when JRS_MCP_ENABLED is empty")
	}
	if cfg.BasePath != "/acugis-report-server" {
		t.Fatalf("base path: %s", cfg.BasePath)
	}
	if cfg.SessionCookieName != DefaultSessionCookieName {
		t.Fatalf("cookie: %s", cfg.SessionCookieName)
	}
}

func TestLoadJRXMLDirOverrideAndCatalina(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "config.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("JRS_JASPER_HOME", tmp)
	t.Setenv("JRS_DB_HOST", "localhost")
	t.Setenv("JRS_DB_NAME", "jasper_reports")
	t.Setenv("JRS_DB_USER", "postgres")
	t.Setenv("JRS_DB_PASS", "x")
	t.Setenv("JRS_AUTH_DISABLED", "1")
	t.Setenv("CATALINA_BASE", "")
	t.Setenv("CATALINA_HOME", "")
	t.Setenv("JRS_JASPER_CONTEXT_PATH", "")
	t.Setenv("JRS_JASPER_REPORTS_CONTAINER_DIR", "")

	override := filepath.Join(tmp, "custom-reports")
	t.Setenv("JRS_JRXML_DIR", override)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load override: %v", err)
	}
	if cfg.Paths.JRXMLDir != filepath.Clean(override) {
		t.Fatalf("override jrxml: %s", cfg.Paths.JRXMLDir)
	}

	catalina := t.TempDir()
	reports := filepath.Join(catalina, "acugis", "reports")
	conf := filepath.Join(catalina, "conf")
	if err := os.MkdirAll(reports, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(conf, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JRS_JRXML_DIR", "")
	t.Setenv("CATALINA_BASE", catalina)
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load catalina: %v", err)
	}
	if cfg.Paths.JRXMLDir != reports {
		t.Fatalf("catalina jrxml: %s want %s", cfg.Paths.JRXMLDir, reports)
	}
	wantCtx := filepath.Join(catalina, "conf", "context.xml")
	if cfg.Paths.JasperContextPath != wantCtx {
		t.Fatalf("catalina context: %s want %s", cfg.Paths.JasperContextPath, wantCtx)
	}
	wantContainer, _ := filepath.Abs(reports)
	if cfg.Paths.JasperReportsContainerDir != filepath.ToSlash(wantContainer) {
		t.Fatalf("container dir: %s want %s", cfg.Paths.JasperReportsContainerDir, filepath.ToSlash(wantContainer))
	}
}

func TestQCartaProfileAndAuthDatabaseEnv(t *testing.T) {
	t.Setenv("JRS_PROFILE", "qcarta")
	t.Setenv("JRS_AUTH_DATABASE_URL", "postgres://central/acugis_meta")
	t.Setenv("JRS_CENTRAL_LOGIN_URL", "/login")
	t.Setenv("JRS_SESSION_COOKIE", "acugis_session")
	t.Setenv("JRS_MYSQL_JDBC_DRIVER", "org.mariadb.jdbc.Driver")
	t.Setenv("JRS_PAT_PREFIXES", "agp_,jrs_")
	t.Setenv("JRS_LISTEN_ADDR", "127.0.0.1:9188")
	t.Setenv("JRS_BASE_PATH", "/report-runtime")
	t.Setenv("JRS_DB_HOST", "127.0.0.1")
	t.Setenv("JRS_DB_PORT", "5432")
	t.Setenv("JRS_DB_NAME", "qcarta_report_runtime")
	t.Setenv("JRS_DB_USER", "postgres")
	t.Setenv("JRS_DB_PASS", "x")
	t.Setenv("JRS_JASPER_HOME", t.TempDir())
	t.Setenv("JRS_JASPER_INTERNAL_REPORT_URL", "http://127.0.0.1:9189/report")
	t.Setenv("JRS_JASPER_INTERNAL_IMAGE_URL", "http://127.0.0.1:9189/acugis_report_image")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.IsQCartaProfile() {
		t.Fatalf("profile=%q", cfg.Profile)
	}
	if cfg.AuthDatabase.URL != "postgres://central/acugis_meta" {
		t.Fatalf("auth url=%q", cfg.AuthDatabase.URL)
	}
	if !cfg.AuthDatabase.HasOverride() {
		t.Fatal("auth database override missing")
	}
	if cfg.SessionCookieName != "acugis_session" {
		t.Fatalf("cookie=%q", cfg.SessionCookieName)
	}
	if cfg.MySQLJDBCDriver != "org.mariadb.jdbc.Driver" {
		t.Fatalf("mysql driver=%q", cfg.MySQLJDBCDriver)
	}
	if len(cfg.PATPrefixes) != 2 || cfg.PATPrefixes[0] != "agp_" {
		t.Fatalf("pat prefixes=%v", cfg.PATPrefixes)
	}
	if cfg.BasePath != "/report-runtime" {
		t.Fatalf("base path=%q", cfg.BasePath)
	}
}

func TestMCPEnabledFromEnv(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "config.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JRS_JASPER_HOME", tmp)
	t.Setenv("JRS_DB_NAME", "jasper_reports")
	t.Setenv("JRS_MCP_ENABLED", "yes")
	t.Setenv("JRS_MCP_ALLOWED_HOSTS", " reports.example.com, Reports.Example.com, alt.example.com ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.MCP.Enabled {
		t.Fatal("JRS_MCP_ENABLED=yes should enable MCP")
	}
	if len(cfg.MCP.AllowedHosts) != 2 || cfg.MCP.AllowedHosts[0] != "reports.example.com" || cfg.MCP.AllowedHosts[1] != "alt.example.com" {
		t.Fatalf("allowed hosts=%v", cfg.MCP.AllowedHosts)
	}
	if cfg.RedactedSummary()["mcp_enabled"] != true {
		t.Fatalf("summary=%v", cfg.RedactedSummary()["mcp_enabled"])
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
