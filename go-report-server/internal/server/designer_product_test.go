package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

type trackingACL struct {
	mu      sync.Mutex
	keys    []string
	groups  [][]string
	deleted []string
	fail    bool
}

func (w *trackingACL) SyncReportAccessGroups(_ context.Context, key, _ string, groupNames []string, _ int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fail {
		return errors.New("acl down")
	}
	w.keys = append(w.keys, key)
	w.groups = append(w.groups, append([]string(nil), groupNames...))
	return nil
}

func (trackingACL) SyncExplorerViewAccessGroups(context.Context, string, string, []string, int64) error {
	return nil
}
func (trackingACL) ListAccessGroups(context.Context) ([]string, error) { return nil, nil }
func (trackingACL) ListResourcePermissionGroups(context.Context, string, string) ([]string, error) {
	return nil, nil
}
func (w *trackingACL) DeleteResource(_ context.Context, resourceType, resourceKey string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deleted = append(w.deleted, resourceType+":"+resourceKey)
	return nil
}

type productDesigner struct {
	mu            sync.Mutex
	jrxml         string
	fail          int
	hits          int
	discards      int
	lastBase      string
	lastOp        string
	lastKey       string
	lastHeaderKey string
	lastSession   string
	lastBody      string
}

func (p *productDesigner) handler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(headerDesignerToken) != token {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/internal/designer/open":
			key := r.Header.Get(headerDesignerReportKey)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"0123456789abcdef0123456789abcdef","reportKey":"` + key + `","reused":false}`))
		case "/internal/designer/materialize":
			raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			var body struct {
				Op           string `json:"op"`
				Key          string `json:"key"`
				Name         string `json:"name"`
				ResourceBase string `json:"resourceBase"`
			}
			_ = json.Unmarshal(raw, &body)
			p.mu.Lock()
			p.hits++
			p.lastBase = body.ResourceBase
			p.lastOp = body.Op
			p.lastKey = body.Key
			p.lastHeaderKey = r.Header.Get(headerDesignerReportKey)
			p.lastSession = r.Header.Get(headerDesignerSession)
			p.lastBody = string(raw)
			fail := p.fail
			p.mu.Unlock()
			if fail != 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(fail)
				_, _ = w.Write([]byte(`{"message":"Could not create the report"}`))
				return
			}
			p.writeIdentity(body.Key, body.Name, body.ResourceBase)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"name": body.Name, "datasourceName": "Production PostGIS"})
		case "/internal/designer/import":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, "bad", http.StatusBadRequest)
				return
			}
			phase := r.FormValue("phase")
			if phase == "inspect" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"Quarterly Sales"}`))
				return
			}
			p.mu.Lock()
			p.hits++
			p.lastBase = r.FormValue("resourceBase")
			p.lastOp = "import"
			p.lastKey = r.FormValue("key")
			fail := p.fail
			p.mu.Unlock()
			if fail != 0 {
				w.WriteHeader(fail)
				_, _ = w.Write([]byte(`{"message":"Could not create the report"}`))
				return
			}
			name := r.FormValue("name")
			if name == "" {
				name = "Quarterly Sales"
			}
			p.writeIdentity(r.FormValue("key"), name, r.FormValue("resourceBase"))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"name": name})
		case "/internal/designer/discard":
			var body struct {
				Key string `json:"key"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			p.mu.Lock()
			p.discards++
			p.mu.Unlock()
			for _, suffix := range []string{".project.json", ".jrxml", ".jasper", ".resource-base"} {
				_ = os.Remove(filepath.Join(p.jrxml, body.Key+suffix))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	})
}

func (p *productDesigner) writeIdentity(key, name, base string) {
	_ = os.WriteFile(filepath.Join(p.jrxml, key+".project.json"), []byte(`{"name":"`+name+`"}`), 0o644)
	_ = os.WriteFile(filepath.Join(p.jrxml, key+".jrxml"), []byte("<jasperReport/>"), 0o644)
	_ = os.WriteFile(filepath.Join(p.jrxml, key+".jasper"), []byte("jasper"), 0o644)
	_ = os.WriteFile(filepath.Join(p.jrxml, key+".resource-base"), []byte(base+"\n"), 0o644)
}

func productFixture(t *testing.T, aclSvc acl.Service) (*Server, *reports.Store, *productDesigner, *trackingACL) {
	t.Helper()
	upDesigner := &productDesigner{}
	up := httptest.NewServer(upDesigner.handler(designerTestToken))
	t.Cleanup(up.Close)
	repo := &designerAuthRepo{byHash: map[string]*auth.User{
		hashToken(t, "editor-token"): {ID: 17, Username: "ada", Enabled: true},
		hashToken(t, "viewer-token"): {ID: 18, Username: "view", Enabled: true},
	}}
	s, store := designerFixture(t, aclSvc, repo, up.URL, designerTestToken)
	upDesigner.jrxml = store.JRXMLDir()
	writer := &trackingACL{}
	s.deps.ACLWriter = writer
	s.testDesignerFolder = func(id int64) error {
		if id == 0 || id == 9 {
			return nil
		}
		return errDesignerFolder
	}
	s.testPlaceReport = func(folderID int64, key, name string, createdBy int64) error {
		writer.mu.Lock()
		writer.keys = append(writer.keys, key)
		writer.mu.Unlock()
		if folderID != 0 && folderID != 9 {
			return errDesignerFolder
		}
		if createdBy != 17 {
			return errors.New("creator")
		}
		return nil
	}
	return s, store, upDesigner, writer
}

func editorPost(t *testing.T, s *Server, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	withSession(req, token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestDesignerCreateHomeAndCollision(t *testing.T) {
	editor := &designerACL{}
	editor.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	s, store, up, aclw := productFixture(t, editor)

	rec := editorPost(t, s, "/api/designer/reports", "editor-token", `{"name":"Finance Sales","folderId":0}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created designerCreated
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Key != "finance-sales" || created.Redirect != "/reports/finance-sales/design" {
		t.Fatalf("created: %+v", created)
	}
	if strings.Contains(rec.Body.String(), designerTestToken) || strings.Contains(rec.Body.String(), store.JRXMLDir()) {
		t.Fatalf("response leaked: %s", rec.Body.String())
	}
	up.mu.Lock()
	base := up.lastBase
	up.mu.Unlock()
	if base != "finance-sales" {
		t.Fatalf("home resource base: %q", base)
	}
	def, err := store.Get("finance-sales")
	if err != nil {
		t.Fatal(err)
	}
	if def.Raw["datasource"] != "Production PostGIS" || def.Raw["description"] != "" {
		t.Fatalf("catalog: %#v", def.Raw)
	}
	if _, ok := def.Raw["schedule"]; ok {
		t.Fatal("new report copied a schedule")
	}
	aclw.mu.Lock()
	if len(aclw.groups) != 1 || len(aclw.groups[0]) != 0 {
		t.Fatalf("acl groups: %#v", aclw.groups)
	}
	aclw.mu.Unlock()
	if _, err := os.Stat(filepath.Join(store.JRXMLDir(), "finance-sales.project.json")); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(filepath.Join(store.JRXMLDir(), "sales-report.project.json"))
	if err != nil {
		t.Fatal(err)
	}
	up.mu.Lock()
	hits := up.hits
	up.mu.Unlock()
	rec = editorPost(t, s, "/api/designer/reports", "editor-token", `{"name":"Sales Report"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("collision: %d %s", rec.Code, rec.Body.String())
	}
	up.mu.Lock()
	if up.hits != hits {
		t.Fatal("collision contacted designer")
	}
	up.mu.Unlock()
	after, err := os.ReadFile(filepath.Join(store.JRXMLDir(), "sales-report.project.json"))
	if err != nil || string(after) != string(before) {
		t.Fatal("collision overwrote the existing report")
	}

	rec = editorPost(t, s, "/api/designer/reports", "editor-token", `{"name":"Folder Sales","folderId":9}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("folder create: %d %s", rec.Code, rec.Body.String())
	}
	up.mu.Lock()
	base = up.lastBase
	up.mu.Unlock()
	if base != "folder-9/folder-sales" {
		t.Fatalf("folder resource base: %q", base)
	}
}

func TestDesignerCreateDeniedAndRollback(t *testing.T) {
	viewer := &designerACL{}
	viewer.allow(false, map[string]bool{}, map[string]bool{"sales-report": true})
	s, store, up, _ := productFixture(t, viewer)
	rec := editorPost(t, s, "/api/designer/reports", "viewer-token", `{"name":"Secret Report"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer create: %d %s", rec.Code, rec.Body.String())
	}
	up.mu.Lock()
	if up.hits != 0 {
		t.Fatal("viewer reached designer")
	}
	up.mu.Unlock()
	req := httptest.NewRequest(http.MethodGet, "/reports/designer/new", nil)
	withSession(req, "viewer-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer form: %d", rec.Code)
	}

	editor := &designerACL{}
	editor.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	s, store, up, aclw := productFixture(t, editor)
	up.fail = http.StatusInternalServerError
	rec = editorPost(t, s, "/api/designer/reports", "editor-token", `{"name":"Broken Compile"}`)
	if rec.Code < 400 {
		t.Fatalf("compile failure: %d %s", rec.Code, rec.Body.String())
	}
	if store.Exists("broken-compile") {
		t.Fatal("catalog row survived compile failure")
	}
	up.mu.Lock()
	discards := up.discards
	up.mu.Unlock()
	if discards != 0 {
		t.Fatal("server discarded after designer reported its own failure")
	}

	up.fail = 0
	aclw.fail = true
	rec = editorPost(t, s, "/api/designer/reports", "editor-token", `{"name":"ACL Fail"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("acl failure: %d %s", rec.Code, rec.Body.String())
	}
	if store.Exists("acl-fail") {
		t.Fatal("catalog row survived acl failure")
	}
	if _, err := os.Stat(filepath.Join(store.JRXMLDir(), "acl-fail.project.json")); !os.IsNotExist(err) {
		t.Fatal("project survived acl failure")
	}
	aclw.mu.Lock()
	if len(aclw.deleted) != 1 || aclw.deleted[0] != acl.ResourceTypeJasperReport+":acl-fail" {
		t.Fatalf("acl delete: %#v", aclw.deleted)
	}
	aclw.mu.Unlock()

	aclw.fail = false
	s.testPlaceReport = func(int64, string, string, int64) error { return errors.New("place failed") }
	rec = editorPost(t, s, "/api/designer/reports", "editor-token", `{"name":"Place Fail"}`)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "place failed") {
		t.Fatalf("place failure: %d %s", rec.Code, rec.Body.String())
	}
	if store.Exists("place-fail") {
		t.Fatal("catalog row survived place failure")
	}
	if _, err := os.Stat(filepath.Join(store.JRXMLDir(), "place-fail.jasper")); !os.IsNotExist(err) {
		t.Fatal("jasper survived place failure")
	}
}

func TestDesignerSaveAsSwitchesIdentity(t *testing.T) {
	editor := &designerACL{}
	editor.allow(true, map[string]bool{"sales-report": true}, map[string]bool{"sales-report": true})
	s, store, up, _ := productFixture(t, editor)
	if err := store.SaveJSON("sales-report", map[string]any{
		"name": "Sales", "description": "Kept", "schedule": "daily", "datasource": "warehouse",
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(store.JSONDir(), "sales-report.report.json"))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/save-as", strings.NewReader(`{"name":"Revised Copy","folderId":9}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("save as: %d %s", rec.Code, rec.Body.String())
	}
	var created designerCreated
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Key != "revised-copy" || created.Redirect != "/reports/revised-copy/design" {
		t.Fatalf("created: %+v", created)
	}
	up.mu.Lock()
	if up.lastOp != "save-as" || up.lastKey != "revised-copy" || up.lastBase != "folder-9/revised-copy" {
		t.Fatalf("materialize: op=%s key=%s base=%s", up.lastOp, up.lastKey, up.lastBase)
	}
	up.mu.Unlock()
	after, err := os.ReadFile(filepath.Join(store.JSONDir(), "sales-report.report.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("save as changed the source catalog")
	}
	def, err := store.Get("revised-copy")
	if err != nil {
		t.Fatal(err)
	}
	if def.Raw["description"] != "" || def.Raw["schedule"] != nil || def.Raw["datasource"] != "Production PostGIS" {
		t.Fatalf("copy catalog: %#v", def.Raw)
	}

	viewer := &designerACL{}
	viewer.allow(false, map[string]bool{}, map[string]bool{"sales-report": true})
	view, _, up2, _ := productFixture(t, viewer)
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/save-as", strings.NewReader(`{"name":"Nope"}`))
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "viewer-token")
	rec = httptest.NewRecorder()
	view.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer save as: %d %s", rec.Code, rec.Body.String())
	}
	up2.mu.Lock()
	if up2.hits != 0 {
		t.Fatal("viewer save as reached designer")
	}
	up2.mu.Unlock()
}

func TestDesignerSaveAsAllocationAndRollback(t *testing.T) {
	editor := &designerACL{}
	editor.allow(true, map[string]bool{"sales-report": true, "other-report": true}, map[string]bool{"sales-report": true, "other-report": true})
	s, store, up, aclw := productFixture(t, editor)
	if err := store.SaveJSON("sales-report", map[string]any{"name": "Sales", "description": "Kept"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveJSON("other-report", map[string]any{"name": "Other"}); err != nil {
		t.Fatal(err)
	}
	sourceBefore, err := os.ReadFile(filepath.Join(store.JSONDir(), "sales-report.report.json"))
	if err != nil {
		t.Fatal(err)
	}
	otherBefore, err := os.ReadFile(filepath.Join(store.JSONDir(), "other-report.report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var placed []int64
	prevPlace := s.testPlaceReport
	s.testPlaceReport = func(folderID int64, key, name string, createdBy int64) error {
		placed = append(placed, folderID)
		return prevPlace(folderID, key, name, createdBy)
	}
	s.testDesignerFolder = func(id int64) error {
		switch id {
		case 0, 9:
			return nil
		case 4:
			return errPermissionDenied
		default:
			return errDesignerFolder
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/save-as", strings.NewReader(`{"name":"Fresh Copy","folderId":0,"key":"sales-report","resourceBase":"../escape"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("home save as: %d %s", rec.Code, rec.Body.String())
	}
	var created designerCreated
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Key != "fresh-copy" || created.Redirect != "/reports/fresh-copy/design" {
		t.Fatalf("created: %+v", created)
	}
	up.mu.Lock()
	if up.lastOp != "save-as" || up.lastKey != "fresh-copy" || up.lastBase != "fresh-copy" || up.lastHeaderKey != "sales-report" {
		t.Fatalf("materialize: op=%s key=%s base=%s header=%s", up.lastOp, up.lastKey, up.lastBase, up.lastHeaderKey)
	}
	if strings.Contains(up.lastBody, "../escape") || strings.Contains(up.lastBody, `"key":"sales-report"`) {
		t.Fatalf("designer accepted a browser destination: %s", up.lastBody)
	}
	up.mu.Unlock()
	if len(placed) != 1 || placed[0] != 0 {
		t.Fatalf("home folder: %#v", placed)
	}
	aclw.mu.Lock()
	if len(aclw.groups) != 1 || len(aclw.groups[0]) != 0 || aclw.keys[0] != "fresh-copy" {
		t.Fatalf("acl: keys=%#v groups=%#v", aclw.keys, aclw.groups)
	}
	aclw.mu.Unlock()
	again, err := os.ReadFile(filepath.Join(store.JSONDir(), "sales-report.report.json"))
	if err != nil || !bytes.Equal(sourceBefore, again) {
		t.Fatal("save as changed the source catalog")
	}
	otherAfter, err := os.ReadFile(filepath.Join(store.JSONDir(), "other-report.report.json"))
	if err != nil || !bytes.Equal(otherBefore, otherAfter) {
		t.Fatal("save as overwrote another report")
	}

	up.mu.Lock()
	hits := up.hits
	up.mu.Unlock()
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/save-as", strings.NewReader(`{"name":"Sales Report","folderId":0,"key":"other-report"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("collision: %d %s", rec.Code, rec.Body.String())
	}
	up.mu.Lock()
	if up.hits != hits {
		t.Fatal("collision called designer")
	}
	up.mu.Unlock()
	if !bytes.Equal(sourceBefore, mustReadProduct(t, filepath.Join(store.JSONDir(), "sales-report.report.json"))) {
		t.Fatal("collision changed the source")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/save-as", strings.NewReader(`{"name":"No Session","folderId":0}`))
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing session: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/save-as", strings.NewReader(`{"name":"Secret Folder","folderId":4}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthorized folder: %d %s", rec.Code, rec.Body.String())
	}
	if store.Exists("secret-folder") {
		t.Fatal("unauthorized folder created a catalog report")
	}

	up.fail = http.StatusInternalServerError
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/save-as", strings.NewReader(`{"name":"Broken Copy","folderId":9}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code < 400 {
		t.Fatalf("materialize failure: %d %s", rec.Code, rec.Body.String())
	}
	if store.Exists("broken-copy") {
		t.Fatal("catalog row survived materialize failure")
	}
	up.mu.Lock()
	if up.discards != 0 {
		t.Fatal("server discarded after designer reported its own failure")
	}
	up.mu.Unlock()
	if !bytes.Equal(sourceBefore, mustReadProduct(t, filepath.Join(store.JSONDir(), "sales-report.report.json"))) {
		t.Fatal("materialize failure changed the source")
	}

	up.fail = 0
	aclw.fail = true
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/save-as", strings.NewReader(`{"name":"ACL Copy","folderId":9}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("acl failure: %d %s", rec.Code, rec.Body.String())
	}
	if store.Exists("acl-copy") {
		t.Fatal("catalog row survived acl failure")
	}
	if _, err := os.Stat(filepath.Join(store.JRXMLDir(), "acl-copy.project.json")); !os.IsNotExist(err) {
		t.Fatal("project survived acl failure")
	}
	aclw.mu.Lock()
	deleted := append([]string(nil), aclw.deleted...)
	aclw.mu.Unlock()
	if len(deleted) != 1 || deleted[0] != acl.ResourceTypeJasperReport+":acl-copy" {
		t.Fatalf("acl delete: %#v", deleted)
	}

	aclw.fail = false
	s.testPlaceReport = func(int64, string, string, int64) error { return errors.New("place failed") }
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/save-as", strings.NewReader(`{"name":"Place Copy","folderId":9}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "place failed") {
		t.Fatalf("place failure: %d %s", rec.Code, rec.Body.String())
	}
	if store.Exists("place-copy") {
		t.Fatal("catalog row survived place failure")
	}
	if _, err := os.Stat(filepath.Join(store.JRXMLDir(), "place-copy.jasper")); !os.IsNotExist(err) {
		t.Fatal("jasper survived place failure")
	}
	if !bytes.Equal(sourceBefore, mustReadProduct(t, filepath.Join(store.JSONDir(), "sales-report.report.json"))) {
		t.Fatal("rollback changed the source catalog")
	}
}

func mustReadProduct(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDesignerWizardCreatesNewReport(t *testing.T) {
	const wizardSecret = "WIZARD_PASSWORD_DO_NOT_LEAK"
	editor := &designerACL{}
	editor.allow(true, map[string]bool{"sales-report": true, "other-report": true}, map[string]bool{"sales-report": true, "other-report": true})
	s, store, up, aclw := productFixture(t, editor)
	if err := store.SaveJSON("other-report", map[string]any{"name": "Other"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(store.JSONDir(), "sales-report.report.json"))
	if err != nil {
		t.Fatal(err)
	}
	otherBefore, err := os.ReadFile(filepath.Join(store.JSONDir(), "other-report.report.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := `{
		"key":"other-report",
		"name":"Hijacked",
		"folderId":9,
		"resourceBase":"../escape",
		"spec":{
			"name":"City Sales",
			"password":"` + wizardSecret + `",
			"selectedFields":["name"],
			"layout":"tabular",
			"query":{"password":"` + wizardSecret + `","manualSql":"SELECT name FROM cities"}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/wizard", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("wizard: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), wizardSecret) || strings.Contains(rec.Body.String(), store.JRXMLDir()) {
		t.Fatalf("wizard response leaked: %s", rec.Body.String())
	}
	var created designerCreated
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Key != "city-sales" || created.Name != "City Sales" || created.Redirect != "/reports/city-sales/design" {
		t.Fatalf("created: %+v", created)
	}
	up.mu.Lock()
	if up.hits != 1 || up.lastOp != "wizard" || up.lastKey != "city-sales" || up.lastBase != "folder-9/city-sales" || up.lastHeaderKey != "sales-report" || up.lastSession != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("materialize: op=%s key=%s base=%s header=%s session=%s hits=%d", up.lastOp, up.lastKey, up.lastBase, up.lastHeaderKey, up.lastSession, up.hits)
	}
	if strings.Contains(up.lastBody, wizardSecret) || strings.Contains(up.lastBody, "other-report") || strings.Contains(up.lastBody, "../escape") {
		t.Fatalf("designer payload: %s", up.lastBody)
	}
	up.mu.Unlock()
	after, err := os.ReadFile(filepath.Join(store.JSONDir(), "sales-report.report.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("wizard changed the source catalog")
	}
	otherAfter, err := os.ReadFile(filepath.Join(store.JSONDir(), "other-report.report.json"))
	if err != nil || !bytes.Equal(otherBefore, otherAfter) {
		t.Fatal("wizard overwrote the report key supplied by the browser")
	}
	def, err := store.Get("city-sales")
	if err != nil || def.Raw["name"] != "City Sales" {
		t.Fatalf("catalog: %v %#v", err, def)
	}
	if _, err := store.Get("sales-report"); err != nil {
		t.Fatal("source left the catalog")
	}
	aclw.mu.Lock()
	found := false
	for _, key := range aclw.keys {
		if key == "city-sales" {
			found = true
		}
	}
	aclw.mu.Unlock()
	if !found {
		t.Fatal("wizard report was not registered in the report ACL")
	}
	base, err := os.ReadFile(filepath.Join(store.JRXMLDir(), "city-sales.resource-base"))
	if err != nil || strings.TrimSpace(string(base)) != "folder-9/city-sales" {
		t.Fatalf("resource base: %q %v", base, err)
	}

	editor.allow(true, map[string]bool{"sales-report": true, "city-sales": true}, map[string]bool{"sales-report": true, "city-sales": true})
	openReq := httptest.NewRequest(http.MethodGet, "/reports/city-sales/design", nil)
	openReq.Header.Set("Accept", "application/json")
	withSession(openReq, "editor-token")
	openRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(openRec, openReq)
	if openRec.Code != http.StatusOK || !strings.Contains(openRec.Body.String(), `"reportKey":"city-sales"`) || !strings.Contains(openRec.Body.String(), `"designer":true`) {
		t.Fatalf("reopen: %d %s", openRec.Code, openRec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/wizard", strings.NewReader(`{"folderId":9,"spec":{"name":"City Sales","selectedFields":["name"]}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("collision: %d %s", rec.Code, rec.Body.String())
	}
	up.mu.Lock()
	if up.hits != 1 {
		t.Fatal("collision called designer")
	}
	up.mu.Unlock()

	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/wizard", strings.NewReader(`{"folderId":9,"key":"sales-report","spec":{"name":"Sales Report","selectedFields":["name"]}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("source name: %d %s", rec.Code, rec.Body.String())
	}
	again, err := os.ReadFile(filepath.Join(store.JSONDir(), "sales-report.report.json"))
	if err != nil || !bytes.Equal(before, again) {
		t.Fatal("source name collision changed the open report")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/wizard", strings.NewReader(`{"folderId":9,"spec":{"name":"No Session"}}`))
	req.Header.Set("Content-Type", "application/json")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing session: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/wizard", strings.NewReader(`{"folderId":4,"spec":{"name":"Bad Folder"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("folder: %d %s", rec.Code, rec.Body.String())
	}

	viewer := &designerACL{}
	viewer.allow(false, map[string]bool{}, map[string]bool{"sales-report": true})
	view, _, up2, _ := productFixture(t, viewer)
	req = httptest.NewRequest(http.MethodPost, "/api/designer/sales-report/wizard", strings.NewReader(`{"spec":{"name":"Nope"}}`))
	req.Header.Set(headerDesignerSession, "0123456789abcdef0123456789abcdef")
	withSession(req, "viewer-token")
	rec = httptest.NewRecorder()
	view.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer wizard: %d %s", rec.Code, rec.Body.String())
	}
	up2.mu.Lock()
	if up2.hits != 0 {
		t.Fatal("viewer wizard reached designer")
	}
	up2.mu.Unlock()
}

func TestDesignerImportCreatesIsolatedIdentity(t *testing.T) {
	editor := &designerACL{}
	editor.allow(true, map[string]bool{}, map[string]bool{})
	s, store, up, _ := productFixture(t, editor)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("name", "Imported Sales")
	_ = w.WriteField("folderId", "0")
	part, err := w.CreateFormFile("file", "project.zip")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("PK\x03\x04fake"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/designer/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	var created designerCreated
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Key != "imported-sales" || created.Redirect != "/reports/imported-sales/design" {
		t.Fatalf("created: %+v", created)
	}
	up.mu.Lock()
	if up.lastOp != "import" || up.lastBase != "imported-sales" {
		t.Fatalf("import materialize: %+v", up)
	}
	up.mu.Unlock()
	if _, err := os.Stat(filepath.Join(store.JRXMLDir(), "imported-sales.jasper")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), store.JRXMLDir()) {
		t.Fatal("import response leaked a path")
	}

	viewer := &designerACL{}
	viewer.allow(false, nil, map[string]bool{})
	view, _, _, _ := productFixture(t, viewer)
	req = httptest.NewRequest(http.MethodPost, "/api/designer/import", strings.NewReader("nope"))
	withSession(req, "viewer-token")
	rec = httptest.NewRecorder()
	view.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer import: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDesignerSPAAssetsAndVisibility(t *testing.T) {
	editor := &designerACL{}
	editor.allow(true, map[string]bool{"sales-report": true, "legacy": true}, map[string]bool{"sales-report": true, "legacy": true})
	s, store, _, _ := productFixture(t, editor)
	assets := t.TempDir()
	if err := os.MkdirAll(filepath.Join(assets, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	js := []byte("console.log('designer')")
	if err := os.WriteFile(filepath.Join(assets, "index.html"), []byte(`<!DOCTYPE html><html><head><title>Old</title></head><body><div id="root"></div><script src="/designer-assets/assets/app.js"></script></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "assets", "app.js"), js, 0o644); err != nil {
		t.Fatal(err)
	}
	s.cfg.Designer.AssetsDir = assets

	req := httptest.NewRequest(http.MethodGet, "/reports/sales-report/design", nil)
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("spa: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "reportServerMode") || !strings.Contains(body, `"reportKey":"sales-report"`) || !strings.Contains(body, "/designer-assets/assets/app.js") {
		t.Fatalf("bootstrap: %s", body)
	}
	assertDesignerReturn(t, body)

	evil := httptest.NewRequest(http.MethodGet, "/reports/sales-report/design?return=https://evil.example/phish&returnUrl=https://qc9.webgis1.com", nil)
	withSession(evil, "editor-token")
	evilRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(evilRec, evil)
	if evilRec.Code != http.StatusOK || strings.Contains(evilRec.Body.String(), "evil.example") || strings.Contains(evilRec.Body.String(), "qc9.webgis1.com") {
		t.Fatalf("query return override: %d %s", evilRec.Code, evilRec.Body.String())
	}
	assertDesignerReturn(t, evilRec.Body.String())
	if strings.Contains(body, designerTestToken) || strings.Contains(body, "127.0.0.1") || strings.Contains(body, store.JRXMLDir()) {
		t.Fatalf("bootstrap leaked: %s", body)
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("html cache: %s", rec.Header().Get("Cache-Control"))
	}

	req = httptest.NewRequest(http.MethodGet, "/designer-assets/assets/app.js", nil)
	withSession(req, "editor-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("asset: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != string(js) {
		t.Fatalf("asset body: %s", rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/designer-assets/index.html", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("index asset: %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/designer-assets/../secret", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("traversal asset: %d", rec.Code)
	}

	user := &auth.User{ID: 17, Username: "ada", Enabled: true}
	areq := httptest.NewRequest(http.MethodGet, "/reports/sales-report", nil)
	areq = areq.WithContext(auth.WithUser(areq.Context(), user))
	native, can := s.annotateDesignerReport(areq, user, "sales-report")
	if !native || !can {
		t.Fatalf("editor visibility: native=%v can=%v", native, can)
	}
	native, can = s.annotateDesignerReport(areq, user, "legacy")
	if native || !can {
		t.Fatalf("published jrxml visibility: native=%v can=%v", native, can)
	}
	if err := os.WriteFile(filepath.Join(store.JRXMLDir(), "compiled-only.jasper"), []byte("jasper"), 0o644); err != nil {
		t.Fatal(err)
	}
	native, can = s.annotateDesignerReport(areq, user, "compiled-only")
	if native || can {
		t.Fatalf("compiled-only visibility: native=%v can=%v", native, can)
	}
	viewer := &auth.User{ID: 18, Username: "view", Enabled: true}
	viewACL := &designerACL{}
	viewACL.allow(false, map[string]bool{}, map[string]bool{"sales-report": true})
	s.deps.ACL = viewACL
	vreq := httptest.NewRequest(http.MethodGet, "/reports/sales-report", nil)
	vreq = vreq.WithContext(auth.WithUser(vreq.Context(), viewer))
	native, can = s.annotateDesignerReport(vreq, viewer, "sales-report")
	if !native || can {
		t.Fatalf("view-only visibility: native=%v can=%v", native, can)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/designer/new", nil)
	withSession(req, "editor-token")
	s.deps.ACL = editor
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body = rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Create Designer Report") || !strings.Contains(body, "Folder") || !strings.Contains(body, `name="name"`) || !strings.Contains(body, `name="description"`) || !strings.Contains(body, `name="folder_id"`) || !strings.Contains(body, "Create Report") {
		t.Fatalf("create form: %d %s", rec.Code, body)
	}
	if !strings.Contains(body, `class="fed-shell-sidebar"`) || !strings.Contains(body, `class="main-content"`) || !strings.Contains(body, `action="/reports/designer/new"`) {
		t.Fatal("create form is outside the report server shell")
	}
}

func TestDesignerImportPageUsesShell(t *testing.T) {
	editor := &designerACL{}
	editor.allow(true, map[string]bool{}, map[string]bool{})
	s, _, _, _ := productFixture(t, editor)
	req := httptest.NewRequest(http.MethodGet, "/reports/designer/import", nil)
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Import Designer Project") || !strings.Contains(body, "Desktop project ZIP") || !strings.Contains(body, `name="file"`) || !strings.Contains(body, `name="folder_id"`) || !strings.Contains(body, `enctype="multipart/form-data"`) {
		t.Fatalf("import form: %d %s", rec.Code, body)
	}
	if strings.Contains(body, `name="description"`) {
		t.Fatal("import form gained a description field")
	}
	if !strings.Contains(body, `class="fed-shell-sidebar"`) || !strings.Contains(body, `class="main-content"`) || !strings.Contains(body, `action="/reports/designer/import"`) || !strings.Contains(body, ">Import<") {
		t.Fatal("import form is outside the report server shell")
	}
}

func TestDesignerCreatePageRedirects(t *testing.T) {
	editor := &designerACL{}
	editor.allow(true, map[string]bool{}, map[string]bool{})
	s, _, _, _ := productFixture(t, editor)
	req := httptest.NewRequest(http.MethodPost, "/reports/designer/new", strings.NewReader("name=Page+Sales&folder_id=0"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/reports/page-sales/design" {
		t.Fatalf("form create: %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func assertDesignerReturn(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, `"returnUrl":"/hubs/reports"`) {
		t.Fatalf("missing reports hub return: %s", body)
	}
	if strings.Contains(body, `history.back`) || strings.Contains(body, "http://") || strings.Contains(body, "https://") {
		t.Fatalf("return url is not the reports hub path: %s", body)
	}
}

func installDesignerSPA(t *testing.T, s *Server) {
	t.Helper()
	assets := t.TempDir()
	if err := os.MkdirAll(filepath.Join(assets, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	html := `<!DOCTYPE html><html><head><title>Designer</title></head><body><div id="root"></div><script src="/designer-assets/assets/app.js"></script></body></html>`
	if err := os.WriteFile(filepath.Join(assets, "index.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	s.cfg.Designer.AssetsDir = assets
}

func openDesigner(t *testing.T, s *Server, path string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	withSession(req, "editor-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("open %s: %d %s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestDesignerReturnURLFollowsReportsLanding(t *testing.T) {
	editor := &designerACL{}
	editor.allow(true, map[string]bool{}, map[string]bool{})
	s, _, _, _ := productFixture(t, editor)
	if s.designerReturnURL() != "/hubs/reports" {
		t.Fatalf("reports landing %q", s.designerReturnURL())
	}
	s.cfg.Profile = config.ProfileQCarta
	if s.designerReturnURL() != "/reports" {
		t.Fatalf("qcarta reports landing %q", s.designerReturnURL())
	}
}

func TestDesignerCreateImportAndEditShareReturn(t *testing.T) {
	editor := &designerACL{}
	editor.allow(true, map[string]bool{
		"sales-report":   true,
		"created-sales":  true,
		"imported-sales": true,
	}, map[string]bool{
		"sales-report":   true,
		"created-sales":  true,
		"imported-sales": true,
	})
	s, _, _, _ := productFixture(t, editor)
	installDesignerSPA(t, s)

	assertDesignerReturn(t, openDesigner(t, s, "/reports/sales-report/design"))

	create := httptest.NewRequest(http.MethodPost, "/reports/designer/new", strings.NewReader("name=Created+Sales&description=From+the+form&folder_id=0"))
	create.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	withSession(create, "editor-token")
	created := httptest.NewRecorder()
	s.Handler().ServeHTTP(created, create)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	assertDesignerReturn(t, openDesigner(t, s, created.Header().Get("Location")))

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("name", "Imported Sales")
	_ = w.WriteField("folder_id", "0")
	part, err := w.CreateFormFile("file", "project.zip")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("PK\x03\x04fake"))
	_ = w.Close()
	imp := httptest.NewRequest(http.MethodPost, "/reports/designer/import", &buf)
	imp.Header.Set("Content-Type", w.FormDataContentType())
	withSession(imp, "editor-token")
	imported := httptest.NewRecorder()
	s.Handler().ServeHTTP(imported, imp)
	if imported.Code != http.StatusSeeOther {
		t.Fatalf("import: %d %s", imported.Code, imported.Body.String())
	}
	assertDesignerReturn(t, openDesigner(t, s, imported.Header().Get("Location")))
}

func TestDesignerHTTPErrorKeepsSafeCompileDetail(t *testing.T) {
	var logs bytes.Buffer
	s := &Server{logger: slog.New(slog.NewTextHandler(&logs, nil))}
	body := []byte(`{"code":"compilation","category":"compilation","message":"Errors were encountered when compiling report expressions class file:","errors":[{"message":"1. cannot find symbol"},{"message":"2. method sum(int) is not applicable"},{"message":"3. incompatible types: java.lang.String cannot be converted to int"},{"message":"/tmp/secret/Foo.java:4: error: cannot find symbol"},{"message":"password=hunter2"}]}`)
	err := s.designerHTTPError(http.StatusUnprocessableEntity, body)
	var he *httpError
	if !errors.As(err, &he) {
		t.Fatal(err)
	}
	for _, want := range []string{"cannot find symbol", "is not applicable", "incompatible types"} {
		if !strings.Contains(he.Message, want) {
			t.Fatalf("missing %q in %s", want, he.Message)
		}
	}
	if strings.Contains(he.Message, "/tmp") || strings.Contains(he.Message, "hunter2") || strings.Contains(he.Message, `\`) {
		t.Fatalf("page message leaked: %s", he.Message)
	}
	logged := logs.String()
	if !strings.Contains(logged, "cannot find symbol") || strings.Contains(logged, "/tmp") || strings.Contains(logged, "hunter2") {
		t.Fatalf("log: %s", logged)
	}
}
