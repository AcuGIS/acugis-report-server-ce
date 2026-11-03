package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/license"
)

func TestDesignerInstalledUnlicensedIsDiscoverable(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	admin := &auth.User{ID: 1, Username: "root", Enabled: true, IsAdmin: true}
	editor := &auth.User{ID: 17, Username: "ada", Enabled: true}
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "admin-token"):  admin,
		hashToken(t, "editor-token"): editor,
	}}
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	expires := now.Add(14 * 24 * time.Hour)
	statusMode := "available"
	var posted atomic.Value
	var hits atomic.Int32
	licenseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		posted.Store(string(body))
		var req struct {
			ProductCode    string `json:"product_code"`
			InstallationID string `json:"installation_id"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/status") {
			eligible := statusMode == "available"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"product_code": req.ProductCode, "trial_enabled": true, "trial_days": 14,
				"eligible": eligible, "used": !eligible, "status": statusMode,
			})
			return
		}
		hits.Add(1)
		if req.ProductCode != license.ProductReportDesignerWeb {
			t.Errorf("trial product %s", req.ProductCode)
		}
		if strings.Contains(string(body), "trial_days") || strings.Contains(string(body), "activation_key") || strings.Contains(string(body), "expires_at") {
			t.Errorf("client controlled the trial: %s", body)
		}
		token := signGateTrial(t, priv, req.InstallationID, now.Add(-time.Minute), expires)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"license": token, "license_id": "lic_gate", "product_code": license.ProductReportDesignerWeb,
			"license_type": license.LicenseTypeTrial, "expires_at": expires.Format(time.RFC3339),
		})
	}))
	defer licenseSrv.Close()

	dir := t.TempDir()
	pro, err := license.NewManager(license.Options{
		StateDir: dir, PublicKey: pub, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pro.Load(); err != nil {
		t.Fatal(err)
	}
	des, err := license.NewManager(license.Options{
		StateDir: dir, PublicKey: pub, Now: func() time.Time { return now },
		ProductCode: license.ProductReportDesignerWeb, LicenseFile: license.DesignerLicenseFileName,
		RequireExistingInstallation: true,
		TrialURL:                    licenseSrv.URL + "/v1/trial",
		TrialStatusURL:              licenseSrv.URL + "/v1/trial/status",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := des.Load(); err != nil {
		t.Fatal(err)
	}
	s.SetLicenseManager(pro)
	s.SetDesignerLicense(des)
	if des.DesignerEntitled() || s.designerCreateURL() == "" || s.designerImportURL() == "" {
		t.Fatalf("discoverability entitled=%v create=%q", des.DesignerEntitled(), s.designerCreateURL())
	}
	req := httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	req = req.WithContext(auth.WithUser(req.Context(), admin))
	native, canDesign := s.annotateDesignerReport(req, admin, "sales-report")
	if !native || !canDesign {
		t.Fatalf("unlicensed designer report hidden native=%v design=%v", native, canDesign)
	}

	pubPage := httptest.NewRequest(http.MethodGet, "/reports/publish_report", nil)
	withSession(pubPage, "admin-token")
	pubRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(pubRec, pubPage)
	pubBody := pubRec.Body.String()
	if pubRec.Code != http.StatusOK || !strings.Contains(pubBody, "Publish Report") || !strings.Contains(pubBody, "Upload a JRXML file") || !strings.Contains(pubBody, "Report Package") {
		t.Fatalf("publish page %d", pubRec.Code)
	}
	if strings.Contains(pubBody, "Create Designer Report") || strings.Contains(pubBody, "Import Designer Project") || strings.Contains(pubBody, "/reports/designer/new") || strings.Contains(pubBody, "/reports/designer/import") {
		t.Fatal("publish page still renders designer links")
	}

	hubPage := httptest.NewRequest(http.MethodGet, "/hubs/reports", nil)
	withSession(hubPage, "admin-token")
	hubRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(hubRec, hubPage)
	hubBody := hubRec.Body.String()
	if hubRec.Code != http.StatusOK || !strings.Contains(hubBody, "Create Designer Report") || !strings.Contains(hubBody, "Import Designer Project") {
		t.Fatalf("reports nav discoverability %d %s", hubRec.Code, hubBody)
	}
	if !strings.Contains(hubBody, `href="/reports/designer/new"`) || !strings.Contains(hubBody, `href="/reports/designer/import"`) {
		t.Fatal("designer nav links do not use the existing routes")
	}

	edReq := httptest.NewRequest(http.MethodGet, "/api/v1/edition", nil)
	edRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(edRec, edReq)
	if edRec.Code != http.StatusOK || !strings.Contains(edRec.Body.String(), `"report_designer":true`) {
		t.Fatalf("edition discoverability %d %s", edRec.Code, edRec.Body.String())
	}

	adminPage := httptest.NewRequest(http.MethodGet, "/reports/designer/new", nil)
	withSession(adminPage, "admin-token")
	adminRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(adminRec, adminPage)
	adminBody := adminRec.Body.String()
	if adminRec.Code != http.StatusOK || !strings.Contains(adminBody, "Report Designer Web is installed but is not currently licensed.") || !strings.Contains(adminBody, "Start 14-Day Trial") || !strings.Contains(adminBody, "Activate License") {
		t.Fatalf("admin gate %d %s", adminRec.Code, adminBody)
	}
	if !strings.Contains(adminBody, `class="fed-shell-body"`) || !strings.Contains(adminBody, `class="fed-shell-sidebar"`) || !strings.Contains(adminBody, `class="main-content"`) {
		t.Fatal("admin gate is outside the report server shell")
	}
	if strings.Contains(adminRec.Body.String(), "Folder") {
		t.Fatal("unlicensed admin entered the designer form")
	}

	editorPage := httptest.NewRequest(http.MethodGet, "/reports/designer/new", nil)
	withSession(editorPage, "editor-token")
	editorRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(editorRec, editorPage)
	editorBody := editorRec.Body.String()
	if editorRec.Code != http.StatusForbidden || !strings.Contains(editorBody, "Report Designer Web is not currently available.") || !strings.Contains(editorBody, "Contact your Report Server administrator.") {
		t.Fatalf("editor gate %d %s", editorRec.Code, editorBody)
	}
	if !strings.Contains(editorBody, `class="fed-shell-body"`) || !strings.Contains(editorBody, `class="fed-shell-sidebar"`) || !strings.Contains(editorBody, `class="main-content"`) {
		t.Fatal("non-admin gate is outside the report server shell")
	}
	if strings.Contains(editorRec.Body.String(), "Start 14-Day Trial") || strings.Contains(editorRec.Body.String(), "Activate License") {
		t.Fatal("non-admin received licensing controls")
	}

	api := httptest.NewRequest(http.MethodPost, "/api/designer/reports", strings.NewReader(`{"name":"Secret"}`))
	api.Header.Set("Content-Type", "application/json")
	withSession(api, "admin-token")
	apiRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(apiRec, api)
	if apiRec.Code != http.StatusForbidden || !strings.Contains(apiRec.Body.String(), "not licensed") || capd.hits.Load() != 0 {
		t.Fatalf("unlicensed execution %d hits=%d %s", apiRec.Code, capd.hits.Load(), apiRec.Body.String())
	}

	design := httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	withSession(design, "admin-token")
	designRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(designRec, design)
	if designRec.Code != http.StatusOK || !strings.Contains(designRec.Body.String(), "Activate License") || capd.hits.Load() != 0 {
		t.Fatalf("design page proxied or skipped the gate %d hits=%d", designRec.Code, capd.hits.Load())
	}

	statusMode = "expired"
	expired := httptest.NewRequest(http.MethodGet, "/reports/designer/import", nil)
	withSession(expired, "admin-token")
	expiredRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(expiredRec, expired)
	if strings.Contains(expiredRec.Body.String(), "Start 14-Day Trial") || !strings.Contains(expiredRec.Body.String(), "Activate License") || strings.Contains(expiredRec.Body.String(), "Desktop project ZIP") {
		t.Fatal("expired trial still offered designer entry into import")
	}

	statusMode = "available"
	form := url.Values{}
	form.Set("action", "start_trial_designer")
	form.Set("trial_days", "365")
	form.Set("expires_at", "2099-01-01T00:00:00Z")
	start := httptest.NewRequest(http.MethodPost, "/reports/designer/new", strings.NewReader(form.Encode()))
	start.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	start.Host = "reports.example.com"
	start.Header.Set("Origin", "https://reports.example.com")
	withSession(start, "admin-token")
	startRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(startRec, start)
	if startRec.Code != http.StatusSeeOther {
		t.Fatalf("start trial %d %s", startRec.Code, startRec.Body.String())
	}
	if !des.DesignerEntitled() || pro.ProfessionalEntitled() || pro.Status().EffectiveEdition != "community" {
		t.Fatalf("after trial designer=%v professional=%v edition=%s", des.DesignerEntitled(), pro.ProfessionalEntitled(), pro.Status().EffectiveEdition)
	}
	if _, err := os.Stat(filepath.Join(dir, license.DesignerLicenseFileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "license.lic")); !os.IsNotExist(err) {
		t.Fatal("designer trial wrote the professional license")
	}
	opened := httptest.NewRequest(http.MethodGet, startRec.Header().Get("Location"), nil)
	withSession(opened, "admin-token")
	openedRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(openedRec, opened)
	openedBody := openedRec.Body.String()
	if openedRec.Code != http.StatusOK || !strings.Contains(openedBody, "Create Designer Report") || !strings.Contains(openedBody, "Folder") || !strings.Contains(openedBody, `name="name"`) || !strings.Contains(openedBody, `name="description"`) || !strings.Contains(openedBody, `name="folder_id"`) || !strings.Contains(openedBody, "Create Report") {
		t.Fatalf("entitled designer flow %d %s", openedRec.Code, openedBody)
	}
	if !strings.Contains(openedBody, `class="fed-shell-body"`) || !strings.Contains(openedBody, `class="main-content"`) {
		t.Fatal("create page is outside the report server shell")
	}
	if hits.Load() != 1 {
		t.Fatalf("trial calls %d", hits.Load())
	}
}

func signGateTrial(t *testing.T, priv ed25519.PrivateKey, install string, issued, expires time.Time) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"schema_version":  1,
		"license_id":      "lic_gate",
		"installation_id": install,
		"product_code":    license.ProductReportDesignerWeb,
		"license_type":    license.LicenseTypeTrial,
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
