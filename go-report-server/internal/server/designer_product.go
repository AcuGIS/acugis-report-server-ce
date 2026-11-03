package server

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/folders"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// Designer report creation is one logical transaction across two processes.
//
//	jrs-server authenticates and authorizes service edit
//	jrs-server validates the folder id and allocates the report key
//	jrs-designer writes an isolated resource base, project.json, JRXML, and Jasper
//	jrs-server writes report.json, the default ACL (Public when no groups are
//	chosen, matching publish), and content_items membership
//	on any failure after the designer write, jrs-server discards those files,
//	deletes report.json, removes the folder row, and deletes the ACL resource
//
// Resource base for a new Designer report is "{folder}/{key}", or "{key}" when
// the report is placed in Home. It is never empty, so images/ cannot see the
// resource-paths root or another report's files. Folder membership is still
// the selected content folder. syncReportResourceBase is not called, because
// that helper points every report in a folder at the same shared directory.
// Later runs keep that per-report base. A catalog move relocates the directory
// to the new "{folder}/{key}" (or "{key}" in Home) instead of collapsing it.
//
// Save As copies the in-memory model onto a new key. It does not copy
// schedules or the source ACL. Import accepts only a Desktop project ZIP.
//
// Wizard Finish uses the same identity transaction as a blank report. The
// URL key is the report the designer session is bound to. The new key is
// allocated from the wizard report name. A key or folder path in the body
// is ignored.

var (
	errDesignerConflict = &httpError{Status: http.StatusConflict, Message: "A report with that name already exists"}
	errDesignerName     = &httpError{Status: http.StatusBadRequest, Message: "Report name must contain a letter or number."}
	errDesignerFolder   = &httpError{Status: http.StatusNotFound, Message: "Folder not found."}
)

type designerCreated struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Redirect string `json:"redirect"`
}

func (s *Server) handleDesignerCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireDesignerUser(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.allowDesignerRequest(w, r, true) {
		return
	}
	if err := s.requireServiceEdit(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		FolderID    int64  `json:"folderId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		s.writeAuthzJSON(w, errReportInvalid)
		return
	}
	created, err := s.createDesignerBlank(r, user, body.Name, body.Description, body.FolderID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleDesignerSaveAs(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireDesignerUser(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.allowDesignerRequest(w, r, true) {
		return
	}
	source := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	if !jasper.IsValidReportKey(source) {
		s.writeAuthzJSON(w, errReportInvalid)
		return
	}
	if err := s.authorizeDesignerEdit(r, user, source); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	var body struct {
		Name     string `json:"name"`
		FolderID int64  `json:"folderId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		s.writeAuthzJSON(w, errReportInvalid)
		return
	}
	sessionID := strings.TrimSpace(r.Header.Get(headerDesignerSession))
	if !designerSessionID.MatchString(sessionID) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Session identifier mismatch"})
		return
	}
	created, err := s.createDesignerSaveAs(r, user, source, sessionID, body.Name, body.FolderID)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleDesignerWizard(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireDesignerUser(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.allowDesignerRequest(w, r, true) {
		return
	}
	source := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	if !jasper.IsValidReportKey(source) {
		s.writeAuthzJSON(w, errReportInvalid)
		return
	}
	if err := s.authorizeDesignerEdit(r, user, source); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	var body struct {
		FolderID int64           `json:"folderId"`
		Spec     json.RawMessage `json:"spec"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		s.writeAuthzJSON(w, errReportInvalid)
		return
	}
	sessionID := strings.TrimSpace(r.Header.Get(headerDesignerSession))
	if !designerSessionID.MatchString(sessionID) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Session identifier mismatch"})
		return
	}
	created, err := s.createDesignerWizard(r, user, source, sessionID, body.FolderID, body.Spec)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleDesignerImport(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireDesignerUser(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.allowDesignerRequest(w, r, true) {
		return
	}
	if err := s.requireServiceEdit(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	payload, filename, name, folderID, err := s.readDesignerImport(r)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	created, err := s.createDesignerImport(r, user, name, folderID, filename, payload)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, created)
}

func (s *Server) createDesignerBlank(r *http.Request, user *auth.User, name, description string, folderID int64) (designerCreated, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return designerCreated{}, errDesignerName
	}
	key, base, err := s.prepareDesignerIdentity(r, user, name, folderID)
	if err != nil {
		return designerCreated{}, err
	}
	var mat designerMaterial
	if err := s.designerCall(r.Context(), user, http.MethodPost, "/internal/designer/materialize", key, "", "application/json", bytes.NewReader(designerJSON(map[string]string{
		"op":           "create",
		"key":          key,
		"name":         name,
		"resourceBase": base,
	})), &mat); err != nil {
		return designerCreated{}, err
	}
	return s.finishDesignerIdentity(r, user, key, name, description, base, folderID, mat)
}

func (s *Server) createDesignerSaveAs(r *http.Request, user *auth.User, source, sessionID, name string, folderID int64) (designerCreated, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return designerCreated{}, errDesignerName
	}
	key, base, err := s.prepareDesignerIdentity(r, user, name, folderID)
	if err != nil {
		return designerCreated{}, err
	}
	if key == source {
		return designerCreated{}, errDesignerConflict
	}
	var mat designerMaterial
	if err := s.designerCall(r.Context(), user, http.MethodPost, "/internal/designer/materialize", source, sessionID, "application/json", bytes.NewReader(designerJSON(map[string]string{
		"op":           "save-as",
		"key":          key,
		"name":         name,
		"resourceBase": base,
		"sourceKey":    source,
		"sessionId":    sessionID,
	})), &mat); err != nil {
		return designerCreated{}, err
	}
	// Description, schedules, and ACL stay fresh. Datasource name comes from
	// Report Server resolution of the portable connection, not from a copied report.json.
	return s.finishDesignerIdentity(r, user, key, name, "", base, folderID, mat)
}

func (s *Server) createDesignerWizard(r *http.Request, user *auth.User, source, sessionID string, folderID int64, spec json.RawMessage) (designerCreated, error) {
	spec, err := scrubWizardSpec(spec)
	if err != nil {
		return designerCreated{}, err
	}
	var named struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(spec, &named); err != nil {
		return designerCreated{}, errReportInvalid
	}
	name := strings.TrimSpace(named.Name)
	if name == "" {
		return designerCreated{}, errDesignerName
	}
	key, base, err := s.prepareDesignerIdentity(r, user, name, folderID)
	if err != nil {
		return designerCreated{}, err
	}
	if key == source {
		return designerCreated{}, errDesignerConflict
	}
	var mat designerMaterial
	if err := s.designerCall(r.Context(), user, http.MethodPost, "/internal/designer/materialize", source, sessionID, "application/json", bytes.NewReader(designerJSON(map[string]any{
		"op":           "wizard",
		"key":          key,
		"name":         name,
		"resourceBase": base,
		"sourceKey":    source,
		"sessionId":    sessionID,
		"spec":         spec,
	})), &mat); err != nil {
		return designerCreated{}, err
	}
	return s.finishDesignerIdentity(r, user, key, name, "", base, folderID, mat)
}

func scrubWizardSpec(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, errReportInvalid
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, errReportInvalid
	}
	dropWizardSecrets(v)
	out, err := json.Marshal(v)
	if err != nil {
		return nil, errReportInvalid
	}
	return out, nil
}

func dropWizardSecrets(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			switch strings.ToLower(k) {
			case "password", "passwd", "secret", "privatekey", "clientsecret", "accesstoken":
				delete(t, k)
			default:
				dropWizardSecrets(child)
			}
		}
	case []any:
		for _, child := range t {
			dropWizardSecrets(child)
		}
	}
}

func (s *Server) createDesignerImport(r *http.Request, user *auth.User, name string, folderID int64, filename string, payload []byte) (designerCreated, error) {
	inspected := struct {
		Name string `json:"name"`
	}{}
	if err := s.designerImportCall(r.Context(), user, "inspect", "", "", "", filename, payload, &inspected); err != nil {
		return designerCreated{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSpace(inspected.Name)
	}
	if name == "" {
		return designerCreated{}, errDesignerName
	}
	key, base, err := s.prepareDesignerIdentity(r, user, name, folderID)
	if err != nil {
		return designerCreated{}, err
	}
	var mat designerMaterial
	if err := s.designerImportCall(r.Context(), user, "commit", key, name, base, filename, payload, &mat); err != nil {
		return designerCreated{}, err
	}
	return s.finishDesignerIdentity(r, user, key, name, "", base, folderID, mat)
}

type designerMaterial struct {
	Name                  string `json:"name"`
	DatasourceName        string `json:"datasourceName"`
	DatasourceUnavailable bool   `json:"datasourceUnavailable"`
	Query                 string `json:"query"`
}

func (s *Server) prepareDesignerIdentity(r *http.Request, user *auth.User, name string, folderID int64) (key, base string, err error) {
	key = jasper.NormalizeReportKey(name)
	if !jasper.IsValidReportKey(key) {
		return "", "", errDesignerName
	}
	if s.designerKeyTaken(key) {
		return "", "", errDesignerConflict
	}
	slash, err := s.designerFolderSlash(r, user, folderID)
	if err != nil {
		return "", "", err
	}
	base, err = designerResourceBase(slash, key)
	if err != nil || base == "" || base == "." {
		return "", "", errReportInvalid
	}
	return key, base, nil
}

func (s *Server) finishDesignerIdentity(r *http.Request, user *auth.User, key, name, description, base string, folderID int64, mat designerMaterial) (designerCreated, error) {
	display := strings.TrimSpace(mat.Name)
	if display == "" {
		display = name
	}
	if err := s.writeDesignerCatalog(r, user, key, display, description, base, folderID, mat); err != nil {
		s.rollbackDesignerIdentity(r.Context(), key, base)
		return designerCreated{}, err
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryContent,
		Action:     audit.ActionCreate,
		ObjectType: audit.ObjectReport,
		ObjectID:   key,
		ObjectName: display,
		Status:     audit.StatusSuccess,
	})
	return designerCreated{Key: key, Name: display, Redirect: s.goURL("/reports/" + key + "/design")}, nil
}

func (s *Server) writeDesignerCatalog(r *http.Request, user *auth.User, key, name, description, base string, folderID int64, mat designerMaterial) error {
	if s.deps.Reports == nil {
		return errStoreUnavailable
	}
	query := strings.TrimSpace(mat.Query)
	var params []map[string]any
	var jrxmlInfo reports.CollectionQueryInfo
	if jrxml, err := os.ReadFile(filepath.Join(s.cfg.Paths.JRXMLDir, key+".jrxml")); err == nil {
		jrxmlInfo = reports.ExtractCollectionQueryInfo(jrxml)
		if query == "" {
			query = jrxmlInfo.Query
		}
		params = reports.CatalogParametersFromJRXML(jrxml)
	}
	datasource := ""
	if !mat.DatasourceUnavailable {
		datasource = strings.TrimSpace(mat.DatasourceName)
	}
	// ArcGIS reports resolve by the JRXML connection name. When that name
	// already exists in jasper_arcgis, bind the catalog to it. Credentials
	// are never copied from Designer.
	if conn := strings.TrimSpace(jrxmlInfo.Connection); jrxmlInfo.IsArcGISCollection() && conn != "" && s.arcgisDatasourceExists(r.Context(), conn) {
		datasource = conn
	}
	if conn := strings.TrimSpace(jrxmlInfo.Connection); jrxmlInfo.IsGeoServerCollection() && conn != "" && s.geoServerDatasourceExists(r.Context(), conn) {
		datasource = conn
	}
	raw := buildPublishDefinition(key, name, strings.TrimSpace(description), datasource, "jasper", "", params, query)
	if err := s.deps.Reports.SaveJSON(key, raw); err != nil {
		return errStoreUnavailable
	}
	if note := s.syncReportACL(r, user, key, name, nil); note != "" {
		return errStoreUnavailable
	}
	if err := s.placeDesignerReport(r.Context(), user, folderID, key, name); err != nil {
		return err
	}
	return nil
}

func (s *Server) rollbackDesignerIdentity(ctx context.Context, key, base string) {
	s.designerDiscard(ctx, key, base)
	if s.deps.Reports != nil {
		_ = s.deps.Reports.Delete(key)
	}
	if s.deps.Folders != nil {
		_ = s.deps.Folders.RemoveReport(ctx, key)
	}
	if s.deps.ACLWriter != nil {
		_ = s.deps.ACLWriter.DeleteResource(ctx, acl.ResourceTypeJasperReport, key)
	}
}

func (s *Server) placeDesignerReport(ctx context.Context, user *auth.User, folderID int64, key, name string) error {
	createdBy := int64(0)
	if user != nil {
		createdBy = user.ID
	}
	if s.testPlaceReport != nil {
		return s.testPlaceReport(folderID, key, name, createdBy)
	}
	if s.deps.Folders == nil {
		return errStoreUnavailable
	}
	return s.deps.Folders.EnsureReportInFolder(ctx, folderID, key, name, createdBy)
}

func designerResourceBase(slashFolder, key string) (string, error) {
	folder, err := reports.NormalizeResourceBase(slashFolder)
	if err != nil {
		return "", err
	}
	if !jasper.IsValidReportKey(key) {
		return "", reports.ErrInvalidKey
	}
	if folder == "" {
		return key, nil
	}
	return reports.NormalizeResourceBase(folder + "/" + key)
}

func (s *Server) designerFolderSlash(r *http.Request, user *auth.User, folderID int64) (string, error) {
	if folderID < 0 {
		return "", errDesignerFolder
	}
	if s.testDesignerFolder != nil {
		if err := s.testDesignerFolder(folderID); err != nil {
			return "", err
		}
		if folderID <= 0 {
			return "", nil
		}
		return "/folder-" + strconv.FormatInt(folderID, 10), nil
	}
	if folderID == 0 {
		return "", nil
	}
	if s.deps.Folders == nil {
		return "", errStoreUnavailable
	}
	if _, err := s.deps.Folders.Get(r.Context(), folderID); err != nil {
		if errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrItemNotFound) {
			return "", errDesignerFolder
		}
		return "", errStoreUnavailable
	}
	ok, err := s.canEditFolder(r, user, folderID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errPermissionDenied
	}
	return s.deps.Folders.SlashFolderPath(r.Context(), folderID)
}

func (s *Server) designerKeyTaken(key string) bool {
	if s.deps.Reports != nil && s.deps.Reports.Exists(key) {
		return true
	}
	dir := strings.TrimSpace(s.cfg.Paths.JRXMLDir)
	if dir == "" {
		return true
	}
	for _, suffix := range []string{".project.json", ".jrxml", ".jasper", ".resource-base"} {
		st, err := os.Stat(filepath.Join(dir, key+suffix))
		if err == nil && st.Mode().IsRegular() && st.Size() > 0 {
			return true
		}
	}
	return false
}

func (s *Server) designerCall(ctx context.Context, user *auth.User, method, path, reportKey, sessionID, contentType string, body io.Reader, dest any) error {
	callCtx, cancel := context.WithTimeout(ctx, s.designerTimeout())
	defer cancel()
	resp, err := s.designerRequest(callCtx, method, path, body, contentType, "application/json", user, reportKey, sessionID, "")
	if err != nil {
		return errDesignerUnavailable
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s.designerHTTPError(resp.StatusCode, buf)
	}
	if dest != nil && len(bytes.TrimSpace(buf)) > 0 {
		if err := json.Unmarshal(buf, dest); err != nil {
			return errDesignerUnavailable
		}
	}
	return nil
}

func (s *Server) designerImportCall(ctx context.Context, user *auth.User, phase, key, name, base, filename string, payload []byte, dest any) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("phase", phase)
	_ = w.WriteField("key", key)
	_ = w.WriteField("name", name)
	_ = w.WriteField("resourceBase", base)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="project.zip"`)
	hdr.Set("Content-Type", "application/zip")
	part, err := w.CreatePart(hdr)
	if err != nil {
		return errDesignerUnavailable
	}
	if _, err := part.Write(payload); err != nil {
		return errDesignerUnavailable
	}
	if err := w.Close(); err != nil {
		return errDesignerUnavailable
	}
	return s.designerCall(ctx, user, http.MethodPost, "/internal/designer/import", "", "", w.FormDataContentType(), &buf, dest)
}

func (s *Server) designerDiscard(ctx context.Context, key, base string) {
	_ = s.designerCall(ctx, &auth.User{ID: 1, Username: "rollback", Enabled: true}, http.MethodPost, "/internal/designer/discard", key, "", "application/json", bytes.NewReader(designerJSON(map[string]string{
		"key":          key,
		"resourceBase": base,
	})), nil)
}

func (s *Server) designerHTTPError(status int, body []byte) error {
	if status == http.StatusConflict {
		return errDesignerConflict
	}
	if status == http.StatusRequestEntityTooLarge {
		return &httpError{Status: status, Message: "Upload exceeds the size limit"}
	}
	if status >= 500 {
		return errDesignerUnavailable
	}
	var parsed struct {
		Code     string `json:"code"`
		Category string `json:"category"`
		Message  string `json:"message"`
		Error    string `json:"error"`
		Errors   []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(body, &parsed)
	msg := strings.TrimSpace(parsed.Message)
	if msg == "" {
		msg = strings.TrimSpace(parsed.Error)
	}
	for _, diag := range parsed.Errors {
		line := strings.TrimSpace(diag.Message)
		if line == "" || strings.Contains(msg, line) {
			continue
		}
		if msg != "" {
			msg += "\n"
		}
		msg += line
	}
	msg = s.scrubPublic(msg)
	compile := parsed.Code == "compilation" || parsed.Category == "compilation" || compilerDiagnostic(msg)
	if compile {
		msg = redactCompilerSlashes(msg)
		if msg == "" {
			msg = "Could not create the report"
		} else {
			if s != nil && s.logger != nil {
				s.logger.Info("designer import compile error", "message", msg)
			} else {
				log.Printf("designer import compile error: %s", msg)
			}
		}
	} else if msg == "" || strings.Contains(msg, `\`) || strings.Contains(msg, "/") {
		msg = "Could not create the report"
	}
	if status < 400 {
		status = http.StatusBadRequest
	}
	return &httpError{Status: status, Message: msg}
}

func compilerDiagnostic(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "compiling report expressions") ||
		strings.Contains(lower, "cannot find symbol") ||
		strings.Contains(lower, "is not applicable") ||
		strings.Contains(lower, "incompatible types")
}

var designerSlashToken = regexp.MustCompile(`[^\s"'<>|]*[/\\][^\s"'<>|]*`)

func redactCompilerSlashes(msg string) string {
	msg = designerSecretAssign.ReplaceAllString(msg, "$1=[redacted]")
	if strings.ContainsAny(msg, `/\`) {
		msg = designerSlashToken.ReplaceAllString(msg, "[path]")
	}
	return strings.TrimSpace(msg)
}

func (s *Server) readDesignerImport(r *http.Request) (payload []byte, filename, name string, folderID int64, err error) {
	limit := s.designerMaxBody()
	r.Body = http.MaxBytesReader(wLimit{r}, r.Body, limit+1<<20)
	if err = r.ParseMultipartForm(32 << 20); err != nil {
		return nil, "", "", 0, &httpError{Status: http.StatusRequestEntityTooLarge, Message: "Upload exceeds the size limit"}
	}
	file, header, ferr := r.FormFile("file")
	if ferr != nil {
		return nil, "", "", 0, &httpError{Status: http.StatusBadRequest, Message: "Choose a project ZIP to import"}
	}
	defer file.Close()
	filename = "project.zip"
	if header != nil && header.Filename != "" {
		filename = filepath.Base(header.Filename)
		if !strings.HasSuffix(strings.ToLower(filename), ".zip") {
			return nil, "", "", 0, &httpError{Status: http.StatusBadRequest, Message: "Import a .zip of the Desktop report project"}
		}
	}
	payload, err = io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(payload)) > limit {
		return nil, "", "", 0, &httpError{Status: http.StatusRequestEntityTooLarge, Message: "Upload exceeds the size limit"}
	}
	name = strings.TrimSpace(r.FormValue("name"))
	folderID, _ = strconv.ParseInt(strings.TrimSpace(r.FormValue("folder_id")), 10, 64)
	if r.FormValue("folderId") != "" {
		folderID, _ = strconv.ParseInt(strings.TrimSpace(r.FormValue("folderId")), 10, 64)
	}
	return payload, filename, name, folderID, nil
}

// wLimit satisfies MaxBytesReader's ResponseWriter requirement without writing.
type wLimit struct{ r *http.Request }

func (wLimit) Header() http.Header       { return http.Header{} }
func (wLimit) Write([]byte) (int, error) { return 0, nil }
func (wLimit) WriteHeader(int)           {}

func designerJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (s *Server) designerFolderChoices(r *http.Request, user *auth.User) []designerFolder {
	out := []designerFolder{{ID: 0, Name: "Home"}}
	if s.deps.Folders == nil || !s.deps.Folders.Configured() {
		return out
	}
	tree, err := s.deps.Folders.Tree(r.Context())
	if err != nil || tree == nil {
		return out
	}
	var walk func(n folders.TreeNode, prefix string)
	walk = func(n folders.TreeNode, prefix string) {
		if n.ParentID == nil {
			for _, child := range n.Children {
				walk(child, "")
			}
			return
		}
		name := n.Name
		if prefix != "" {
			name = prefix + " / " + n.Name
		}
		if n.ID > 0 {
			if ok, err := s.canEditFolder(r, user, n.ID); err == nil && ok {
				out = append(out, designerFolder{ID: n.ID, Name: name})
			}
		}
		if len(out) > 200 {
			return
		}
		for _, child := range n.Children {
			walk(child, name)
		}
	}
	walk(*tree, "")
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}

func (s *Server) handleDesignerCreatePage(w http.ResponseWriter, r *http.Request) {
	s.handleDesignerFormPage(w, r, false)
}

func (s *Server) handleDesignerImportPage(w http.ResponseWriter, r *http.Request) {
	s.handleDesignerFormPage(w, r, true)
}

func (s *Server) handleDesignerFormPage(w http.ResponseWriter, r *http.Request, importing bool) {
	user := auth.FromContext(r.Context())
	if err := s.requireDesignerUser(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	if !s.allowDesignerRequest(w, r, false) {
		return
	}
	if err := s.requireServiceEdit(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	title := "Create Designer Report"
	if importing {
		title = "Import Designer Project"
	}
	var message string
	if r.Method == http.MethodPost {
		var created designerCreated
		var err error
		if importing {
			payload, filename, name, folderID, rerr := s.readDesignerImport(r)
			if rerr != nil {
				err = rerr
			} else {
				created, err = s.createDesignerImport(r, user, name, folderID, filename, payload)
			}
		} else {
			if err = r.ParseForm(); err == nil {
				folderID, _ := strconv.ParseInt(r.FormValue("folder_id"), 10, 64)
				created, err = s.createDesignerBlank(r, user, r.FormValue("name"), r.FormValue("description"), folderID)
			}
		}
		if err == nil {
			http.Redirect(w, r, created.Redirect, http.StatusSeeOther)
			return
		}
		var he *httpError
		if errors.As(err, &he) {
			message = he.Message
		} else {
			message = "Could not create the report"
		}
	}
	s.renderDesignerForm(w, title, message, importing, s.designerFolderChoices(r, user))
}

//go:embed designer_form.html
var designerFormHTML string

var designerFormTemplate = template.Must(template.New("designer-form").Parse(designerFormHTML))

func (s *Server) renderDesignerForm(w http.ResponseWriter, title, message string, importing bool, folders []designerFolder) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	data := designerFormData{
		materialChromeData: s.materialChrome(title),
		SidebarNav:         s.renderSidebarNavHTML("reports"),
		Message:            message,
		Importing:          importing,
		Folders:            folders,
		Action:             s.goURL("/reports/designer/new"),
		Cancel:             s.reportsCatalogURL(),
		Subtitle:           "Name the report and choose a folder. Designer opens after the report is created.",
		SubmitLabel:        "Create Report",
	}
	if importing {
		data.Action = s.goURL("/reports/designer/import")
		data.Subtitle = "Upload a desktop Designer project and choose a folder."
		data.SubmitLabel = "Import"
	}
	if err := designerFormTemplate.Execute(w, data); err != nil {
		s.logger.Error("render designer form", "error", err)
	}
}

type designerFormData struct {
	materialChromeData
	SidebarNav  template.HTML
	Message     string
	Importing   bool
	Folders     []designerFolder
	Action      string
	Cancel      string
	Subtitle    string
	SubmitLabel string
}

func (s *Server) annotateDesignerReport(r *http.Request, user *auth.User, key string) (native, canDesign bool) {
	native, err := s.designerNative(key)
	if err != nil {
		return false, false
	}
	if !native {
		ok, jerr := s.designerEditableJRXML(key)
		if jerr != nil || !ok {
			return false, false
		}
	}
	if err := s.requireServiceEdit(r, user); err != nil {
		return native, false
	}
	edit, err := s.canEditReport(r, user, key)
	if err != nil {
		return native, false
	}
	if !edit && !s.isPlatformAdmin(user) {
		return native, false
	}
	if !s.designerConfigured() {
		return native, false
	}
	return native, true
}
