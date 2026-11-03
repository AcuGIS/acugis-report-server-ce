package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/license"
)

func grantDesignerEntitlement(t *testing.T, s *Server) {
	t.Helper()
	s.SetDesignerLicense(designerLicenseManager(t, license.ProductReportDesignerWeb, testInstallForDesigner(), time.Now().Add(24*time.Hour), true))
}

func designerLicenseManager(t *testing.T, product, install string, expires time.Time, validSig bool) *license.Manager {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "installation-id"), []byte(testInstallForDesigner()+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	token := signDesignerACL1(t, priv, product, install, now.Add(-time.Hour), expires)
	if !validSig {
		token = token[:len(token)-8] + "deadbeef"
	}
	if err := os.WriteFile(filepath.Join(dir, license.DesignerLicenseFileName), []byte(token+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	mgr, err := license.NewManager(license.Options{
		StateDir:                    dir,
		PublicKey:                   pub,
		ProductCode:                 license.ProductReportDesignerWeb,
		LicenseFile:                 license.DesignerLicenseFileName,
		RequireExistingInstallation: true,
		Now:                         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	return mgr
}

func testInstallForDesigner() string {
	return "11111111-1111-4111-8111-111111111111"
}

func signDesignerACL1(t *testing.T, priv ed25519.PrivateKey, product, install string, issued, expires time.Time) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"schema_version":  1,
		"license_id":      "lic_designer_test",
		"installation_id": install,
		"product_code":    product,
		"issued_at":       issued.UTC().Format(time.RFC3339),
		"expires_at":      expires.UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawURLEncoding.EncodeToString(raw)
	msg := []byte("ACL1." + b64)
	sig := ed25519.Sign(priv, msg)
	return "ACL1." + b64 + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestDesignerRoutesRejectWithoutEntitlement(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	s.SetDesignerLicense(designerLicenseManager(t, license.ProductReportDesignerWeb, testInstallForDesigner(), time.Now().Add(-time.Hour), true))
	assets := t.TempDir()
	if err := os.MkdirAll(filepath.Join(assets, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.cfg.Designer.AssetsDir = assets

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/reports/sales-report/design"},
		{http.MethodGet, "/reports/designer/new"},
		{http.MethodPost, "/reports/designer/new"},
		{http.MethodGet, "/reports/designer/import"},
		{http.MethodPost, "/api/designer/reports"},
		{http.MethodPost, "/api/designer/import"},
		{http.MethodPost, "/api/designer/sales-report/save-as"},
		{http.MethodPost, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/preview"},
		{http.MethodPost, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/query/compile"},
		{http.MethodPost, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/export"},
		{http.MethodGet, "/api/designer/sales-report/connections"},
		{http.MethodGet, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/resources"},
		{http.MethodGet, "/designer-assets/assets/app.js"},
	}
	for _, tc := range paths {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		req.Header.Set("Accept", "application/json")
		withSession(req, "editor-token")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "not licensed") {
			t.Errorf("%s %s: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	if capd.hits.Load() != 0 {
		t.Fatalf("unlicensed designer was proxied %d times", capd.hits.Load())
	}

	health := httptest.NewRequest(http.MethodGet, "/health", nil)
	hrec := httptest.NewRecorder()
	s.Handler().ServeHTTP(hrec, health)
	if hrec.Code != http.StatusOK {
		t.Fatalf("health without web license: %d", hrec.Code)
	}

	anon := httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	arec := httptest.NewRecorder()
	s.Handler().ServeHTTP(arec, anon)
	if arec.Code != http.StatusSeeOther {
		t.Fatalf("anonymous design: %d want login redirect", arec.Code)
	}
}

func TestDesignerTrialEntitlementMatchesPurchased(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	s.SetDesignerLicense(designerTypedManager(t, license.LicenseTypeTrial, time.Now().Add(24*time.Hour)))

	req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/preview", strings.NewReader(`{}`))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if !s.designerEntitled() || (rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "not licensed")) {
		t.Fatalf("active trial denied: entitled=%v %d %s", s.designerEntitled(), rec.Code, rec.Body.String())
	}
	if capd.hits.Load() == 0 {
		t.Fatalf("trial did not pass the designer gate: %d %s", rec.Code, rec.Body.String())
	}
	before := capd.hits.Load()

	s.SetDesignerLicense(designerTypedManager(t, license.LicenseTypeTrial, time.Now().Add(-time.Hour)))
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/preview", strings.NewReader(`{}`))
	req.Header.Set("Accept", "application/json")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "not licensed") {
		t.Fatalf("expired trial: %d %s", rec.Code, rec.Body.String())
	}
	if capd.hits.Load() != before {
		t.Fatal("expired trial was proxied")
	}
}

func designerTypedManager(t *testing.T, licenseType string, expires time.Time) *license.Manager {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "installation-id"), []byte(testInstallForDesigner()+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	raw, err := json.Marshal(map[string]any{
		"schema_version":  1,
		"license_id":      "lic_designer_trial",
		"installation_id": testInstallForDesigner(),
		"product_code":    license.ProductReportDesignerWeb,
		"license_type":    licenseType,
		"issued_at":       now.Add(-time.Hour).Format(time.RFC3339),
		"expires_at":      expires.UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawURLEncoding.EncodeToString(raw)
	msg := []byte("ACL1." + b64)
	sig := ed25519.Sign(priv, msg)
	token := "ACL1." + b64 + "." + base64.RawURLEncoding.EncodeToString(sig)
	if err := os.WriteFile(filepath.Join(dir, license.DesignerLicenseFileName), []byte(token+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	mgr, err := license.NewManager(license.Options{
		StateDir:                    dir,
		PublicKey:                   pub,
		ProductCode:                 license.ProductReportDesignerWeb,
		LicenseFile:                 license.DesignerLicenseFileName,
		RequireExistingInstallation: true,
		Now:                         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestDesignerWrongProductTokenDoesNotEntitle(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("wrong-product token reached designer: %s", r.URL.Path)
	}))
	defer up.Close()
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	s.SetDesignerLicense(designerLicenseManager(t, license.ProductReportServerProfessional, testInstallForDesigner(), time.Now().Add(24*time.Hour), true))
	req := httptest.NewRequest(http.MethodGet, "/api/designer/sales-report/connections", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("professional token as web token: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDesignerPackageAbsentDoesNotOfferLinks(t *testing.T) {
	s := New(testConfig(), Dependencies{}, nil)
	if s.designerOffered() || s.designerCreateURL() != "" {
		t.Fatal("unconfigured designer was offered")
	}
}
