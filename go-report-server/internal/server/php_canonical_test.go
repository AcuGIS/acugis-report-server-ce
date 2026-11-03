package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"log/slog"
)

func TestStripPublicPHP(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"/admin/users.php", "/admin/users", true},
		{"/schedules.php", "/schedules", true},
		{"/reports/index.php", "/reports/", true},
		{"/explorer/index.php", "/explorer/", true},
		{"/dashboards/index.php", "/dashboards/", true},
		{"/dataset-intelligence/index.php", "/dataset-intelligence/", true},
		{"/reports/view.php", "/reports/view", true},
		{"/reports/execute_report.php", "/reports/execute", true},
		{"/login.php", "/login", true},
		{"/hubs/reports.php", "/hubs/reports", true},
		{"/action/schedule.php", "/action/schedule", true},
		{"/reports/datasources.php", "/reports/datasources", true},
		{"/reports", "/reports", false},
		{"/api/resource-sync/index.php", "/api/resource-sync/", true},
	} {
		got, ok := stripPublicPHP(tc.in)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("stripPublicPHP(%q)=(%q,%v) want (%q,%v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestCanonicalPublicPHPPathSkipsAPIAndHonorsBasePath(t *testing.T) {
	t.Parallel()
	s := New(testConfig(), Dependencies{}, slog.Default())
	if _, ok := s.canonicalPublicPHPPath("/api/resource-sync/index.php"); ok {
		t.Fatal("API PHP path must not be canonicalized")
	}
	clean, ok := s.canonicalPublicPHPPath("/admin/users.php")
	if !ok || clean != "/admin/users" {
		t.Fatalf("got %q ok=%v", clean, ok)
	}

	cfg := testConfig()
	cfg.BasePath = "/go"
	s = New(cfg, Dependencies{}, slog.Default())
	if _, ok := s.canonicalPublicPHPPath("/go/api/resource-sync/index.php"); ok {
		t.Fatal("prefixed API PHP path must not be canonicalized")
	}
	clean, ok = s.canonicalPublicPHPPath("/go/reports/index.php")
	if !ok || clean != "/go/reports/" {
		t.Fatalf("got %q ok=%v", clean, ok)
	}
	clean, ok = s.canonicalPublicPHPPath("/go/login.php")
	if !ok || clean != "/go/login" {
		t.Fatalf("login got %q ok=%v", clean, ok)
	}
}

func TestPHPGETRedirectsToCanonical(t *testing.T) {
	cfg := testConfig()
	cfg.AuthDisabled = true
	s := New(cfg, Dependencies{}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/admin/users.php?id=3", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status=%d want 301 body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/users?id=3" {
		t.Fatalf("Location=%q", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/index.php?q=1", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("index status=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/reports/?q=1" {
		t.Fatalf("index Location=%q", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/view.php?report=demo&datasource=DS", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("view.php status=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/reports/view?report=demo&datasource=DS" {
		t.Fatalf("view.php Location=%q", loc)
	}
}

func TestPHPPOSTDoesNotRedirect(t *testing.T) {
	s := authEnabledTestServer(t, nil, denyACL{})

	req := httptest.NewRequest(http.MethodPost, "/login.php", strings.NewReader("username=&password="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusMovedPermanently {
		t.Fatalf("POST login.php redirected: %s", rec.Header().Get("Location"))
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("POST login.php status=%d Location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if rec.Header().Get("Location") != "" {
		t.Fatalf("POST login.php Location=%s", rec.Header().Get("Location"))
	}
	if !strings.Contains(rec.Body.String(), "Sign in") {
		t.Fatalf("POST login.php lost handler: %s", rec.Body.String())
	}
}

func TestAPIPHPathUnchanged(t *testing.T) {
	cfg := testConfig()
	cfg.AuthDisabled = true
	s := New(cfg, Dependencies{}, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/resource-sync/index.php", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusMovedPermanently {
		t.Fatalf("API PHP path redirected to %s", rec.Header().Get("Location"))
	}
}
