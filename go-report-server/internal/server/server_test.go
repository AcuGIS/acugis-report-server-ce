package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/filesystem"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

func testConfig() config.Config {
	return config.Config{
		Environment: "test",
		ListenAddr:  "127.0.0.1:0",
		BasePath:    "/",
		AppVersion:  "test",
		Edition:     "professional",
		Database: config.DatabaseConfig{
			Host: "localhost",
			Port: 5432,
			Name: "jasper_reports",
			User: "postgres",
		},
		Jasper: config.JasperConfig{
			ReportURL: "http://127.0.0.1:9099/report",
			ImageURL:  "http://127.0.0.1:9099/acugis_report_image",
			Timeout:   time.Second,
		},
		Paths: config.PathConfig{
			AppRoot:       "/tmp",
			ReportJSONDir: "/tmp/missing-json",
			JRXMLDir:      "/tmp/missing-jrxml",
		},
		SessionCookieName: "jrs_session",
	}
}

func TestHealthEndpoint(t *testing.T) {
	s := New(testConfig(), Dependencies{}, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["service"] != config.ServiceName {
		t.Fatalf("unexpected body: %+v", body)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing secure header")
	}
	if rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Fatalf("X-Frame-Options=%q want SAMEORIGIN (viewer iframe)", rec.Header().Get("X-Frame-Options"))
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "strict-origin" {
		t.Fatalf("Referrer-Policy=%q want strict-origin", got)
	}
}

func TestReferrerPolicyDoesNotNullPostOrigin(t *testing.T) {
	s := New(testConfig(), Dependencies{}, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	got := rec.Header().Get("Referrer-Policy")
	if got == "no-referrer" || got == "" {
		t.Fatalf("Referrer-Policy=%q; no-referrer makes browsers send Origin: null on POST", got)
	}
	if got != "strict-origin" {
		t.Fatalf("Referrer-Policy=%q want strict-origin", got)
	}
}

func TestReadyFailsWithoutDependencies(t *testing.T) {
	cfg := testConfig()
	paths := filesystem.New(cfg.Paths)
	s := New(cfg, Dependencies{
		DB:     nil,
		Paths:  paths,
		Jasper: jasper.New(cfg.Jasper),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "not_ready" {
		t.Fatalf("unexpected status: %+v", body)
	}
}

func TestDiagnosticsRedactsSecrets(t *testing.T) {
	cfg := testConfig()
	cfg.Database.Password = "super-secret-db-password"
	s := New(cfg, Dependencies{
		Jasper: jasper.New(cfg.Jasper),
		Paths:  filesystem.New(cfg.Paths),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "super-secret-db-password") {
		t.Fatal("diagnostics leaked database password")
	}
	if strings.Contains(raw, "password=") {
		t.Fatal("diagnostics leaked connection string fragment")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["go_version"]; !ok {
		t.Fatal("missing go_version")
	}
	if body["jasper_reachable"] != "false" && body["jasper_reachable"] != "unknown" {
		t.Fatalf("expected jasper_reachable false or unknown, got %v", body["jasper_reachable"])
	}
}

func TestBasePathMount(t *testing.T) {
	cfg := testConfig()
	cfg.BasePath = "/acugis-report-server"
	s := New(cfg, Dependencies{}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/acugis-report-server/health", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mounted health status=%d", rec.Code)
	}
}
