package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/embedorigins"
	"github.com/acugis/acugis-report-server-go/internal/embedtoken"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func newEmbedOriginServer(env []string, authDisabled bool) (*Server, *embedorigins.Policy) {
	policy := embedorigins.New(embedorigins.NewMemory())
	cfg := testConfig()
	cfg.AuthDisabled = authDisabled
	cfg.EmbedAllowedOrigins = append([]string(nil), env...)
	cfg.EmbedTokenSecret = "unit-test-embed-secret"
	cfg.EmbedTokenTTL = time.Minute
	s := New(cfg, Dependencies{
		Auth:         auth.NewService(nil, auth.Options{AuthDisabled: authDisabled}),
		ACL:          acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: authDisabled}),
		EmbedOrigins: policy,
	}, slog.Default())
	return s, policy
}

func embedAdminUser() *auth.User {
	return &auth.User{ID: 1, Username: "admin", IsAdmin: true, Enabled: true}
}

func doEmbedOrigin(s *Server, method, origin string, user *auth.User) *httptest.ResponseRecorder {
	var body io.Reader
	if method != http.MethodGet {
		raw, _ := json.Marshal(map[string]string{"origin": origin})
		body = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, "/api/v1/admin/embed/origins", body)
	if user != nil {
		req = req.WithContext(auth.WithUser(req.Context(), user))
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeOrigins(t *testing.T, rec *httptest.ResponseRecorder) []embedorigins.Entry {
	t.Helper()
	var payload struct {
		Origins []embedorigins.Entry `json:"origins"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return payload.Origins
}

func TestAdminEmbedOriginsCRUD(t *testing.T) {
	s, _ := newEmbedOriginServer([]string{"https://env.example.com", "*"}, false)
	admin := embedAdminUser()

	rec := doEmbedOrigin(s, http.MethodGet, "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeOrigins(t, rec)
	if len(listed) != 2 || listed[0].Origin != "https://env.example.com" || listed[0].Removable || listed[1].Origin != "*" || listed[1].Removable {
		t.Fatalf("list=%+v", listed)
	}

	rec = doEmbedOrigin(s, http.MethodPost, "https://APP.Example.com:443", admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add status=%d body=%s", rec.Code, rec.Body.String())
	}
	added := decodeOrigins(t, rec)
	if len(added) != 3 || added[2].Origin != "https://app.example.com" || !added[2].Removable || added[2].Source != embedorigins.SourceAdmin {
		t.Fatalf("added=%+v", added)
	}

	rec = doEmbedOrigin(s, http.MethodPost, "https://app.example.com", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := decodeOrigins(t, rec); len(got) != 3 {
		t.Fatalf("duplicate created another row: %+v", got)
	}

	rec = doEmbedOrigin(s, http.MethodDelete, "https://env.example.com", admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete system status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doEmbedOrigin(s, http.MethodDelete, "*", admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete * status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doEmbedOrigin(s, http.MethodGet, "", admin)
	if got := decodeOrigins(t, rec); len(got) != 3 || got[0].Origin != "https://env.example.com" || got[1].Origin != "*" {
		t.Fatalf("system origins changed: %+v", got)
	}

	rec = doEmbedOrigin(s, http.MethodDelete, "https://app.example.com", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete admin status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := decodeOrigins(t, rec); len(got) != 2 {
		t.Fatalf("after delete=%+v", got)
	}
}

func TestAdminEmbedOriginsRejectsInvalid(t *testing.T) {
	s, _ := newEmbedOriginServer(nil, false)
	admin := embedAdminUser()
	rejected := []string{
		"*",
		"*.example.com",
		"https://app.example.com/path",
		"https://app.example.com?x=1",
		"https://app.example.com#frag",
		"javascript:alert(1)",
		"https://user:pass@app.example.com",
		"example.com",
	}
	for _, origin := range rejected {
		rec := doEmbedOrigin(s, http.MethodPost, origin, admin)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("origin %q status=%d body=%s", origin, rec.Code, rec.Body.String())
		}
	}
	if got := decodeOrigins(t, doEmbedOrigin(s, http.MethodGet, "", admin)); len(got) != 0 {
		t.Fatalf("invalid origins were stored: %+v", got)
	}
}

func TestAdminEmbedOriginsNonAdminDenied(t *testing.T) {
	s, _ := newEmbedOriginServer(nil, false)
	user := &auth.User{ID: 2, Username: "bob", IsAdmin: false, Enabled: true}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		rec := doEmbedOrigin(s, method, "https://app.example.com", user)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d body=%s", method, rec.Code, rec.Body.String())
		}
	}
	anon := doEmbedOrigin(s, http.MethodPost, "https://app.example.com", nil)
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status=%d body=%s", anon.Code, anon.Body.String())
	}
}

func TestAdminEmbeddedAnalyticsPage(t *testing.T) {
	s, _ := newEmbedOriginServer(nil, false)
	admin := embedAdminUser()
	req := httptest.NewRequest(http.MethodGet, "/admin/embedded-analytics", nil)
	req = req.WithContext(auth.WithUser(req.Context(), admin))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("page status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Embedded Analytics",
		"Allowed Origins",
		"Applications hosted on these origins may embed reports and dashboards from this Report Server.",
		"Add Origin",
		"System configured",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing %q", want)
		}
	}
	for _, hidden := range []string{"ACUGIS_EMBED_ALLOWED_ORIGINS", "frame-ancestors", "environment variable"} {
		if strings.Contains(body, hidden) {
			t.Fatalf("page exposes %q", hidden)
		}
	}

	phpReq := httptest.NewRequest(http.MethodGet, "/admin/embedded-analytics.php", nil)
	phpReq = phpReq.WithContext(auth.WithUser(phpReq.Context(), admin))
	phpRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(phpRec, phpReq)
	if phpRec.Code != http.StatusMovedPermanently || phpRec.Header().Get("Location") != "/admin/embedded-analytics" {
		t.Fatalf("php alias status=%d location=%q", phpRec.Code, phpRec.Header().Get("Location"))
	}

	denied := httptest.NewRequest(http.MethodGet, "/admin/embedded-analytics", nil)
	denied = denied.WithContext(auth.WithUser(denied.Context(), &auth.User{ID: 2, Username: "bob", IsAdmin: false, Enabled: true}))
	deniedRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(deniedRec, denied)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin page status=%d", deniedRec.Code)
	}
}

func TestAdministrationHubListsEmbeddedAnalytics(t *testing.T) {
	s, _ := newEmbedOriginServer(nil, false)
	meta, ok := s.hubContentForEdition("administration")
	if !ok {
		t.Fatal("administration hub missing")
	}
	found := false
	for _, item := range meta.Items {
		if item.Label == "Embedded Analytics" && item.Href == "admin/embedded-analytics" {
			found = true
		}
	}
	if !found {
		t.Fatalf("hub items=%+v", meta.Items)
	}
}

func TestEffectiveOriginsApplyWithoutRestart(t *testing.T) {
	s, _ := newEmbedOriginServer([]string{"https://env.example.com", "*"}, true)

	csp := embedCSP(t, s)
	if !strings.Contains(csp, "https://env.example.com") || !strings.Contains(csp, "*") {
		t.Fatalf("csp=%q", csp)
	}
	if strings.Contains(csp, "admin.example.com") {
		t.Fatalf("admin origin present before add: %q", csp)
	}
	if corsAllowed(t, s, "https://admin.example.com") {
		t.Fatal("CORS allowed before add")
	}

	rec := doEmbedOrigin(s, http.MethodPost, "https://admin.example.com", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add status=%d body=%s", rec.Code, rec.Body.String())
	}
	csp = embedCSP(t, s)
	if !strings.Contains(csp, "https://env.example.com") || !strings.Contains(csp, "*") || !strings.Contains(csp, "https://admin.example.com") {
		t.Fatalf("merged csp=%q", csp)
	}
	if !corsAllowed(t, s, "https://admin.example.com") {
		t.Fatal("CORS did not pick up the admin origin")
	}
	if corsAllowed(t, s, "https://evil.example.com") {
		t.Fatal("wildcard environment origin must not become a CORS matcher")
	}

	mint := mintEmbed(t, s, "https://evil.example.com")
	if mint.Code != http.StatusForbidden || !strings.Contains(mint.Body.String(), "EMBED_ORIGIN_DENIED") {
		t.Fatalf("unauthorized mint status=%d body=%s", mint.Code, mint.Body.String())
	}
	mint = mintEmbed(t, s, "https://admin.example.com")
	if strings.Contains(mint.Body.String(), "EMBED_ORIGIN_DENIED") {
		t.Fatalf("admin origin still denied: %s", mint.Body.String())
	}
	mint = mintEmbed(t, s, "https://env.example.com")
	if strings.Contains(mint.Body.String(), "EMBED_ORIGIN_DENIED") {
		t.Fatalf("environment origin denied: %s", mint.Body.String())
	}

	denied := embedWithOrigin(t, s, "https://token.example.com", "https://other.example.com")
	if denied.Code != http.StatusForbidden || !strings.Contains(denied.Body.String(), "EMBED_ORIGIN_DENIED") {
		t.Fatalf("token origin status=%d body=%s", denied.Code, denied.Body.String())
	}
	rec = doEmbedOrigin(s, http.MethodPost, "https://other.example.com", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add other status=%d", rec.Code)
	}
	allowed := embedWithOrigin(t, s, "https://token.example.com", "https://other.example.com")
	if strings.Contains(allowed.Body.String(), "EMBED_ORIGIN_DENIED") {
		t.Fatalf("token origin still denied after add: %s", allowed.Body.String())
	}

	rec = doEmbedOrigin(s, http.MethodDelete, "*", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete * status=%d", rec.Code)
	}
	if csp = embedCSP(t, s); !strings.Contains(csp, "frame-ancestors") || !strings.Contains(csp, "*") {
		t.Fatalf("csp lost wildcard: %q", csp)
	}
}

func TestEmptyEffectiveOriginsStayClosed(t *testing.T) {
	s, _ := newEmbedOriginServer(nil, true)
	csp := embedCSP(t, s)
	if csp != "frame-ancestors 'none'" {
		t.Fatalf("csp=%q", csp)
	}
	if corsAllowed(t, s, "https://app.example.com") {
		t.Fatal("empty list must not emit CORS")
	}
	mint := mintEmbed(t, s, "https://app.example.com")
	if mint.Code != http.StatusForbidden || !strings.Contains(mint.Body.String(), "EMBED_ORIGIN_DENIED") {
		t.Fatalf("mint status=%d body=%s", mint.Code, mint.Body.String())
	}

	dir := t.TempDir()
	writeEmbedReport(t, dir, "invoice")
	s.cfg.Paths.ReportJSONDir = dir
	s.deps.Reports = reports.NewStore(dir, "", "")
	tok := issueEmbedToken(t, s, embedtoken.ContentReport, "invoice", "")
	req := httptest.NewRequest(http.MethodGet, "/embed/report/invoice?et="+tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "window.ACUGIS_EMBED_ALLOWED_ORIGINS = []") {
		t.Fatal("empty viewer allow-list was not injected")
	}
}

func TestViewerPostMessageUsesEffectiveOrigins(t *testing.T) {
	dir := t.TempDir()
	writeEmbedReport(t, dir, "invoice")
	s, policy := newEmbedOriginServer([]string{"https://property-pro.example.com"}, true)
	s.cfg.Paths.ReportJSONDir = dir
	s.deps.Reports = reports.NewStore(dir, "", "")
	if _, err := policy.Add(t.Context(), s.cfg.EmbedAllowedOrigins, "https://ADMIN.example.com:443", 1); err != nil {
		t.Fatal(err)
	}
	tok := issueEmbedToken(t, s, embedtoken.ContentReport, "invoice", "https://property-pro.example.com")
	req := httptest.NewRequest(http.MethodGet, "/embed/report/invoice?et="+tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `window.ACUGIS_EMBED_ALLOWED_ORIGINS = ["https://property-pro.example.com","https://admin.example.com"]`) {
		t.Fatal("viewer postMessage allow-list is not the effective origin list")
	}
}

func embedCSP(t *testing.T, s *Server) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/embed/report/invoice", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Header().Get("Content-Security-Policy")
}

func corsAllowed(t *testing.T, s *Server, origin string) bool {
	t.Helper()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/embed/token", nil)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Header().Get("Access-Control-Allow-Origin") == origin
}

func mintEmbed(t *testing.T, s *Server, origin string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"content_type":"report","content_id":"invoice","origin":"` + origin + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/embed/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func embedWithOrigin(t *testing.T, s *Server, tokenOrigin, requestOrigin string) *httptest.ResponseRecorder {
	t.Helper()
	tok := issueEmbedToken(t, s, embedtoken.ContentReport, "invoice", tokenOrigin)
	req := httptest.NewRequest(http.MethodGet, "/embed/report/invoice?et="+tok, nil)
	req.Header.Set("Origin", requestOrigin)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func issueEmbedToken(t *testing.T, s *Server, ct embedtoken.ContentType, id, origin string) string {
	t.Helper()
	tok, _, err := embedtoken.Issue(s.cfg.EmbedTokenSecret, embedtoken.Claims{
		UserID:      1,
		Username:    "alice",
		ContentType: ct,
		ContentID:   id,
		Origin:      origin,
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func writeEmbedReport(t *testing.T, dir, key string) {
	t.Helper()
	body := `{
		"name": "Invoice",
		"report": "` + key + `",
		"datasource": "properties",
		"type": "jasper",
		"parameters": []
	}`
	if err := os.WriteFile(filepath.Join(dir, key+".report.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
