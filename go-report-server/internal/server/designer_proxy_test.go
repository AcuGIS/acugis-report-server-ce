package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

const designerTestToken = "proxy-test-token-value"

type designerAuthRepo struct {
	byHash map[string]*auth.User
}

func (m *designerAuthRepo) Configured() bool { return true }

func (m *designerAuthRepo) ResolveUserFromSession(_ context.Context, hash string) (*auth.User, error) {
	u := m.byHash[hash]
	if u == nil {
		return nil, auth.ErrInvalidSession
	}
	if !u.Enabled {
		return nil, auth.ErrDisabledUser
	}
	return u, nil
}

type designerACL struct {
	mu          sync.Mutex
	serviceEdit bool
	reportEdit  map[string]bool
	reportView  map[string]bool
}

func (a *designerACL) allow(service bool, edit, view map[string]bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.serviceEdit = service
	a.reportEdit = edit
	a.reportView = view
}

func (a *designerACL) CanViewReport(_ context.Context, _ *auth.User, key string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.reportView[key], nil
}
func (a *designerACL) CanEditReport(_ context.Context, _ *auth.User, key string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.reportEdit[key], nil
}
func (a *designerACL) CanAdminReport(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (a *designerACL) CanViewExplorerView(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (a *designerACL) CanEditExplorerView(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (a *designerACL) CanAccessReportImage(ctx context.Context, u *auth.User, key string) (bool, error) {
	return a.CanViewReport(ctx, u, key)
}
func (a *designerACL) CanAccessReportService(context.Context, *auth.User) (bool, error) {
	return true, nil
}
func (a *designerACL) CheckPermission(_ context.Context, _ *auth.User, resourceType, _, permission string) (acl.Decision, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if resourceType == acl.ResourceTypeService && permission == acl.PermissionEdit {
		return acl.Decision{Allowed: a.serviceEdit, Permission: permission}, nil
	}
	return acl.Decision{Permission: permission}, nil
}
func (a *designerACL) Configured() bool   { return true }
func (a *designerACL) AuthDisabled() bool { return false }

type capturedDesigner struct {
	mu         sync.Mutex
	hits       atomic.Int64
	last       http.Request
	lastHdr    http.Header
	lastMethod string
	lastPath   string
	lastQ      string
	body       string
}

func (c *capturedDesigner) handler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.hits.Add(1)
		buf, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		c.mu.Lock()
		c.lastHdr = r.Header.Clone()
		c.lastMethod = r.Method
		c.lastPath = r.URL.Path
		c.lastQ = r.URL.RawQuery
		c.body = string(buf)
		c.mu.Unlock()
		if r.Header.Get(headerDesignerToken) != token {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/internal/designer/open":
			key := r.Header.Get(headerDesignerReportKey)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"0123456789abcdef0123456789abcdef","reportKey":"` + key + `","reused":false,"projectVersion":"v1"}`))
		case "/internal/designer/logout":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"discarded":1}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	})
}

func designerFixture(t *testing.T, aclSvc acl.Service, repo auth.SessionResolver, upstream string, token string) (*Server, *reports.Store) {
	t.Helper()
	jsonDir := t.TempDir()
	jrxmlDir := t.TempDir()
	store := reports.NewStore(jsonDir, t.TempDir(), jrxmlDir)
	if err := store.SaveJSON("sales-report", map[string]any{"name": "Sales"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jrxmlDir, "sales-report.project.json"), []byte(`{"name":"Sales"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveJSON("legacy", map[string]any{"name": "Legacy"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jrxmlDir, "legacy.jrxml"), []byte("<jasperReport/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jrxmlDir, "legacy.jasper"), []byte("jasper"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.AuthDisabled = false
	cfg.Paths.JRXMLDir = jrxmlDir
	cfg.Paths.ReportJSONDir = jsonDir
	cfg.Designer.Upstream = upstream
	cfg.Designer.Token = token
	cfg.Designer.Timeout = 3 * time.Second
	cfg.Designer.MaxBody = 64 << 20
	if aclSvc == nil {
		aclSvc = denyACL{}
	}
	s := New(cfg, Dependencies{
		Reports: store,
		ACL:     aclSvc,
		Auth:    auth.NewService(repo, auth.Options{CookieName: "jrs_session", AuthDisabled: false}),
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	grantDesignerEntitlement(t, s)
	return s, store
}

func hashToken(t *testing.T, raw string) string {
	t.Helper()
	h, err := auth.HashSessionToken(raw)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func withSession(req *http.Request, raw string) {
	req.AddCookie(&http.Cookie{Name: "jrs_session", Value: raw})
}

func TestDesignerAuthz(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true, "legacy": true}, map[string]bool{"sales-report": true, "legacy": true})
	viewerACL := &designerACL{}
	viewerACL.allow(false, map[string]bool{}, map[string]bool{"sales-report": true, "legacy": true})
	serviceOnly := &designerACL{}
	serviceOnly.allow(true, map[string]bool{}, map[string]bool{"sales-report": true})
	reportOnly := &designerACL{}
	reportOnly.allow(false, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})

	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()

	editor := &auth.User{ID: 17, Username: "ada", Enabled: true}
	viewer := &auth.User{ID: 18, Username: "view", Enabled: true}
	disabled := &auth.User{ID: 19, Username: "old", Enabled: false}
	admin := &auth.User{ID: 1, Username: "root", Enabled: true, IsAdmin: true}
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"):   editor,
		hashToken(t, "viewer-token"):   viewer,
		hashToken(t, "disabled-token"): disabled,
		hashToken(t, "admin-token"):    admin,
	}}

	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)

	// Unauthenticated browser route redirects. API stays JSON.
	req := httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/login") {
		t.Fatalf("anon design: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/save", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon api: %d %s", rec.Code, rec.Body.String())
	}

	// Disabled user.
	req = httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "disabled-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("disabled: %d %s", rec.Code, rec.Body.String())
	}

	// Viewer.
	viewSrv, _ := designerFixture(t, viewerACL, repo, up.URL, designerTestToken)
	req = httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "viewer-token")
	rec = httptest.NewRecorder()
	viewSrv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer: %d %s", rec.Code, rec.Body.String())
	}

	// Service edit without report edit, and report edit without service edit.
	for _, tc := range []struct {
		name string
		acl  acl.Service
	}{
		{"service-only", serviceOnly},
		{"report-only", reportOnly},
	} {
		srv, _ := designerFixture(t, tc.acl, repo, up.URL, designerTestToken)
		req = httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
		req.Header.Set("Accept", "application/json")
		withSession(req, "editor-token")
		rec = httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}

	// Editor.
	before := capd.hits.Load()
	req = httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("editor: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"sessionId":"0123456789abcdef0123456789abcdef"`) {
		t.Fatalf("shell: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), designerTestToken) || strings.Contains(rec.Body.String(), "127.0.0.1") {
		t.Fatalf("shell leaked internal detail: %s", rec.Body.String())
	}
	if capd.hits.Load() == before {
		t.Fatal("editor open did not reach designer")
	}

	// Administrator bypasses a denying ACL through the existing admin flag.
	adminSrv, _ := designerFixture(t, denyACL{}, repo, up.URL, designerTestToken)
	req = httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "admin-token")
	rec = httptest.NewRecorder()
	adminSrv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDesignerProxyTrustBoundary(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	evilHits := atomic.Int64{}
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		evilHits.Add(1)
		http.Error(w, "evil", http.StatusOK)
	}))
	defer evil.Close()

	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)

	sessionID := "0123456789abcdef0123456789abcdef"
	body := `{"report":"other-report","path":"/tmp/secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/sessions/"+sessionID+"/save?upstream="+evil.URL+"&report=other-report&keep=1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(headerDesignerToken, "browser-token")
	req.Header.Set(headerDesignerUserID, "999")
	req.Header.Set(headerDesignerUsername, "mallory")
	req.Header.Set(headerDesignerReportKey, "other-report")
	// Any Authorization value is consumed by existing PAT authentication and
	// does not fall back to the cookie. Hop-by-hop proxy credentials must not
	// be forwarded either.
	req.Header.Set("Proxy-Authorization", "Basic not-forwarded")
	req.Header.Set(headerDesignerSession, sessionID)
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("proxy: %d %s", rec.Code, rec.Body.String())
	}
	if evilHits.Load() != 0 {
		t.Fatal("browser-controlled target was contacted")
	}
	capd.mu.Lock()
	gotPath := capd.lastPath
	gotHdr := capd.lastHdr.Clone()
	gotQ := capd.lastQ
	gotBody := capd.body
	capd.mu.Unlock()
	if gotPath != "/api/designer/sessions/"+sessionID+"/save" {
		t.Fatalf("upstream path: %s", gotPath)
	}
	if gotHdr.Get(headerDesignerToken) != designerTestToken {
		t.Fatalf("token was not overwritten: %q", gotHdr.Get(headerDesignerToken))
	}
	if gotHdr.Get(headerDesignerUserID) != "17" || gotHdr.Get(headerDesignerUsername) != "ada" {
		t.Fatalf("identity not overwritten: id=%s user=%s", gotHdr.Get(headerDesignerUserID), gotHdr.Get(headerDesignerUsername))
	}
	if gotHdr.Get(headerDesignerReportKey) != "sales-report" {
		t.Fatalf("report key not overwritten: %s", gotHdr.Get(headerDesignerReportKey))
	}
	if cookie := gotHdr.Get("Cookie"); cookie != "" {
		t.Fatalf("forwarded cookie: %s", cookie)
	}
	if authz := gotHdr.Get("Authorization"); authz != "" {
		t.Fatalf("forwarded authorization: %s", authz)
	}
	if proxyAuth := gotHdr.Get("Proxy-Authorization"); proxyAuth != "" {
		t.Fatalf("forwarded proxy authorization: %s", proxyAuth)
	}
	if strings.Contains(gotQ, "upstream") || strings.Contains(gotQ, "report") || !strings.Contains(gotQ, "keep=1") {
		t.Fatalf("query: %s", gotQ)
	}
	if gotBody != body {
		t.Fatalf("body: %s", gotBody)
	}

	// URL/header session mismatch is rejected before the upstream call.
	hits := capd.hits.Load()
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/sessions/"+sessionID+"/save", strings.NewReader(`{}`))
	req.Header.Set(headerDesignerSession, "abcdefabcdefabcdefabcdefabcdefab")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mismatch: %d %s", rec.Code, rec.Body.String())
	}
	if capd.hits.Load() != hits {
		t.Fatal("mismatch was forwarded")
	}

	// Internal open is not a public proxy route.
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/internal/designer/open", nil)
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("internal via proxy: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDesignerProxyRejectsBadUpstreamConfig(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()

	s, _ := designerFixture(t, editorACL, repo, up.URL, "")
	req := httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing token: %d %s", rec.Code, rec.Body.String())
	}
	if capd.hits.Load() != 0 {
		t.Fatal("missing token still proxied")
	}
	if strings.Contains(rec.Body.String(), up.URL) {
		t.Fatalf("loopback leaked: %s", rec.Body.String())
	}

	s, _ = designerFixture(t, editorACL, repo, "http://example.com:8787", designerTestToken)
	req = httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("public upstream: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDesignerLegacyAndUnavailable(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true, "legacy": true}, map[string]bool{"sales-report": true, "legacy": true})
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("compiled report open contacted designer: %s", r.URL.Path)
	}))
	defer up.Close()
	s, store := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	if err := store.SaveJSON("compiled-only", map[string]any{"name": "Compiled"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.JRXMLDir(), "compiled-only.jasper"), []byte("jasper"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/reports/compiled-only/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "not Designer-editable") {
		t.Fatalf("legacy design: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/reports/legacy/config", nil)
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy config: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "not Designer-editable") {
		t.Fatalf("legacy view treated as designer: %s", rec.Body.String())
	}

	// Designer process is down. Report Server health still answers.
	down, _ := designerFixture(t, editorACL, repo, "http://127.0.0.1:1", designerTestToken)
	req = httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	down.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "Designer unavailable") {
		t.Fatalf("down: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "127.0.0.1") || strings.Contains(rec.Body.String(), designerTestToken) {
		t.Fatalf("down leaked detail: %s", rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	rec = httptest.NewRecorder()
	down.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/reports/legacy/config", nil)
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	down.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy still available while designer is down: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDesignerACLRevocation(t *testing.T) {
	aclSvc := &designerACL{}
	aclSvc.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	s, _ := designerFixture(t, aclSvc, repo, up.URL, designerTestToken)
	req := httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("open: %d %s", rec.Code, rec.Body.String())
	}
	hits := capd.hits.Load()
	aclSvc.allow(true, map[string]bool{"sales-report": false}, map[string]bool{"sales-report": true})
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/save", strings.NewReader(`{}`))
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("revoked: %d %s", rec.Code, rec.Body.String())
	}
	if capd.hits.Load() != hits {
		t.Fatal("revoked save was forwarded")
	}
	req = httptest.NewRequest(http.MethodGet, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/resources/file?path=images/logo.png", nil)
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("revoked resource: %d %s", rec.Code, rec.Body.String())
	}
	if capd.hits.Load() != hits {
		t.Fatal("revoked resource URL was forwarded")
	}
}

func TestDesignerForwardsResourcePath(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	sessionID := "0123456789abcdef0123456789abcdef"
	req := httptest.NewRequest(http.MethodGet, "/api/designer/sales-report/sessions/"+sessionID+"/resources/file?path=images/logo.png&upstream=http://evil.example&report=other-report", nil)
	req.Header.Set(headerDesignerSession, sessionID)
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("resource proxy: %d %s", rec.Code, rec.Body.String())
	}
	capd.mu.Lock()
	gotQ := capd.lastQ
	gotPath := capd.lastPath
	capd.mu.Unlock()
	if gotPath != "/api/designer/sessions/"+sessionID+"/resources/file" {
		t.Fatalf("path: %s", gotPath)
	}
	if !strings.Contains(gotQ, "path=images") || strings.Contains(gotQ, "upstream") || strings.Contains(gotQ, "report=") {
		t.Fatalf("query: %s", gotQ)
	}
}

func TestDesignerLogoutNotifiesUpstream(t *testing.T) {
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	s, _ := designerFixture(t, denyACL{}, repo, up.URL, designerTestToken)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	capd.mu.Lock()
	gotPath := capd.lastPath
	gotHdr := capd.lastHdr.Clone()
	capd.mu.Unlock()
	if gotPath != "/internal/designer/logout" {
		t.Fatalf("logout path: %s", gotPath)
	}
	if gotHdr.Get(headerDesignerUserID) != "17" {
		t.Fatalf("logout user: %s", gotHdr.Get(headerDesignerUserID))
	}
	if gotHdr.Get(headerDesignerToken) != designerTestToken {
		t.Fatal("logout token missing")
	}
	if gotHdr.Get("Cookie") != "" {
		t.Fatal("logout forwarded the session cookie")
	}
}

func TestDesignerBodyLimit(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	s.cfg.Designer.MaxBody = 32
	req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/save", strings.NewReader(strings.Repeat("x", 100)))
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body limit: %d %s", rec.Code, rec.Body.String())
	}
	if capd.hits.Load() != 0 {
		t.Fatal("oversized body was forwarded")
	}
}

func TestDesignerConfigFailClosed(t *testing.T) {
	cfg := testConfig()
	cfg.Database.ConnectTimeout = time.Second
	cfg.Designer.Upstream = "http://example.com:8787"
	cfg.Designer.Token = "secret"
	cfg.Designer.Timeout = time.Second
	cfg.Designer.MaxBody = 1024
	if err := cfg.Validate(); err == nil {
		t.Fatal("public upstream accepted")
	}
	cfg.Designer.Upstream = "http://127.0.0.1:8787"
	cfg.Designer.Token = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("missing token accepted")
	}
	cfg.Designer.Token = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := config.ParseLoopbackURL("http://127.0.0.1:8787/designer"); err == nil {
		t.Fatal("upstream path accepted")
	}
	summary := cfg.RedactedSummary()
	for _, v := range summary {
		if s, ok := v.(string); ok && strings.Contains(s, "secret") {
			t.Fatal("token leaked into redacted summary")
		}
	}
}

func TestDesignerViewOnlyCannotSavePreviewCompileExport(t *testing.T) {
	viewerACL := &designerACL{}
	viewerACL.allow(false, map[string]bool{}, map[string]bool{"sales-report": true})
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "viewer-token"): {ID: 18, Username: "view", Enabled: true},
	}}
	s, _ := designerFixture(t, viewerACL, repo, up.URL, designerTestToken)
	for _, op := range []string{"save", "preview", "compile", "export"} {
		hits := capd.hits.Load()
		req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/"+op, strings.NewReader(`{"report":"other-report"}`))
		req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
		req.Header.Set("Content-Type", "application/json")
		withSession(req, "viewer-token")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: %d %s", op, rec.Code, rec.Body.String())
		}
		if capd.hits.Load() != hits {
			t.Fatalf("%s was forwarded", op)
		}
	}
}

func TestDesignerSaveConflictPassthrough(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(headerDesignerToken) != designerTestToken {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/internal/designer/open" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"0123456789abcdef0123456789abcdef","reportKey":"sales-report","reused":false}`))
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"conflict","message":"This report was changed after you opened it. Reload before saving.","category":"conflict","errors":[{"line":4,"message":"Invalid text field expression"}]}`))
	}))
	defer up.Close()
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/sessions/0123456789abcdef0123456789abcdef/save", strings.NewReader(`{}`))
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Reload before saving") || !strings.Contains(rec.Body.String(), "Invalid text field expression") {
		t.Fatalf("conflict: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDesignerShellExposesSessionBootstrap(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	assets := t.TempDir()
	if err := os.WriteFile(filepath.Join(assets, "index.html"), []byte(`<!DOCTYPE html><html><head><title>Designer</title></head><body><div id="root"></div><script type="module" src="/designer-assets/assets/index-abc.js"></script></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	s.cfg.Designer.AssetsDir = assets
	req := httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("shell: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "__ACUGIS_DESIGNER__") || !strings.Contains(body, "0123456789abcdef0123456789abcdef") || !strings.Contains(body, "sales-report") {
		t.Fatalf("bootstrap missing: %s", body)
	}
	if strings.Contains(body, designerTestToken) || strings.Contains(body, "127.0.0.1") {
		t.Fatalf("shell leaked internal detail: %s", body)
	}
	if !strings.Contains(body, "/designer-assets/assets/index-abc.js") || !strings.Contains(body, "Sales — AcuGIS Report Designer") {
		t.Fatalf("spa: %s", body)
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("cache: %s", rec.Header().Get("Cache-Control"))
	}
	req = httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "__ACUGIS_DESIGNER__") {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body.String())
	}
}

// TestDesignerConnectionManagerReachesUpstream reproduces the deployed
// Connection Manager failure: Report Server mode rewrites
// /api/designer/connections/test to /api/designer/{reportKey}/connections/test,
// and that request must reach jrs-designer's POST /api/designer/connections/test
// instead of the proxy answering {"error":"not found"}.
func TestDesignerConnectionManagerReachesUpstream(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	capd := &capturedDesigner{}
	up := httptest.NewServer(capd.handler(designerTestToken))
	defer up.Close()
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	sessionID := "0123456789abcdef0123456789abcdef"

	cases := []struct {
		name     string
		method   string
		public   string
		upstream string
		body     string
		query    string
	}{
		{
			name:     "test postgresql",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/connections/test",
			upstream: "/api/designer/connections/test",
			body:     `{"provider":"postgis","host":"db.internal","database":"gis","username":"gis"}`,
		},
		{
			name:     "list",
			method:   http.MethodGet,
			public:   "/api/designer/sales-report/connections",
			upstream: "/api/designer/connections",
		},
		{
			name:     "save",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/connections",
			upstream: "/api/designer/connections",
			body:     `{"name":"GIS","provider":"postgis","host":"db.internal","database":"gis"}`,
		},
		{
			name:     "edit",
			method:   http.MethodPut,
			public:   "/api/designer/sales-report/connections/17",
			upstream: "/api/designer/connections/17",
			body:     `{"id":"17","name":"GIS","provider":"postgis"}`,
		},
		{
			name:     "delete",
			method:   http.MethodDelete,
			public:   "/api/designer/sales-report/connections/17",
			upstream: "/api/designer/connections/17",
		},
		{
			name:     "duplicate",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/connections/17/duplicate",
			upstream: "/api/designer/connections/17/duplicate",
			body:     `{}`,
		},
		{
			name:     "schemas",
			method:   http.MethodGet,
			public:   "/api/designer/sales-report/connections/17/schemas?provider=postgis",
			upstream: "/api/designer/connections/17/schemas",
			query:    "provider=postgis",
		},
		{
			name:     "tables",
			method:   http.MethodGet,
			public:   "/api/designer/sales-report/connections/17/tables?schema=public&provider=postgis",
			upstream: "/api/designer/connections/17/tables",
			query:    "provider=postgis&schema=public",
		},
		{
			name:     "columns",
			method:   http.MethodGet,
			public:   "/api/designer/sales-report/connections/17/columns?schema=public&table=cities&provider=postgis",
			upstream: "/api/designer/connections/17/columns",
			query:    "provider=postgis&schema=public&table=cities",
		},
		{
			name:     "foreign keys",
			method:   http.MethodGet,
			public:   "/api/designer/sales-report/connections/17/foreign-keys?schema=public&table=cities",
			upstream: "/api/designer/connections/17/foreign-keys",
			query:    "schema=public&table=cities",
		},
		{
			name:     "suggest joins",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/connections/17/suggest-joins",
			upstream: "/api/designer/connections/17/suggest-joins",
			body:     `{"existing":[],"table":{"name":"cities"}}`,
		},
		{
			name:     "query preview",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/connections/17/preview",
			upstream: "/api/designer/connections/17/preview",
			body:     `{"sql":"SELECT 1","params":{}}`,
		},
		{
			name:     "geometry",
			method:   http.MethodGet,
			public:   "/api/designer/sales-report/connections/17/geometry?schema=public&table=cities&column=geom",
			upstream: "/api/designer/connections/17/geometry",
			query:    "column=geom&schema=public&table=cities",
		},
		{
			name:     "arcgis layers",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/connections/arcgis/layers",
			upstream: "/api/designer/connections/arcgis/layers",
			body:     `{"id":"4","provider":"arcgis"}`,
		},
		{
			name:     "map discover",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/maps/discover",
			upstream: "/api/designer/maps/discover",
			body:     `{"connectionId":"4"}`,
		},
		{
			name:     "calculated fields",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/sessions/" + sessionID + "/calculated-fields",
			upstream: "/api/designer/sessions/" + sessionID + "/calculated-fields",
			body:     `{"name":"FullName","expression":"F{first}"}`,
		},
		{
			name:     "summaries",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/sessions/" + sessionID + "/summary-expressions",
			upstream: "/api/designer/sessions/" + sessionID + "/summary-expressions",
			body:     `{"name":"Total","expression":"SUM(F{amount})"}`,
		},
		{
			name:     "analysis",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/sessions/" + sessionID + "/analysis/sql",
			upstream: "/api/designer/sessions/" + sessionID + "/analysis/sql",
			body:     `{"sourceSql":"SELECT 1","spec":{}}`,
		},
		{
			name:     "export",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/sessions/" + sessionID + "/export",
			upstream: "/api/designer/sessions/" + sessionID + "/export",
			body:     `{"name":"sales.jrxml"}`,
		},
		{
			name:     "project save",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/sessions/" + sessionID + "/save",
			upstream: "/api/designer/sessions/" + sessionID + "/save",
			body:     `{}`,
		},
		{
			name:     "project open",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/sessions/" + sessionID + "/open",
			upstream: "/api/designer/sessions/" + sessionID + "/open",
			body:     `{"report":"sales-report"}`,
		},
		{
			name:     "report preview",
			method:   http.MethodPost,
			public:   "/api/designer/sales-report/sessions/" + sessionID + "/preview",
			upstream: "/api/designer/sessions/" + sessionID + "/preview",
			body:     `{}`,
		},
		{
			name:     "themes",
			method:   http.MethodGet,
			public:   "/api/designer/sales-report/themes",
			upstream: "/api/designer/themes",
		},
		{
			name:     "templates",
			method:   http.MethodGet,
			public:   "/api/designer/sales-report/templates",
			upstream: "/api/designer/templates",
		},
		{
			name:     "report list",
			method:   http.MethodGet,
			public:   "/api/designer/sales-report/reports",
			upstream: "/api/designer/reports",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := capd.hits.Load()
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.public, body)
			req.Header.Set("Accept", "application/json")
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set(headerDesignerSession, sessionID)
			req.Header.Set(headerDesignerToken, "browser-token")
			req.Header.Set(headerDesignerReportKey, "other-report")
			withSession(req, "editor-token")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s: %d %s", tc.method, tc.public, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "not found") {
				t.Fatalf("proxy answered not found: %s", rec.Body.String())
			}
			if capd.hits.Load() != before+1 {
				t.Fatalf("upstream hits %d, want %d", capd.hits.Load(), before+1)
			}
			capd.mu.Lock()
			gotMethod := capd.lastMethod
			gotPath := capd.lastPath
			gotQ := capd.lastQ
			gotBody := capd.body
			gotHdr := capd.lastHdr.Clone()
			capd.mu.Unlock()
			if gotMethod != tc.method {
				t.Fatalf("upstream method %s, want %s", gotMethod, tc.method)
			}
			if gotPath != tc.upstream {
				t.Fatalf("upstream path %s, want %s", gotPath, tc.upstream)
			}
			if gotHdr.Get(headerDesignerToken) != designerTestToken {
				t.Fatalf("token %q", gotHdr.Get(headerDesignerToken))
			}
			if gotHdr.Get(headerDesignerUserID) != "17" || gotHdr.Get(headerDesignerUsername) != "ada" {
				t.Fatalf("identity id=%s user=%s", gotHdr.Get(headerDesignerUserID), gotHdr.Get(headerDesignerUsername))
			}
			if gotHdr.Get(headerDesignerReportKey) != "sales-report" {
				t.Fatalf("report key %s", gotHdr.Get(headerDesignerReportKey))
			}
			if gotHdr.Get(headerDesignerSession) != sessionID {
				t.Fatalf("session %s", gotHdr.Get(headerDesignerSession))
			}
			if tc.body != "" {
				if gotHdr.Get("Content-Type") != "application/json" {
					t.Fatalf("content type %q", gotHdr.Get("Content-Type"))
				}
				if gotBody != tc.body {
					t.Fatalf("body %s", gotBody)
				}
			}
			if tc.query == "" {
				if gotQ != "" {
					t.Fatalf("unexpected query %s", gotQ)
				}
			} else if !sameQuery(gotQ, tc.query) {
				t.Fatalf("query %s, want %s", gotQ, tc.query)
			}
		})
	}

	// Paths outside the Designer API surface stay on Report Server.
	blocked := []string{
		"/api/designer/sales-report/internal/designer/open",
		"/api/designer/sales-report/maps/other",
		"/api/designer/sales-report/reports/import",
		"/api/designer/sales-report/sessions/not-a-session-id/save",
		"/api/designer/sales-report/connections/../sessions/" + sessionID + "/save",
	}
	hits := capd.hits.Load()
	for _, path := range blocked {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set(headerDesignerSession, sessionID)
		withSession(req, "editor-token")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "not found") {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	if capd.hits.Load() != hits {
		t.Fatalf("rejected paths were forwarded: %d", capd.hits.Load()-hits)
	}
}

func TestDesignerProxyPreservesNotImplemented(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(headerDesignerToken) != designerTestToken {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/designer/connections":
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"code":"not_available","message":"Datasource administration stays in Report Server"}`))
		case "/api/designer/maps/discover":
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"code":"not_available","message":"This Designer operation is not available through Report Server yet"}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom ` + designerTestToken + `"}`))
		}
	}))
	defer up.Close()
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	sessionID := "0123456789abcdef0123456789abcdef"

	req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/connections", strings.NewReader(`{"name":"GIS"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, sessionID)
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), "Datasource administration stays in Report Server") {
		t.Fatalf("connection admin: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/maps/discover", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, sessionID)
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), "This Designer operation is not available through Report Server yet") {
		t.Fatalf("map discover: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/sessions/"+sessionID+"/save", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, sessionID)
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "Designer unavailable") || strings.Contains(rec.Body.String(), designerTestToken) {
		t.Fatalf("upstream 500: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDesignerProxyPreservesSafeConnectionErrors(t *testing.T) {
	const (
		postedPassword = "S3cret-Connection-Password"
		requestID      = "req-upstream-err-1"
		sqlMessage     = `pq: password authentication failed for user "gis"`
	)
	postedBody := `{"provider":"postgis","host":"db.internal","database":"gis","username":"gis","password":"` + postedPassword + `"}`
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	sessionID := "0123456789abcdef0123456789abcdef"

	var logs bytes.Buffer
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if r.Header.Get(headerDesignerToken) != designerTestToken {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		writeUpstream := func(status int, payload any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(payload)
		}
		switch r.URL.RawQuery {
		case "case=sql":
			writeUpstream(http.StatusBadGateway, map[string]any{
				"code":     "sql",
				"message":  sqlMessage,
				"category": "sql",
				"stack":    "goroutine 1\n/var/lib/acugis/secret.key",
				"password": postedPassword,
			})
		case "case=timeout":
			writeUpstream(http.StatusGatewayTimeout, map[string]string{
				"code":    "timeout",
				"message": "The database did not respond in time",
			})
		case "case=map-timeout":
			writeUpstream(http.StatusGatewayTimeout, map[string]string{
				"code":    "timeout",
				"message": "The map service did not respond in time",
			})
		case "case=datasources":
			writeUpstream(http.StatusServiceUnavailable, map[string]string{
				"code":    "not_available",
				"message": "Report Server datasources are unavailable",
			})
		case "case=leak":
			writeUpstream(http.StatusInternalServerError, map[string]string{
				"code":    "runtime",
				"message": "password=" + postedPassword + "\n/var/lib/acugis/secret.key\ngoroutine 1 [running]:\nmain.go:12",
				"detail":  postedPassword,
				"stack":   "goroutine 1",
			})
		case "case=malformed":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("upstream exploded " + postedPassword + " /var/lib/acugis/secret.key"))
		case "case=sql-unsafe":
			writeUpstream(http.StatusBadGateway, map[string]string{
				"code":     "sql",
				"category": "sql",
				"message":  "dial tcp /var/lib/acugis/secret.key: password=" + postedPassword,
			})
		default:
			writeUpstream(http.StatusInternalServerError, map[string]string{
				"message": "boom " + designerTestToken,
			})
		}
	}))
	defer up.Close()
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	s.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	post := func(rawQuery string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/connections/test?"+rawQuery, strings.NewReader(postedBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set(headerDesignerSession, sessionID)
		req.Header.Set("X-Request-ID", requestID)
		withSession(req, "editor-token")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}

	t.Run("sql reaches browser", func(t *testing.T) {
		rec := post("case=sql")
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got["code"] != "sql" || got["category"] != "sql" || got["message"] != sqlMessage || got["error"] != sqlMessage {
			t.Fatalf("body %#v", got)
		}
		if _, ok := got["stack"]; ok {
			t.Fatalf("stack leaked: %#v", got)
		}
		if _, ok := got["password"]; ok {
			t.Fatalf("password field leaked: %#v", got)
		}
		if strings.Contains(rec.Body.String(), postedPassword) || strings.Contains(rec.Body.String(), "/var/lib") || strings.Contains(rec.Body.String(), "goroutine") {
			t.Fatalf("response leaked upstream detail: %s", rec.Body.String())
		}
	})

	t.Run("timeout reaches browser", func(t *testing.T) {
		rec := post("case=timeout")
		if rec.Code != http.StatusGatewayTimeout {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got["code"] != "timeout" || got["message"] != "The database did not respond in time" || got["error"] != "The database did not respond in time" {
			t.Fatalf("body %#v", got)
		}
		if strings.Contains(rec.Body.String(), "Designer unavailable") {
			t.Fatalf("timeout replaced: %s", rec.Body.String())
		}
	})

	t.Run("map timeout reaches browser", func(t *testing.T) {
		rec := post("case=map-timeout")
		if rec.Code != http.StatusGatewayTimeout || !strings.Contains(rec.Body.String(), "The map service did not respond in time") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("datasource unavailable stays specific", func(t *testing.T) {
		rec := post("case=datasources")
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "Report Server datasources are unavailable") || strings.Contains(rec.Body.String(), "Designer unavailable") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("untrusted 5xx does not leak", func(t *testing.T) {
		rec := post("case=leak")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Designer unavailable") {
			t.Fatalf("body %s", body)
		}
		for _, leak := range []string{postedPassword, "/var/lib", "goroutine", "main.go", "stack", designerTestToken} {
			if strings.Contains(body, leak) {
				t.Fatalf("response leaked %q: %s", leak, body)
			}
		}
		text := logs.String()
		if !strings.Contains(text, "upstream_status=500") || !strings.Contains(text, "code=runtime") || !strings.Contains(text, "path=/api/designer/sales-report/connections/test") || !strings.Contains(text, "request_id="+requestID) {
			t.Fatalf("translation log missing diagnostics:\n%s", text)
		}
	})

	t.Run("malformed upstream becomes unavailable", func(t *testing.T) {
		rec := post("case=malformed")
		body := rec.Body.String()
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(body, "Designer unavailable") {
			t.Fatalf("status %d body %s", rec.Code, body)
		}
		if strings.Contains(body, postedPassword) || strings.Contains(body, "/var/lib") {
			t.Fatalf("malformed body leaked: %s", body)
		}
		text := logs.String()
		if !strings.Contains(text, "upstream_status=502") || !strings.Contains(text, "code=unstructured") {
			t.Fatalf("malformed translation log:\n%s", text)
		}
	})

	t.Run("unsafe sql message is not forwarded", func(t *testing.T) {
		rec := post("case=sql-unsafe")
		body := rec.Body.String()
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(body, "Designer unavailable") {
			t.Fatalf("status %d body %s", rec.Code, body)
		}
		if strings.Contains(body, postedPassword) || strings.Contains(body, "/var/lib") {
			t.Fatalf("unsafe sql leaked: %s", body)
		}
	})

	if strings.Contains(logs.String(), postedPassword) || strings.Contains(logs.String(), `"password"`) || strings.Contains(logs.String(), "db.internal") {
		t.Fatalf("logs contain connection credentials or request body:\n%s", logs.String())
	}
}

func TestDesignerProxyTransportFailureStaysUnavailable(t *testing.T) {
	const postedPassword = "S3cret-Connection-Password"
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	var logs bytes.Buffer
	s, _ := designerFixture(t, editorACL, repo, "http://127.0.0.1:1", designerTestToken)
	s.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	body := `{"provider":"postgis","password":"` + postedPassword + `","host":"db.internal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/connections/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "Designer unavailable") {
		t.Fatalf("transport: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), postedPassword) || strings.Contains(rec.Body.String(), "127.0.0.1") || strings.Contains(rec.Body.String(), designerTestToken) {
		t.Fatalf("transport leaked: %s", rec.Body.String())
	}
	if strings.Contains(logs.String(), postedPassword) || strings.Contains(logs.String(), "db.internal") {
		t.Fatalf("transport logs contain credentials:\n%s", logs.String())
	}
}

func TestDesignerProxySessionMappingsUnchanged(t *testing.T) {
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.RawQuery {
		case "case=session_not_found":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"session_not_found","message":"Session not found"}`))
		case "case=session_mismatch":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"session_mismatch","message":"Session identifier mismatch"}`))
		case "case=session_expired":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"session_expired","message":"Session expired"}`))
		case "case=forbidden":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":"forbidden","message":"Session does not match the authorized user and report"}`))
		case "case=not_editable":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"not_editable","message":"Report is not Designer-editable."}`))
		case "case=unauthenticated":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"unauthenticated","message":"authentication required ` + designerTestToken + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer up.Close()
	s, _ := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	sessionID := "0123456789abcdef0123456789abcdef"

	cases := []struct {
		query  string
		status int
		want   string
	}{
		{"case=session_not_found", http.StatusNotFound, "session expired"},
		{"case=session_mismatch", http.StatusBadRequest, "Session identifier mismatch"},
		{"case=session_expired", http.StatusUnauthorized, "session expired"},
		{"case=forbidden", http.StatusForbidden, "Permission denied"},
		{"case=not_editable", http.StatusConflict, "Report is not Designer-editable."},
		{"case=unauthenticated", http.StatusServiceUnavailable, "Designer unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/connections/test?"+tc.query, strings.NewReader(`{"password":"S3cret-Connection-Password"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(headerDesignerSession, sessionID)
			withSession(req, "editor-token")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("%s: %d %s", tc.query, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), designerTestToken) || strings.Contains(rec.Body.String(), "S3cret-Connection-Password") {
				t.Fatalf("mapping leaked: %s", rec.Body.String())
			}
		})
	}

	req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/connections/test", strings.NewReader(`{"password":"S3cret-Connection-Password"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "Designer unavailable") {
		t.Fatalf("local auth: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPublishedJRXMLEditInDesigner(t *testing.T) {
	var hits atomic.Int32
	var opened atomic.Value
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/designer/open" {
			t.Errorf("unexpected designer call %s", r.URL.Path)
			http.Error(w, "no", http.StatusBadGateway)
			return
		}
		hits.Add(1)
		key := r.Header.Get(headerDesignerReportKey)
		opened.Store(key)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"0123456789abcdef0123456789abcdef","reportKey":"` + key + `","reused":false}`))
	}))
	defer up.Close()
	editorACL := &designerACL{}
	editorACL.allow(true, map[string]bool{"legacy": true, "compiled-only": true, "sales-report": true}, map[string]bool{"legacy": true, "compiled-only": true, "sales-report": true})
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
	}}
	s, store := designerFixture(t, editorACL, repo, up.URL, designerTestToken)
	if err := store.SaveJSON("compiled-only", map[string]any{"name": "Compiled"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.JRXMLDir(), "compiled-only.jasper"), []byte("jasper"), 0o644); err != nil {
		t.Fatal(err)
	}

	user := &auth.User{ID: 17, Username: "ada", Enabled: true}
	areq := httptest.NewRequest(http.MethodGet, "/reports/legacy", nil)
	areq = areq.WithContext(auth.WithUser(areq.Context(), user))
	native, can := s.annotateDesignerReport(areq, user, "legacy")
	if native || !can {
		t.Fatalf("published jrxml: native=%v can=%v", native, can)
	}
	native, can = s.annotateDesignerReport(areq, user, "compiled-only")
	if native || can {
		t.Fatalf("compiled report: native=%v can=%v", native, can)
	}

	req := httptest.NewRequest(http.MethodGet, "/reports/legacy/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"reportKey":"legacy"`) || !strings.Contains(rec.Body.String(), `"designer":true`) {
		t.Fatalf("design: %d %s", rec.Code, rec.Body.String())
	}
	gotKey, _ := opened.Load().(string)
	if hits.Load() != 1 || gotKey != "legacy" {
		t.Fatalf("designer open hits=%d key=%q", hits.Load(), gotKey)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/compiled-only/design", nil)
	req.Header.Set("Accept", "application/json")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "not Designer-editable") {
		t.Fatalf("compiled design: %d %s", rec.Code, rec.Body.String())
	}
	if hits.Load() != 1 {
		t.Fatalf("compiled report contacted designer: hits=%d", hits.Load())
	}
}

func sameQuery(got, want string) bool {
	gq, err1 := url.ParseQuery(got)
	wq, err2 := url.ParseQuery(want)
	if err1 != nil || err2 != nil {
		return false
	}
	return gq.Encode() == wq.Encode()
}
