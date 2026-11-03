package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

type memDSStore struct {
	mu   sync.Mutex
	next int64
	rows map[string]datasources.Datasource
}

func newMemDSStore() *memDSStore {
	return &memDSStore{next: 1, rows: map[string]datasources.Datasource{}}
}

func (m *memDSStore) Find(_ context.Context, name string) (*datasources.Datasource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ds, ok := m.rows[name]
	if !ok {
		return nil, datasources.ErrNotFound
	}
	cp := ds
	return &cp, nil
}
func (m *memDSStore) ListNames(context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.rows))
	for name := range m.rows {
		out = append(out, name)
	}
	return out, nil
}
func (m *memDSStore) ListEnabled(context.Context) ([]datasources.Datasource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]datasources.Datasource, 0, len(m.rows))
	for _, ds := range m.rows {
		out = append(out, ds)
	}
	return out, nil
}
func (m *memDSStore) Upsert(_ context.Context, ds datasources.Datasource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.rows[ds.Name]; ok {
		ds.ID = existing.ID
		if ds.Password == "" {
			ds.Password = existing.Password
		}
	} else {
		ds.ID = m.next
		m.next++
	}
	m.rows[ds.Name] = ds
	return nil
}
func (m *memDSStore) DeleteByID(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, ds := range m.rows {
		if ds.ID == id {
			delete(m.rows, name)
			return nil
		}
	}
	return nil
}
func (m *memDSStore) EnsureDBTypeColumn(context.Context) error  { return nil }
func (m *memDSStore) EnsureAuthColumns(context.Context) error   { return nil }
func (m *memDSStore) EnsureOptionsColumn(context.Context) error { return nil }
func (m *memDSStore) TableAvailable(context.Context) bool        { return true }

func TestDatasourcesPageAndContext(t *testing.T) {
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)
	ctxPath := filepath.Join(appRoot, "docker", "report-server", "context.xml")

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.JasperContextPath = ctxPath

	store := newMemDSStore()
	s := New(cfg, Dependencies{
		ACL:         acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: true}),
		Datasources: store,
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/reports/datasources", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Datasources", "Add New Data Source", "Generate Context XML", "/reports/generate_jasper_context", "/reports/edit_datasource", `name="return_to"`, `id="generate-context-form"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(body, `href="`+s.goURL("/reports/generate_jasper_context")+`"`) {
		t.Fatal("Generate Context XML should be a form action, not a navigation href")
	}
	if strings.Contains(body, "Authentication") || strings.Contains(body, "id=\"datasourceForm\"") {
		t.Fatalf("list page should not include add form, body=%s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/edit_datasource", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit status=%d body=%s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	for _, want := range []string{"Add Datasource", "Authentication", "Key Pair", "id=\"datasourceForm\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("edit page missing %q", want)
		}
	}

	form := strings.NewReader("name=MainPG&db_type=postgresql&host=localhost&port=5432&dbname=reports&username=jrs&password=secret&schema_name=public")
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_datasource.php", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/reports/datasources") || strings.Contains(loc, ".php") || !strings.Contains(loc, "message=") {
		t.Fatalf("unexpected redirect location=%q", loc)
	}

	req = httptest.NewRequest(http.MethodGet, loc, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list after save status=%d body=%s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, "Datasource saved") || !strings.Contains(body, "MainPG") {
		t.Fatalf("save body=%s", body)
	}
	if !strings.Contains(body, `class="btn-small waves-effect waves-light blue edit-ds-btn"`) {
		t.Fatalf("missing Edit button in body=%s", body)
	}
	if !strings.Contains(body, `/reports/edit_datasource?name=MainPG`) {
		t.Fatalf("missing edit link in body=%s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/edit_datasource?name=MainPG", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit existing status=%d body=%s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, `data-name="MainPG"`) || !strings.Contains(body, `data-host="localhost"`) {
		t.Fatalf("missing edit bootstrap data attributes in body=%s", body)
	}

	// Blank password on update must keep the existing secret.
	form = strings.NewReader("name=MainPG&db_type=postgresql&host=localhost&port=5432&dbname=reports&username=jrs&password=&schema_name=public")
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_datasource.php", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("update status=%d body=%s", rec.Code, rec.Body.String())
	}
	got, err := store.Find(context.Background(), "MainPG")
	if err != nil {
		t.Fatalf("find after blank-password update: %v", err)
	}
	if got.Password != "secret" {
		t.Fatalf("password cleared on blank update: %q", got.Password)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/generate_jasper_context", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("context status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Context XML generated at") {
		t.Fatalf("context body=%s", rec.Body.String())
	}

	form = strings.NewReader("return_to=datasources")
	req = httptest.NewRequest(http.MethodPost, "/reports/generate_jasper_context", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("ui context status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc = rec.Header().Get("Location")
	if !strings.Contains(loc, "/reports/datasources") || !strings.Contains(loc, "message=") || !strings.Contains(loc, url.QueryEscape("Jasper context regenerated successfully.")) {
		t.Fatalf("unexpected ui context location=%q", loc)
	}
	req = httptest.NewRequest(http.MethodGet, loc, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list after context status=%d body=%s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, "Jasper context regenerated successfully.") || !strings.Contains(body, "MainPG") {
		t.Fatalf("ui context flash body=%s", body)
	}
	if strings.Contains(body, "Context XML generated at") {
		t.Fatal("ui context must not render the raw generator response")
	}
	raw, err := os.ReadFile(ctxPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `name="jdbc/MainPG"`) {
		t.Fatalf("context.xml=%s", raw)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/datasources", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"MainPG"`) {
		t.Fatalf("api list=%s", rec.Body.String())
	}
}

func TestGenerateContextUIFailureStaysOnDatasources(t *testing.T) {
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.JasperContextPath = filepath.Join(appRoot, "docker", "report-server", "context.xml")

	s := New(cfg, Dependencies{
		ACL:         acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: true}),
		Datasources: newMemDSStore(),
	}, slog.Default())

	form := strings.NewReader("return_to=datasources")
	req := httptest.NewRequest(http.MethodPost, "/reports/generate_jasper_context", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/reports/datasources") || !strings.Contains(loc, "error=") {
		t.Fatalf("location=%q", loc)
	}
	if strings.Contains(rec.Body.String(), "No datasources found") && rec.Header().Get("Content-Type") == "text/plain" {
		t.Fatal("ui failure must not replace the application with a plain-text page")
	}

	req = httptest.NewRequest(http.MethodGet, loc, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "No datasources found") || !strings.Contains(body, "Generate Context XML") {
		t.Fatalf("expected datasources UI with error, body=%s", body)
	}
}

func TestQCartaDatasourcesOmitsSidebarBrand(t *testing.T) {
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Profile = config.ProfileQCarta
	cfg.Paths.AppRoot = appRoot

	s := New(cfg, Dependencies{
		ACL:         acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: true}),
		Datasources: newMemDSStore(),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/reports/datasources", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "fed-shell-sidebar-brand") || strings.Contains(body, ">QCarta<") || strings.Contains(body, ">AcuGIS Report Server<") {
		t.Fatalf("qcarta datasources sidebar still has a product header: %s", body)
	}
}
