package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

type stubNamerFinder struct {
	names []string
}

func (s stubNamerFinder) Find(ctx context.Context, name string) (*datasources.Datasource, error) {
	return &datasources.Datasource{Name: name, DBType: "postgresql"}, nil
}

func (s stubNamerFinder) ListNames(context.Context) ([]string, error) {
	return s.names, nil
}

type noopACLWriter struct{}

func (noopACLWriter) SyncReportAccessGroups(context.Context, string, string, []string, int64) error {
	return nil
}
func (noopACLWriter) SyncExplorerViewAccessGroups(context.Context, string, string, []string, int64) error {
	return nil
}
func (noopACLWriter) ListAccessGroups(context.Context) ([]string, error) {
	return []string{"Public", "Editors"}, nil
}
func (noopACLWriter) ListResourcePermissionGroups(context.Context, string, string) ([]string, error) {
	return nil, nil
}
func (noopACLWriter) DeleteResource(context.Context, string, string) error {
	return nil
}

func TestPublishEditDeleteFlow(t *testing.T) {
	jsonDir := t.TempDir()
	jrxmlDir := t.TempDir()
	thumbDir := t.TempDir()
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)
	mustWrite(t, filepath.Join(appRoot, "reports", "portal.css"), `.portal-home{}`)

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.ReportJSONDir = jsonDir
	cfg.Paths.JRXMLDir = jrxmlDir
	cfg.Paths.ThumbnailDir = thumbDir
	store := reports.NewStore(jsonDir, thumbDir, jrxmlDir)

	s := New(cfg, Dependencies{
		Reports:     store,
		ACL:         acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: true}),
		Datasources: stubNamerFinder{names: []string{"MainDS"}},
		ACLWriter:   noopACLWriter{},
	}, slog.Default())

	// Publish page GET
	req := httptest.NewRequest(http.MethodGet, "/reports/publish_report", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("publish get status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Publish Report") || !strings.Contains(rec.Body.String(), "MainDS") {
		t.Fatalf("publish body missing content: %s", rec.Body.String())
	}

	// Publish via multipart
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("report_name", "Demo Report")
	_ = w.WriteField("report_description", "Desc")
	_ = w.WriteField("report_type", "jasper")
	_ = w.WriteField("data_source", "MainDS")
	_ = w.WriteField("access_groups[]", "Public")
	part, err := w.CreateFormFile("jrxml_file", "demo.jrxml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, `<?xml version="1.0"?><jasperReport name="demo"/>`)
	_ = w.Close()

	req = httptest.NewRequest(http.MethodPost, "/api/v1/reports", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish api status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["key"] != "demo" {
		t.Fatalf("created=%v", created)
	}
	if _, err := os.Stat(filepath.Join(jsonDir, "demo.report.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(jrxmlDir, "demo.jrxml")); err != nil {
		t.Fatal(err)
	}

	// Catalog New menu links to Go publish; edit/delete are action-driven via API/JS.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	bodyStr := rec.Body.String()
	for _, want := range []string{
		"/reports/publish_report",
		`data-new="resource"`,
		`fe-layout`,
	} {
		if !strings.Contains(bodyStr, want) {
			t.Fatalf("catalog missing %q", want)
		}
	}
	if strings.Contains(bodyStr, "/acugis-report-server/reports/publish_report.php") {
		t.Fatal("catalog still linking publish to PHP mount")
	}

	// Edit page GET
	req = httptest.NewRequest(http.MethodGet, "/reports/edit_report?report=demo", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit get status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Edit Report") || !strings.Contains(rec.Body.String(), "Demo Report") {
		t.Fatalf("edit body=%s", rec.Body.String())
	}

	// Edit POST
	editBody := &bytes.Buffer{}
	ew := multipart.NewWriter(editBody)
	_ = ew.WriteField("report_name", "Demo Updated")
	_ = ew.WriteField("report_description", "Updated")
	_ = ew.WriteField("report_type", "jasper")
	_ = ew.WriteField("data_source", "MainDS")
	_ = ew.Close()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/reports/demo", editBody)
	req.Header.Set("Content-Type", ew.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit api status=%d body=%s", rec.Code, rec.Body.String())
	}
	def, err := store.Get("demo")
	if err != nil {
		t.Fatal(err)
	}
	if def.DisplayName() != "Demo Updated" {
		t.Fatalf("name=%s", def.DisplayName())
	}

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/reports/demo", nil)
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	if store.Exists("demo") {
		t.Fatal("expected deleted")
	}
}
