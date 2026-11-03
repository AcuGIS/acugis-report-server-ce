package server

import (
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/bigquery"
	"github.com/acugis/acugis-report-server-go/internal/cassandra"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/duckdb"
)

//go:embed datasources.html
var datasourcesHTML string

//go:embed edit_datasource.html
var editDatasourceHTML string

type dsRowView struct {
	ID                      int64
	Name                    string
	DBType                  string
	TypeLabel               string
	AuthLabel               string
	AuthType                string
	Host                    string
	Port                    int
	HostPort                string
	DBName                  string
	Username                string
	SchemaName              string
	PublicKeyResourceUUID   string
	PrivateKeyResourceUUID  string
	CredentialsResourceUUID string
	Location                string
	HasPassword             bool
	HasCredentials          bool
	ConnectionMethod        string
	LocalDatacenter         string
	DuckMode                string
	DuckRemoteSource        string
	DuckFormat              string
	DuckAuthType            string
	DuckHeaderKeys          string
	Subtitle                string
	DocSource               string
	DocResource             string
	DocDelimiter            string
	DocCollection           string
	DocSheet                string
	DocURL                  string
	DocFilename             string
}

type datasourcesPageData struct {
	materialChromeData
	ContextURL string
	EditURL    string
	Errors     []string
	FlashError string
	Message    string
	DBTypes    []datasources.DBTypeMeta
	Rows       []dsRowView
}

type editDatasourcePageData struct {
	materialChromeData
	PageTitle string
	BackURL   string
	Errors    []string
	Message   string
	DBTypes   []datasources.DBTypeMeta
	EditRow   *dsRowView
}

var datasourcesTemplate = template.Must(template.New("datasources").Parse(datasourcesHTML))
var editDatasourceTemplate = template.Must(template.New("edit_datasource").Parse(editDatasourceHTML))

func (s *Server) dsStore() datasources.Store {
	if store, ok := s.deps.Datasources.(datasources.Store); ok {
		return store
	}
	return nil
}

func (s *Server) canAdminReportService(r *http.Request, user *auth.User) (bool, error) {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return true, nil
	}
	if user != nil && user.IsPlatformAdmin() {
		return true, nil
	}
	if s.deps.ACL == nil {
		return false, errStoreUnavailable
	}
	d, err := s.deps.ACL.CheckPermission(r.Context(), user, acl.ResourceTypeService, s.reportServiceKey(), acl.PermissionAdmin)
	if err != nil {
		return false, errStoreUnavailable
	}
	return d.Allowed, nil
}

func (s *Server) requireServiceAdmin(r *http.Request, user *auth.User) error {
	ok, err := s.canAdminReportService(r, user)
	if err != nil {
		return err
	}
	if !ok {
		return errPermissionDenied
	}
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return nil
	}
	if user == nil || !auth.IsAuthenticated(r.Context()) {
		if user != nil && user.Username == "local-dev" && user.IsAdmin {
			return nil
		}
		return errAuthRequired
	}
	return nil
}

func (s *Server) handleDatasourcesPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	data := datasourcesPageData{
		materialChromeData: s.materialChrome("Datasources"),
		ContextURL:         s.goURL("/reports/generate_jasper_context"),
		EditURL:            s.goURL("/reports/edit_datasource"),
		DBTypes:            s.datasourceTypeMetas(),
		FlashError:         strings.TrimSpace(r.URL.Query().Get("error")),
		Message:            strings.TrimSpace(r.URL.Query().Get("message")),
	}

	store := s.dsStore()
	if store == nil {
		data.Errors = append(data.Errors, "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables.")
		s.renderDatasources(w, data)
		return
	}
	if !store.TableAvailable(r.Context()) {
		data.Errors = append(data.Errors, "Missing public.jasper_datasource. Load database/schema.sql into the application database.")
		s.renderDatasources(w, data)
		return
	}
	_ = store.EnsureDBTypeColumn(r.Context())
	_ = store.EnsureAuthColumns(r.Context())
	_ = store.EnsureOptionsColumn(r.Context())

	if r.Method == http.MethodPost {
		s.handleDatasourcesPOST(w, r, store, &data)
		return
	}

	s.loadDatasourceRows(r, store, &data)
	s.renderDatasources(w, data)
}

func (s *Server) handleEditDatasourcePage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	data := editDatasourcePageData{
		materialChromeData: s.materialChrome("Datasources"),
		PageTitle:          "Add Datasource",
		BackURL:            s.goURL("/reports/datasources"),
		DBTypes:            s.datasourceTypeMetas(),
	}

	store := s.dsStore()
	if store == nil {
		data.Errors = append(data.Errors, "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables.")
		s.renderEditDatasource(w, data)
		return
	}
	if !store.TableAvailable(r.Context()) {
		data.Errors = append(data.Errors, "Missing public.jasper_datasource. Load database/schema.sql into the application database.")
		s.renderEditDatasource(w, data)
		return
	}
	_ = store.EnsureDBTypeColumn(r.Context())
	_ = store.EnsureAuthColumns(r.Context())
	_ = store.EnsureOptionsColumn(r.Context())

	if r.Method == http.MethodPost {
		s.handleEditDatasourcePOST(w, r, store, &data)
		return
	}

	editName := strings.TrimSpace(r.URL.Query().Get("name"))
	if editName != "" {
		if row, ok := s.lookupDatasourceRow(r, store, editName, r.URL.Query().Get("db_type")); ok {
			data.EditRow = &row
			data.PageTitle = "Edit Datasource"
		} else {
			data.Errors = append(data.Errors, "Datasource not found.")
		}
	}
	s.renderEditDatasource(w, data)
}

func (s *Server) handleDatasourcesPOST(w http.ResponseWriter, r *http.Request, store datasources.Store, data *datasourcesPageData) {
	wantsJSON := wantsJSONResponse(r)
	if err := parseDatasourceRequest(r); err != nil {
		data.Errors = append(data.Errors, "Failed to parse form.")
		if wantsJSON {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": data.Errors[0]})
			return
		}
		s.renderDatasources(w, *data)
		return
	}

	action := strings.TrimSpace(r.FormValue("action"))
	if action == "delete" {
		id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
		dbType := datasources.NormalizeDBType(r.FormValue("db_type"))
		if id > 0 && dbType == "arcgis" {
			s.deleteArcGISDatasourceForm(r, id, data)
			if wantsJSON {
				if len(data.Errors) > 0 {
					s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": strings.Join(data.Errors, " "), "errors": data.Errors})
					return
				}
				s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": data.Message})
				return
			}
			s.loadDatasourceRows(r, store, data)
			s.renderDatasources(w, *data)
			return
		}
		if id > 0 && dbType == "geoserver" {
			s.deleteGeoServerDatasourceForm(r, id, data)
			if wantsJSON {
				if len(data.Errors) > 0 {
					s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": strings.Join(data.Errors, " "), "errors": data.Errors})
					return
				}
				s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": data.Message})
				return
			}
			s.loadDatasourceRows(r, store, data)
			s.renderDatasources(w, *data)
			return
		}
		if id > 0 {
			docLogical := s.documentLogicalForID(r, store, id)
			if err := store.DeleteByID(r.Context(), id); err != nil {
				data.Errors = append(data.Errors, "Failed to delete datasource.")
				s.recordAudit(r, audit.Event{
					Category:   audit.CategoryDatasource,
					Action:     audit.ActionDelete,
					ObjectType: audit.ObjectDatasource,
					ObjectID:   strconv.FormatInt(id, 10),
					Status:     audit.StatusFailure,
					Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
				})
			} else {
				datasources.RemoveDocument(s.datasourceFilesDir(), docLogical)
				data.Message = "Datasource deleted."
				s.recordAudit(r, audit.Event{
					Category:   audit.CategoryDatasource,
					Action:     audit.ActionDelete,
					ObjectType: audit.ObjectDatasource,
					ObjectID:   strconv.FormatInt(id, 10),
					Status:     audit.StatusSuccess,
				})
			}
		}
		if wantsJSON {
			if len(data.Errors) > 0 {
				s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": strings.Join(data.Errors, " "), "errors": data.Errors})
				return
			}
			s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": data.Message})
			return
		}
		s.loadDatasourceRows(r, store, data)
		s.renderDatasources(w, *data)
		return
	}

	// Save/upsert still accepted on the list URL for API compatibility.
	editData := editDatasourcePageData{
		materialChromeData: data.materialChromeData,
		PageTitle:          "Add Datasource",
		BackURL:            s.goURL("/reports/datasources"),
		DBTypes:            data.DBTypes,
	}
	s.handleEditDatasourcePOST(w, r, store, &editData)
}

func (s *Server) handleEditDatasourcePOST(w http.ResponseWriter, r *http.Request, store datasources.Store, data *editDatasourcePageData) {
	wantsJSON := wantsJSONResponse(r)
	if err := parseDatasourceRequest(r); err != nil {
		data.Errors = append(data.Errors, "Failed to parse form.")
		if wantsJSON {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": data.Errors[0]})
			return
		}
		s.renderEditDatasource(w, *data)
		return
	}

	ds, verrs := datasourceFromForm(r)
	data.Errors = append(data.Errors, verrs...)
	if len(data.Errors) == 0 && ds.IsDocument() {
		if err := s.applyDocumentForm(r, store, &ds); err != nil {
			data.Errors = append(data.Errors, err.Error())
		}
	}
	if len(data.Errors) == 0 && !datasources.AllowedDBType(ds.DBType, s.qcartaDBTypeKeys()) {
		data.Errors = append(data.Errors, "Datasource engine is not available in the QCarta report runtime.")
	}
	if len(data.Errors) == 0 && ds.IsArcGIS() {
		s.saveArcGISDatasourceForm(r, ds, data)
	} else if len(data.Errors) == 0 && ds.IsGeoServer() {
		s.saveGeoServerDatasourceForm(r, ds, data)
	} else if len(data.Errors) == 0 && ds.IsDuckDB() {
		// Preserve / merge secrets on edit so blank form fields do not wipe credentials.
		if existing, err := store.ListEnabled(r.Context()); err == nil {
			for _, row := range existing {
				if !strings.EqualFold(row.Name, ds.Name) {
					continue
				}
				if duckdb.NormalizeAuthType(ds.Option(datasources.OptionAuthType)) == duckdb.AuthHeaders {
					keys := duckdb.HTTPHeaderKeys(datasources.DuckDBConfig(ds))
					if strings.TrimSpace(ds.Password) == "" {
						ds.Password = row.Password
					} else {
						ds.Password = duckdb.MergeHeaderSecretJSON(row.Password, ds.Password, keys)
					}
					if ds.Option(datasources.OptionHTTPHeaderKeys) == "" {
						if prev := row.Option(datasources.OptionHTTPHeaderKeys); prev != "" {
							ds.Options[datasources.OptionHTTPHeaderKeys] = prev
						}
					}
				} else if strings.TrimSpace(ds.Password) == "" {
					ds.Password = "" // Upsert CASE keeps existing
				}
				break
			}
		}
	}
	if len(data.Errors) == 0 && !ds.IsArcGIS() && s.arcgisDatasourceExists(r.Context(), ds.Name) {
		data.Errors = append(data.Errors, fmt.Sprintf("An ArcGIS Feature Server datasource named %q already exists.", ds.Name))
	}
	if len(data.Errors) == 0 && !ds.IsGeoServer() && s.geoServerDatasourceExists(r.Context(), ds.Name) {
		data.Errors = append(data.Errors, fmt.Sprintf("A GeoServer datasource named %q already exists.", ds.Name))
	}
	if len(data.Errors) == 0 && !ds.IsArcGIS() && !ds.IsGeoServer() {
		saveAction := audit.ActionCreate
		objectID := ""
		if existing, ferr := store.Find(r.Context(), ds.Name); ferr == nil && existing != nil && existing.ID > 0 {
			saveAction = audit.ActionUpdate
			objectID = strconv.FormatInt(existing.ID, 10)
		}
		if err := store.Upsert(r.Context(), ds); err != nil {
			if errors.Is(err, datasources.ErrInvalidInput) && ds.IsBigQuery() {
				data.Errors = append(data.Errors, "Invalid BigQuery datasource — Name, Project ID, and a Service Account JSON resource are required.")
			} else if errors.Is(err, datasources.ErrInvalidInput) && ds.IsDuckDB() {
				data.Errors = append(data.Errors, "Invalid DuckDB datasource — Name and Database File (or Remote Source for Data Files) are required.")
			} else {
				data.Errors = append(data.Errors, "Failed to save datasource.")
			}
			s.logger.Warn("save datasource", "error", err)
			s.recordAudit(r, audit.Event{
				Category:   audit.CategoryDatasource,
				Action:     saveAction,
				ObjectType: audit.ObjectDatasource,
				ObjectID:   objectID,
				ObjectName: ds.Name,
				Status:     audit.StatusFailure,
				Details: map[string]any{
					"type":  ds.DBType,
					"error": audit.SanitizeErrorMessage(err.Error()),
				},
			})
		} else {
			if ds.IsCassandra() {
				data.Message = "Datasource saved. Cassandra/Astra does not use context.xml — reports fill via Go collection data (Test Connection, then run the report)."
			} else if ds.IsBigQuery() {
				data.Message = "Datasource saved. Google BigQuery does not use context.xml — reports fill via Go collection data (Test Connection, then run the report)."
			} else if ds.IsDuckDB() {
				data.Message = "Datasource saved. DuckDB does not use context.xml — reports fill via Tomcat DuckDB JDBC (REPORT_CONNECTION). Use Test Connection, then run the report."
			} else if ds.IsDocument() {
				data.Message = "Datasource saved. The document is stored in the datasource catalog for Designer Web."
			} else {
				data.Message = "Datasource saved. Regenerate Jasper context before running reports against it."
			}
			s.recordAudit(r, audit.Event{
				Category:   audit.CategoryDatasource,
				Action:     saveAction,
				ObjectType: audit.ObjectDatasource,
				ObjectID:   objectID,
				ObjectName: ds.Name,
				Status:     audit.StatusSuccess,
				Details:    map[string]any{"type": ds.DBType},
			})
		}
	}

	if wantsJSON {
		if len(data.Errors) > 0 {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": strings.Join(data.Errors, " "), "errors": data.Errors})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": data.Message})
		return
	}

	if len(data.Errors) > 0 {
		if row, ok := s.lookupDatasourceRow(r, store, ds.Name, ds.DBType); ok {
			data.EditRow = &row
			data.PageTitle = "Edit Datasource"
		} else {
			row := dsToRowView(ds)
			data.EditRow = &row
			data.PageTitle = "Edit Datasource"
		}
		s.renderEditDatasource(w, *data)
		return
	}

	listURL := s.goURL("/reports/datasources")
	if data.Message != "" {
		listURL += "?message=" + url.QueryEscape(data.Message)
	}
	http.Redirect(w, r, listURL, http.StatusSeeOther)
}

func (s *Server) loadDatasourceRows(r *http.Request, store datasources.Store, data *datasourcesPageData) {
	if len(data.Errors) > 0 {
		return
	}
	rows, err := store.ListEnabled(r.Context())
	if err != nil {
		data.Errors = append(data.Errors, "Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables.")
		return
	}
	data.Rows = make([]dsRowView, 0, len(rows)+4)
	for _, row := range rows {
		data.Rows = append(data.Rows, dsToRowView(row))
	}
	for _, row := range s.listArcGISDatasources(r.Context()) {
		data.Rows = append(data.Rows, dsToRowView(row))
	}
	for _, row := range s.listGeoServerDatasources(r.Context()) {
		data.Rows = append(data.Rows, dsToRowView(row))
	}
	sort.Slice(data.Rows, func(i, j int) bool {
		return strings.ToLower(data.Rows[i].Name) < strings.ToLower(data.Rows[j].Name)
	})
}

func dsToRowView(row datasources.Datasource) dsRowView {
	hostPort := fmt.Sprintf("%s:%d", row.Host, row.Port)
	if row.IsSQLite() {
		hostPort = "—"
	} else if row.IsAstra() {
		hostPort = row.Host
	} else if row.IsBigQuery() {
		hostPort = "—"
	} else if row.IsDuckDB() || row.IsArcGIS() || row.IsGeoServer() || row.IsDocument() {
		hostPort = "—"
	}
	connMethod := ""
	localDC := ""
	if row.IsCassandra() {
		connMethod = row.CassandraConnectionMethod()
		localDC = row.Option(datasources.OptionLocalDatacenter)
	}
	typeLabel := datasources.MetaForDBType(row.DBType).Label
	authLabel := authTypeLabel(row.AuthType)
	if row.IsCassandra() {
		if row.IsAstra() {
			typeLabel = "Apache Cassandra (Astra)"
			authLabel = "Token"
		} else {
			typeLabel = "Apache Cassandra"
		}
	}
	if row.IsBigQuery() {
		typeLabel = "Google BigQuery"
		authLabel = "Service Account"
	}
	if row.IsArcGIS() {
		typeLabel = "ArcGIS Feature Server"
		if strings.TrimSpace(row.Username) == "" {
			authLabel = "Anonymous"
		} else {
			authLabel = "Password"
		}
	}
	if row.IsGeoServer() {
		typeLabel = "GeoServer"
		if strings.TrimSpace(row.Username) == "" {
			authLabel = "Anonymous"
		} else {
			authLabel = "Password"
		}
	}
	duckMode := ""
	duckRemote := ""
	duckFormat := ""
	duckAuth := ""
	duckHeaders := ""
	subtitle := ""
	if row.IsDuckDB() {
		typeLabel = "DuckDB"
		cfg := datasources.DuckDBConfig(row)
		duckMode = duckdb.ConnectionMode(cfg)
		duckAuth = duckdb.AuthType(cfg)
		switch duckAuth {
		case duckdb.AuthBasic:
			authLabel = "Basic Auth"
		case duckdb.AuthBearer:
			authLabel = "Bearer Token"
		case duckdb.AuthHeaders:
			authLabel = "Custom Headers"
		default:
			authLabel = "None"
		}
		if duckMode == duckdb.ModeFiles {
			entries := duckdb.FileEntries(cfg)
			if len(entries) > 0 {
				duckRemote = duckdb.RedactRemoteURL(entries[0].RemoteSource())
				_, fmtName, err := duckdb.ReaderFunctionForEntry(entries[0])
				if err != nil {
					fmtName = entries[0].ResolvedFormat()
				}
				duckFormat = fmtName
			}
			duckHeaders = row.Option(datasources.OptionHTTPHeaderKeys)
		}
		subtitle = row.DuckDBSubtitle()
		if duckMode == duckdb.ModeFiles && duckRemote != "" {
			hostPort = duckRemote
		}
	}
	docSource := ""
	docResource := ""
	docDelimiter := ""
	docCollection := ""
	docSheet := ""
	docURL := ""
	docFilename := ""
	if row.IsDocument() {
		authLabel = "—"
		docResource = row.Option(datasources.OptionResourceFile)
		if docResource == "" {
			docResource = strings.TrimSpace(row.DBName)
		}
		docDelimiter = row.Option(datasources.OptionDelimiter)
		docCollection = row.Option(datasources.OptionCollection)
		docSheet = row.Option(datasources.OptionSheet)
		docURL = row.Option(datasources.OptionJSONURL)
		docSource = row.Option(datasources.OptionJSONSource)
		if row.IsJSONURL() {
			docSource = datasources.JSONSourceURL
			hostPort = docURL
			subtitle = docURL
		} else {
			if row.IsJSON() && docSource == "" {
				docSource = datasources.JSONSourceFile
			}
			docFilename = documentBaseName(docResource)
			hostPort = docFilename
			subtitle = docFilename
		}
	}
	credUUID := row.CredentialsResourceUUID()
	return dsRowView{
		ID:                      row.ID,
		Name:                    row.Name,
		DBType:                  datasources.NormalizeDBType(row.DBType),
		TypeLabel:               typeLabel,
		AuthType:                datasources.NormalizeAuthType(row.AuthType),
		AuthLabel:               authLabel,
		Host:                    row.Host,
		Port:                    row.Port,
		HostPort:                hostPort,
		DBName:                  row.DBName,
		Username:                row.Username,
		SchemaName:              row.SchemaName,
		PublicKeyResourceUUID:   row.PublicKeyResourceUUID,
		PrivateKeyResourceUUID:  row.PrivateKeyResourceUUID,
		CredentialsResourceUUID: credUUID,
		Location:                row.BigQueryLocation(),
		HasPassword:             strings.TrimSpace(row.Password) != "",
		HasCredentials:          credUUID != "",
		ConnectionMethod:        connMethod,
		LocalDatacenter:         localDC,
		DuckMode:                duckMode,
		DuckRemoteSource:        duckRemote,
		DuckFormat:              duckFormat,
		DuckAuthType:            duckAuth,
		DuckHeaderKeys:          duckHeaders,
		Subtitle:                subtitle,
		DocSource:               docSource,
		DocResource:             docResource,
		DocDelimiter:            docDelimiter,
		DocCollection:           docCollection,
		DocSheet:                docSheet,
		DocURL:                  docURL,
		DocFilename:             docFilename,
	}
}

func documentBaseName(logical string) string {
	logical = strings.ReplaceAll(strings.TrimSpace(logical), `\`, "/")
	if i := strings.LastIndex(logical, "/"); i >= 0 {
		return logical[i+1:]
	}
	return logical
}

// datasourceFromForm builds a Datasource from POST form fields with Cassandra-aware validation.
func datasourceFromForm(r *http.Request) (datasources.Datasource, []string) {
	var errs []string
	meta := datasources.MetaForDBType(r.FormValue("db_type"))
	port, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("port")))
	if port <= 0 {
		port = meta.Port
	}
	ds := datasources.Datasource{
		Name:                   strings.TrimSpace(r.FormValue("name")),
		DBType:                 datasources.NormalizeDBType(r.FormValue("db_type")),
		Host:                   strings.TrimSpace(r.FormValue("host")),
		Port:                   port,
		DBName:                 strings.TrimSpace(r.FormValue("dbname")),
		Username:               strings.TrimSpace(r.FormValue("username")),
		Password:               r.FormValue("password"),
		SchemaName:             strings.TrimSpace(r.FormValue("schema_name")),
		AuthType:               datasources.NormalizeAuthType(r.FormValue("auth_type")),
		PublicKeyResourceUUID:  strings.TrimSpace(r.FormValue("public_key_resource_uuid")),
		PrivateKeyResourceUUID: strings.TrimSpace(r.FormValue("private_key_resource_uuid")),
	}
	if ds.SchemaName == "" && ds.DBType == "postgresql" {
		ds.SchemaName = "public"
	}
	if ds.SchemaName == "" && ds.DBType == "snowflake" {
		ds.SchemaName = "PUBLIC"
	}
	if ds.IsDocument() {
		return documentFieldsFromForm(r, ds)
	}
	if ds.IsArcGIS() {
		ds.AuthType = datasources.AuthPassword
		ds.Port = 0
		ds.DBName = ""
		ds.SchemaName = ""
		ds.PublicKeyResourceUUID = ""
		ds.PrivateKeyResourceUUID = ""
		ds.Options = nil
		serviceURL := strings.TrimSpace(r.FormValue("service_url"))
		if serviceURL == "" {
			serviceURL = ds.Host
		}
		ds.Host = serviceURL
		if ds.Name == "" || serviceURL == "" {
			errs = append(errs, "Name and Service URL are required.")
		} else if strings.Contains(ds.Name, ".") {
			errs = append(errs, "Datasource name must not contain a period.")
		}
		return ds, errs
	}
	if ds.IsGeoServer() {
		ds.AuthType = datasources.AuthPassword
		ds.Port = 0
		ds.DBName = ""
		ds.SchemaName = ""
		ds.PublicKeyResourceUUID = ""
		ds.PrivateKeyResourceUUID = ""
		ds.Options = nil
		baseURL := strings.TrimSpace(r.FormValue("service_url"))
		if baseURL == "" {
			baseURL = ds.Host
		}
		ds.Host = baseURL
		if ds.Name == "" || baseURL == "" {
			errs = append(errs, "Name and GeoServer URL are required.")
		} else if strings.Contains(ds.Name, ".") {
			errs = append(errs, "Datasource name must not contain a period.")
		}
		return ds, errs
	}
	if ds.IsCassandra() {
		ds.AuthType = datasources.AuthPassword
		ds.PublicKeyResourceUUID = ""
		ds.PrivateKeyResourceUUID = ""
		method := strings.ToLower(strings.TrimSpace(r.FormValue("connection_method")))
		if method == "" {
			method = strings.ToLower(strings.TrimSpace(r.FormValue("connectionMethod")))
		}
		if method == datasources.ConnectionMethodAstra || method == "datastax" || method == "datastax_astra" {
			method = datasources.ConnectionMethodAstra
		} else {
			method = datasources.ConnectionMethodStandard
		}
		ds.Options = map[string]string{
			datasources.OptionConnectionMethod: method,
		}
		if method == datasources.ConnectionMethodAstra {
			ds.Port = 0
			ds.Username = ""
			if ds.DBName == "" {
				ds.DBName = "default_keyspace"
			}
		} else {
			localDC := strings.TrimSpace(r.FormValue("local_datacenter"))
			if localDC == "" {
				localDC = strings.TrimSpace(r.FormValue("localDatacenter"))
			}
			if localDC != "" {
				ds.Options[datasources.OptionLocalDatacenter] = localDC
			}
			if ds.Port <= 0 {
				ds.Port = 9042
			}
		}
		if ds.Name == "" || ds.Host == "" || ds.DBName == "" {
			errs = append(errs, "Name, host/API endpoint, and keyspace are required.")
		}
		return ds, errs
	}
	if ds.IsBigQuery() {
		ds.Host = ""
		ds.Port = 0
		ds.Username = ""
		ds.Password = ""
		ds.AuthType = datasources.AuthPassword
		ds.PublicKeyResourceUUID = ""
		ds.PrivateKeyResourceUUID = ""
		credUUID := strings.TrimSpace(r.FormValue("credentials_resource_uuid"))
		if credUUID == "" {
			credUUID = strings.TrimSpace(r.FormValue("credentialsResourceUuid"))
		}
		defaultDS := strings.TrimSpace(r.FormValue("default_dataset"))
		if defaultDS == "" {
			defaultDS = strings.TrimSpace(r.FormValue("schema_name"))
		}
		location := strings.TrimSpace(r.FormValue("location"))
		ds.SchemaName = defaultDS
		ds.Options = map[string]string{
			datasources.OptionProjectID: ds.DBName,
		}
		if defaultDS != "" {
			ds.Options[datasources.OptionDefaultDataset] = defaultDS
		}
		if location != "" {
			ds.Options[datasources.OptionLocation] = location
		}
		if credUUID != "" {
			ds.Options[datasources.OptionCredentialsResourceUUID] = credUUID
		}
		if ds.Name == "" || ds.DBName == "" {
			errs = append(errs, "Name and Project ID are required.")
		}
		return ds, errs
	}
	if ds.IsDuckDB() {
		ds.Host = ""
		ds.Port = 0
		ds.AuthType = datasources.AuthPassword
		ds.PublicKeyResourceUUID = ""
		ds.PrivateKeyResourceUUID = ""
		mode := strings.ToLower(strings.TrimSpace(r.FormValue("duckdb_mode")))
		if mode == "" {
			mode = strings.ToLower(strings.TrimSpace(r.FormValue("mode")))
		}
		if mode == "files" || mode == "file" || mode == "datafiles" || mode == "data_files" {
			mode = datasources.DuckDBModeFiles
		} else {
			mode = datasources.DuckDBModeDatabase
		}
		ds.Options = map[string]string{
			datasources.OptionMode: mode,
		}
		if mode == datasources.DuckDBModeFiles {
			remote := strings.TrimSpace(r.FormValue("duckdb_remote_source"))
			if remote == "" {
				remote = strings.TrimSpace(r.FormValue("remote_source"))
			}
			format := strings.TrimSpace(r.FormValue("duckdb_format"))
			if format == "" {
				format = strings.TrimSpace(r.FormValue("format"))
			}
			if format == "" {
				format = duckdb.FormatAuto
			}
			alias := strings.TrimSpace(r.FormValue("duckdb_alias"))
			if alias == "" {
				alias = strings.TrimSpace(ds.SchemaName)
			}
			authType := strings.ToLower(strings.TrimSpace(r.FormValue("duckdb_auth_type")))
			if authType == "" {
				authType = strings.ToLower(strings.TrimSpace(r.FormValue("authType")))
			}
			authType = duckdb.NormalizeAuthType(authType)
			ds.Options[datasources.OptionAuthType] = authType
			ds.Options[datasources.OptionSourceType] = duckdb.SourceTypeHTTP

			headerKeysRaw := strings.TrimSpace(r.FormValue("duckdb_header_keys"))
			if headerKeysRaw == "" {
				headerKeysRaw = strings.TrimSpace(r.FormValue("httpHeaderKeys"))
			}
			var headerKeys []string
			if headerKeysRaw != "" {
				if strings.HasPrefix(headerKeysRaw, "[") {
					headerKeys = duckdb.HTTPHeaderKeys(duckdb.Config{Options: map[string]string{duckdb.OptionHTTPHeaderKeys: headerKeysRaw}})
				} else {
					for _, part := range strings.Split(headerKeysRaw, ",") {
						part = strings.TrimSpace(part)
						if part != "" {
							headerKeys = append(headerKeys, part)
						}
					}
				}
			}
			if authType == duckdb.AuthHeaders {
				ds.Options[datasources.OptionHTTPHeaderKeys] = duckdb.EncodeHTTPHeaderKeys(headerKeys)
				// Password may be JSON map of header values, or single value for first key.
				secret := strings.TrimSpace(ds.Password)
				if secret != "" && !strings.HasPrefix(secret, "{") && len(headerKeys) == 1 {
					ds.Password = duckdb.EncodeHeaderSecretValues(map[string]string{headerKeys[0]: secret})
				}
			} else {
				ds.Options[datasources.OptionHTTPHeaderKeys] = "[]"
			}
			if authType != duckdb.AuthBasic {
				ds.Username = ""
			}
			if remote == "" {
				errs = append(errs, "Remote source URL is required for DuckDB Data Files mode.")
			} else if err := duckdb.ValidateRemoteSource(remote); err != nil {
				errs = append(errs, err.Error())
			} else {
				if alias == "" {
					alias = ""
				}
				entry := duckdb.FileEntry{Alias: alias, Source: remote, Format: format}
				ds.Options[datasources.OptionFiles] = duckdb.EncodeFileEntries([]duckdb.FileEntry{entry})
				entries := duckdb.FileEntries(duckdb.Config{Options: ds.Options})
				if len(entries) > 0 {
					ds.DBName = entries[0].Alias
					ds.SchemaName = entries[0].Alias
				} else if ds.DBName == "" {
					ds.DBName = "remote"
				}
			}
			if ds.Name == "" {
				errs = append(errs, "Name is required.")
			}
		} else {
			ds.Username = ""
			ds.Password = ""
			ds.SchemaName = ""
			delete(ds.Options, datasources.OptionFiles)
			delete(ds.Options, datasources.OptionAuthType)
			delete(ds.Options, datasources.OptionHTTPHeaderKeys)
			delete(ds.Options, datasources.OptionSourceType)
			if ds.Name == "" || ds.DBName == "" {
				errs = append(errs, "Name and Database File path are required.")
			}
		}
		return ds, errs
	}
	if ds.AuthType == datasources.AuthKeyPair {
		if ds.PrivateKeyResourceUUID == "" && ds.PublicKeyResourceUUID == "" {
			errs = append(errs, "Key Pair authentication requires selecting at least a private or public key resource.")
		}
	}
	if len(errs) == 0 && (ds.Name == "" || ds.DBName == "") {
		errs = append(errs, "Name and database/service (or file path) are required.")
	} else if len(errs) == 0 && ds.DBType != "sqlite" && ds.DBType != "duckdb" && (ds.Host == "" || ds.Username == "") {
		errs = append(errs, "Name, host, database/service, and username are required.")
	} else if len(errs) == 0 && ds.DBType != "sqlite" && ds.DBType != "duckdb" && ds.Port <= 0 {
		errs = append(errs, "Port must be a positive integer.")
	}
	return ds, errs
}

func authTypeLabel(t string) string {
	switch datasources.NormalizeAuthType(t) {
	case datasources.AuthOAuth:
		return "OAuth"
	case datasources.AuthKeyPair:
		return "Key Pair"
	default:
		return "Password"
	}
}

func (s *Server) renderDatasources(w http.ResponseWriter, data datasourcesPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := datasourcesTemplate.Execute(w, data); err != nil {
		s.logger.Error("render datasources", "error", err)
	}
}

func (s *Server) renderEditDatasource(w http.ResponseWriter, data editDatasourcePageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := editDatasourceTemplate.Execute(w, data); err != nil {
		s.logger.Error("render edit datasource", "error", err)
	}
}

func contextUIReturn(r *http.Request) bool {
	if r == nil || wantsJSONResponse(r) || strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	if r.Method != http.MethodPost {
		return false
	}
	_ = r.ParseForm()
	return strings.EqualFold(strings.TrimSpace(r.FormValue("return_to")), "datasources")
}

func (s *Server) redirectDatasourcesFlash(w http.ResponseWriter, r *http.Request, message, errMsg string) {
	dest := s.goURL("/reports/datasources")
	if strings.TrimSpace(errMsg) != "" {
		dest += "?error=" + url.QueryEscape(strings.TrimSpace(errMsg))
	} else if strings.TrimSpace(message) != "" {
		dest += "?message=" + url.QueryEscape(strings.TrimSpace(message))
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (s *Server) handleGenerateJasperContext(w http.ResponseWriter, r *http.Request) {
	uiReturn := contextUIReturn(r)
	user := auth.FromContext(r.Context())
	// PHP generate_jasper_context.php requires service edit (not admin).
	if err := s.requireServiceEdit(r, user); err != nil {
		if uiReturn {
			s.redirectDatasourcesFlash(w, r, "", "Unable to regenerate Jasper context.")
			return
		}
		if wantsJSONResponse(r) {
			s.writeAuthzJSON(w, err)
			return
		}
		http.Error(w, err.Error(), statusFromErr(err))
		return
	}
	store := s.dsStore()
	if store == nil {
		msg := "No datasources found. Add rows to public.jasper_datasource first."
		if uiReturn {
			s.redirectDatasourcesFlash(w, r, "", msg)
			return
		}
		http.Error(w, msg+"\n", http.StatusInternalServerError)
		return
	}
	rows, err := store.ListEnabled(r.Context())
	if err != nil || len(rows) == 0 {
		msg := "No datasources found. Add rows to public.jasper_datasource first."
		if uiReturn {
			s.redirectDatasourcesFlash(w, r, "", msg)
			return
		}
		http.Error(w, msg+"\n", http.StatusInternalServerError)
		return
	}
	rows, skipNotes, err := s.attachPrivateKeysForContext(r, rows)
	if err != nil {
		if uiReturn {
			s.redirectDatasourcesFlash(w, r, "", err.Error())
			return
		}
		http.Error(w, err.Error()+"\n", http.StatusInternalServerError)
		return
	}
	skipNotes = append(skipNotes, datasources.ContextSkipNotes(rows)...)
	written := datasources.CountJDBCContextResources(rows)
	path := strings.TrimSpace(s.cfg.Paths.JasperContextPath)
	if err := datasources.WriteContextXML(path, rows); err != nil {
		if uiReturn {
			s.redirectDatasourcesFlash(w, r, "", err.Error())
			return
		}
		http.Error(w, err.Error()+"\n", http.StatusInternalServerError)
		return
	}
	if uiReturn {
		s.redirectDatasourcesFlash(w, r, "Jasper context regenerated successfully.", "")
		return
	}
	if wantsJSONResponse(r) || strings.HasPrefix(r.URL.Path, "/api/") {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"ok":               true,
			"path":             path,
			"jdbc_resources":   written,
			"datasource_count": len(rows),
			"notes":            skipNotes,
		})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "Context XML generated at %s\n", path)
	_, _ = fmt.Fprintf(w, "Wrote %d JDBC Resource(s) from %d datasource(s).\n", written, len(rows))
	for _, note := range skipNotes {
		_, _ = fmt.Fprintf(w, "Note: %s\n", note)
	}
}

// attachPrivateKeysForContext resolves folder Resource UUIDs into PEM bytes for key_pair datasources.
func (s *Server) attachPrivateKeysForContext(r *http.Request, rows []datasources.Datasource) ([]datasources.Datasource, []string, error) {
	var notes []string
	out := make([]datasources.Datasource, len(rows))
	copy(out, rows)
	for i := range out {
		ds := &out[i]
		if datasources.NormalizeAuthType(ds.AuthType) != datasources.AuthKeyPair {
			continue
		}
		if datasources.NormalizeDBType(ds.DBType) != "snowflake" {
			notes = append(notes, fmt.Sprintf("%s: key_pair auth is currently applied for Snowflake only; skipped.", ds.Name))
			continue
		}
		uuid := strings.TrimSpace(ds.PrivateKeyResourceUUID)
		if uuid == "" {
			notes = append(notes, fmt.Sprintf("%s: key_pair selected but no private key resource UUID; omitted from context.xml.", ds.Name))
			ds.PrivateKeyPEM = nil
			continue
		}
		if s.deps.Resources == nil || !s.deps.Resources.Configured() {
			return nil, nil, fmt.Errorf("datasource %s uses key_pair auth but content resources are unavailable", ds.Name)
		}
		_, data, err := s.deps.Resources.ReadBytes(r.Context(), uuid)
		if err != nil {
			return nil, nil, fmt.Errorf("datasource %s: failed to load private key resource %s: %v", ds.Name, uuid, err)
		}
		if len(data) == 0 {
			return nil, nil, fmt.Errorf("datasource %s: private key resource %s is empty", ds.Name, uuid)
		}
		ds.PrivateKeyPEM = data
	}
	return out, notes, nil
}

func (s *Server) handleAPIDatasourcesList(w http.ResponseWriter, r *http.Request) {
	// PHP api/datasources/index.php is unauthenticated name list — keep Go gated lightly
	// with catalog access when auth is enabled (safer for /go).
	user := auth.FromContext(r.Context())
	if err := s.requireCatalogAccess(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	names := s.listDatasourceNames(r)
	items := make([]map[string]string, 0, len(names))
	for _, name := range names {
		items = append(items, map[string]string{"name": name})
	}
	s.writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAPIDatasourceCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceAdmin(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	store := s.dsStore()
	if store == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "datasource store unavailable"})
		return
	}
	data := editDatasourcePageData{DBTypes: datasources.DBTypeMetasFor(s.qcartaDBTypeKeys())}
	r.Header.Set("Accept", "application/json")
	s.handleEditDatasourcePOST(w, r, store, &data)
}

// handleAPIDatasourceTest POST /api/v1/datasources/test — Cassandra/Astra, BigQuery, or DuckDB connectivity check.
func (s *Server) handleAPIDatasourceTest(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceAdmin(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if err := parseDatasourceRequest(r); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "Failed to parse form."})
		return
	}
	ds, verrs := datasourceFromForm(r)
	if len(verrs) > 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": strings.Join(verrs, " ")})
		return
	}
	if ds.IsArcGIS() {
		s.handleAPIArcGISTest(w, r, ds)
		return
	}
	if ds.IsGeoServer() {
		s.handleAPIGeoServerTest(w, r, ds)
		return
	}
	if ds.IsBigQuery() {
		s.handleAPIBigQueryTest(w, r, ds)
		return
	}
	if ds.IsDuckDB() {
		s.handleAPIDuckDBTest(w, r, ds)
		return
	}
	if ds.IsDocument() {
		s.handleAPIDocumentTest(w, r, ds)
		return
	}
	if !ds.IsCassandra() {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "Test Connection is currently available for Apache Cassandra / DataStax Astra, Google BigQuery, DuckDB, ArcGIS Feature Server, and GeoServer."})
		return
	}
	start := time.Now()
	// Blank password/token while editing: reload secret from catalog by name.
	if strings.TrimSpace(ds.Password) == "" && strings.TrimSpace(ds.Name) != "" && s.deps.Datasources != nil {
		if existing, err := s.deps.Datasources.Find(r.Context(), ds.Name); err == nil && existing != nil {
			ds.Password = existing.Password
		}
	}
	result, err := cassandra.TestConnection(datasources.CassandraConfig(ds))
	if err != nil {
		s.recordDatasourceTest(r, ds, false, start, err.Error())
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	msg := "Connected"
	if result != nil && strings.TrimSpace(result.Message) != "" {
		msg = result.Message
	}
	s.recordDatasourceTest(r, ds, true, start, "")
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (s *Server) handleAPIBigQueryTest(w http.ResponseWriter, r *http.Request, ds datasources.Datasource) {
	start := time.Now()
	// Preserve credential resource UUID when the form omits it during edit.
	if ds.CredentialsResourceUUID() == "" && strings.TrimSpace(ds.Name) != "" && s.deps.Datasources != nil {
		if existing, err := s.deps.Datasources.Find(r.Context(), ds.Name); err == nil && existing != nil {
			if uuid := existing.CredentialsResourceUUID(); uuid != "" {
				if ds.Options == nil {
					ds.Options = map[string]string{}
				}
				ds.Options[datasources.OptionCredentialsResourceUUID] = uuid
			}
		}
	}
	credJSON, err := s.resolveBigQueryCredentials(r.Context(), ds)
	if err != nil {
		s.recordDatasourceTest(r, ds, false, start, err.Error())
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	result, err := bigquery.TestConnection(datasources.BigQueryConfig(ds, credJSON))
	if err != nil {
		s.recordDatasourceTest(r, ds, false, start, err.Error())
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	msg := "Connected"
	if result != nil && strings.TrimSpace(result.Message) != "" {
		msg = result.Message
	}
	s.recordDatasourceTest(r, ds, true, start, "")
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (s *Server) handleAPIDuckDBTest(w http.ResponseWriter, r *http.Request, ds datasources.Datasource) {
	start := time.Now()
	// Blank secrets while editing: reload from catalog; merge header secrets when partial.
	if strings.TrimSpace(ds.Name) != "" && s.deps.Datasources != nil {
		if existing, err := s.deps.Datasources.Find(r.Context(), ds.Name); err == nil && existing != nil {
			cfg := datasources.DuckDBConfig(ds)
			if duckdb.ConnectionMode(cfg) == duckdb.ModeFiles {
				auth := duckdb.AuthType(cfg)
				if strings.TrimSpace(ds.Password) == "" {
					ds.Password = existing.Password
				} else if auth == duckdb.AuthHeaders {
					keys := duckdb.HTTPHeaderKeys(cfg)
					ds.Password = duckdb.MergeHeaderSecretJSON(existing.Password, ds.Password, keys)
				}
				if auth == duckdb.AuthBasic && strings.TrimSpace(ds.Username) == "" {
					ds.Username = existing.Username
				}
				if strings.TrimSpace(ds.Option(datasources.OptionHTTPHeaderKeys)) == "" {
					if prev := existing.Option(datasources.OptionHTTPHeaderKeys); prev != "" {
						if ds.Options == nil {
							ds.Options = map[string]string{}
						}
						ds.Options[datasources.OptionHTTPHeaderKeys] = prev
					}
				}
			}
		}
	}
	result, err := duckdb.TestConnection(datasources.DuckDBConfig(ds))
	if err != nil {
		s.recordDatasourceTest(r, ds, false, start, err.Error())
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	msg := "Connected"
	if result != nil && strings.TrimSpace(result.Message) != "" {
		msg = result.Message
	}
	s.recordDatasourceTest(r, ds, true, start, "")
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (s *Server) recordDatasourceTest(r *http.Request, ds datasources.Datasource, ok bool, start time.Time, errMsg string) {
	status := audit.StatusSuccess
	details := map[string]any{"type": ds.DBType}
	if !ok {
		status = audit.StatusFailure
		if errMsg != "" {
			details["error"] = audit.SanitizeErrorMessage(errMsg)
		}
	}
	objectID := ""
	if ds.ID > 0 {
		objectID = strconv.FormatInt(ds.ID, 10)
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryDatasource,
		Action:     audit.ActionTest,
		ObjectType: audit.ObjectDatasource,
		ObjectID:   objectID,
		ObjectName: ds.Name,
		Status:     status,
		DurationMS: audit.DurationMS(time.Since(start)),
		Details:    details,
	})
}

func (s *Server) handleAPIDatasourceDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceAdmin(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	store := s.dsStore()
	if store == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "datasource store unavailable"})
		return
	}
	idStr := strings.TrimSpace(r.URL.Query().Get("id"))
	if idStr == "" {
		idStr = strings.TrimSpace(chi.URLParam(r, "id"))
	}
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	if datasources.NormalizeDBType(r.URL.Query().Get("db_type")) == "arcgis" {
		data := &datasourcesPageData{}
		s.deleteArcGISDatasourceForm(r, id, data)
		if len(data.Errors) > 0 {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": data.Errors[0]})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": data.Message})
		return
	}
	if datasources.NormalizeDBType(r.URL.Query().Get("db_type")) == "geoserver" {
		data := &datasourcesPageData{}
		s.deleteGeoServerDatasourceForm(r, id, data)
		if len(data.Errors) > 0 {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": data.Errors[0]})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": data.Message})
		return
	}
	docLogical := s.documentLogicalForID(r, store, id)
	if err := store.DeleteByID(r.Context(), id); err != nil {
		s.recordAudit(r, audit.Event{
			Category:   audit.CategoryDatasource,
			Action:     audit.ActionDelete,
			ObjectType: audit.ObjectDatasource,
			ObjectID:   strconv.FormatInt(id, 10),
			Status:     audit.StatusFailure,
			Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
		})
		if errors.Is(err, datasources.ErrInvalidInput) {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete datasource"})
		return
	}
	datasources.RemoveDocument(s.datasourceFilesDir(), docLogical)
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryDatasource,
		Action:     audit.ActionDelete,
		ObjectType: audit.ObjectDatasource,
		ObjectID:   strconv.FormatInt(id, 10),
		Status:     audit.StatusSuccess,
	})
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func statusFromErr(err error) int {
	var he *httpError
	if errors.As(err, &he) {
		return he.Status
	}
	return http.StatusInternalServerError
}
