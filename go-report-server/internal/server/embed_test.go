package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/embedtoken"
)

func TestEmbedRoutesRequireTokenAndFrameAncestors(t *testing.T) {
	cfg := testConfig()
	cfg.EmbedAllowedOrigins = []string{"https://app.example.com"}
	cfg.EmbedTokenSecret = "unit-test-embed-secret"
	cfg.EmbedTokenTTL = time.Minute
	srv := New(cfg, Dependencies{}, slog.Default())
	handler := srv.routes()

	// Normal UI keeps X-Frame-Options
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("health XFO=%q", got)
	}

	// Embed without token → 401
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/embed/report/Invoice", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("embed without token: status %d body %s", rec.Code, rec.Body.String())
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors") {
		t.Fatalf("expected CSP frame-ancestors, got %q", csp)
	}
	if rec.Header().Get("X-Frame-Options") != "" {
		t.Fatalf("embed route must not set X-Frame-Options")
	}

	// PAT rejected on embed route
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/embed/report/Invoice?et=jrs_notallowed", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("PAT on embed: status %d", rec.Code)
	}

	// Valid emb_ token still fails ACL/content load without store, but not AUTH_REQUIRED
	tok, _, err := embedtoken.Issue(cfg.EmbedTokenSecret, embedtoken.Claims{
		UserID:      1,
		Username:    "alice",
		ContentType: embedtoken.ContentReport,
		ContentID:   "Invoice",
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/embed/report/Invoice?et="+tok, nil)
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized && strings.Contains(rec.Body.String(), "embed token required") {
		t.Fatalf("valid token should not fail AUTH_REQUIRED: %s", rec.Body.String())
	}
}

func TestMintEmbedTokenRequiresAuth(t *testing.T) {
	cfg := testConfig()
	cfg.EmbedTokenSecret = "unit-test-embed-secret"
	cfg.EmbedAllowedOrigins = []string{"https://app.example.com"}
	srv := New(cfg, Dependencies{}, slog.Default())
	handler := srv.routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/embed/token", strings.NewReader(`{"content_type":"report","content_id":"Invoice"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://app.example.com")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("mint without auth: status %d body %s", rec.Code, rec.Body.String())
	}
}
