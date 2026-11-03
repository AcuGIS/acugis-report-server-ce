package report_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	report "github.com/acugis/acugis-report-go"
)

func TestBearerAuthAndMe(t *testing.T) {
	var gotAuth string
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		if r.URL.Path != "/api/v1/auth/me" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "username": "admin", "is_admin": true,
		})
	}))
	defer srv.Close()

	client, err := report.NewClient(srv.URL, report.WithToken("secret-token"))
	if err != nil {
		t.Fatal(err)
	}
	me, err := client.Auth.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if me.Username != "admin" {
		t.Fatalf("username=%q", me.Username)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if gotUA != report.UserAgent {
		t.Fatalf("ua=%q", gotUA)
	}
}

func TestReportListAndRun(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/reports", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count": 1,
			"items": []map[string]any{{"key": "nycdata", "name": "NYC Data", "type": "map"}},
		})
	})
	mux.HandleFunc("/reports/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("_repName") != "nycdata" {
			t.Fatalf("repName=%q", r.URL.Query().Get("_repName"))
		}
		if r.URL.Query().Get("_repFormat") != "pdf" {
			t.Fatalf("format=%q", r.URL.Query().Get("_repFormat"))
		}
		// multi-value params
		if got := r.URL.Query()["tags"]; len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("tags=%v", got)
		}
		if r.URL.Query().Get("city") != "NYC" {
			t.Fatalf("city=%q", r.URL.Query().Get("city"))
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 test"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := report.NewClient(srv.URL, report.WithToken("t"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := client.Reports.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if list.Count != 1 || list.Items[0].Key != "nycdata" {
		t.Fatalf("list=%+v", list)
	}
	bin, err := client.Reports.Run(context.Background(), "nycdata", &report.RunOptions{
		Format: "pdf",
		Params: map[string]any{
			"city": "NYC",
			"tags": []string{"a", "b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bin.ContentType, "pdf") {
		t.Fatalf("ct=%q", bin.ContentType)
	}
	if !bytes.HasPrefix(bin.Data, []byte("%PDF")) {
		t.Fatalf("data=%q", bin.Data)
	}
}

func TestBinaryDownloadAndMultipartPublish(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/resources/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("resource-bytes"))
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v1/reports", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		ct := r.Header.Get("Content-Type")
		media, params, err := mime.ParseMediaType(ct)
		if err != nil || media != "multipart/form-data" {
			t.Fatalf("content-type=%q err=%v", ct, err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		var sawJRXML, sawDS bool
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(part)
			switch part.FormName() {
			case "jrxml_file":
				sawJRXML = true
				if !bytes.Contains(b, []byte("<jasperReport")) {
					t.Fatalf("jrxml=%q", b)
				}
			case "data_source":
				sawDS = string(b) == "reporting_db"
			}
		}
		if !sawJRXML || !sawDS {
			t.Fatalf("missing parts jrxml=%v ds=%v", sawJRXML, sawDS)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "key": "demo"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := report.NewClient(srv.URL, report.WithToken("t"))
	if err != nil {
		t.Fatal(err)
	}
	dl, err := client.Resources.Download(context.Background(), "550e8400-e29b-41d4-a716-446655440000", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(dl.Data) != "resource-bytes" {
		t.Fatalf("data=%q", dl.Data)
	}
	pub, err := client.Reports.Publish(context.Background(), report.PublishOptions{
		JRXMLFile:     strings.NewReader(`<?xml version="1.0"?><jasperReport/>`),
		JRXMLFileName: "demo.jrxml",
		DataSource:    "reporting_db",
		ReportName:    "Demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pub.Key != "demo" {
		t.Fatalf("pub=%+v", pub)
	}
}

func TestFoldersAndResourceUpload(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/folders/tree", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "name": "Home"}})
	})
	mux.HandleFunc("/api/v1/folders/browse", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"folders": []map[string]any{{"id": 2, "name": "Finance"}},
			"items":   []map[string]any{},
		})
	})
	mux.HandleFunc("/api/v1/resources", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/") {
			t.Fatalf("ct=%q", ct)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uuid": "550e8400-e29b-41d4-a716-446655440000", "display_name": "key.pub",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := report.NewClient(srv.URL, report.WithToken("t"))
	if err != nil {
		t.Fatal(err)
	}
	tree, err := client.Folders.Tree(context.Background())
	if err != nil || len(tree) != 1 || tree[0].Name != "Home" {
		t.Fatalf("tree=%v err=%v", tree, err)
	}
	browse, err := client.Folders.Browse(context.Background(), 0)
	if err != nil || len(browse.Folders) != 1 {
		t.Fatalf("browse=%v err=%v", browse, err)
	}
	res, err := client.Resources.Upload(context.Background(), report.UploadResourceOptions{
		DisplayName:  "key.pub",
		ResourceType: "public_key",
		File:         strings.NewReader("ssh-rsa AAAAB3"),
		FileName:     "key.pub",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.UUID == "" {
		t.Fatal("missing uuid")
	}
}

func TestErrorParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid token", "hint": "rotate"})
	}))
	defer srv.Close()

	client, err := report.NewClient(srv.URL, report.WithToken("bad"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Auth.Me(context.Background())
	var authErr *report.AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("want AuthenticationError, got %T %v", err, err)
	}
	if authErr.Message() != "invalid token" {
		t.Fatalf("msg=%q", authErr.Message())
	}
	if authErr.Hint() != "rotate" {
		t.Fatalf("hint=%q", authErr.Hint())
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := report.NewClient(srv.URL, report.WithToken("t"), report.WithTimeout(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Auth.Me(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
	var to *report.TimeoutError
	var netErr *report.NetworkError
	if !errors.As(err, &to) && !errors.As(err, &netErr) {
		t.Fatalf("want TimeoutError or NetworkError, got %T %v", err, err)
	}
}

func TestMultiValueParamsEncode(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()
	client, err := report.NewClient(srv.URL, report.WithToken("t"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Reports.Run(context.Background(), "r1", &report.RunOptions{
		Params: map[string]any{"ids": []string{"1", "2", "3"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rawQuery, "ids=1") || !strings.Contains(rawQuery, "ids=2") || !strings.Contains(rawQuery, "ids=3") {
		t.Fatalf("query=%q", rawQuery)
	}
}

func TestReservedNotImplemented(t *testing.T) {
	client, err := report.NewClient("http://example.com", report.WithToken("t"))
	if err != nil {
		t.Fatal(err)
	}
	err = client.Users.NotImplemented()
	if !errors.Is(err, report.ErrNotImplemented) {
		t.Fatalf("got %v", err)
	}
}

func TestRunToFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-file"))
	}))
	defer srv.Close()
	client, err := report.NewClient(srv.URL, report.WithToken("t"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pdf")
	_, err = client.Reports.RunToFile(context.Background(), "demo", path, &report.RunOptions{Format: "pdf"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "%PDF-file" {
		t.Fatalf("file=%q", b)
	}
}

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv(report.EnvURL, "http://localhost:9999")
	t.Setenv(report.EnvToken, "env-token")
	client, err := report.NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if client.BaseURL() != "http://localhost:9999" {
		t.Fatalf("url=%q", client.BaseURL())
	}
	if client.Token() != "env-token" {
		t.Fatalf("token=%q", client.Token())
	}
}
