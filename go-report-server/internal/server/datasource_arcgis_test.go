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
	"github.com/acugis/acugis-report-server-go/internal/arcgis"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

type memArcStore struct {
	mu   sync.Mutex
	next int64
	rows map[string]arcgis.Connection
}

func newMemArcStore() *memArcStore {
	return &memArcStore{next: 1, rows: map[string]arcgis.Connection{}}
}

func (m *memArcStore) ListEnabled(context.Context) ([]arcgis.Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]arcgis.Connection, 0, len(m.rows))
	for _, row := range m.rows {
		out = append(out, row)
	}
	return out, nil
}

func (m *memArcStore) FindArcGIS(_ context.Context, name string) (arcgis.Credentials, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[name]
	if !ok {
		return arcgis.Credentials{}, arcgis.ErrNotFound
	}
	return arcgis.Credentials{Name: row.Name, URL: row.URL, Username: row.Username, Password: row.Password}, nil
}

func (m *memArcStore) Upsert(_ context.Context, c arcgis.Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.rows[c.Name]; ok {
		c.ID = existing.ID
	} else {
		c.ID = m.next
		m.next++
	}
	m.rows[c.Name] = c
	return nil
}

func (m *memArcStore) DeleteByID(_ context.Context, id int64) error {
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

func (m *memArcStore) TableAvailable(context.Context) bool { return true }

func TestArcGISFeatureServerDatasourceCRUD(t *testing.T) {
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)
	propsPath := filepath.Join(appRoot, "arcgis-connections.properties")

	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.ArcGISConnectionsPath = propsPath

	jdbc := newMemDSStore()
	arc := newMemArcStore()
	// An existing map connection must appear as a datasource without a second row.
	if err := arc.Upsert(context.Background(), arcgis.Connection{
		Name: "MapNeighborhoods",
		URL:  "https://services.example.com/arcgis/rest/services/Neighborhoods/FeatureServer",
	}); err != nil {
		t.Fatal(err)
	}

	s := New(cfg, Dependencies{
		ACL:         acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: true}),
		Datasources: jdbc,
		ArcGIS:      arc,
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/reports/edit_datasource", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ArcGIS Feature Server") {
		t.Fatalf("add page status=%d body missing type", rec.Code)
	}

	form := strings.NewReader("name=WeatherFS&db_type=arcgis&service_url=https://services.example.com/arcgis/rest/services/Weer/FeatureServer/0&username=editor&password=s3cret")
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_datasource", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save status=%d body=%s", rec.Code, rec.Body.String())
	}
	cred, err := arc.FindArcGIS(context.Background(), "WeatherFS")
	if err != nil {
		t.Fatal(err)
	}
	if cred.URL != "https://services.example.com/arcgis/rest/services/Weer/FeatureServer" || cred.Username != "editor" || cred.Password != "s3cret" {
		t.Fatalf("stored=%+v", cred)
	}
	if _, err := jdbc.Find(context.Background(), "WeatherFS"); err == nil {
		t.Fatal("ArcGIS datasource was also written to jasper_datasource")
	}
	props, err := os.ReadFile(propsPath)
	if err != nil || !strings.Contains(string(props), "WeatherFS.url=") || !strings.Contains(string(props), "MapNeighborhoods.url=") {
		t.Fatalf("properties=%s err=%v", props, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/datasources", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "WeatherFS") || !strings.Contains(body, "MapNeighborhoods") || !strings.Contains(body, "ArcGIS Feature Server") {
		t.Fatalf("list missing arcgis rows: %s", body)
	}
	if strings.Contains(body, "s3cret") {
		t.Fatal("datasource list exposed the password")
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/edit_datasource?name=WeatherFS&db_type=arcgis", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body = rec.Body.String()
	if !strings.Contains(body, `data-db-type="arcgis"`) || !strings.Contains(body, `data-host="https://services.example.com/arcgis/rest/services/Weer/FeatureServer"`) {
		t.Fatalf("edit bootstrap missing arcgis fields")
	}
	if strings.Contains(body, "s3cret") {
		t.Fatal("edit page exposed the password")
	}

	form = strings.NewReader("name=WeatherFS&db_type=arcgis&service_url=https://services.example.com/arcgis/rest/services/Weer/FeatureServer&username=editor&password=")
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_datasource", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("blank password update status=%d body=%s", rec.Code, rec.Body.String())
	}
	cred, err = arc.FindArcGIS(context.Background(), "WeatherFS")
	if err != nil || cred.Password != "s3cret" {
		t.Fatalf("password not preserved: %+v %v", cred, err)
	}

	form = strings.NewReader("name=WeatherFS&db_type=arcgis&service_url=https://services.example.com/arcgis/rest/services/Weer/FeatureServer&username=&password=")
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_datasource", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	cred, err = arc.FindArcGIS(context.Background(), "WeatherFS")
	if err != nil || cred.Username != "" || cred.Password != "" {
		t.Fatalf("anonymous update=%+v %v", cred, err)
	}

	if err := jdbc.Upsert(context.Background(), datasources.Datasource{Name: "MainPG", DBType: "postgresql", Host: "localhost", DBName: "db", Username: "u", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	form = strings.NewReader("name=MainPG&db_type=arcgis&service_url=https://services.example.com/arcgis/rest/services/Other/FeatureServer")
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_datasource", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("collision status=%d body=%s", rec.Code, rec.Body.String())
	}

	names := s.listDatasourceNames(httptest.NewRequest(http.MethodGet, "/", nil))
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "WeatherFS") || !strings.Contains(joined, "MapNeighborhoods") || !strings.Contains(joined, "MainPG") {
		t.Fatalf("publish names=%v", names)
	}

	rows, _ := arc.ListEnabled(context.Background())
	var weatherID int64
	for _, row := range rows {
		if row.Name == "WeatherFS" {
			weatherID = row.ID
		}
	}
	if weatherID == 0 {
		t.Fatal("missing weather id")
	}
	form = strings.NewReader("action=delete&id=" + strconv.FormatInt(weatherID, 10) + "&db_type=arcgis")
	req = httptest.NewRequest(http.MethodPost, "/reports/datasources", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if _, err := arc.FindArcGIS(context.Background(), "WeatherFS"); err == nil {
		t.Fatal("arcgis row was not deleted")
	}
	if _, err := jdbc.Find(context.Background(), "MainPG"); err != nil {
		t.Fatal("deleting the arcgis id also removed the jdbc datasource")
	}
}

func TestArcGISDatasourceTestConnectionMessages(t *testing.T) {
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.ArcGISConnectionsPath = filepath.Join(appRoot, "arcgis-connections.properties")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid username or password."}}`))
			return
		}
		_, _ = w.Write([]byte(`{"layers":[{"id":0,"name":"Public"}]}`))
	}))
	defer srv.Close()

	s := New(cfg, Dependencies{
		ACL:         acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: true}),
		Datasources: newMemDSStore(),
		ArcGIS:      newMemArcStore(),
	}, slog.Default())

	form := strings.NewReader("name=PublicFS&db_type=arcgis&service_url=" + srv.URL + "/arcgis/rest/services/Public/FeatureServer")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/test", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("anonymous test status=%d body=%s", rec.Code, rec.Body.String())
	}

	form = strings.NewReader("name=SecureFS&db_type=arcgis&service_url=" + srv.URL + "/arcgis/rest/services/Secure/FeatureServer&username=editor&password=wrong")
	req = httptest.NewRequest(http.MethodPost, "/api/v1/datasources/test", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Invalid username or password") || strings.Contains(rec.Body.String(), "internal") {
		t.Fatalf("credential test body=%s", rec.Body.String())
	}

	form = strings.NewReader("name=Bad&db_type=arcgis&service_url=notaurl")
	req = httptest.NewRequest(http.MethodPost, "/api/v1/datasources/test", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "invalid service URL") && !strings.Contains(rec.Body.String(), "FeatureServer") {
		t.Fatalf("invalid url body=%s", rec.Body.String())
	}
}

func TestArcGISPublishRequiresMatchingConnection(t *testing.T) {
	s := &Server{}
	jrxml := []byte(`<jasperReport>
  <property name="acugis.datasource.provider" value="arcgis"/>
  <property name="acugis.datasource.connection" value="WeatherFS"/>
  <query language="collection"><![CDATA[SELECT "a" FROM "layers"."t"]]></query>
</jasperReport>`)
	msg, note := s.arcGISPublishDatasourceError(httptest.NewRequest(http.MethodPost, "/", nil), jrxml, "Other")
	if !strings.Contains(msg, "WeatherFS") || note != "" {
		t.Fatalf("missing connection msg=%q note=%q", msg, note)
	}
}
