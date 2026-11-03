package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

type loginTestRepo struct {
	user *auth.User
	hash string
}

func (m *loginTestRepo) Configured() bool { return true }

func (m *loginTestRepo) ResolveUserFromSession(_ context.Context, tokenHash string) (*auth.User, error) {
	if m.hash != "" && tokenHash == m.hash && m.user != nil {
		return m.user, nil
	}
	return nil, auth.ErrInvalidSession
}

func (m *loginTestRepo) AuthenticatePassword(_ context.Context, username, password string) error {
	if username == "alice" && password == "secret" {
		return nil
	}
	return auth.ErrInvalidCredentials
}

func (m *loginTestRepo) CreateSession(_ context.Context, tokenHash, _ string, _ time.Time) error {
	m.hash = tokenHash
	return nil
}

func (m *loginTestRepo) DeleteSession(context.Context, string) error { return nil }

func authEnabledTestServer(t *testing.T, repo auth.SessionResolver, aclSvc acl.Service) *Server {
	t.Helper()
	cfg := testConfig()
	cfg.AuthDisabled = false
	if aclSvc == nil {
		aclSvc = denyACL{}
	}
	return New(cfg, Dependencies{
		Reports: reports.NewStore(t.TempDir(), "", ""),
		ACL:     aclSvc,
		Auth:    auth.NewService(repo, auth.Options{CookieName: "jrs_session", AuthDisabled: false}),
	}, slog.Default())
}

func TestUnauthenticatedBrowserRedirectsToLogin(t *testing.T) {
	s := authEnabledTestServer(t, nil, denyACL{})

	for _, path := range []string{"/", "/reports", "/reports/", "/profile", "/hubs/reports", "/schedules"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		loc := rec.Header().Get("Location")
		wantPrefix := "/login?next="
		if !strings.HasPrefix(loc, wantPrefix) {
			t.Fatalf("%s Location=%q", path, loc)
		}
		next, err := url.QueryUnescape(strings.TrimPrefix(loc, wantPrefix))
		if err != nil {
			t.Fatalf("%s next unescape: %v", path, err)
		}
		if next != path {
			t.Fatalf("%s next=%q", path, next)
		}
		if strings.Contains(rec.Body.String(), "authentication required") && rec.Code == http.StatusUnauthorized {
			t.Fatalf("%s leaked plain 401", path)
		}
	}
}

func TestLoginPageAccessibleWhileLoggedOut(t *testing.T) {
	s := authEnabledTestServer(t, nil, denyACL{})

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "" {
		t.Fatalf("login redirected: %s", rec.Header().Get("Location"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sign in") || !strings.Contains(body, `name="username"`) {
		t.Fatalf("login HTML missing form: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/login.php", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("login.php status=%d want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("login.php Location=%s", loc)
	}

	req = httptest.NewRequest(http.MethodPost, "/login.php", strings.NewReader("username=&password="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty POST status=%d (redirect loop? Location=%s)", rec.Code, rec.Header().Get("Location"))
	}
	if rec.Header().Get("Location") != "" {
		t.Fatalf("empty POST redirected to %s", rec.Header().Get("Location"))
	}
}

func TestLoginSucceedsAndDoesNotLoop(t *testing.T) {
	repo := &loginTestRepo{user: &auth.User{ID: 7, Username: "alice", Enabled: true, IsAdmin: true}}
	s := authEnabledTestServer(t, repo, denyACL{})

	form := url.Values{
		"username": {"alice"},
		"password": {"secret"},
		"next":     {"/reports/index.php"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login.php", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/reports/index.php" {
		t.Fatalf("login Location=%q", loc)
	}
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "jrs_session" && c.Value != "" {
			session = c
			break
		}
	}
	if session == nil {
		t.Fatal("login did not set jrs_session")
	}

	req = httptest.NewRequest(http.MethodGet, "/login", nil)
	req.AddCookie(session)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("authed login status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/reports/" {
		t.Fatalf("authed login Location=%q", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(session)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("authed catalog without ACL status=%d (want 403, not login redirect) body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("authenticated 403 redirected to %s", loc)
	}
}

func TestUnauthenticatedAPIKeepsJSON401(t *testing.T) {
	s := authEnabledTestServer(t, nil, denyACL{})

	for _, path := range []string{"/api/v1/reports", "/api/v1/auth/me", "/api/v1/auth/tokens"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		if loc := rec.Header().Get("Location"); loc != "" {
			t.Fatalf("%s redirected to login: %s", path, loc)
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Fatalf("%s Content-Type=%q body=%s", path, ct, rec.Body.String())
		}
		if strings.Contains(strings.ToLower(rec.Body.String()), "<html") {
			t.Fatalf("%s returned HTML login page: %s", path, rec.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s json: %v body=%s", path, err, rec.Body.String())
		}
		if body["error"] != "authentication required" {
			t.Fatalf("%s error=%q", path, body["error"])
		}
	}
}

func TestLoginRedirectHonorsBasePath(t *testing.T) {
	cfg := testConfig()
	cfg.AuthDisabled = false
	cfg.BasePath = "/go"
	s := New(cfg, Dependencies{
		Reports: reports.NewStore(t.TempDir(), "", ""),
		ACL:     denyACL{},
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: false}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/go/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/go/login?next=") {
		t.Fatalf("Location=%q", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/login.php", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("login.php status=%d Location=%s", rec.Code, rec.Header().Get("Location"))
	}
	if loc := rec.Header().Get("Location"); loc != "/go/login" {
		t.Fatalf("prefixed login.php Location=%s", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/go/login", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status=%d Location=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if rec.Header().Get("Location") != "" {
		t.Fatalf("prefixed login redirected: %s", rec.Header().Get("Location"))
	}
}

func TestLoginStaticAssetsRemainPublic(t *testing.T) {
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "reports", "portal.css"), `.portal-home{color:#26a69a}`)
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell.css"), `.fed-shell{}`)

	cfg := testConfig()
	cfg.AuthDisabled = false
	cfg.Paths.AppRoot = appRoot
	s := New(cfg, Dependencies{
		Reports: reports.NewStore(t.TempDir(), "", ""),
		ACL:     denyACL{},
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: false}),
	}, slog.Default())

	for _, path := range []string{"/reports/portal.css", "/shared/ui/shell.css"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		if loc := rec.Header().Get("Location"); loc != "" {
			t.Fatalf("%s redirected to %s", path, loc)
		}
	}
}

func TestWriteAuthzHTMLPermissionDeniedNotRedirect(t *testing.T) {
	s := New(testConfig(), Dependencies{}, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.writeAuthzHTML(rec, req, errPermissionDenied)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("403 redirected to %s", loc)
	}
}

func TestHealthRemainsPublic(t *testing.T) {
	s := authEnabledTestServer(t, nil, denyACL{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
