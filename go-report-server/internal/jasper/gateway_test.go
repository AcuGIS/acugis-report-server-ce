package jasper_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

func testGateway(t *testing.T, reportURL, imageURL string, timeout time.Duration, exists jasper.ReportExistsFunc) jasper.Gateway {
	t.Helper()
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := jasper.New(config.JasperConfig{
		ReportURL: reportURL,
		ImageURL:  imageURL,
		Timeout:   timeout,
	})
	return jasper.NewGatewayFromClient(client, jasper.Options{ReportExists: exists})
}

func TestParameterForwardingRawQuery(t *testing.T) {
	var gotQuery string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="r.pdf"`)
		_, _ = w.Write([]byte("%PDF-1.4"))
	}))
	defer srv.Close()

	gw := testGateway(t, srv.URL, srv.URL+"/img", time.Second, func(string) bool { return true })
	raw := "_repName=demo&_repFormat=pdf&ids=1&ids=2&city=%C3%A9&q=a%26b"
	resp, err := gw.ExecuteReport(context.Background(), jasper.ExecuteRequest{
		Method:     http.MethodGet,
		ReportName: "demo",
		Format:     "pdf",
		RawQuery:   raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Close()
	if gotMethod != http.MethodGet {
		t.Fatalf("method=%s", gotMethod)
	}
	if gotQuery != raw {
		t.Fatalf("query not forwarded unchanged:\n got=%q\nwant=%q", gotQuery, raw)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, []byte("%PDF-1.4")) {
		t.Fatalf("body=%q", body)
	}
	if resp.Header.Get("Content-Disposition") == "" {
		t.Fatal("missing Content-Disposition")
	}
}

func TestParameterForwardingPOSTBody(t *testing.T) {
	var gotCT string
	var gotBody string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b\n1,2\n"))
	}))
	defer srv.Close()

	gw := testGateway(t, srv.URL, srv.URL+"/img", time.Second, func(string) bool { return true })
	payload := "ids=1&ids=2&name=%E2%9C%93"
	resp, err := gw.ExecuteReport(context.Background(), jasper.ExecuteRequest{
		Method:      http.MethodPost,
		ReportName:  "demo",
		Format:      "csv",
		RawQuery:    "_repName=demo&_repFormat=csv",
		Body:        strings.NewReader(payload),
		ContentType: "application/x-www-form-urlencoded",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Close()
	if gotMethod != http.MethodPost {
		t.Fatalf("method=%s", gotMethod)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type=%s", gotCT)
	}
	if gotBody != payload {
		t.Fatalf("body=%q", gotBody)
	}
}

func TestTimeoutHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	gw := testGateway(t, srv.URL, srv.URL+"/img", 50*time.Millisecond, func(string) bool { return true })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := gw.ExecuteReport(ctx, jasper.ExecuteRequest{
		Method:     http.MethodGet,
		ReportName: "demo",
		Format:     "pdf",
		RawQuery:   "_repName=demo&_repFormat=pdf",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	ge, ok := jasper.AsError(err)
	if !ok {
		t.Fatalf("expected gateway error, got %T %v", err, err)
	}
	if ge.HTTPStatus() != http.StatusGatewayTimeout {
		t.Fatalf("status=%d code=%s", ge.HTTPStatus(), ge.Code)
	}
}

func TestStreamingResponse(t *testing.T) {
	chunks := []string{"AAA", "BBB", "CCC"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Length", "9")
		flusher, _ := w.(http.Flusher)
		for _, c := range chunks {
			_, _ = w.Write([]byte(c))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	gw := testGateway(t, srv.URL, srv.URL+"/img", time.Second, func(string) bool { return true })
	resp, err := gw.ExecuteReport(context.Background(), jasper.ExecuteRequest{
		Method:     http.MethodGet,
		ReportName: "demo",
		Format:     "pdf",
		RawQuery:   "_repName=demo&_repFormat=pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Close()
	if resp.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("ct=%s", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Content-Length") != "9" {
		t.Fatalf("cl=%s", resp.Header.Get("Content-Length"))
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "AAABBBCCC" {
		t.Fatalf("body=%q", got)
	}
}

func TestLargeResponseStreaming(t *testing.T) {
	const size = 8 << 20 // 8 MiB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		buf := make([]byte, 64*1024)
		for i := range buf {
			buf[i] = byte(i)
		}
		remaining := size
		for remaining > 0 {
			n := len(buf)
			if n > remaining {
				n = remaining
			}
			_, _ = w.Write(buf[:n])
			remaining -= n
		}
	}))
	defer srv.Close()

	gw := testGateway(t, srv.URL, srv.URL+"/img", 10*time.Second, func(string) bool { return true })
	resp, err := gw.ExecuteReport(context.Background(), jasper.ExecuteRequest{
		Method:     http.MethodGet,
		ReportName: "demo",
		Format:     "xlsx",
		RawQuery:   "_repName=demo&_repFormat=xlsx",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Close()

	// Consume in small buffers — constant memory from the caller's perspective.
	var total int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		total += int64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if total != size {
		t.Fatalf("bytes=%d want=%d", total, size)
	}
}

func TestInvalidReportName(t *testing.T) {
	gw := testGateway(t, "http://127.0.0.1:9/report", "http://127.0.0.1:9/img", time.Second, nil)
	_, err := gw.ExecuteReport(context.Background(), jasper.ExecuteRequest{
		Method:     http.MethodGet,
		ReportName: "!!!",
		Format:     "pdf",
	})
	ge, ok := jasper.AsError(err)
	if !ok || ge.HTTPStatus() != http.StatusBadRequest {
		t.Fatalf("err=%v", err)
	}
}

func TestUnknownReport(t *testing.T) {
	gw := testGateway(t, "http://127.0.0.1:9/report", "http://127.0.0.1:9/img", time.Second, func(string) bool {
		return false
	})
	_, err := gw.ExecuteReport(context.Background(), jasper.ExecuteRequest{
		Method:     http.MethodGet,
		ReportName: "missing",
		Format:     "pdf",
		RawQuery:   "_repName=missing&_repFormat=pdf",
	})
	ge, ok := jasper.AsError(err)
	if !ok || ge.HTTPStatus() != http.StatusNotFound {
		t.Fatalf("err=%v", err)
	}
}

func TestTomcatUnavailable(t *testing.T) {
	gw := testGateway(t, "http://127.0.0.1:1/report", "http://127.0.0.1:1/img", 500*time.Millisecond, func(string) bool {
		return true
	})
	_, err := gw.ExecuteReport(context.Background(), jasper.ExecuteRequest{
		Method:     http.MethodGet,
		ReportName: "demo",
		Format:     "pdf",
		RawQuery:   "_repName=demo&_repFormat=pdf",
	})
	ge, ok := jasper.AsError(err)
	if !ok {
		t.Fatalf("expected gateway error, got %v", err)
	}
	if ge.HTTPStatus() != http.StatusServiceUnavailable && ge.HTTPStatus() != http.StatusGatewayTimeout {
		t.Fatalf("status=%d code=%s msg=%s", ge.HTTPStatus(), ge.Code, ge.Message)
	}
}

func TestHeaderPreservation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="x.pdf"`)
		w.Header().Set("Cache-Control", "private, max-age=0")
		w.Header().Set("X-Secret-Internal", "should-not-forward")
		_, _ = w.Write([]byte("pdf"))
	}))
	defer srv.Close()

	gw := testGateway(t, srv.URL, srv.URL+"/img", time.Second, func(string) bool { return true })
	resp, err := gw.ExecuteReport(context.Background(), jasper.ExecuteRequest{
		Method:     http.MethodGet,
		ReportName: "demo",
		Format:     "pdf",
		RawQuery:   "_repName=demo&_repFormat=pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Close()
	if resp.Header.Get("Content-Type") != "application/pdf" {
		t.Fatal("missing content-type")
	}
	if resp.Header.Get("Content-Disposition") == "" {
		t.Fatal("missing content-disposition")
	}
	if resp.Header.Get("Cache-Control") == "" {
		t.Fatal("missing cache-control")
	}
	if resp.Header.Get("X-Secret-Internal") != "" {
		t.Fatal("leaked internal header")
	}
}

func TestErrorTranslation(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{jasper.ErrBadRequest("bad"), 400},
		{jasper.ErrUnauthorized("nope"), 401},
		{jasper.ErrForbidden("no"), 403},
		{jasper.ErrNotFound("missing"), 404},
		{jasper.ErrUnavailable("down", errors.New("connection refused")), 503},
		{context.DeadlineExceeded, 504},
	}
	for _, tc := range cases {
		ge := jasper.Translate(tc.err)
		if ge.HTTPStatus() != tc.status {
			t.Fatalf("%v => %d want %d", tc.err, ge.HTTPStatus(), tc.status)
		}
		if strings.Contains(strings.ToLower(ge.PublicMessage()), "exception") {
			t.Fatalf("public message leaked stack-ish text: %s", ge.PublicMessage())
		}
	}

	ge := jasper.Translate(errors.New("java.lang.NullPointerException\n\tat net.sf.jasperreports.engine.foo"))
	if ge.HTTPStatus() != 500 {
		t.Fatalf("status=%d", ge.HTTPStatus())
	}
	if strings.Contains(ge.PublicMessage(), "NullPointer") || strings.Contains(ge.PublicMessage(), "jasperreports") {
		t.Fatalf("leaked java details: %s", ge.PublicMessage())
	}
}

func TestImageProxy(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", "4")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
	}))
	defer srv.Close()

	gw := testGateway(t, srv.URL+"/report", srv.URL, time.Second, nil)
	raw := "_repName=demo&image=img_0_0_0&uuid=abc"
	resp, err := gw.FetchReportImage(context.Background(), jasper.ImageRequest{
		ReportName: "demo",
		RawQuery:   raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Close()
	if gotQuery != raw {
		t.Fatalf("query=%q", gotQuery)
	}
	if resp.Header.Get("Content-Type") != "image/png" {
		t.Fatal("missing image content-type")
	}
}

func TestImageInvalidReportForbidden(t *testing.T) {
	gw := testGateway(t, "http://127.0.0.1:9/report", "http://127.0.0.1:9/img", time.Second, nil)
	_, err := gw.FetchReportImage(context.Background(), jasper.ImageRequest{
		ReportName: "",
		RawQuery:   "image=x",
	})
	ge, ok := jasper.AsError(err)
	if !ok || ge.HTTPStatus() != http.StatusForbidden {
		t.Fatalf("err=%v", err)
	}
}

func TestHTMLImageRewrite(t *testing.T) {
	cases := []struct {
		name string
		html string
		old  string
	}{
		{
			name: "php",
			html: `<html><img src="/report_image.php?image=chart1&uuid=u1"/></html>`,
			old:  "/report_image.php?image=",
		},
		{
			name: "tomcat10",
			html: `<html><img src="/acugis_report_image?image=img_0_0_0.png&uuid=u1"/></html>`,
			old:  "/acugis_report_image?image=",
		},
		{
			name: "tomcat10-context-path",
			html: `<html><img src="/acugis-report-server/acugis_report_image?image=img_0_0_0.png&uuid=u1"/></html>`,
			old:  "/acugis_report_image?image=",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(tc.html))
			}))
			defer srv.Close()

			gw := testGateway(t, srv.URL, srv.URL+"/img", time.Second, func(string) bool { return true })
			resp, err := gw.ExecuteReport(context.Background(), jasper.ExecuteRequest{
				Method:         http.MethodGet,
				ReportName:     "demo",
				Format:         "html",
				RawQuery:       "_repName=demo&_repFormat=html",
				ImageProxyBase: "/report-image",
			})
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Close()
			body, _ := io.ReadAll(resp.Body)
			got := string(body)
			if !strings.Contains(got, `/report-image?_repName=demo&image=`) {
				t.Fatalf("rewrite missing: %s", got)
			}
			if strings.Contains(got, tc.old) {
				t.Fatalf("original url still present: %s", got)
			}
			if strings.Contains(got, `/acugis-report-server/report-image`) {
				t.Fatalf("context path leaked into proxy url: %s", got)
			}
			if resp.Header.Get("Content-Length") != "" {
				t.Fatal("content-length should be removed after rewrite")
			}
		})
	}
}

func TestNormalizeReportKey(t *testing.T) {
	if got := jasper.NormalizeReportKey("My Report"); got != "my-report" {
		t.Fatalf("got=%q", got)
	}
	if !jasper.IsValidReportKey("demo_1") {
		t.Fatal("expected valid")
	}
	if jasper.IsValidReportKey("") {
		t.Fatal("expected invalid empty")
	}
}
