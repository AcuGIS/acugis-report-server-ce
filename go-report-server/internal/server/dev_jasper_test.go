package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

type countingGateway struct {
	inner jasper.Gateway
	calls atomic.Int64
}

func (c *countingGateway) ExecuteReport(ctx context.Context, req jasper.ExecuteRequest) (*jasper.ProxyResponse, error) {
	c.calls.Add(1)
	return c.inner.ExecuteReport(ctx, req)
}

func (c *countingGateway) ExecuteReportQuery(ctx context.Context, req jasper.ReportQueryRequest) (*jasper.ReportQueryResult, error) {
	return c.inner.ExecuteReportQuery(ctx, req)
}

func (c *countingGateway) FetchReportImage(ctx context.Context, req jasper.ImageRequest) (*jasper.ProxyResponse, error) {
	c.calls.Add(1)
	return c.inner.FetchReportImage(ctx, req)
}

func (c *countingGateway) Status() jasper.Status   { return c.inner.Status() }
func (c *countingGateway) Timeout() time.Duration  { return c.inner.Timeout() }
func (c *countingGateway) Client() *jasper.Client  { return c.inner.Client() }

type denyACL struct{}

func (denyACL) CanViewReport(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (denyACL) CanEditReport(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (denyACL) CanAdminReport(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (denyACL) CanViewExplorerView(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (denyACL) CanEditExplorerView(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (denyACL) CanAccessReportImage(context.Context, *auth.User, string) (bool, error) {
	return false, nil
}
func (denyACL) CanAccessReportService(context.Context, *auth.User) (bool, error) {
	return false, nil
}
func (denyACL) CheckPermission(context.Context, *auth.User, string, string, string) (acl.Decision, error) {
	return acl.Decision{Source: acl.SourceDefaultDeny}, nil
}
func (denyACL) Configured() bool   { return true }
func (denyACL) AuthDisabled() bool { return false }

type failDBACL struct{}

func (failDBACL) CanViewReport(context.Context, *auth.User, string) (bool, error) {
	return false, acl.ErrDatabase
}
func (failDBACL) CanEditReport(context.Context, *auth.User, string) (bool, error) {
	return false, acl.ErrDatabase
}
func (failDBACL) CanAdminReport(context.Context, *auth.User, string) (bool, error) {
	return false, acl.ErrDatabase
}
func (failDBACL) CanViewExplorerView(context.Context, *auth.User, string) (bool, error) {
	return false, acl.ErrDatabase
}
func (failDBACL) CanEditExplorerView(context.Context, *auth.User, string) (bool, error) {
	return false, acl.ErrDatabase
}
func (failDBACL) CanAccessReportImage(context.Context, *auth.User, string) (bool, error) {
	return false, acl.ErrDatabase
}
func (failDBACL) CanAccessReportService(context.Context, *auth.User) (bool, error) {
	return false, acl.ErrDatabase
}
func (failDBACL) CheckPermission(context.Context, *auth.User, string, string, string) (acl.Decision, error) {
	return acl.Decision{}, acl.ErrDatabase
}
func (failDBACL) Configured() bool   { return true }
func (failDBACL) AuthDisabled() bool { return false }

func authDisabledACL() acl.Service {
	return acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true})
}

type mockConfiguredRepo struct{}

func (mockConfiguredRepo) Configured() bool { return true }
func (mockConfiguredRepo) UserGroupNames(context.Context, int64) ([]string, error) {
	return nil, nil
}
func (mockConfiguredRepo) HasGroupPermission(context.Context, string, string, string, []string) (bool, error) {
	return false, nil
}
func (mockConfiguredRepo) HasAnyJasperReportPermission(context.Context, string, []string) (bool, error) {
	return false, nil
}
func (mockConfiguredRepo) HasAnyResourceTypePermission(context.Context, string, string, []string) (bool, error) {
	return false, nil
}
func (mockConfiguredRepo) FindResource(context.Context, string, string) (*acl.Resource, error) {
	return nil, nil
}

func TestDevReportStreamsFromMockJasper(t *testing.T) {
	jasperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "_repName=demo") {
			t.Fatalf("unexpected query %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="demo.pdf"`)
		_, _ = w.Write([]byte("%PDF-mock"))
	}))
	defer jasperSrv.Close()

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Jasper.ReportURL = jasperSrv.URL
	cfg.Jasper.ImageURL = jasperSrv.URL + "/img"
	cfg.Jasper.Timeout = time.Second

	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{
		ReportExists: func(string) bool { return true },
	})
	s := New(cfg, Dependencies{
		Gateway: gw,
		Jasper:  gw.Client(),
		ACL:     authDisabledACL(),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/dev/report?_repName=demo&_repFormat=pdf", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("ct=%s", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "demo.pdf") {
		t.Fatalf("cd=%s", rec.Header().Get("Content-Disposition"))
	}
	if rec.Body.String() != "%PDF-mock" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestDevReportImage(t *testing.T) {
	jasperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNG!"))
	}))
	defer jasperSrv.Close()

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Jasper.ReportURL = jasperSrv.URL + "/report"
	cfg.Jasper.ImageURL = jasperSrv.URL
	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{})
	s := New(cfg, Dependencies{
		Gateway: gw,
		ACL:     authDisabledACL(),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/dev/report-image?_repName=demo&image=x", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body) != "PNG!" {
		t.Fatalf("body=%q", body)
	}
}

func TestDevReportImageSVGContentType(t *testing.T) {
	// Upstream mislabels SVG QR bytes as PNG (historical Tomcat servlet default).
	jasperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("<svg xmlns='http://www.w3.org/2000/svg'/>"))
	}))
	defer jasperSrv.Close()

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Jasper.ReportURL = jasperSrv.URL + "/report"
	cfg.Jasper.ImageURL = jasperSrv.URL
	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{})
	s := New(cfg, Dependencies{
		Gateway: gw,
		ACL:     authDisabledACL(),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/report-image?_repName=prcascade&image=img_0_2_1.svg&uuid=abc", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("Content-Type=%q want image/svg+xml", got)
	}
}

func TestContentTypeForReportImage(t *testing.T) {
	cases := map[string]string{
		"img_0_2_1.svg": "image/svg+xml",
		"img_0_0_0.png": "image/png",
		"chart.JPG":     "image/jpeg",
		"x":             "",
	}
	for in, want := range cases {
		if got := contentTypeForReportImage(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestDevJasperStatus(t *testing.T) {
	cfg := testConfig()
	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{
		ReportExists: func(string) bool { return true },
	})
	s := New(cfg, Dependencies{Gateway: gw, Jasper: gw.Client()}, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/dev/jasper-status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"timeout"`) {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatal("status leaked password field")
	}
	if !strings.Contains(rec.Body.String(), `"auth_disabled"`) {
		t.Fatalf("missing auth_disabled: %s", rec.Body.String())
	}
}

func TestDevReportInvalidName(t *testing.T) {
	cfg := testConfig()
	cfg.AuthDisabled = true
	gw := jasper.NewGateway(cfg.Jasper, jasper.Options{ReportExists: func(string) bool { return true }})
	s := New(cfg, Dependencies{
		Gateway: gw,
		ACL:     authDisabledACL(),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/dev/report?_repName=!!!&_repFormat=pdf", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDevReportForbiddenDoesNotReachJasper(t *testing.T) {
	jasperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("jasper must not be called")
	}))
	defer jasperSrv.Close()

	cfg := testConfig()
	cfg.Jasper.ReportURL = jasperSrv.URL
	cfg.Jasper.ImageURL = jasperSrv.URL + "/img"
	inner := jasper.NewGateway(cfg.Jasper, jasper.Options{ReportExists: func(string) bool { return true }})
	gw := &countingGateway{inner: inner}
	s := New(cfg, Dependencies{
		Gateway: gw,
		ACL:     denyACL{},
		Auth:    auth.NewService(nil, auth.Options{}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/dev/report?_repName=demo&_repFormat=pdf", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gw.calls.Load() != 0 {
		t.Fatalf("upstream calls=%d", gw.calls.Load())
	}
}

func TestDevReportImageInvalidKeyDoesNotReachJasper(t *testing.T) {
	jasperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("jasper must not be called")
	}))
	defer jasperSrv.Close()

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Jasper.ImageURL = jasperSrv.URL
	inner := jasper.NewGateway(cfg.Jasper, jasper.Options{})
	gw := &countingGateway{inner: inner}
	s := New(cfg, Dependencies{
		Gateway: gw,
		ACL:     authDisabledACL(),
		Auth:    auth.NewService(nil, auth.Options{AuthDisabled: true}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/dev/report-image?_repName=!!!&image=x", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}
	if gw.calls.Load() != 0 {
		t.Fatalf("upstream calls=%d", gw.calls.Load())
	}
}

func TestDevReportDatabaseFailure(t *testing.T) {
	cfg := testConfig()
	inner := jasper.NewGateway(cfg.Jasper, jasper.Options{ReportExists: func(string) bool { return true }})
	gw := &countingGateway{inner: inner}
	s := New(cfg, Dependencies{
		Gateway: gw,
		ACL:     failDBACL{},
		Auth:    auth.NewService(nil, auth.Options{}),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/dev/report?_repName=demo&_repFormat=pdf", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gw.calls.Load() != 0 {
		t.Fatalf("upstream calls=%d", gw.calls.Load())
	}
}
