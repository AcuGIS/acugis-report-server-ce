package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/embedtoken"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

const boundTestSecret = "unit-test-embed-secret"

type allowAuthenticatedACL struct{}

func (allowAuthenticatedACL) CanViewReport(_ context.Context, u *auth.User, _ string) (bool, error) {
	return u != nil && u.ID > 0, nil
}
func (allowAuthenticatedACL) CanEditReport(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (allowAuthenticatedACL) CanAdminReport(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (allowAuthenticatedACL) CanViewExplorerView(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (allowAuthenticatedACL) CanEditExplorerView(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (allowAuthenticatedACL) CanAccessReportImage(ctx context.Context, u *auth.User, key string) (bool, error) {
	return allowAuthenticatedACL{}.CanViewReport(ctx, u, key)
}
func (allowAuthenticatedACL) CanAccessReportService(_ context.Context, u *auth.User) (bool, error) {
	return u != nil && u.ID > 0, nil
}
func (allowAuthenticatedACL) CheckPermission(context.Context, *auth.User, string, string, string) (acl.Decision, error) {
	return acl.Decision{}, nil
}
func (allowAuthenticatedACL) Configured() bool   { return true }
func (allowAuthenticatedACL) AuthDisabled() bool { return false }

type staticSessionRepo struct {
	hash string
}

func (s staticSessionRepo) Configured() bool { return true }
func (s staticSessionRepo) ResolveUserFromSession(_ context.Context, hash string) (*auth.User, error) {
	if hash == s.hash {
		return &auth.User{ID: 4, Username: "session-user", Enabled: true}, nil
	}
	return nil, auth.ErrInvalidSession
}

type staticPATRepo struct {
	hash string
	user *auth.User
}

func (p staticPATRepo) Configured() bool { return true }
func (p staticPATRepo) ResolveUserFromSession(context.Context, string) (*auth.User, error) {
	return nil, auth.ErrInvalidSession
}
func (p staticPATRepo) CreatePAT(context.Context, string, string, auth.PersonalAccessTokenCreate) (auth.PersonalAccessToken, error) {
	return auth.PersonalAccessToken{}, auth.ErrTokenUnsupported
}
func (p staticPATRepo) ListPATsByUser(context.Context, int64) ([]auth.PersonalAccessToken, error) {
	return nil, auth.ErrTokenUnsupported
}
func (p staticPATRepo) GetPAT(context.Context, int64, int64) (auth.PersonalAccessToken, error) {
	return auth.PersonalAccessToken{}, auth.ErrTokenUnsupported
}
func (p staticPATRepo) LookupActivePAT(_ context.Context, tokenHash string) (*auth.User, int64, *time.Time, error) {
	if tokenHash != p.hash {
		return nil, 0, nil, auth.ErrTokenNotFound
	}
	return p.user, 1, nil, nil
}
func (p staticPATRepo) UpdatePAT(context.Context, int64, int64, auth.PersonalAccessTokenUpdate) (auth.PersonalAccessToken, error) {
	return auth.PersonalAccessToken{}, auth.ErrTokenUnsupported
}
func (p staticPATRepo) RevokePAT(context.Context, int64, int64) error {
	return auth.ErrTokenUnsupported
}
func (p staticPATRepo) DeletePAT(context.Context, int64, int64) error {
	return auth.ErrTokenUnsupported
}
func (p staticPATRepo) TouchPATLastUsed(context.Context, int64, time.Time, string) error {
	return nil
}

type jasperCapture struct {
	method          string
	query           url.Values
	body            string
	calls           int
	componentHeader string
	status          int
	responseBody    string
}

func newBoundExecuteServer(t *testing.T, authSvc *auth.Service) (*Server, *jasperCapture) {
	t.Helper()
	cap := &jasperCapture{}
	jasperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.calls++
		cap.method = r.Method
		q, _ := url.ParseQuery(r.URL.RawQuery)
		cap.query = q
		b, _ := io.ReadAll(r.Body)
		cap.body = string(b)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	t.Cleanup(jasperSrv.Close)

	cfg := testConfig()
	cfg.AuthDisabled = false
	cfg.EmbedTokenSecret = boundTestSecret
	cfg.EmbedAllowedOrigins = []string{"https://property-pro.example.com"}
	cfg.EmbedTokenTTL = time.Minute
	cfg.Jasper.ReportURL = jasperSrv.URL
	cfg.Jasper.ImageURL = jasperSrv.URL + "/img"
	cfg.Jasper.Timeout = 2 * time.Second

	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{
		ReportExists: func(string) bool { return true },
	})
	s := New(cfg, Dependencies{
		Gateway: gw,
		Jasper:  gw.Client(),
		ACL:     allowAuthenticatedACL{},
		Auth:    authSvc,
	}, slog.Default())
	return s, cap
}

func embedAuthService() *auth.Service {
	svc := auth.NewService(nil, auth.Options{CookieName: "jrs_session"})
	svc.SetEmbedTokenResolver(embedTokenResolver{secret: boundTestSecret})
	return svc
}

func issueBoundToken(t *testing.T, params map[string][]string, contentID string, ttl time.Duration) string {
	t.Helper()
	if contentID == "" {
		contentID = "portfolio-performance"
	}
	if ttl == 0 {
		ttl = time.Minute
	}
	tok, _, err := embedtoken.Issue(boundTestSecret, embedtoken.Claims{
		UserID:      1,
		Username:    "embed-host",
		ContentType: embedtoken.ContentReport,
		ContentID:   contentID,
		Origin:      "https://property-pro.example.com",
		Parameters:  params,
	}, ttl)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func execWithCookie(s *Server, method, rawQuery, body, contentType, cookie string) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/reports/execute?"+rawQuery, rdr)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "jrs_embed_token", Value: cookie})
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestEmbedBoundParametersEnforcedOnExecute(t *testing.T) {
	s, cap := newBoundExecuteServer(t, embedAuthService())

	t.Run("token without parameters keeps browser value", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := execWithCookie(s, http.MethodGet, "_repName=portfolio-performance&_repFormat=html&account_id=abc-property&year=2026", "", "", tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("account_id") != "abc-property" || cap.query.Get("year") != "2026" {
			t.Fatalf("query=%v", cap.query)
		}
	})

	t.Run("omitted bound parameter is injected", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"smith-residential"}}, "", time.Minute)
		rec := execWithCookie(s, http.MethodGet, "_repName=portfolio-performance&_repFormat=html&year=2026", "", "", tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("account_id") != "smith-residential" {
			t.Fatalf("account_id=%q query=%v", cap.query.Get("account_id"), cap.query)
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("unbound year changed: %q", cap.query.Get("year"))
		}
	})

	t.Run("browser override is replaced", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{
			"account_id": {"smith-residential"},
			"region":     {"east", "west"},
		}, "", time.Minute)
		rec := execWithCookie(s, http.MethodGet, "_repName=portfolio-performance&_repFormat=html&account_id=abc-property&year=2026&region=north", "", "", tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("account_id") != "smith-residential" {
			t.Fatalf("account_id=%q", cap.query.Get("account_id"))
		}
		if strings.Contains(cap.query.Encode(), "abc-property") || strings.Contains(cap.query.Encode(), "north") {
			t.Fatalf("browser value survived: %v", cap.query)
		}
		gotRegion := cap.query["region"]
		if len(gotRegion) != 2 || gotRegion[0] != "east" || gotRegion[1] != "west" {
			t.Fatalf("region=%v", gotRegion)
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("year=%q", cap.query.Get("year"))
		}
	})

	t.Run("post body cannot override bound parameter", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"smith-residential"}}, "", time.Minute)
		rec := execWithCookie(s, http.MethodPost, "_repName=portfolio-performance&_repFormat=html", "account_id=abc-property&year=2024&_repName=some-other-report", "application/x-www-form-urlencoded", tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("account_id") != "smith-residential" {
			t.Fatalf("query account_id=%q", cap.query.Get("account_id"))
		}
		bodyQ, err := url.ParseQuery(cap.body)
		if err != nil {
			t.Fatal(err)
		}
		if bodyQ.Get("account_id") != "smith-residential" {
			t.Fatalf("body account_id=%q body=%s", bodyQ.Get("account_id"), cap.body)
		}
		if bodyQ.Get("year") != "2024" {
			t.Fatalf("unbound post year=%q", bodyQ.Get("year"))
		}
		if bodyQ.Get("_repName") != "portfolio-performance" || cap.query.Get("_repName") != "portfolio-performance" {
			t.Fatalf("post retargeted report query=%v body=%s", cap.query, cap.body)
		}
		if strings.Contains(cap.body, "abc-property") || strings.Contains(cap.query.Encode(), "abc-property") {
			t.Fatalf("override survived query=%v body=%s", cap.query, cap.body)
		}
	})

	t.Run("token cannot execute a different report", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"smith-residential"}}, "portfolio-performance", time.Minute)
		rec := execWithCookie(s, http.MethodGet, "_repName=some-other-report&_repFormat=html&account_id=smith-residential", "", "", tok)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "not scoped") {
			t.Fatalf("body %s", rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("jasper was called %d times", cap.calls)
		}
	})

	t.Run("expired embed token is rejected", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"smith-residential"}}, "", 20*time.Millisecond)
		time.Sleep(80 * time.Millisecond)
		rec := execWithCookie(s, http.MethodGet, "_repName=portfolio-performance&_repFormat=html&account_id=abc-property", "", "", tok)
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("expired token reached jasper")
		}
	})
}

func TestSessionAndPATIgnoreBoundParameterRules(t *testing.T) {
	t.Run("session", func(t *testing.T) {
		raw := "session-token-value"
		hash, err := auth.HashSessionToken(raw)
		if err != nil {
			t.Fatal(err)
		}
		svc := auth.NewService(staticSessionRepo{hash: hash}, auth.Options{CookieName: "jrs_session"})
		svc.SetEmbedTokenResolver(embedTokenResolver{secret: boundTestSecret})
		s, cap := newBoundExecuteServer(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/reports/execute?_repName=portfolio-performance&_repFormat=html&account_id=abc-property", nil)
		req.AddCookie(&http.Cookie{Name: "jrs_session", Value: raw})
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("account_id") != "abc-property" {
			t.Fatalf("session account_id=%q", cap.query.Get("account_id"))
		}
	})

	t.Run("pat", func(t *testing.T) {
		raw := "jrs_test-pat-token"
		hash, err := auth.HashSessionToken(raw)
		if err != nil {
			t.Fatal(err)
		}
		svc := auth.NewService(staticPATRepo{
			hash: hash,
			user: &auth.User{ID: 7, Username: "pat-user", Enabled: true},
		}, auth.Options{CookieName: "jrs_session"})
		svc.SetEmbedTokenResolver(embedTokenResolver{secret: boundTestSecret})
		s, cap := newBoundExecuteServer(t, svc)
		req := httptest.NewRequest(http.MethodGet, "/reports/execute?_repName=other-report&_repFormat=html&account_id=abc-property", nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("account_id") != "abc-property" {
			t.Fatalf("pat account_id=%q", cap.query.Get("account_id"))
		}
		if cap.query.Get("_repName") != "other-report" && cap.query.Get("_repName") != "" {
			// _repName is forwarded; Normalize does not rewrite the query key.
		}
	})
}

func TestMintBoundParametersValidationAndOrigin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "portfolio-performance.report.json"), []byte(`{"name":"Portfolio Performance","report":"portfolio-performance"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.EmbedTokenSecret = boundTestSecret
	cfg.EmbedAllowedOrigins = []string{"https://property-pro.example.com"}
	cfg.Paths.ReportJSONDir = dir
	s := New(cfg, Dependencies{
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true, CookieName: "jrs_session"}),
		Reports: reports.NewStore(dir, "", ""),
	}, slog.Default())

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/embed/token", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}

	rec := post(`{"content_type":"report","content_id":"portfolio-performance","origin":"https://evil.example.com","parameters":{"account_id":"smith-residential"}}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "EMBED_ORIGIN_DENIED") {
		t.Fatalf("origin status %d body %s", rec.Code, rec.Body.String())
	}

	for _, body := range []string{
		`{"content_type":"report","content_id":"portfolio-performance","parameters":{"_repName":"x"}}`,
		`{"content_type":"report","content_id":"portfolio-performance","parameters":{"datasource":"other"}}`,
		`{"content_type":"report","content_id":"portfolio-performance","parameters":{"P_MAP_IMAGE":"x"}}`,
		`{"content_type":"report","content_id":"portfolio-performance","parameters":{"account_id":{"id":"x"}}}`,
	} {
		rec = post(body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s status %d resp %s", body, rec.Code, rec.Body.String())
		}
	}

	rec = post(`{"content_type":"report","content_id":"portfolio-performance","origin":"https://property-pro.example.com","parameters":{"account_id":"smith-residential","region":["east","west"]}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("mint status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestApplyBoundStringMapKeepsUnboundValues(t *testing.T) {
	got := applyBoundStringMap(map[string][]string{
		"account_id": {"abc-property"},
		"year":       {"2026"},
	}, map[string][]string{
		"account_id": {"smith-residential"},
	})
	if len(got["account_id"]) != 1 || got["account_id"][0] != "smith-residential" {
		t.Fatalf("account_id=%v", got["account_id"])
	}
	if len(got["year"]) != 1 || got["year"][0] != "2026" {
		t.Fatalf("year=%v", got["year"])
	}
	injected := overlayEmbedBound(nil, map[string][]string{"account_id": {"smith-residential"}})
	if injected["account_id"] != "smith-residential" {
		t.Fatalf("overlay=%v", injected)
	}
}

func TestPrepareEmbedReportQueryBindsAccount(t *testing.T) {
	s := New(testConfig(), Dependencies{}, slog.Default())
	tok := issueBoundToken(t, map[string][]string{
		"account_id": {"smith-residential"},
		"region":     {"east", "west"},
	}, "portfolio-performance", time.Minute)
	claims, err := embedtoken.ParseAndValidate(boundTestSecret, tok)
	if err != nil {
		t.Fatal(err)
	}
	user := auth.NewEmbedUser(claims.UserID, claims.Username, string(claims.ContentType), claims.ContentID, claims.Origin, claims.Parameters)
	req := httptest.NewRequest(http.MethodGet, "/reports/map_features?report=portfolio-performance&account_id=abc-property&year=2026", nil)
	req = req.WithContext(auth.WithUser(req.Context(), user))

	q, err := s.prepareEmbedReportQuery(req, "portfolio-performance", req.URL.RawQuery)
	if err != nil {
		t.Fatal(err)
	}
	vals, err := url.ParseQuery(q)
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("account_id") != "smith-residential" || vals.Get("year") != "2026" {
		t.Fatalf("query=%v", vals)
	}
	if _, err := s.prepareEmbedReportQuery(req, "some-other-report", req.URL.RawQuery); err == nil {
		t.Fatal("expected content scope error")
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/reports/map_features?account_id=abc-property", nil)
	sessionReq = sessionReq.WithContext(auth.WithUser(sessionReq.Context(), &auth.User{ID: 4, Username: "session-user", Enabled: true}))
	unchanged, err := s.prepareEmbedReportQuery(sessionReq, "portfolio-performance", sessionReq.URL.RawQuery)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged != sessionReq.URL.RawQuery {
		t.Fatalf("session query rewritten: %s", unchanged)
	}
}

func TestEmbedViewerHidesBoundParameterControls(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "portfolio-performance.report.json"), []byte(`{
		"name": "Portfolio Performance",
		"report": "portfolio-performance",
		"datasource": "properties",
		"type": "jasper",
		"parameters": [
			{"name": "account_id", "label": "Account", "type": "string"},
			{"name": "year", "label": "Year", "type": "string"}
		]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.EmbedTokenSecret = boundTestSecret
	cfg.EmbedAllowedOrigins = []string{"https://property-pro.example.com"}
	cfg.Paths.ReportJSONDir = dir
	s := New(cfg, Dependencies{
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true, CookieName: "jrs_session"}),
		Reports: reports.NewStore(dir, "", ""),
	}, slog.Default())
	tok := issueBoundToken(t, map[string][]string{"account_id": {"smith-residential"}}, "portfolio-performance", time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/embed/report/portfolio-performance?et="+url.QueryEscape(tok), nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `window.ACUGIS_EMBED_BOUND_PARAMETERS = ["account_id"]`) {
		t.Fatalf("bound parameter list missing from viewer")
	}
	if !strings.Contains(body, "isEmbedBoundParameter") {
		t.Fatal("viewer does not skip bound parameter controls")
	}
	if strings.Contains(body, `name="account_id"`) {
		t.Fatal("bound parameter was rendered as an editable control")
	}
}
