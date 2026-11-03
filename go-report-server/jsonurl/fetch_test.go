package jsonurl

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchSuccessRedirectAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method %s", r.Method)
		}
		switch r.URL.Path {
		case "/ok":
			_, _ = io.WriteString(w, `[{"id":1}]`)
		case "/missing":
			http.Error(w, "secret-internal-path", http.StatusNotFound)
		case "/big":
			_, _ = io.WriteString(w, strings.Repeat("x", 64))
		case "/5":
			http.Redirect(w, r, "/4", http.StatusFound)
		case "/4":
			http.Redirect(w, r, "/3", http.StatusFound)
		case "/3":
			http.Redirect(w, r, "/2", http.StatusFound)
		case "/2":
			http.Redirect(w, r, "/1", http.StatusFound)
		case "/1":
			http.Redirect(w, r, "/0", http.StatusFound)
		case "/0":
			_, _ = io.WriteString(w, `[{"id":2}]`)
		case "/6":
			http.Redirect(w, r, "/5", http.StatusFound)
		case "/meta":
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
		case "/slow":
			time.Sleep(300 * time.Millisecond)
			_, _ = io.WriteString(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	pol := defaultPolicy()
	pol.permitPrivate = true

	body, err := fetch(context.Background(), srv.URL+"/ok", pol)
	if err != nil || string(body) != `[{"id":1}]` {
		t.Fatalf("body %s err %v", body, err)
	}
	body, err = fetch(context.Background(), srv.URL+"/1", pol)
	if err != nil || string(body) != `[{"id":2}]` {
		t.Fatalf("redirect body %s err %v", body, err)
	}
	if _, err := fetch(context.Background(), srv.URL+"/6", pol); !errors.Is(err, ErrRejected) {
		t.Fatalf("redirect limit %v", err)
	}
	if _, err := fetch(context.Background(), srv.URL+"/meta", pol); !errors.Is(err, ErrRejected) || strings.Contains(err.Error(), "169.254") {
		t.Fatalf("metadata redirect %v", err)
	}
	_, err = fetch(context.Background(), srv.URL+"/missing", pol)
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") || strings.Contains(err.Error(), "secret-internal") || strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("status %v", err)
	}
	small := pol
	small.maxBytes = 8
	if _, err := fetch(context.Background(), srv.URL+"/big", small); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("size %v", err)
	}
	slow := pol
	slow.timeout = 50 * time.Millisecond
	if _, err := fetch(context.Background(), srv.URL+"/slow", slow); !errors.Is(err, ErrFailed) || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("timeout %v", err)
	}
}

func TestSSRFRejection(t *testing.T) {
	original := lookupHost
	t.Cleanup(func() { lookupHost = original })
	lookupHost = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.1.2.3"), net.ParseIP("8.8.8.8")}, nil
	}
	cases := []string{
		"http://127.0.0.1/latest/meta-data",
		"http://[::1]/",
		"http://10.1.2.3/a",
		"http://192.168.1.20/a",
		"http://172.16.0.5/a",
		"http://169.254.169.254/latest/meta-data",
		"http://100.100.100.200/",
		"http://localhost/admin",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://user:secret@example.com/data.json",
		"file:///etc/passwd",
		"http://2130706433/",
		"http://example.com/data.json",
		"://bad",
		"https://",
	}
	for _, raw := range cases {
		err := Validate(raw)
		if raw == "http://example.com/data.json" {
			if err != nil {
				t.Fatalf("public shape %v", err)
			}
			_, err = Get(context.Background(), raw)
		} else if strings.HasPrefix(raw, "://") || raw == "https://" {
			err = Validate(raw)
		}
		if raw == "http://example.com/data.json" {
			if !errors.Is(err, ErrRejected) || strings.Contains(err.Error(), "10.1.2.3") || strings.Contains(err.Error(), "example.com") {
				t.Fatalf("dns %s -> %v", raw, err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("accepted %s", raw)
		}
		if strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "passwd") || strings.Contains(err.Error(), "169.254") || strings.Contains(err.Error(), "metadata") {
			t.Fatalf("leaked %s -> %v", raw, err)
		}
	}
}
