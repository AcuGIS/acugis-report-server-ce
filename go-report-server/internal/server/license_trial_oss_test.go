//go:build oss

package server

import (
	"crypto/ed25519"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/license"
)

func TestCommunityPackageDoesNotOfferProfessionalTrial(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product_code": license.ProductReportServerProfessional, "trial_enabled": true,
			"trial_days": 14, "eligible": true, "used": false, "status": "available",
		})
	}))
	defer upstream.Close()
	mgr, err := license.NewManager(license.Options{
		StateDir: t.TempDir(), PublicKey: pub, TrialStatusURL: upstream.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if mgr.Status().InstalledPackageCapability != "community" {
		t.Fatalf("capability %s", mgr.Status().InstalledPackageCapability)
	}
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Edition = "community"
	s := New(cfg, Dependencies{Auth: auth.NewService(nil, auth.Options{AuthDisabled: true})}, slog.Default())
	s.SetLicenseManager(mgr)
	req := httptest.NewRequest(http.MethodGet, "/admin/license", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Start 14-Day Trial") {
		t.Fatal("community package offered a professional trial")
	}
	if !strings.Contains(body, "Install AcuGIS Report Server Professional to start or activate Professional features.") {
		t.Fatal("community package missing professional install guidance")
	}
	if !strings.Contains(body, "Does not require Report Server Professional") {
		t.Fatal("community page implied designer requires professional")
	}
}

func TestCommunityPackageRejectsProfessionalTrialPost(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	mgr, err := license.NewManager(license.Options{
		StateDir: t.TempDir(), PublicKey: pub, TrialURL: upstream.URL, TrialStatusURL: upstream.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Edition = "community"
	s := New(cfg, Dependencies{Auth: auth.NewService(nil, auth.Options{AuthDisabled: true})}, slog.Default())
	s.SetLicenseManager(mgr)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/license/trial", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "reports.example.com"
	req.Header.Set("Origin", "https://reports.example.com")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("api status %d %s", rec.Code, rec.Body.String())
	}
	if mgr.ProfessionalEntitled() || hits.Load() != 0 {
		t.Fatalf("community api trial entitled=%v hits=%d", mgr.ProfessionalEntitled(), hits.Load())
	}

	form := url.Values{}
	form.Set("action", "start_trial")
	form.Set("trial_days", "365")
	page := httptest.NewRequest(http.MethodPost, "/admin/license", strings.NewReader(form.Encode()))
	page.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	page.Host = "reports.example.com"
	page.Header.Set("Origin", "https://reports.example.com")
	pageRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(pageRec, page)
	if mgr.ProfessionalEntitled() || hits.Load() != 0 {
		t.Fatalf("community form trial entitled=%v hits=%d body=%s", mgr.ProfessionalEntitled(), hits.Load(), pageRec.Body.String())
	}
	if strings.Contains(pageRec.Body.String(), "Start 14-Day Trial") {
		t.Fatal("rejected community trial still offered a start button")
	}
}
