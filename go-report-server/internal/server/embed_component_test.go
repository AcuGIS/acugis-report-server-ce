package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/embedtoken"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

func componentURL(reportKey, embedID, token, rawQuery string) string {
	u := "/embed/report/" + reportKey + "/component/" + url.PathEscape(embedID) + "?et=" + url.QueryEscape(token)
	if rawQuery != "" {
		u += "&" + rawQuery
	}
	return u
}

func TestEmbedReportComponent(t *testing.T) {
	origin := "https://acugis.cloud"
	s, cap := newContentEmbedServer(t, []string{origin})

	t.Run("missing token is rejected", func(t *testing.T) {
		cap.calls = 0
		rec := doGet(s, "/embed/report/portfolio-performance/component/total-units", nil, nil)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "AUTH_REQUIRED") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
		assertEmbedFraming(t, rec, "frame-ancestors "+origin)
	})

	t.Run("personal access token is rejected", func(t *testing.T) {
		cap.calls = 0
		rec := doGet(s, "/embed/report/portfolio-performance/component/total-units?et=jrs_notallowed", nil, nil)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "AUTH_REQUIRED") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("malformed token is rejected", func(t *testing.T) {
		cap.calls = 0
		rec := doGet(s, "/embed/report/portfolio-performance/component/total-units?et=emb_not-a-valid-token", nil, nil)
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
		rec := doGet(s, componentURL("portfolio-performance", "total-units", tok, ""), nil, nil)
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
		rec := doGet(s, componentURL("portfolio-performance", "total-units", tok, "year=2026"), nil, nil)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ACCESS_DENIED") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
	})

	t.Run("malformed embed ids are rejected", func(t *testing.T) {
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		for _, id := range []string{"Monthly Occupancy", "1chart", "chart!", "chartA", "a" + strings.Repeat("b", 64)} {
			cap.calls = 0
			rec := doGet(s, componentURL("portfolio-performance", id, tok, "year=2026"), nil, nil)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_PARAMETER") {
				t.Fatalf("id %q status %d body %s", id, rec.Code, rec.Body.String())
			}
			if cap.calls != 0 {
				t.Fatalf("id %q reached jasper", id)
			}
		}
	})

	t.Run("valid embed id is sealed and framed", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, componentURL("portfolio-performance", "monthly-occupancy", tok, "year=2026&property_id=123&account_id=smith-residential&_componentId=total-units"), nil, map[string]string{
			jasper.EmbedComponentHeader: "0",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 1 {
			t.Fatalf("jasper calls=%d", cap.calls)
		}
		if cap.componentHeader != jasper.EmbedComponentHeaderValue {
			t.Fatalf("component header=%q", cap.componentHeader)
		}
		if cap.query.Get("_componentId") != "monthly-occupancy" {
			t.Fatalf("_componentId=%q query=%s", cap.query.Get("_componentId"), cap.query.Encode())
		}
		if vals := cap.query["_componentId"]; len(vals) != 1 {
			t.Fatalf("_componentId values=%v", vals)
		}
		if cap.query.Get("_repName") != "portfolio-performance" || cap.query.Get("_repFormat") != "html" {
			t.Fatalf("identity query=%s", cap.query.Encode())
		}
		if cap.query.Get("_dataSource") != "properties" || cap.query.Get("datasource") != "" {
			t.Fatalf("datasource query=%s", cap.query.Encode())
		}
		if cap.query.Get("account_id") != "abc-property" || strings.Contains(cap.query.Encode(), "smith-residential") {
			t.Fatalf("account_id query=%s", cap.query.Encode())
		}
		if cap.query.Get("year") != "2026" || cap.query.Get("property_id") != "123" {
			t.Fatalf("host parameters query=%s", cap.query.Encode())
		}
		if cap.query.Get("et") != "" {
			t.Fatalf("token forwarded query=%s", cap.query.Encode())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "<html>ok</html>") || !strings.Contains(body, `post("load"`) {
			t.Fatalf("component document missing bridge: %s", body)
		}
		for _, chrome := range []string{"paramSidebar", "Fit Width", "reportFrame", "<iframe", "applyBtn"} {
			if strings.Contains(body, chrome) {
				t.Fatalf("viewer chrome %q in component response", chrome)
			}
		}
		assertEmbedFraming(t, rec, "frame-ancestors "+origin)
		var cookie *http.Cookie
		for _, c := range rec.Result().Cookies() {
			if c.Name == "jrs_embed_token" {
				cookie = c
			}
		}
		if cookie == nil || !cookie.HttpOnly || cookie.Value != tok {
			t.Fatalf("embed cookie=%+v", cookie)
		}
	})

	t.Run("embed id is not rewritten", func(t *testing.T) {
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		for _, id := range []string{"total-units", "total_units", "chart1", "a", "a" + strings.Repeat("b", 63)} {
			cap.calls = 0
			rec := doGet(s, componentURL("portfolio-performance", id, tok, ""), nil, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("id %q status %d body %s", id, rec.Code, rec.Body.String())
			}
			if cap.query.Get("_componentId") != id {
				t.Fatalf("id %q forwarded as %q", id, cap.query.Get("_componentId"))
			}
			if cap.componentHeader != jasper.EmbedComponentHeaderValue {
				t.Fatalf("id %q header=%q", id, cap.componentHeader)
			}
		}
	})

	t.Run("absent text parameter stays a present empty string", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, componentURL("portfolio-performance", "total-units", tok, "year=2026"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		vals, ok := cap.query["property_id"]
		if !ok || len(vals) != 1 || vals[0] != "" || !strings.Contains(cap.query.Encode(), "property_id=") {
			t.Fatalf("property_id query=%s", cap.query.Encode())
		}
		if cap.query.Get("account_id") != "abc-property" || cap.query.Get("_componentId") != "total-units" {
			t.Fatalf("query=%s", cap.query.Encode())
		}
	})

	t.Run("absent numeric date boolean and select parameters stay absent", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, componentURL("portfolio-performance", "total-units", tok, ""), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		for _, name := range []string{"year", "as_of", "include_vacant", "region"} {
			if _, present := cap.query[name]; present {
				t.Fatalf("%s synthesized query=%s", name, cap.query.Encode())
			}
		}
		if _, ok := cap.query["property_id"]; !ok || cap.query.Get("property_id") != "" {
			t.Fatalf("property_id query=%s", cap.query.Encode())
		}
	})

	t.Run("client datasource cannot override the catalog", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, componentURL("portfolio-performance", "monthly-occupancy", tok, "_dataSource=evil&datasource=evil&year=2026"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.query.Get("_dataSource") != "properties" || strings.Contains(cap.query.Encode(), "evil") {
			t.Fatalf("query=%s", cap.query.Encode())
		}
		if _, present := cap.query["datasource"]; present {
			t.Fatalf("datasource key reintroduced: %s", cap.query.Encode())
		}
		if cap.query.Get("_componentId") != "monthly-occupancy" {
			t.Fatalf("_componentId=%q", cap.query.Get("_componentId"))
		}
	})

	t.Run("tomcat component error uses the content bridge", func(t *testing.T) {
		cap.calls = 0
		cap.status = http.StatusNotFound
		cap.responseBody = "COMPONENT_NOT_FOUND: no design element with this key"
		t.Cleanup(func() {
			cap.status = 0
			cap.responseBody = ""
		})
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, componentURL("portfolio-performance", "not-a-component", tok, ""), nil, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "COMPONENT_NOT_FOUND") || !strings.Contains(body, `post("error"`) {
			t.Fatalf("bridge body=%s", body)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("content-type %q", rec.Header().Get("Content-Type"))
		}
		assertEmbedFraming(t, rec, "frame-ancestors "+origin)
	})

	t.Run("content route does not arm component export", func(t *testing.T) {
		cap.calls = 0
		cap.status = 0
		cap.responseBody = ""
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, contentURL(tok, "_componentId=monthly-occupancy&year=2026"), nil, map[string]string{
			jasper.EmbedComponentHeader: jasper.EmbedComponentHeaderValue,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.componentHeader != "" {
			t.Fatalf("content forwarded component header %q", cap.componentHeader)
		}
		if _, present := cap.query["_componentId"]; present {
			t.Fatalf("content forwarded _componentId query=%s", cap.query.Encode())
		}
		if cap.query.Get("_repName") != "portfolio-performance" || cap.query.Get("_repFormat") != "html" {
			t.Fatalf("identity query=%s", cap.query.Encode())
		}
		if cap.query.Get("year") != "2026" || cap.query.Get("account_id") != "abc-property" {
			t.Fatalf("query=%s", cap.query.Encode())
		}
	})

	t.Run("viewer route does not execute a component", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)
		rec := doGet(s, "/embed/report/portfolio-performance?et="+url.QueryEscape(tok)+"&_componentId=monthly-occupancy", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.calls != 0 {
			t.Fatalf("viewer executed jasper calls=%d header=%q", cap.calls, cap.componentHeader)
		}
		if !strings.Contains(rec.Body.String(), "reportFrame") {
			t.Fatalf("viewer chrome missing: %s", rec.Body.String())
		}
		assertEmbedFraming(t, rec, "frame-ancestors "+origin)
	})

	t.Run("ordinary execute cannot activate component mode", func(t *testing.T) {
		cap.calls = 0
		tok := issueBoundToken(t, map[string][]string{"account_id": {"abc-property"}}, "portfolio-performance", time.Minute)
		rec := doGet(s, "/reports/execute?_repName=portfolio-performance&_repFormat=html&year=2026&_componentId=monthly-occupancy", []*http.Cookie{{Name: "jrs_embed_token", Value: tok}}, map[string]string{
			jasper.EmbedComponentHeader: jasper.EmbedComponentHeaderValue,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if cap.componentHeader != "" {
			t.Fatalf("execute forwarded component header %q", cap.componentHeader)
		}
		if _, present := cap.query["_componentId"]; present || strings.Contains(cap.query.Encode(), "monthly-occupancy") {
			t.Fatalf("execute forwarded component id query=%s", cap.query.Encode())
		}
		if cap.query.Get("year") != "2026" || cap.query.Get("account_id") != "abc-property" {
			t.Fatalf("query=%s", cap.query.Encode())
		}
		if _, present := cap.query["property_id"]; present {
			t.Fatalf("execute synthesized property_id query=%s", cap.query.Encode())
		}

		cap.calls = 0
		post := httptest.NewRequest(http.MethodPost, "/reports/execute?_repName=portfolio-performance&_repFormat=html&_componentId=from-query", strings.NewReader("_componentId=monthly-occupancy&year=2024"))
		post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		post.Header.Set(jasper.EmbedComponentHeader, jasper.EmbedComponentHeaderValue)
		post.AddCookie(&http.Cookie{Name: "jrs_embed_token", Value: tok})
		postRec := httptest.NewRecorder()
		s.Handler().ServeHTTP(postRec, post)
		if postRec.Code != http.StatusOK {
			t.Fatalf("post status %d body %s", postRec.Code, postRec.Body.String())
		}
		if cap.componentHeader != "" {
			t.Fatalf("post forwarded component header %q", cap.componentHeader)
		}
		if _, present := cap.query["_componentId"]; present {
			t.Fatalf("post query=%s", cap.query.Encode())
		}
		if strings.Contains(cap.body, "_componentId") || strings.Contains(cap.body, "monthly-occupancy") {
			t.Fatalf("post body=%s", cap.body)
		}

		cap.calls = 0
		jsonPost := httptest.NewRequest(http.MethodPost, "/reports/execute?_repName=portfolio-performance&_repFormat=html&_componentId=from-query", strings.NewReader(`{"_componentId":"monthly-occupancy","year":2024,"parameters":{"_componentId":"nested"}}`))
		jsonPost.Header.Set("Content-Type", "application/json")
		jsonPost.Header.Set(jasper.EmbedComponentHeader, jasper.EmbedComponentHeaderValue)
		jsonPost.AddCookie(&http.Cookie{Name: "jrs_embed_token", Value: tok})
		jsonRec := httptest.NewRecorder()
		s.Handler().ServeHTTP(jsonRec, jsonPost)
		if jsonRec.Code != http.StatusOK {
			t.Fatalf("json status %d body %s", jsonRec.Code, jsonRec.Body.String())
		}
		if cap.componentHeader != "" || strings.Contains(cap.body, "_componentId") || strings.Contains(cap.body, "monthly-occupancy") {
			t.Fatalf("json header=%q body=%s query=%s", cap.componentHeader, cap.body, cap.query.Encode())
		}
	})
}

func TestEmbedComponentIDIsNotOverridable(t *testing.T) {
	if _, _, err := embedtoken.Issue(boundTestSecret, embedtoken.Claims{
		UserID:      1,
		Username:    "embed-host",
		ContentType: embedtoken.ContentReport,
		ContentID:   "portfolio-performance",
		Parameters:  map[string][]string{"_componentId": {"from-token"}},
	}, time.Minute); err == nil {
		t.Fatal("token accepted _componentId")
	}

	raw := applyBoundToRawQuery("_componentId=from-browser&year=2026", map[string][]string{
		"_componentId": {"from-token"},
		"account_id":   {"abc-property"},
	})
	pinned := pinReportIdentity(raw, "portfolio-performance", "html")
	q, err := url.ParseQuery(pinned)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := q["_componentId"]; present {
		t.Fatalf("pin left _componentId query=%s", pinned)
	}
	if q.Get("_repName") != "portfolio-performance" || q.Get("_repFormat") != "html" || q.Get("account_id") != "abc-property" {
		t.Fatalf("pinned=%s", pinned)
	}
	sealed := sealEmbedComponentQuery(pinned, "monthly-occupancy")
	got, err := url.ParseQuery(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if got.Get("_componentId") != "monthly-occupancy" || len(got["_componentId"]) != 1 {
		t.Fatalf("sealed=%s", sealed)
	}
	if got.Get("account_id") != "abc-property" || got.Get("year") != "2026" {
		t.Fatalf("sealed=%s", sealed)
	}

	untouched := "year=2026&_repName=portfolio-performance"
	if stripEmbedComponentQuery(untouched) != untouched {
		t.Fatalf("strip rewrote %q", stripEmbedComponentQuery(untouched))
	}
	stripped := stripEmbedComponentQuery(untouched + "&_componentId=monthly-occupancy")
	parsed, err := url.ParseQuery(stripped)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := parsed["_componentId"]; present || parsed.Get("year") != "2026" {
		t.Fatalf("stripped=%s", stripped)
	}
}

func TestComponentModeHeaderIsNotCopiedFromTheClient(t *testing.T) {
	var forwarded string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = r.Header.Get(jasper.EmbedComponentHeader)
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>ok</html>")
	}))
	defer srv.Close()
	gw := jasper.NewGateway(config.JasperConfig{ReportURL: srv.URL, ImageURL: srv.URL + "/img", Timeout: time.Second}, jasper.Options{ReportExists: func(string) bool { return true }})
	resp, err := gw.ExecuteReport(t.Context(), jasper.ExecuteRequest{
		Method:     http.MethodGet,
		ReportName: "demo",
		Format:     "html",
		RawQuery:   "_repName=demo&_repFormat=html&_componentId=monthly-occupancy",
		Header:     http.Header{jasper.EmbedComponentHeader: []string{jasper.EmbedComponentHeaderValue}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Close()
	if forwarded != "" {
		t.Fatalf("client header forwarded: %q", forwarded)
	}

	var saw string
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = r.Header.Get(jasper.EmbedComponentHeader)
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>ok</html>")
	}))
	defer srv2.Close()
	gw2 := jasper.NewGateway(config.JasperConfig{ReportURL: srv2.URL, ImageURL: srv2.URL + "/img", Timeout: time.Second}, jasper.Options{ReportExists: func(string) bool { return true }})
	resp, err = gw2.ExecuteReport(t.Context(), jasper.ExecuteRequest{
		Method:         http.MethodGet,
		ReportName:     "demo",
		Format:         "html",
		RawQuery:       "_repName=demo&_repFormat=html&_componentId=total-units",
		EmbedComponent: true,
		Header:         http.Header{jasper.EmbedComponentHeader: []string{"0"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Close()
	if saw != jasper.EmbedComponentHeaderValue {
		t.Fatalf("marker=%q", saw)
	}
}

func componentPageHTML(width, height int, id string) string {
	return fmt.Sprintf(
		`<html><body text="#000000" link="#000000" alink="#000000" vlink="#000000"><table role="none" width="100%%" cellpadding="0" cellspacing="0" border="0"><tr><td width="50%%">&nbsp;</td><td align="center"><table id="JR_PAGE_ANCHOR_0_1" role="none" class="jrPage" data-jr-height="%d" cellpadding="0" cellspacing="0" border="0" style="empty-cells: show; width: %dpx; border-collapse: collapse;"><tr role="none" valign="top" style="height:0"><td style="width:%dpx"></td></tr><tr valign="top" style="height:%dpx"><td id="%s"></td></tr></table></td><td width="50%%">&nbsp;</td></tr></table></body></html>`,
		height, width, width, height, id,
	)
}

func TestComponentBridgeReportsNaturalPageBox(t *testing.T) {
	origin := "https://acugis.cloud"
	s, cap := newContentEmbedServer(t, []string{origin})
	tok := issueBoundToken(t, nil, "portfolio-performance", time.Minute)

	// Fixture sizes from the earlier component probe, plus the design box of
	// the deployed Portfolio Performance components. This process does not
	// apply CSS, so the test checks that Jasper's markup is preserved and
	// that the bridge installs a viewport-independent layout measurement.
	// Chrome measurement of the real exports is recorded in componentMeasureJS.
	cases := []struct {
		id     string
		width  int
		height int
	}{
		{id: "monthly-occupancy", width: 320, height: 180},
		{id: "total-units", width: 220, height: 28},
		{id: "total-units-report", width: 80, height: 24},
		{id: "monthly-occupancy-report", width: 544, height: 96},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			cap.calls = 0
			cap.status = 0
			cap.responseBody = componentPageHTML(tc.width, tc.height, tc.id)
			t.Cleanup(func() { cap.responseBody = "" })
			rec := doGet(s, componentURL("portfolio-performance", tc.id, tok, "year=2026"), nil, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, fmt.Sprintf("width: %dpx", tc.width)) || !strings.Contains(body, fmt.Sprintf(`data-jr-height="%d"`, tc.height)) {
				t.Fatalf("jasper page box missing from component html")
			}
			if !strings.Contains(body, `width="100%"`) || !strings.Contains(body, `&nbsp;`) {
				t.Fatalf("centering table missing")
			}
			script := body
			if i := strings.Index(body, "<script>"); i >= 0 {
				script = body[i:]
			}
			for _, needle := range []string{
				`document.querySelector(".jrPage")`,
				`getBoundingClientRect()`,
				`getComputedStyle(`,
				`selectNodeContents(`,
				`Math.ceil(`,
				`post("resize"`,
				`post("load"`,
				`source:"acugis-report"`,
			} {
				if !strings.Contains(script, needle) {
					t.Fatalf("component bridge missing %q", needle)
				}
			}
			for _, banned := range []string{
				"innerWidth",
				"scrollWidth",
				"scrollHeight",
				"clientWidth",
				`getAttribute("style")`,
				`getAttribute("data-jr-height")`,
				`width="100%"`,
			} {
				if strings.Contains(script, banned) {
					t.Fatalf("component bridge uses viewport measurement %q", banned)
				}
			}
		})
	}

	t.Run("content keeps document measurement", func(t *testing.T) {
		cap.calls = 0
		cap.status = 0
		cap.responseBody = componentPageHTML(320, 180, "monthly-occupancy")
		t.Cleanup(func() { cap.responseBody = "" })
		rec := doGet(s, contentURL(tok, "year=2026"), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, contentMeasureJS) || !strings.Contains(body, contentPublishJS) {
			t.Fatalf("content measure changed")
		}
		if strings.Contains(body, `querySelector(".jrPage")`) {
			t.Fatalf("content bridge reads .jrPage")
		}
		if !strings.Contains(body, "width: 320px") || !strings.Contains(body, `data-jr-height="180"`) {
			t.Fatalf("content response rewrote the jasper page")
		}
	})
}
