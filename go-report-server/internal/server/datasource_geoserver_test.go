package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
)

type memGeoStore struct {
	mu   sync.Mutex
	next int64
	rows map[string]geoserver.Connection
}

func newMemGeoStore() *memGeoStore {
	return &memGeoStore{next: 1, rows: map[string]geoserver.Connection{}}
}

func (m *memGeoStore) ListEnabled(context.Context) ([]geoserver.Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]geoserver.Connection, 0, len(m.rows))
	for _, row := range m.rows {
		out = append(out, row)
	}
	return out, nil
}

func (m *memGeoStore) FindGeoServer(_ context.Context, name string) (geoserver.Credentials, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[name]
	if !ok {
		return geoserver.Credentials{}, geoserver.ErrNotFound
	}
	return geoserver.Credentials{Name: row.Name, URL: row.URL, Username: row.Username, Password: row.Password}, nil
}

func (m *memGeoStore) Upsert(_ context.Context, c geoserver.Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.rows[c.Name]; ok {
		c.ID = existing.ID
		if strings.TrimSpace(c.Password) == "" && strings.TrimSpace(c.Username) != "" {
			c.Password = existing.Password
		}
	} else {
		c.ID = m.next
		m.next++
	}
	m.rows[c.Name] = c
	return nil
}

func (m *memGeoStore) DeleteByID(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, row := range m.rows {
		if row.ID == id {
			delete(m.rows, name)
			return nil
		}
	}
	return nil
}

func (m *memGeoStore) TableAvailable(context.Context) bool { return true }

func TestGeoServerDatasourceCRUD(t *testing.T) {
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)
	propsPath := filepath.Join(appRoot, "geoserver-connections.properties")

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.GeoServerConnectionsPath = propsPath

	jdbc := newMemDSStore()
	geo := newMemGeoStore()
	if err := geo.Upsert(context.Background(), geoserver.Connection{
		Name: "MapRoads",
		URL:  "https://maps.example.com/geoserver",
	}); err != nil {
		t.Fatal(err)
	}

	s := New(cfg, Dependencies{
		ACL:         acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: true}),
		Datasources: jdbc,
		GeoServer:   geo,
		ArcGIS:      newMemArcStore(),
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/reports/edit_datasource", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `value="geoserver"`) || !strings.Contains(body, "GeoServer") {
		t.Fatalf("add page status=%d missing GeoServer option", rec.Code)
	}
	if !strings.Contains(body, "ArcGIS Feature Server") {
		t.Fatal("ArcGIS Feature Server option disappeared")
	}

	form := strings.NewReader("name=ProdGeoServer&db_type=geoserver&service_url=https://maps.example.com/geoserver/wfs&username=editor&password=s3cret")
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_datasource", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save status=%d body=%s", rec.Code, rec.Body.String())
	}
	cred, err := geo.FindGeoServer(context.Background(), "ProdGeoServer")
	if err != nil {
		t.Fatal(err)
	}
	if cred.URL != "https://maps.example.com/geoserver/wfs" || cred.Username != "editor" || cred.Password != "s3cret" {
		t.Fatalf("stored=%+v", cred)
	}
	if _, err := jdbc.Find(context.Background(), "ProdGeoServer"); err == nil {
		t.Fatal("GeoServer datasource was also written to jasper_datasource")
	}
	props, err := os.ReadFile(propsPath)
	if err != nil || !strings.Contains(string(props), "ProdGeoServer.url=") || !strings.Contains(string(props), "MapRoads.url=") {
		t.Fatalf("properties=%s err=%v", props, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/datasources", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body = rec.Body.String()
	if !strings.Contains(body, "ProdGeoServer") || !strings.Contains(body, "MapRoads") || !strings.Contains(body, "GeoServer") {
		t.Fatalf("list missing geoserver rows")
	}
	if strings.Contains(body, "s3cret") {
		t.Fatal("datasource list exposed the password")
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/edit_datasource?name=ProdGeoServer&db_type=geoserver", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body = rec.Body.String()
	if !strings.Contains(body, `data-db-type="geoserver"`) || !strings.Contains(body, `data-host="https://maps.example.com/geoserver/wfs"`) {
		t.Fatalf("edit bootstrap missing geoserver fields")
	}
	if strings.Contains(body, "s3cret") {
		t.Fatal("edit page exposed the password")
	}

	form = strings.NewReader("name=ProdGeoServer&db_type=geoserver&service_url=https://maps.example.com/geoserver&username=editor&password=")
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_datasource", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("blank password update status=%d body=%s", rec.Code, rec.Body.String())
	}
	cred, err = geo.FindGeoServer(context.Background(), "ProdGeoServer")
	if err != nil || cred.Password != "s3cret" || cred.URL != "https://maps.example.com/geoserver" {
		t.Fatalf("password not preserved: %+v %v", cred, err)
	}

	form = strings.NewReader("name=ProdGeoServer&db_type=geoserver&service_url=https://maps.example.com/geoserver&username=&password=")
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_datasource", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	cred, err = geo.FindGeoServer(context.Background(), "ProdGeoServer")
	if err != nil || cred.Username != "" || cred.Password != "" {
		t.Fatalf("anonymous update=%+v %v", cred, err)
	}

	if err := jdbc.Upsert(context.Background(), datasources.Datasource{Name: "MainPG", DBType: "postgresql", Host: "localhost", DBName: "db", Username: "u", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	form = strings.NewReader("name=MainPG&db_type=geoserver&service_url=https://maps.example.com/geoserver")
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_datasource", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("collision status=%d body=%s", rec.Code, rec.Body.String())
	}

	rows, _ := geo.ListEnabled(context.Background())
	var prodID int64
	for _, row := range rows {
		if row.Name == "ProdGeoServer" {
			prodID = row.ID
		}
	}
	if prodID == 0 {
		t.Fatal("missing geoserver id")
	}
	form = strings.NewReader("action=delete&id=" + strconv.FormatInt(prodID, 10) + "&db_type=geoserver")
	req = httptest.NewRequest(http.MethodPost, "/reports/datasources", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if _, err := geo.FindGeoServer(context.Background(), "ProdGeoServer"); err == nil {
		t.Fatal("geoserver row was not deleted")
	}
	if _, err := jdbc.Find(context.Background(), "MainPG"); err != nil {
		t.Fatal("deleting the geoserver id also removed the jdbc datasource")
	}
	if _, err := geo.FindGeoServer(context.Background(), "MapRoads"); err != nil {
		t.Fatal("deleting one geoserver row removed another")
	}
}

func TestGeoServerDatasourceTestConnection(t *testing.T) {
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.GeoServerConnectionsPath = filepath.Join(appRoot, "geoserver-connections.properties")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("request") != "GetCapabilities" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "" && !strings.Contains(r.Header.Get("Authorization"), "Basic ") {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`<wfs:WFS_Capabilities service="WFS"/>`))
	}))
	defer srv.Close()

	s := New(cfg, Dependencies{
		ACL:         acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: true}),
		Datasources: newMemDSStore(),
		GeoServer:   newMemGeoStore(),
	}, slog.Default())

	form := strings.NewReader("name=PublicGS&db_type=geoserver&service_url=" + srv.URL + "/geoserver")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/test", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) || !strings.Contains(rec.Body.String(), "anonymous") {
		t.Fatalf("anonymous test status=%d body=%s", rec.Code, rec.Body.String())
	}

	form = strings.NewReader("name=SecureGS&db_type=geoserver&service_url=" + srv.URL + "/geoserver&username=editor&password=s3cret")
	req = httptest.NewRequest(http.MethodPost, "/api/v1/datasources/test", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("credential test status=%d body=%s", rec.Code, rec.Body.String())
	}

	form = strings.NewReader("name=Bad&db_type=geoserver&service_url=notaurl")
	req = httptest.NewRequest(http.MethodPost, "/api/v1/datasources/test", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "invalid GeoServer URL") {
		t.Fatalf("invalid url body=%s", rec.Body.String())
	}
}
