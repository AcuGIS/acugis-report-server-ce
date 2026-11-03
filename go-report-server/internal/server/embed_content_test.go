package server

import (
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

	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func newContentEmbedServer(t *testing.T, origins []string) (*Server, *jasperCapture) {
	t.Helper()
	cap := &jasperCapture{}
	jasperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.calls++
		cap.method = r.Method
		cap.componentHeader = r.Header.Get(jasper.EmbedComponentHeader)
		q, _ := url.ParseQuery(r.URL.RawQuery)
		cap.query = q
		b, _ := io.ReadAll(r.Body)
		cap.body = string(b)
		status := http.StatusOK
		body := "<html>ok</html>"
		if cap.status != 0 {
			status = cap.status
		}
		if cap.responseBody != "" {
			body = cap.responseBody
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(jasperSrv.Close)

	dir := t.TempDir()
	reportsJSON := map[string]string{
		"portfolio-performance.report.json": `{
		"name": "Portfolio Performance",
		"report": "portfolio-performance",
		"datasource": "properties",
		"type": "jasper",
		"parameters": [
			{"name": "year", "type": "number", "default": 2026},
			{"name": "account_id", "type": "string"},
			{"name": "property_id", "type": "string"},
			{"name": "as_of", "type": "date"},
			{"name": "include_vacant", "type": "boolean"},
			{"name": "region", "type": "select"},
			{"name": "_dataSource", "type": "string"},
			{"name": "datasource", "type": "string"}
		]
	}`,
		"name-fallback.report.json": `{
		"name": "Name Fallback",
		"report": "name-fallback",
		"datasourceName": "named-source",
		"type": "jasper"
	}`,
		"no-datasource.report.json": `{
		"name": "No Datasource",
		"report": "no-datasource",
		"type": "jasper"
	}`,
	}
	for name, body := range reportsJSON {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := testConfig()
	cfg.AuthDisabled = false
	cfg.EmbedTokenSecret = boundTestSecret
	cfg.EmbedAllowedOrigins = origins
	cfg.EmbedTokenTTL = time.Minute
	cfg.Jasper.ReportURL = jasperSrv.URL
	cfg.Jasper.ImageURL = jasperSrv.URL + "/img"
	cfg.Jasper.Timeout = 2 * time.Second
	cfg.Paths.ReportJSONDir = dir

	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{
		ReportExists: func(string) bool { return true },
	})
	s := New(cfg, Dependencies{
		Gateway: gw,
		Jasper:  gw.Client(),
		ACL:     allowAuthenticatedACL{},
		Auth:    embedAuthService(),
		Reports: reports.NewStore(dir, "", ""),
	}, slog.Default())
	return s, cap
}

func contentURL(token, rawQuery string) string {
	u := "/embed/report/portfolio-performance/content?et=" + url.QueryEscape(token)
	if rawQuery != "" {
		u += "&" + rawQuery
	}
	return u
}

func TestEmbedReportContent(t *testing.T) {
	origin := "https://acugis.cloud"
	s, cap := newContentEmbedServer(t, []string{origin})

	t.Run("missing token is rejected", func(t *testing.T) {
		cap.calls = 0
		rec := doGet(s, "/embed/report/portfolio-performance/content", nil, nil)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "embed token required") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
		if got := rec.Header().Get("X-Frame-Options"); got != "" {
			t.Fatalf("X-Frame-Options=%q", got)
		}
	})

	t.Run("jrs token is rejected", func(t *testing.T) {
		cap.calls = 0
		rec := doGet(s, "/embed/report/portfolio-performance/content?et=jrs_notallowed", nil, nil)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "personal access tokens") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("invalid token is rejected", func(t *testing.T) {
		cap.calls = 0
		rec := doGet(s, "/embed/report/portfolio-performance/content?et=emb_not-a-valid-token", nil, nil)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "TOKEN_INVALID") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, nil, "portfolio-performance", 20*time.Millisecond)
		time.Sleep(80 * time.Millisecond)
		rec := doGet(s, contentURL(tok, ""), nil, nil)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "TOKEN_EXPIRED") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("token for report a cannot request report b", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "other-report", time.Minute)
		rec := doGet(s, contentURL(tok, "year=2026"), nil, nil)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "not scoped") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("content is jasper html with parameters cookie and framing", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, "year=2026&property_id=123&account_id=smith-residential"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
		if cap.query.Get("_repName") != "portfolio-performance" {
			t.Fatalf("_repName=%q query=%v", cap.query.Get("_repName"), cap.query)
		}
		if cap.query.Get("_repFormat") != "html" {
			t.Fatalf("_repFormat=%q", cap.query.Get("_repFormat"))
		}
		if cap.query.Get("_dataSource") != "properties" {
			t.Fatalf("_dataSource=%q query=%v", cap.query.Get("_dataSource"), cap.query)
		}
		if cap.query.Get("year") != "2026" || cap.query.Get("property_id") != "123" {
			t.Fatalf("parameters query=%v", cap.query)
		}
		if cap.query.Get("account_id") != "abc-property" || strings.Contains(cap.query.Encode(), "smith-residential") {
			t.Fatalf("bound account_id query=%v", cap.query)
		}
		if cap.query.Get("et") != "" || strings.Contains(cap.query.Encode(), "emb_") {
			t.Fatalf("token forwarded to jasper: %v", cap.query)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "<html>ok</html>") {
			t.Fatalf("jasper html missing: %s", body)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("content-type %q", rec.Header().Get("Content-Type"))
		}
		for _, chrome := range []string{"paramSidebar", "Fit Width", "reportFrame", "<iframe", "applyBtn", ">Apply<"} {
			if strings.Contains(body, chrome) {
				t.Fatalf("viewer chrome %q in content response", chrome)
			}
		}
		if !strings.Contains(body, `source:"acugis-report"`) || !strings.Contains(body, `post("load"`) {
			t.Fatalf("content bridge missing from jasper html")
		}
		if strings.Contains(body, "_dataSource") || strings.Contains(body, "properties") {
			t.Fatalf("catalog datasource leaked into content document: %s", body)
		}

		var cookie *http.Cookie
		for _, c := range rec.Result().Cookies() {
			if c.Name == "jrs_embed_token" {
				cookie = c
			}
		}
		if cookie == nil || !cookie.HttpOnly || cookie.Value != tok || cookie.Path != "/" {
			t.Fatalf("embed cookie=%+v", cookie)
		}
		if got := rec.Header().Get("X-Frame-Options"); got != "" {
			t.Fatalf("X-Frame-Options=%q", got)
		}
		if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors "+origin {
			t.Fatalf("CSP=%q", got)
		}
	})

	t.Run("report identity and format cannot be overridden", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, "_repName=other-report&_repFormat=pdf&report=another-report&year=2026"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
		if cap.query.Get("_repName") != "portfolio-performance" || cap.query.Get("_repFormat") != "html" {
			t.Fatalf("identity query=%v", cap.query)
		}
		if cap.query.Get("report") != "" {
			t.Fatalf("report alias survived: %q", cap.query.Get("report"))
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("year=%q", cap.query.Get("year"))
		}
		if strings.Contains(cap.query.Encode(), "other-report") || strings.Contains(cap.query.Encode(), "another-report") || cap.query.Get("_repFormat") == "pdf" {
			t.Fatalf("override survived query=%v", cap.query)
		}
		if cap.query.Get("_dataSource") != "properties" {
			t.Fatalf("_dataSource=%q query=%v", cap.query.Get("_dataSource"), cap.query)
		}
	})

	t.Run("catalog datasource wins over client datasource values", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, "_dataSource=evil&datasource=evil&year=2026&account_id=smith-residential"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
		if cap.query.Get("_repName") != "portfolio-performance" || cap.query.Get("_repFormat") != "html" {
			t.Fatalf("identity query=%v", cap.query)
		}
		if cap.query.Get("_dataSource") != "properties" {
			t.Fatalf("_dataSource=%q query=%v", cap.query.Get("_dataSource"), cap.query)
		}
		if _, present := cap.query["datasource"]; present {
			t.Fatalf("client datasource survived: %s", cap.query.Encode())
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("year=%q", cap.query.Get("year"))
		}
		if cap.query.Get("account_id") != "abc-property" || strings.Contains(cap.query.Encode(), "smith-residential") {
			t.Fatalf("bound account_id query=%v", cap.query)
		}
		encoded := cap.query.Encode()
		if strings.Contains(encoded, "evil") {
			t.Fatalf("client datasource survived query=%v", cap.query)
		}
		body := rec.Body.String()
		if strings.Contains(body, "_dataSource") || strings.Contains(body, "properties") || strings.Contains(body, "evil") {
			t.Fatalf("datasource leaked into content document: %s", body)
		}
	})

	t.Run("datasourceName is the catalog fallback", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, nil, "name-fallback", time.Minute)
		rec := doGet(s, "/embed/report/name-fallback/content?et="+url.QueryEscape(tok)+"&_dataSource=evil&year=2026", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("_repName") != "name-fallback" || cap.query.Get("_repFormat") != "html" {
			t.Fatalf("identity query=%v", cap.query)
		}
		if cap.query.Get("_dataSource") != "named-source" {
			t.Fatalf("_dataSource=%q query=%v", cap.query.Get("_dataSource"), cap.query)
		}
		if cap.query.Get("year") != "2026" || strings.Contains(cap.query.Encode(), "evil") {
			t.Fatalf("query=%v", cap.query)
		}
		if _, present := cap.query["property_id"]; present {
			t.Fatalf("property_id synthesized for a report that does not declare it: %s", cap.query.Encode())
		}
	})

	t.Run("empty catalog datasource is left unset", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, nil, "no-datasource", time.Minute)
		rec := doGet(s, "/embed/report/no-datasource/content?et="+url.QueryEscape(tok)+"&_dataSource=evil&datasource=evil&year=2026", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
		if cap.query.Get("_repName") != "no-datasource" || cap.query.Get("_repFormat") != "html" {
			t.Fatalf("identity query=%v", cap.query)
		}
		if cap.query.Get("_dataSource") != "" || cap.query.Get("datasource") != "" {
			t.Fatalf("invented datasource query=%v", cap.query)
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("year=%q", cap.query.Get("year"))
		}
		if strings.Contains(cap.query.Encode(), "evil") || strings.Contains(cap.query.Encode(), "_dataSource") {
			t.Fatalf("client datasource survived query=%v", cap.query)
		}
	})

	t.Run("absent string parameter is a present empty string", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, "year=2026"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		vals, ok := cap.query["property_id"]
		if !ok || len(vals) != 1 || vals[0] != "" {
			t.Fatalf("property_id must be present and empty, query=%s", cap.query.Encode())
		}
		if !strings.Contains(cap.query.Encode(), "property_id=") {
			t.Fatalf("property_id omitted from encoded query %s", cap.query.Encode())
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("year=%q query=%s", cap.query.Get("year"), cap.query.Encode())
		}
		if cap.query.Get("account_id") != "abc-property" {
			t.Fatalf("account_id=%q query=%s", cap.query.Get("account_id"), cap.query.Encode())
		}
	})

	t.Run("explicit string value is preserved", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, "year=2026&property_id=123"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("property_id") != "123" {
			t.Fatalf("property_id=%q query=%s", cap.query.Get("property_id"), cap.query.Encode())
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("year=%q", cap.query.Get("year"))
		}
	})

	t.Run("explicit empty string stays present", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, "property_id="), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		vals, ok := cap.query["property_id"]
		if !ok || len(vals) != 1 || vals[0] != "" {
			t.Fatalf("explicit empty property_id lost, query=%s", cap.query.Encode())
		}
	})

	t.Run("token bound string parameter stays authoritative", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, "year=2026&account_id=smith-residential"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("account_id") != "abc-property" || strings.Contains(cap.query.Encode(), "smith-residential") {
			t.Fatalf("bound account_id lost, query=%s", cap.query.Encode())
		}
		vals, ok := cap.query["property_id"]
		if !ok || len(vals) != 1 || vals[0] != "" {
			t.Fatalf("property_id=%v query=%s", vals, cap.query.Encode())
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("year=%q", cap.query.Get("year"))
		}
	})

	t.Run("absent numeric date boolean and select parameters stay absent", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, ""), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		for _, name := range []string{"year", "as_of", "include_vacant", "region"} {
			if _, present := cap.query[name]; present {
				t.Fatalf("%s synthesized as blank, query=%s", name, cap.query.Encode())
			}
		}
		vals, ok := cap.query["property_id"]
		if !ok || len(vals) != 1 || vals[0] != "" {
			t.Fatalf("property_id must be present and empty, query=%s", cap.query.Encode())
		}
		if cap.query.Get("account_id") != "abc-property" {
			t.Fatalf("account_id=%q query=%s", cap.query.Get("account_id"), cap.query.Encode())
		}
	})

	t.Run("supplied numeric value is preserved", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, "year=2026"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("year=%q query=%s", cap.query.Get("year"), cap.query.Encode())
		}
		if _, present := cap.query["as_of"]; present || cap.query.Has("include_vacant") {
			t.Fatalf("non-text parameter synthesized, query=%s", cap.query.Encode())
		}
	})

	t.Run("portfolio performance host parameters", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, "year=2026&account_id=smith-residential&_dataSource=evil&datasource=evil"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("_repName") != "portfolio-performance" || cap.query.Get("_repFormat") != "html" {
			t.Fatalf("identity query=%s", cap.query.Encode())
		}
		if cap.query.Get("_dataSource") != "properties" {
			t.Fatalf("datasource query=%s", cap.query.Encode())
		}
		if _, present := cap.query["datasource"]; present {
			t.Fatalf("datasource key reintroduced: %s", cap.query.Encode())
		}
		if strings.Contains(cap.query.Encode(), "evil") {
			t.Fatalf("client datasource survived query=%s", cap.query.Encode())
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("year=%q query=%s", cap.query.Get("year"), cap.query.Encode())
		}
		vals, ok := cap.query["property_id"]
		if !ok || len(vals) != 1 || vals[0] != "" || !strings.Contains(cap.query.Encode(), "property_id=") {
			t.Fatalf("property_id not present empty, query=%s", cap.query.Encode())
		}
		if cap.query.Get("account_id") != "abc-property" || strings.Contains(cap.query.Encode(), "smith-residential") {
			t.Fatalf("account_id query=%s", cap.query.Encode())
		}
	})

	t.Run("ordinary execute does not synthesize blank text parameters", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, "/reports/execute?_repName=portfolio-performance&_repFormat=html&year=2026", []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if _, present := cap.query["property_id"]; present {
			t.Fatalf("execute synthesized property_id, query=%s", cap.query.Encode())
		}
		if cap.query.Get("year") != "2026" {
			t.Fatalf("year=%q query=%s", cap.query.Get("year"), cap.query.Encode())
		}
		if cap.query.Get("account_id") != "abc-property" {
			t.Fatalf("account_id=%q query=%s", cap.query.Get("account_id"), cap.query.Encode())
		}
	})

	t.Run("viewer route still renders viewer chrome", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, "/embed/report/portfolio-performance?et="+url.QueryEscape(tok), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, chrome := range []string{"paramSidebar", "Fit Width", "reportFrame", "<iframe", "applyBtn"} {
			if !strings.Contains(body, chrome) {
				t.Fatalf("viewer missing %q", chrome)
			}
		}
		if strings.Contains(body, "<html>ok</html>") {
			t.Fatal("viewer route returned the jasper document")
		}
		if cap.calls != 0 {
			t.Fatalf("viewer route executed jasper directly, calls=%d", cap.calls)
		}
		if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors "+origin {
			t.Fatalf("viewer CSP=%q", got)
		}
	})

	t.Run("execute and dev report framing stay unchanged", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)

		health := doGet(s, "/health", nil, nil)
		assertSameOriginFraming(t, health)

		bare := doGet(s, "/reports/execute?_repName=portfolio-performance&_repFormat=html&et="+url.QueryEscape(tok), nil, nil)
		if bare.Code == http.StatusOK {
			t.Fatalf("execute accepted et without a grant: %s", bare.Body.String())
		}
		assertSameOriginFraming(t, bare)
		if strings.Contains(bare.Header().Get("Content-Security-Policy"), origin) {
			t.Fatalf("execute CSP followed et: %q", bare.Header().Get("Content-Security-Policy"))
		}
		if cap.calls != 0 {
			t.Fatalf("et on execute reached jasper, calls=%d", cap.calls)
		}

		granted := doGet(s, "/reports/execute?_repName=portfolio-performance&_repFormat=html", []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if granted.Code != http.StatusOK {
			t.Fatalf("cookie execute status %d body %s", granted.Code, granted.Body.String())
		}
		assertEmbedFraming(t, granted, "frame-ancestors 'self' "+origin)

		dev := doGet(s, "/dev/report?_repName=portfolio-performance&_repFormat=html", []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, nil)
		if dev.Code != http.StatusOK {
			t.Fatalf("dev status %d body %s", dev.Code, dev.Body.String())
		}
		assertSameOriginFraming(t, dev)
	})
}
