package server

import (
	"bytes"
	"context"
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
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

type scopedACL struct {
	view, edit, admin, serviceView, serviceEdit bool
}

func (a scopedACL) CanViewReport(context.Context, *auth.User, string) (bool, error) {
	return a.view, nil
}
func (a scopedACL) CanEditReport(context.Context, *auth.User, string) (bool, error) {
	return a.edit, nil
}
func (a scopedACL) CanAdminReport(context.Context, *auth.User, string) (bool, error) {
	return a.admin, nil
}
func (a scopedACL) CanViewExplorerView(context.Context, *auth.User, string) (bool, error) {
	return a.view, nil
}
func (a scopedACL) CanEditExplorerView(context.Context, *auth.User, string) (bool, error) {
	return a.edit, nil
}
func (a scopedACL) CanAccessReportImage(context.Context, *auth.User, string) (bool, error) {
	return a.view, nil
}
func (a scopedACL) CanAccessReportService(context.Context, *auth.User) (bool, error) {
	return a.serviceView, nil
}
func (a scopedACL) CheckPermission(_ context.Context, _ *auth.User, resourceType, _ string, permission string) (acl.Decision, error) {
	allowed := false
	switch permission {
	case acl.PermissionView:
		if resourceType == acl.ResourceTypeService {
			allowed = a.serviceView
		} else {
			allowed = a.view
		}
	case acl.PermissionEdit:
		if resourceType == acl.ResourceTypeService {
			allowed = a.serviceEdit
		} else {
			allowed = a.edit
		}
	case acl.PermissionAdmin:
		allowed = a.admin
	}
	return acl.Decision{Allowed: allowed}, nil
}
func (scopedACL) Configured() bool   { return true }
func (scopedACL) AuthDisabled() bool { return false }

type recordingACLWriter struct {
	deleted []string
}

func (recordingACLWriter) SyncReportAccessGroups(context.Context, string, string, []string, int64) error {
	return nil
}
func (recordingACLWriter) SyncExplorerViewAccessGroups(context.Context, string, string, []string, int64) error {
	return nil
}
func (recordingACLWriter) ListAccessGroups(context.Context) ([]string, error) {
	return []string{"Public", "Editors"}, nil
}
func (recordingACLWriter) ListResourcePermissionGroups(context.Context, string, string) ([]string, error) {
	return nil, nil
}
func (r *recordingACLWriter) DeleteResource(_ context.Context, resourceType, resourceKey string) error {
	r.deleted = append(r.deleted, resourceType+":"+resourceKey)
	return nil
}

func qcartaManageServer(t *testing.T, jsonDir, jrxmlDir string, aclSvc acl.Service, writer acl.ResourceWriter) *Server {
	t.Helper()
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)
	cfg := testConfig()
	cfg.Profile = config.ProfileQCarta
	cfg.AuthDisabled = aclSvc == nil
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.ReportJSONDir = jsonDir
	cfg.Paths.JRXMLDir = jrxmlDir
	cfg.Paths.ThumbnailDir = t.TempDir()
	if aclSvc == nil {
		aclSvc = acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true, ServiceKey: acl.ServiceKeyReportRuntime})
	}
	if writer == nil {
		writer = noopACLWriter{}
	}
	authDisabled := cfg.AuthDisabled
	return New(cfg, Dependencies{
		Reports:     reports.NewStore(jsonDir, cfg.Paths.ThumbnailDir, jrxmlDir),
		ACL:         aclSvc,
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: authDisabled}),
		Datasources: stubNamerFinder{names: []string{"MainDS"}},
		ACLWriter:   writer,
	}, slog.Default())
}

func writeDemoReport(t *testing.T, jsonDir, jrxmlDir, key, name string) {
	t.Helper()
	mustWrite(t, filepath.Join(jsonDir, key+".report.json"), `{
		"name": "`+name+`",
		"report": "`+key+`",
		"datasource": "MainDS",
		"description": "`+name+`",
		"type": "jasper",
		"format": "html",
		"parameters": [{"name":"parcel_id","type":"string","label":"Parcel","default":"100"}]
	}`)
	if jrxmlDir != "" {
		mustWrite(t, filepath.Join(jrxmlDir, key+".jrxml"), `<?xml version="1.0"?><jasperReport name="`+key+`"><parameter name="parcel_id" class="java.lang.String"/></jasperReport>`)
	}
}

func TestQCartaReportsTableShowsExecuteEditDelete(t *testing.T) {
	jsonDir := t.TempDir()
	jrxmlDir := t.TempDir()
	writeDemoReport(t, jsonDir, jrxmlDir, "parcel_card", "Parcel Card")
	s := qcartaManageServer(t, jsonDir, jrxmlDir, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`class="qcarta-reports-table"`,
		`>Report</th>`,
		`>Description</th>`,
		`>Actions</th>`,
		`Parcel Card`,
		`>Execute</a>`,
		`/reports/edit_report?report=parcel_card`,
		`>Edit</a>`,
		`/reports/delete_report?report=parcel_card`,
		`>Delete</a>`,
		`_repName=parcel_card`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("list missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "<ul>") {
		t.Fatal("QCarta reports list should be a table, not a bullet list")
	}
}

func TestQCartaViewerCannotSeeOrUseEditDelete(t *testing.T) {
	jsonDir := t.TempDir()
	jrxmlDir := t.TempDir()
	writeDemoReport(t, jsonDir, jrxmlDir, "parcel_card", "Parcel Card")
	viewerACL := scopedACL{view: true, serviceView: true}
	s := qcartaManageServer(t, jsonDir, jrxmlDir, viewerACL, nil)
	viewer := &auth.User{ID: 9, Username: "viewer", Enabled: true}

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	req = req.WithContext(auth.WithUser(req.Context(), viewer))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, ">Execute</a>") {
		t.Fatal("viewer should still see Execute")
	}
	if strings.Contains(body, ">Edit</a>") || strings.Contains(body, ">Delete</a>") {
		t.Fatalf("viewer must not see Edit/Delete: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/edit_report?report=parcel_card", nil)
	req = req.WithContext(auth.WithUser(req.Context(), viewer))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("edit GET status=%d want 403 body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/delete_report?report=parcel_card", nil)
	req = req.WithContext(auth.WithUser(req.Context(), viewer))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("delete GET status=%d want 403 body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/reports/delete_report", strings.NewReader("report=parcel_card"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithUser(req.Context(), viewer))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("delete POST status=%d want 403", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(jsonDir, "parcel_card.report.json")); err != nil {
		t.Fatal("viewer delete must not remove the report")
	}
}

func TestQCartaEditorCanEditAndDelete(t *testing.T) {
	jsonDir := t.TempDir()
	jrxmlDir := t.TempDir()
	writeDemoReport(t, jsonDir, jrxmlDir, "parcel_card", "Parcel Card")
	writeDemoReport(t, jsonDir, jrxmlDir, "keep_me", "Keep Me")
	writer := &recordingACLWriter{}
	editorACL := scopedACL{view: true, edit: true, admin: true, serviceView: true, serviceEdit: true}
	s := qcartaManageServer(t, jsonDir, jrxmlDir, editorACL, writer)
	editor := &auth.User{ID: 3, Username: "editor", Enabled: true}

	req := httptest.NewRequest(http.MethodGet, "/reports/edit_report?report=parcel_card", nil)
	req = req.WithContext(auth.WithUser(req.Context(), editor))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit GET status=%d body=%s", rec.Code, rec.Body.String())
	}
	editBody := rec.Body.String()
	if !strings.Contains(editBody, "Edit Report") || !strings.Contains(editBody, "Parcel Card") {
		t.Fatalf("edit page=%s", editBody)
	}
	if strings.Contains(editBody, `id="tab-map-btn"`) {
		t.Fatal("QCarta edit must not show the Map tab")
	}

	editBuf := &bytes.Buffer{}
	ew := multipart.NewWriter(editBuf)
	_ = ew.WriteField("report_name", "Parcel Card Updated")
	_ = ew.WriteField("report_description", "Updated desc")
	_ = ew.WriteField("report_type", "jasper")
	_ = ew.WriteField("data_source", "MainDS")
	_ = ew.WriteField("params[0][name]", "parcel_id")
	_ = ew.WriteField("params[0][type]", "string")
	_ = ew.WriteField("params[0][label]", "Parcel ID")
	_ = ew.Close()
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_report?report=parcel_card", editBuf)
	req.Header.Set("Content-Type", ew.FormDataContentType())
	req = req.WithContext(auth.WithUser(req.Context(), editor))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("edit POST status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/reports" {
		t.Fatalf("edit redirect=%q", loc)
	}
	def, err := s.deps.Reports.Get("parcel_card")
	if err != nil {
		t.Fatal(err)
	}
	if def.DisplayName() != "Parcel Card Updated" {
		t.Fatalf("name=%s", def.DisplayName())
	}
	if def.Datasource() != "MainDS" {
		t.Fatalf("datasource=%s", def.Datasource())
	}
	params := def.Parameters()
	if len(params) == 0 || params[0]["label"] != "Parcel ID" {
		t.Fatalf("params=%#v", params)
	}
	if params[0]["default"] != "100" {
		t.Fatalf("default was dropped: %#v", params[0])
	}

	replaceBuf := &bytes.Buffer{}
	rw := multipart.NewWriter(replaceBuf)
	_ = rw.WriteField("report_name", "Parcel Card Updated")
	_ = rw.WriteField("report_description", "Updated desc")
	_ = rw.WriteField("report_type", "jasper")
	_ = rw.WriteField("data_source", "MainDS")
	_ = rw.WriteField("params[0][name]", "parcel_id")
	_ = rw.WriteField("params[0][type]", "string")
	_ = rw.WriteField("params[0][label]", "Parcel ID")
	part, err := rw.CreateFormFile("jrxml_file", "parcel_card.jrxml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, `<?xml version="1.0"?><jasperReport name="parcel_card"><parameter name="parcel_id" class="java.lang.String"/><parameter name="owner" class="java.lang.String"/></jasperReport>`)
	_ = rw.Close()
	req = httptest.NewRequest(http.MethodPost, "/reports/edit_report?report=parcel_card", replaceBuf)
	req.Header.Set("Content-Type", rw.FormDataContentType())
	req = req.WithContext(auth.WithUser(req.Context(), editor))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("replace POST status=%d body=%s", rec.Code, rec.Body.String())
	}
	def, err = s.deps.Reports.Get("parcel_card")
	if err != nil {
		t.Fatal(err)
	}
	if def.Key != "parcel_card" {
		t.Fatalf("key changed to %s", def.Key)
	}
	jrxml, err := os.ReadFile(filepath.Join(jrxmlDir, "parcel_card.jrxml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jrxml), `name="owner"`) {
		t.Fatalf("JRXML not replaced: %s", jrxml)
	}
	byName := paramMapByName(def.Parameters())
	if byName["owner"] == nil {
		t.Fatalf("owner param not imported: %#v", def.Parameters())
	}
	if byName["parcel_id"]["label"] != "Parcel ID" {
		t.Fatalf("manual label lost: %#v", byName["parcel_id"])
	}

	req = httptest.NewRequest(http.MethodGet, "/reports/delete_report?report=parcel_card", nil)
	req = req.WithContext(auth.WithUser(req.Context(), editor))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete confirm status=%d", rec.Code)
	}
	confirm := rec.Body.String()
	if !strings.Contains(confirm, "Parcel Card Updated") || !strings.Contains(confirm, "Delete") {
		t.Fatalf("confirm heading missing: %s", confirm)
	}
	if !strings.Contains(confirm, "This removes the published report from QCarta Reports.") {
		t.Fatal("confirm must explain QCarta delete")
	}
	if s.deps.Reports.Exists("parcel_card") == false {
		t.Fatal("GET delete must not remove the report")
	}

	req = httptest.NewRequest(http.MethodPost, "/reports/delete_report", strings.NewReader("report=parcel_card"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithUser(req.Context(), editor))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("delete POST status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/reports" {
		t.Fatalf("delete redirect=%q", loc)
	}
	if s.deps.Reports.Exists("parcel_card") {
		t.Fatal("expected parcel_card deleted")
	}
	if !s.deps.Reports.Exists("keep_me") {
		t.Fatal("unrelated report must remain")
	}
	if len(writer.deleted) != 1 || writer.deleted[0] != acl.ResourceTypeJasperReport+":parcel_card" {
		t.Fatalf("ACL delete=%v", writer.deleted)
	}
}

func TestQCartaJasperReplaceKeepsKey(t *testing.T) {
	jsonDir := t.TempDir()
	jrxmlDir := t.TempDir()
	writeDemoReport(t, jsonDir, jrxmlDir, "parcel_card", "Parcel Card")
	s := qcartaManageServer(t, jsonDir, jrxmlDir, nil, nil)

	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	_ = w.WriteField("report_name", "Parcel Card")
	_ = w.WriteField("report_description", "Parcel Card")
	_ = w.WriteField("report_type", "jasper")
	_ = w.WriteField("data_source", "MainDS")
	part, err := w.CreateFormFile("jasper_file", "parcel_card.jasper")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("FAKE-JASPER"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/reports/parcel_card", buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(jrxmlDir, "parcel_card.jasper"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "FAKE-JASPER" {
		t.Fatalf("jasper=%q", data)
	}
	if !s.deps.Reports.Exists("parcel_card") {
		t.Fatal("report key must be preserved")
	}
}

func TestQCartaPublishedJRXMLOffersEditInDesigner(t *testing.T) {
	jsonDir := t.TempDir()
	jrxmlDir := t.TempDir()
	writeDemoReport(t, jsonDir, jrxmlDir, "parcel_card", "Parcel Card")
	mustWrite(t, filepath.Join(jsonDir, "compiled-only.report.json"), `{
		"name": "Compiled Only",
		"report": "compiled-only",
		"datasource": "MainDS",
		"description": "Compiled",
		"type": "jasper",
		"format": "html"
	}`)
	if err := os.WriteFile(filepath.Join(jrxmlDir, "compiled-only.jasper"), []byte("jasper"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := qcartaManageServer(t, jsonDir, jrxmlDir, nil, nil)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("menu render contacted designer: %s", r.URL.Path)
	}))
	defer up.Close()
	s.cfg.Designer.Upstream = up.URL
	s.cfg.Designer.Token = "qcarta-menu-token"
	grantDesignerEntitlement(t, s)

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`/reports/edit_report?report=parcel_card`,
		`>Edit</a>`,
		`/reports/parcel_card/design`,
		`>Edit in Designer</a>`,
		`/reports/edit_report?report=compiled-only`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("menu missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "/reports/compiled-only/design") {
		t.Fatalf("compiled report offered designer: %s", body)
	}
}

func TestStandaloneDeleteGetStillDeletes(t *testing.T) {
	jsonDir := t.TempDir()
	jrxmlDir := t.TempDir()
	mustWrite(t, filepath.Join(jsonDir, "demo.report.json"), `{"name":"Demo","report":"demo","datasource":"MainDS","type":"jasper"}`)
	appRoot := t.TempDir()
	mustWrite(t, filepath.Join(appRoot, "shared", "ui", "shell-bootstrap.js"), `/* shell */`)
	cfg := testConfig()
	cfg.AuthDisabled = true
	cfg.Paths.AppRoot = appRoot
	cfg.Paths.ReportJSONDir = jsonDir
	cfg.Paths.JRXMLDir = jrxmlDir
	store := reports.NewStore(jsonDir, t.TempDir(), jrxmlDir)
	s := New(cfg, Dependencies{
		Reports:     store,
		ACL:         acl.NewService(&mockConfiguredRepo{}, acl.Options{AuthDisabled: true}),
		Auth:        auth.NewService(nil, auth.Options{AuthDisabled: true}),
		Datasources: stubNamerFinder{names: []string{"MainDS"}},
		ACLWriter:   noopACLWriter{},
	}, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/reports/delete_report?report=demo", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if store.Exists("demo") {
		t.Fatal("standalone GET delete must still remove the report")
	}
}

var _ datasources.Namer = stubNamerFinder{}
