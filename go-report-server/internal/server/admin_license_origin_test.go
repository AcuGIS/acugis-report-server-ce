package server

import (
	"bytes"
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

func TestSameOriginPost(t *testing.T) {
	if sameOriginPost(nil) {
		t.Fatal("nil request was allowed")
	}

	cases := []struct {
		name   string
		origin string
		host   string
		xfh    string
		xfp    string
		useTLS bool
		setXFH bool
		want   bool
	}{
		{
			name:   "direct matching origin and host",
			origin: "https://reports.example.com",
			host:   "reports.example.com",
			want:   true,
		},
		{
			name:   "apache proxy uses forwarded host",
			origin: "https://public.example.com",
			host:   "127.0.0.1:9088",
			xfh:    "public.example.com",
			setXFH: true,
			xfp:    "https",
			want:   true,
		},
		{
			name:   "https default port on host",
			origin: "https://public.example.com",
			host:   "public.example.com:443",
			xfp:    "https",
			want:   true,
		},
		{
			name:   "https default port on origin",
			origin: "https://public.example.com:443",
			host:   "public.example.com",
			useTLS: true,
			want:   true,
		},
		{
			name:   "hostname case differs",
			origin: "https://Public.Example.com",
			host:   "public.example.com",
			want:   true,
		},
		{
			name:   "http default port",
			origin: "http://public.example.com:80",
			host:   "public.example.com",
			want:   true,
		},
		{
			name:   "cross origin rejected",
			origin: "https://evil.example.com",
			host:   "public.example.com",
			xfh:    "public.example.com",
			setXFH: true,
			want:   false,
		},
		{
			name:   "https port is not ignored on http",
			origin: "http://public.example.com:443",
			host:   "public.example.com",
			want:   false,
		},
		{
			name:   "different explicit port rejected",
			origin: "https://public.example.com:8443",
			host:   "public.example.com",
			xfp:    "https",
			want:   false,
		},
		{
			name:   "malformed origin rejected",
			origin: "https://",
			host:   "public.example.com",
			want:   false,
		},
		{
			name:   "unparsed origin rejected",
			origin: "http://[",
			host:   "public.example.com",
			want:   false,
		},
		{
			name:   "non-http origin rejected",
			origin: "null",
			host:   "public.example.com",
			want:   false,
		},
		{
			name:   "qc9 literal origin null rejected",
			origin: "null",
			host:   "qc9.webgis1.com",
			xfh:    "qc9.webgis1.com",
			setXFH: true,
			xfp:    "https",
			want:   false,
		},
		{
			name:   "qc9 same origin allowed",
			origin: "https://qc9.webgis1.com",
			host:   "127.0.0.1:9088",
			xfh:    "qc9.webgis1.com",
			setXFH: true,
			xfp:    "https",
			want:   true,
		},
		{
			name: "missing origin allowed",
			host: "public.example.com",
			want: true,
		},
		{
			name:   "blank forwarded host falls back to host",
			origin: "https://public.example.com",
			host:   "public.example.com",
			xfh:    "   ",
			setXFH: true,
			want:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/admin/license", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.setXFH {
				req.Header.Set("X-Forwarded-Host", tc.xfh)
			}
			if tc.xfp != "" {
				req.Header.Set("X-Forwarded-Proto", tc.xfp)
			}
			if tc.useTLS {
				req.TLS = &tls.ConnectionState{}
			}
			if got := sameOriginPost(req); got != tc.want {
				t.Fatalf("sameOriginPost=%v want %v", got, tc.want)
			}
		})
	}
}

func TestLicenseFormIsSameDocumentPost(t *testing.T) {
	html := adminLicenseHTML
	if !strings.Contains(html, `<form method="post" action=""`) {
		t.Fatal("license activation form is not a same-document POST")
	}
	if !strings.Contains(html, `name="action" value="activate"`) {
		t.Fatal("license form missing activate action")
	}
	for _, forbidden := range []string{"<iframe", "sandbox", "srcdoc", "blob:", "data:"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("license page contains %q", forbidden)
		}
	}
	if strings.Contains(html, "fetch(") || strings.Contains(html, "XMLHttpRequest") {
		t.Fatal("license page submits through script instead of the document form")
	}
	if !strings.Contains(html, `name="action" value="activate_designer"`) {
		t.Fatal("designer activation form is missing")
	}
}

func TestDesignerActivationRejectsOpaqueAndCrossOrigin(t *testing.T) {
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Designer.Upstream = "http://127.0.0.1:19090"
	cfg.Designer.Token = "designer-origin-test-token"
	s := New(cfg, Dependencies{
		Auth: auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	form := url.Values{}
	form.Set("action", "activate_designer")
	form.Set("activation_key_designer", "ACUGIS-WEB-TEST-KEY")

	reject := func(origin, host, forwarded string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/admin/license", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Host = host
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Forwarded-Host", forwarded)
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), "The license request was rejected.") {
			t.Fatalf("origin %q was not rejected: %s", origin, rec.Body.String())
		}
	}
	reject("null", "qc9.webgis1.com", "qc9.webgis1.com")
	reject("https://evil.example.com", "127.0.0.1:9088", "public.example.com")

	same := httptest.NewRequest(http.MethodPost, "/admin/license", strings.NewReader(form.Encode()))
	same.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	same.Host = "127.0.0.1:9088"
	same.Header.Set("Origin", "https://public.example.com")
	same.Header.Set("X-Forwarded-Host", "public.example.com")
	same.Header.Set("X-Forwarded-Proto", "https")
	sameRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(sameRec, same)
	if strings.Contains(sameRec.Body.String(), "The license request was rejected.") {
		t.Fatal("same-origin designer activation was rejected")
	}
	if !strings.Contains(sameRec.Body.String(), "Report Designer Web is not installed.") {
		t.Fatalf("same-origin designer post did not pass the origin check: %s", sameRec.Body.String())
	}

	apiReject := func(origin string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/license/designer/activate", bytes.NewBufferString(`{"activation_key":"ACUGIS-WEB-TEST-KEY"}`))
		req.Host = "127.0.0.1:9088"
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Forwarded-Host", "public.example.com")
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "The license request was rejected.") {
			t.Fatalf("api origin %q: %d %s", origin, rec.Code, rec.Body.String())
		}
	}
	apiReject("null")
	apiReject("https://evil.example.com")

	apiSame := httptest.NewRequest(http.MethodPost, "/api/v1/admin/license/designer/activate", bytes.NewBufferString(`{"activation_key":"ACUGIS-WEB-TEST-KEY"}`))
	apiSame.Host = "127.0.0.1:9088"
	apiSame.Header.Set("Origin", "https://public.example.com")
	apiSame.Header.Set("X-Forwarded-Host", "public.example.com")
	apiSame.Header.Set("X-Forwarded-Proto", "https")
	apiRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(apiRec, apiSame)
	if apiRec.Code != http.StatusNotFound || !strings.Contains(apiRec.Body.String(), "Report Designer Web is not installed.") {
		t.Fatalf("same-origin designer API did not pass the origin check: %d %s", apiRec.Code, apiRec.Body.String())
	}
}
