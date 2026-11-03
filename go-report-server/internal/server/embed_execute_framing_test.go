package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/embedorigins"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

func newFramingExecuteServer(t *testing.T, authSvc *auth.Service, env []string, policy *embedorigins.Policy) (*Server, *jasperCapture) {
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
	cfg.EmbedAllowedOrigins = env
	cfg.EmbedTokenTTL = time.Minute
	cfg.Jasper.ReportURL = jasperSrv.URL
	cfg.Jasper.ImageURL = jasperSrv.URL + "/img"
	cfg.Jasper.Timeout = 2 * time.Second

	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{
		ReportExists: func(string) bool { return true },
	})
	s := New(cfg, Dependencies{
		Gateway:      gw,
		Jasper:       gw.Client(),
		ACL:          allowAuthenticatedACL{},
		Auth:         authSvc,
		EmbedOrigins: policy,
	}, slog.Default())
	return s, cap
}

func framingSession(t *testing.T) (*auth.Service, string) {
	t.Helper()
	raw := "session-token-value"
	hash, err := auth.HashSessionToken(raw)
	if err != nil {
		t.Fatal(err)
	}
	svc := auth.NewService(staticSessionRepo{hash: hash}, auth.Options{CookieName: "jrs_session"})
	svc.SetEmbedTokenResolver(embedTokenResolver{secret: boundTestSecret})
	return svc, raw
}

func doGet(s *Server, rawURL string, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, rawURL, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func assertSameOriginFraming(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("X-Frame-Options=%q", got)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); strings.Contains(csp, "frame-ancestors") {
		t.Fatalf("external framing permission in CSP=%q", csp)
	}
}

func assertEmbedFraming(t *testing.T, rec *httptest.ResponseRecorder, wantCSP string) {
	t.Helper()
	if got := rec.Header().Get("X-Frame-Options"); got != "" {
		t.Fatalf("X-Frame-Options=%q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != wantCSP {
		t.Fatalf("CSP=%q want %q", got, wantCSP)
	}
}

func TestEmbedExecuteFraming(t *testing.T) {
	envOrigin := "https://property-pro.example.com"
	executeURL := "/reports/execute?_repName=portfolio-performance&_repFormat=html"

	t.Run("session execute keeps sameorigin", func(t *testing.T) {
		svc, raw := framingSession(t)
		s, cap := newFramingExecuteServer(t, svc, []string{envOrigin}, nil)
		rec := doGet(s, executeURL, []*http.Cookie{{Name: "jrs_session", Value: raw}}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		assertSameOriginFraming(t, rec)
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("valid embed token uses frame ancestors", func(t *testing.T) {
		s, cap := newFramingExecuteServer(t, embedAuthService(), []string{envOrigin}, nil)
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, executeURL, []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		assertEmbedFraming(t, rec, "frame-ancestors 'self' "+envOrigin)
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("invalid embed cookie stays sameorigin", func(t *testing.T) {
		s, cap := newFramingExecuteServer(t, embedAuthService(), []string{envOrigin}, nil)
		rec := doGet(s, executeURL, []*http.Cookie{{Name: "jrs_embed_token", Value: "emb_not-a-valid-token"}}, nil)
		if rec.Code == http.StatusOK {
			t.Fatalf("invalid token executed: %s", rec.Body.String())
		}
		assertSameOriginFraming(t, rec)
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("embed flag without grant stays sameorigin", func(t *testing.T) {
		svc, raw := framingSession(t)
		s, cap := newFramingExecuteServer(t, svc, []string{envOrigin}, nil)
		rec := doGet(s, executeURL+"&embed=1", []*http.Cookie{{Name: "jrs_session", Value: raw}}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		assertSameOriginFraming(t, rec)
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("query token without grant stays sameorigin", func(t *testing.T) {
		svc, raw := framingSession(t)
		s, cap := newFramingExecuteServer(t, svc, []string{envOrigin}, nil)
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, executeURL+"&et="+tok, []*http.Cookie{{Name: "jrs_session", Value: raw}}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		assertSameOriginFraming(t, rec)
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("foreign origin and referer without grant stay sameorigin", func(t *testing.T) {
		svc, raw := framingSession(t)
		s, cap := newFramingExecuteServer(t, svc, []string{envOrigin}, nil)
		rec := doGet(s, executeURL, []*http.Cookie{{Name: "jrs_session", Value: raw}}, map[string]string{
			"Origin":  "https://evil.example",
			"Referer": "https://evil.example/app",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		assertSameOriginFraming(t, rec)
		if strings.Contains(rec.Header().Get("Content-Security-Policy"), "evil.example") {
			t.Fatalf("CSP reflected foreign origin: %q", rec.Header().Get("Content-Security-Policy"))
		}
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("expired embed token stays sameorigin", func(t *testing.T) {
		s, cap := newFramingExecuteServer(t, embedAuthService(), []string{envOrigin}, nil)
		tok := issueBoundToken(t, nil, "portfolio-performance", 20*time.Millisecond)
		time.Sleep(80 * time.Millisecond)
		rec := doGet(s, executeURL, []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if rec.Code == http.StatusOK {
			t.Fatalf("expired token executed: %s", rec.Body.String())
		}
		assertSameOriginFraming(t, rec)
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("token for report a cannot frame report b", func(t *testing.T) {
		s, cap := newFramingExecuteServer(t, embedAuthService(), []string{envOrigin}, nil)
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, "/reports/execute?_repName=other-report&_repFormat=html", []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "not scoped") {
			t.Fatalf("body %s", rec.Body.String())
		}
		assertSameOriginFraming(t, rec)
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("admin origin appears in execute csp", func(t *testing.T) {
		policy := embedorigins.New(embedorigins.NewMemory())
		if _, err := policy.Add(context.Background(), nil, "https://acugis.cloud", 1); err != nil {
			t.Fatal(err)
		}
		s, cap := newFramingExecuteServer(t, embedAuthService(), nil, policy)
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, executeURL, []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		assertEmbedFraming(t, rec, "frame-ancestors 'self' https://acugis.cloud")
		for _, origin := range s.effectiveEmbedOrigins() {
			if origin == "self" || origin == "'self'" {
				t.Fatalf("effective origins include self: %v", s.effectiveEmbedOrigins())
			}
		}
		for _, entry := range policy.Entries(nil) {
			if entry.Origin == "self" || entry.Origin == "'self'" {
				t.Fatalf("admin origin %q", entry.Origin)
			}
		}
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("environment and admin origins both appear", func(t *testing.T) {
		env := []string{"https://reports.example"}
		policy := embedorigins.New(embedorigins.NewMemory())
		if _, err := policy.Add(context.Background(), env, "https://acugis.cloud", 1); err != nil {
			t.Fatal(err)
		}
		s, cap := newFramingExecuteServer(t, embedAuthService(), env, policy)
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, executeURL, []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		assertEmbedFraming(t, rec, "frame-ancestors 'self' https://reports.example https://acugis.cloud")
		for _, origin := range s.effectiveEmbedOrigins() {
			if origin == "self" || origin == "'self'" {
				t.Fatalf("effective origins include self: %v", s.effectiveEmbedOrigins())
			}
		}
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("acugis cloud execute allows self and the external origin", func(t *testing.T) {
		s, cap := newFramingExecuteServer(t, embedAuthService(), []string{"https://acugis.cloud"}, nil)
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, executeURL, []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		assertEmbedFraming(t, rec, "frame-ancestors 'self' https://acugis.cloud")
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
		for _, origin := range s.effectiveEmbedOrigins() {
			if origin == "self" || origin == "'self'" {
				t.Fatalf("effective origins include self: %v", s.effectiveEmbedOrigins())
			}
		}
	})

	t.Run("empty effective origins stay none", func(t *testing.T) {
		// No external origin stays frame-ancestors 'none'. Adding 'self' here
		// would let any same-origin page frame the report after the external
		// allow-list was deliberately closed.
		s, cap := newFramingExecuteServer(t, embedAuthService(), nil, nil)
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, executeURL, []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		assertEmbedFraming(t, rec, "frame-ancestors 'none'")
		if strings.Contains(rec.Header().Get("Content-Security-Policy"), "self") {
			t.Fatalf("empty list must not add self: %q", rec.Header().Get("Content-Security-Policy"))
		}
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("dev report keeps sameorigin with valid embed grant", func(t *testing.T) {
		s, cap := newFramingExecuteServer(t, embedAuthService(), []string{envOrigin}, nil)
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, "/dev/report?_repName=portfolio-performance&_repFormat=html", []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		assertSameOriginFraming(t, rec)
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("embed routes and unrelated routes keep existing framing", func(t *testing.T) {
		s, _ := newFramingExecuteServer(t, embedAuthService(), []string{envOrigin}, nil)
		health := doGet(s, "/health", nil, nil)
		if health.Code != http.StatusOK {
			t.Fatalf("health status %d", health.Code)
		}
		assertSameOriginFraming(t, health)

		ready := doGet(s, "/ready", nil, nil)
		assertSameOriginFraming(t, ready)

		embed := doGet(s, "/embed/report/portfolio-performance", nil, nil)
		if embed.Code != http.StatusUnauthorized {
			t.Fatalf("embed without token status %d body %s", embed.Code, embed.Body.String())
		}
		if got := embed.Header().Get("X-Frame-Options"); got != "" {
			t.Fatalf("embed X-Frame-Options=%q", got)
		}
		if csp := embed.Header().Get("Content-Security-Policy"); csp != "frame-ancestors "+envOrigin {
			t.Fatalf("embed CSP=%q", csp)
		}

		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		embedOK := doGet(s, "/embed/report/portfolio-performance?et="+tok, nil, nil)
		if got := embedOK.Header().Get("X-Frame-Options"); got != "" {
			t.Fatalf("valid embed page X-Frame-Options=%q", got)
		}
		if csp := embedOK.Header().Get("Content-Security-Policy"); csp != "frame-ancestors "+envOrigin {
			t.Fatalf("valid embed page CSP=%q", csp)
		}
	})
}

func TestAuthenticatedExecuteFramingDoesNotPublishSelf(t *testing.T) {
	s, policy := newEmbedOriginServer([]string{"https://reports.example"}, false)
	if _, err := policy.Add(context.Background(), []string{"https://reports.example"}, "https://acugis.cloud", 1); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.applyAuthenticatedEmbedFraming(rec)
	if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'self' https://reports.example https://acugis.cloud" {
		t.Fatalf("CSP=%q", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "" {
		t.Fatalf("X-Frame-Options=%q", got)
	}
	for _, origin := range s.effectiveEmbedOrigins() {
		if origin == "self" || origin == "'self'" {
			t.Fatalf("effective origins include self: %v", s.effectiveEmbedOrigins())
		}
	}
	listed := decodeOrigins(t, doEmbedOrigin(s, http.MethodGet, "", embedAdminUser()))
	for _, entry := range listed {
		if entry.Origin == "self" || entry.Origin == "'self'" {
			t.Fatalf("admin listed %q", entry.Origin)
		}
	}
	if len(listed) != 2 || listed[0].Origin != "https://reports.example" || listed[1].Origin != "https://acugis.cloud" {
		t.Fatalf("admin origins=%+v", listed)
	}
}
