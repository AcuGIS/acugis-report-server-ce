package server

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/embeddedmap"
	"github.com/acugis/acugis-report-server-go/internal/folders"
	"github.com/acugis/acugis-report-server-go/internal/governance"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func geometryKindFromPostGIS(t string) string {
	u := strings.ToUpper(strings.TrimSpace(t))
	switch {
	case strings.Contains(u, "POINT"):
		return "point"
	case strings.Contains(u, "LINE"):
		return "line"
	case strings.Contains(u, "POLYGON"):
		return "polygon"
	case u == "":
		return "unknown"
	default:
		return "mixed"
	}
}

func (s *Server) materialChrome(title string) materialChromeData {
	mount := s.shellMount()
	home := s.goURL("/hubs/reports")
	if s.qcartaProfile() {
		home = s.goURL("/workspace")
	}
	return materialChromeData{
		Title:            title,
		ProductName:      productName(),
		BrandHome:        home,
		ShowSidebarBrand: !s.qcartaProfile(),
		FederationApp:    s.federationAppID(),
		ShellMount:       jsQuoted(mount),
		PHPMount:         jsQuoted(strings.TrimRight(config.PHPAppMount, "/")),
		AssetsBase:       jsQuoted(s.assetsBase()),
		AssetsURL:        s.assetsBase(),
		CSS:              template.CSS(materialPageCSS),
	}
}

func (s *Server) listDatasourceNames(r *http.Request) []string {
	var names []string
	if namer, ok := s.deps.Datasources.(datasources.Namer); ok && namer != nil {
		listed, err := namer.ListNames(r.Context())
		if err != nil {
			s.logger.Warn("list datasources", "error", err)
		} else {
			names = listed
		}
	}
	return mergeDatasourceNames(names, mergeDatasourceNames(s.arcgisDatasourceNames(r.Context()), s.geoServerDatasourceNames(r.Context())))
}

func (s *Server) cassandraDatasourceSet(r *http.Request) map[string]bool {
	out := map[string]bool{}
	store := s.dsStore()
	if store == nil {
		return out
	}
	rows, err := store.ListEnabled(r.Context())
	if err != nil {
		return out
	}
	for _, row := range rows {
		if row.IsCassandra() {
			out[row.Name] = true
		}
	}
	return out
}

func (s *Server) bigQueryDatasourceSet(r *http.Request) map[string]bool {
	out := map[string]bool{}
	store := s.dsStore()
	if store == nil {
		return out
	}
	rows, err := store.ListEnabled(r.Context())
	if err != nil {
		return out
	}
	for _, row := range rows {
		if row.IsBigQuery() {
			out[row.Name] = true
		}
	}
	return out
}

func (s *Server) duckDBDatasourceSet(r *http.Request) map[string]bool {
	out := map[string]bool{}
	store := s.dsStore()
	if store == nil {
		return out
	}
	rows, err := store.ListEnabled(r.Context())
	if err != nil {
		return out
	}
	for _, row := range rows {
		if row.IsDuckDB() {
			out[row.Name] = true
		}
	}
	return out
}

func (s *Server) isCassandraDatasource(r *http.Request, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || s.deps.Datasources == nil {
		return false
	}
	ds, err := s.deps.Datasources.Find(r.Context(), name)
	return err == nil && ds != nil && ds.IsCassandra()
}

func (s *Server) listAccessGroups(r *http.Request) []string {
	if s.deps.ACLWriter == nil {
		return nil
	}
	groups, err := s.deps.ACLWriter.ListAccessGroups(r.Context())
	if err != nil {
		s.logger.Warn("list access groups", "error", err)
		return nil
	}
	return groups
}

func (s *Server) syncReportACL(r *http.Request, user *auth.User, key, displayName string, groups []string) string {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return ""
	}
	if s.deps.ACLWriter == nil {
		return "Application database unavailable; report was not registered for ACL."
	}
	createdBy := int64(0)
	if user != nil {
		createdBy = user.ID
	}
	if err := s.deps.ACLWriter.SyncReportAccessGroups(r.Context(), key, displayName, groups, createdBy); err != nil {
		s.logger.Warn("sync report ACL", "error", err, "report", key)
		return "Application database unavailable; report was not registered for ACL."
	}
	return ""
}

func (s *Server) requireServiceEdit(r *http.Request, user *auth.User) error {
	ok, err := s.canEditReportService(r, user)
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

func (s *Server) handlePublishPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceEdit(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	data := publishPageData{
		materialChromeData:   s.materialChrome("Publish Report"),
		DataSources:          s.listDatasourceNames(r),
		CassandraDatasources: s.cassandraDatasourceSet(r),
		BigQueryDatasources:  s.bigQueryDatasourceSet(r),
		DuckDBDatasources:    s.duckDBDatasourceSet(r),
		ArcGISDatasources:    s.arcGISDatasourceSet(r),
		AccessGroups:         s.listAccessGroups(r),
		AllowMapReports:      s.allowMapReports(),
	}
	data.HasDatasources = len(data.DataSources) > 0

	if r.Method == http.MethodPost {
		s.handlePublishPOST(w, r, user, &data)
		return
	}
	s.renderPublish(w, data)
}

func (s *Server) handlePublishPOST(w http.ResponseWriter, r *http.Request, user *auth.User, data *publishPageData) {
	wantsJSON := wantsJSONResponse(r)
	form, multipart, err := s.parseReportForm(r)
	if err != nil {
		data.Message = "JRXML upload failed. Please try again."
		data.MessageType = "danger"
		if wantsJSON {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": data.Message})
			return
		}
		s.renderPublish(w, *data)
		return
	}

	var formErrors []string
	var notes []string
	filename, jrxmlData, hasJRXML, err := readUpload(multipart, "jrxml_file")
	if err != nil || !hasJRXML || len(jrxmlData) == 0 {
		formErrors = append(formErrors, "JRXML file upload is missing.")
	}
	reportKey := reportKeyFromFilename(filename)
	if reportKey == "" {
		formErrors = append(formErrors, "Report file name must contain at least one letter or number.")
	}
	reportName := strings.TrimSpace(firstFormValue(form, "report_name", ""))
	if reportName == "" {
		reportName = reportKey
	}
	reportDescription := strings.TrimSpace(firstFormValue(form, "report_description", ""))
	dataSource := strings.TrimSpace(firstFormValue(form, "data_source", ""))
	if dataSource == "" {
		formErrors = append(formErrors, "Select a datasource.")
	}
	if msg, note := s.arcGISPublishDatasourceError(r, jrxmlData, dataSource); msg != "" {
		formErrors = append(formErrors, msg)
	} else if note != "" {
		notes = append(notes, note)
	}
	if msg, note := s.geoServerPublishDatasourceError(r, jrxmlData, dataSource); msg != "" {
		formErrors = append(formErrors, msg)
	} else if note != "" {
		notes = append(notes, note)
	}
	reportType := reports.NormalizeReportType(firstFormValue(form, "report_type", "jasper"))
	if msg := s.rejectQCartaMapReport(reportType); msg != "" {
		formErrors = append(formErrors, msg)
	} else if reportType == "map" {
		if err := s.requireEditionFeature(edition.FeatureMapReports); err != nil {
			formErrors = append(formErrors, errFeatureUnavailable.Message)
		}
	}
	if msg := s.validateMapDatasource(r, dataSource, reportType); msg != "" {
		formErrors = append(formErrors, msg)
	}
	groups := formAccessGroups(form)
	groupsCSV := formAccessGroupsCSV(groups)
	parameters, paramErrs := parseParametersFromForm(form)
	formErrors = append(formErrors, paramErrs...)

	manualQuery := strings.TrimSpace(firstFormValue(form, "query", ""))
	if manualQuery == "" {
		manualQuery = strings.TrimSpace(firstFormValue(form, "cql", ""))
	}
	var projectQuery string
	_, projectData, hasProject, perr := readUpload(multipart, "project_json")
	if hasProject {
		if perr != nil {
			notes = append(notes, "project.json upload failed.")
		} else {
			projectQuery = reports.ExtractQueryFromProjectJSON(projectData)
		}
	}
	jrxmlQuery := ""
	if len(jrxmlData) > 0 {
		jrxmlQuery = reports.ExtractCollectionQueryFromJRXML(jrxmlData)
	}
	var querySQL string
	if s.isCassandraDatasource(r, dataSource) {
		// CONNECTION is the Server datasource; QUERY comes from the report (JRXML first).
		querySQL = resolveCassandraReportQuery(jrxmlQuery, projectQuery, manualQuery)
		switch {
		case strings.TrimSpace(jrxmlQuery) != "" && querySQL == strings.TrimSpace(jrxmlQuery):
			notes = append(notes, "Using CQL from report JRXML (Report Designer).")
		case strings.TrimSpace(projectQuery) != "" && querySQL == strings.TrimSpace(projectQuery):
			notes = append(notes, "Imported CQL from project.json.")
		case querySQL == "":
			formErrors = append(formErrors, "Cassandra reports need a CQL query in the JRXML (from Report Designer). For imported reports without one, use Advanced to provide CQL or project.json.")
		}
	} else {
		querySQL = manualQuery
		if querySQL == "" && projectQuery != "" {
			querySQL = projectQuery
		}
		if querySQL == "" && jrxmlQuery != "" {
			querySQL = jrxmlQuery
		}
	}

	// First publish only: if the user did not define parameters, seed name+type from JRXML.
	// Lookups/SQL stay in the JRXML and are applied at runtime. Republish never auto-imports.
	firstPublish := reportKey != "" && (s.deps.Reports == nil || !reportJSONExists(s, reportKey))
	if firstPublish && len(parameters) == 0 && len(jrxmlData) > 0 && len(formErrors) == 0 {
		parameters = mapreport.PublishParametersFromJRXML(jrxmlData)
		if len(parameters) > 0 {
			notes = append(notes, "Imported "+strconv.Itoa(len(parameters))+" parameter(s) from JRXML (names and types).")
		}
	}

	if len(formErrors) == 0 {
		if s.deps.Reports == nil {
			formErrors = append(formErrors, "Report store unavailable.")
		} else if err := s.deps.Reports.EnsureDirs(); err != nil {
			formErrors = append(formErrors, "Failed to prepare report directories.")
		} else if !s.deps.Reports.JRXMLWritable() {
			formErrors = append(formErrors, "Report directory is not writable: "+s.deps.Reports.JRXMLDir())
		} else if err := s.deps.Reports.SaveJRXML(reportKey, jrxmlData); err != nil {
			formErrors = append(formErrors, "Failed to save JRXML file to destination.")
		} else {
			raw := buildPublishDefinition(reportKey, reportName, reportDescription, dataSource, reportType, groupsCSV, parameters, querySQL)
			if err := s.deps.Reports.SaveJSON(reportKey, raw); err != nil {
				formErrors = append(formErrors, "JRXML uploaded but failed to create JSON definition file.")
			} else {
				_, jasperData, hasJasper, jerr := readUpload(multipart, "jasper_file")
				if hasJasper {
					if jerr != nil || len(jasperData) == 0 {
						notes = append(notes, "JRXML uploaded, but .jasper upload failed.")
					} else if err := s.deps.Reports.SaveJasper(reportKey, jasperData); err != nil {
						notes = append(notes, "JRXML uploaded, but failed to save uploaded .jasper file.")
					} else {
						notes = append(notes, "Uploaded .jasper saved successfully.")
					}
				} else {
					notes = append(notes, s.deps.Reports.CompileJRXML(reportKey))
				}
				_, thumbData, hasThumb, terr := readUpload(multipart, "thumbnail_file")
				if hasThumb {
					if terr != nil || len(thumbData) == 0 {
						notes = append(notes, "Thumbnail upload failed.")
					} else if err := s.deps.Reports.SaveThumbnail(reportKey, thumbData); err != nil {
						notes = append(notes, "Thumbnail upload received but could not be saved.")
					} else {
						notes = append(notes, "Thumbnail uploaded successfully.")
					}
				}
				if note := s.syncReportACL(r, user, reportKey, reportName, groups); note != "" {
					notes = append(notes, note)
				}
				if s.deps.Folders != nil {
					var createdBy int64
					if user != nil {
						createdBy = user.ID
					}
					_ = s.deps.Folders.EnsureReportInHome(r.Context(), reportKey, reportName, createdBy)
					// Publish files an unfiled report in Home. A Designer per-report
					// resource base stays as it is. Legacy and missing bases still
					// follow the catalog folder.
					if it, err := s.deps.Folders.Repo.GetItemByKey(r.Context(), folders.ContentTypeReport, reportKey); err == nil && it != nil {
						s.refreshPublishedResourceBase(r.Context(), reportKey, it.FolderID)
					} else {
						s.refreshPublishedResourceBase(r.Context(), reportKey, 0)
					}
				}
				data.Message = "Report published successfully."
				data.MessageType = "success"
				data.PublishNotes = notes
				data.RunReportURL = s.goURL("/reports/" + reportKey)
				if dataSource != "" {
					data.RunReportURL += "?datasource=" + urlQueryEscape(dataSource)
				}
				data.ViewJSONURL = s.goURL("/api/v1/reports/" + reportKey)
				s.recordAudit(r, audit.Event{
					Category:   audit.CategoryContent,
					Action:     audit.ActionCreate,
					ObjectType: audit.ObjectReport,
					ObjectID:   reportKey,
					ObjectName: reportName,
					Status:     audit.StatusSuccess,
				})
				if wantsJSON {
					s.writeJSON(w, http.StatusCreated, map[string]any{
						"ok":       true,
						"key":      reportKey,
						"message":  data.Message,
						"notes":    notes,
						"run_url":  data.RunReportURL,
						"json_url": data.ViewJSONURL,
					})
					return
				}
				s.renderPublish(w, *data)
				return
			}
		}
	}

	data.Message = strings.Join(formErrors, " ")
	data.MessageType = "danger"
	if wantsJSON {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": data.Message, "errors": formErrors})
		return
	}
	s.renderPublish(w, *data)
}

func (s *Server) renderPublish(w http.ResponseWriter, data publishPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := publishTemplate.Execute(w, data); err != nil {
		s.logger.Error("render publish", "error", err)
	}
}

func reportJSONExists(s *Server, key string) bool {
	if s == nil || s.deps.Reports == nil || key == "" {
		return false
	}
	_, err := s.deps.Reports.Get(key)
	return err == nil
}

func (s *Server) handleEditPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceEdit(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	key := jasper.NormalizeReportKey(r.URL.Query().Get("report"))
	if key == "" {
		key = jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	}
	if !jasper.IsValidReportKey(key) {
		http.Redirect(w, r, s.goURL("/"), http.StatusFound)
		return
	}
	ok, err := s.canEditReport(r, user, key)
	if err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	if !ok && !s.isPlatformAdmin(user) {
		s.writeAuthzHTML(w, r, errPermissionDenied)
		return
	}
	if s.deps.Reports == nil {
		http.Error(w, "Report store unavailable.", http.StatusServiceUnavailable)
		return
	}
	def, err := s.deps.Reports.Get(key)
	if err != nil {
		if errors.Is(err, reports.ErrNotFound) {
			http.Redirect(w, r, s.goURL("/"), http.StatusFound)
			return
		}
		s.logger.Error("edit load report", "error", err)
		http.Error(w, "Failed to load report.", http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodPost {
		s.handleEditPOST(w, r, user, def)
		return
	}
	s.renderEdit(w, r, def, nil)
}

func (s *Server) handleEditPOST(w http.ResponseWriter, r *http.Request, user *auth.User, def *reports.Definition) {
	wantsJSON := wantsJSONResponse(r)
	form, multipart, err := s.parseReportForm(r)
	if err != nil {
		s.renderEdit(w, r, def, []string{"Failed to parse form."})
		return
	}
	var formErrors []string
	reportName := strings.TrimSpace(firstFormValue(form, "report_name", ""))
	reportDescription := strings.TrimSpace(firstFormValue(form, "report_description", ""))
	dataSource := strings.TrimSpace(firstFormValue(form, "data_source", ""))
	reportType := reports.NormalizeReportType(firstFormValue(form, "report_type", "jasper"))
	selectedGroups := formAccessGroups(form)
	groupsCSV := formAccessGroupsCSV(selectedGroups)
	parameters, paramErrs := parseParametersFromForm(form)
	formErrors = append(formErrors, paramErrs...)

	manualQuery := strings.TrimSpace(firstFormValue(form, "query", ""))
	if manualQuery == "" {
		manualQuery = strings.TrimSpace(firstFormValue(form, "cql", ""))
	}
	var projectQuery string
	_, projectData, hasProject, perr := readUpload(multipart, "project_json")
	if hasProject {
		if perr != nil {
			formErrors = append(formErrors, "project.json upload failed.")
		} else {
			projectQuery = reports.ExtractQueryFromProjectJSON(projectData)
		}
	}
	jrxmlQuery := ""
	var replacementJRXML []byte
	if _, data, present, uerr := readUpload(multipart, "jrxml_file"); present && uerr == nil && len(data) > 0 {
		replacementJRXML = data
		jrxmlQuery = reports.ExtractCollectionQueryFromJRXML(data)
	} else if s.deps.Reports != nil {
		jrxmlQuery = reports.LoadCollectionQueryFromJRXMLFile(filepath.Join(s.deps.Reports.JRXMLDir(), def.Key+".jrxml"))
	}
	parameters = reconcileEditParameters(def.Parameters(), parameters, replacementJRXML)

	var querySQL string
	if s.isCassandraDatasource(r, dataSource) {
		querySQL = resolveCassandraReportQuery(jrxmlQuery, projectQuery, manualQuery)
		// Preserve catalog query when report already had one and nothing new was supplied.
		if querySQL == "" {
			querySQL = strings.TrimSpace(def.QuerySQL())
		}
	} else {
		querySQL = manualQuery
		if querySQL == "" && projectQuery != "" {
			querySQL = projectQuery
		}
		if querySQL == "" && jrxmlQuery != "" {
			querySQL = jrxmlQuery
		}
	}

	if msg := s.rejectQCartaMapReport(reportType); msg != "" {
		formErrors = append(formErrors, msg)
	} else if reportType == "map" {
		if err := s.requireEditionFeature(edition.FeatureMapReports); err != nil {
			formErrors = append(formErrors, errFeatureUnavailable.Message)
		}
	}
	if dataSource == "" {
		formErrors = append(formErrors, "Select a datasource.")
	} else if msg := s.validateMapDatasource(r, dataSource, reportType); msg != "" {
		formErrors = append(formErrors, msg)
	}
	checkJRXML := replacementJRXML
	if len(checkJRXML) == 0 && s.deps.Reports != nil {
		if b, rerr := os.ReadFile(filepath.Join(s.deps.Reports.JRXMLDir(), def.Key+".jrxml")); rerr == nil {
			checkJRXML = b
		}
	}
	if msg, _ := s.arcGISPublishDatasourceError(r, checkJRXML, dataSource); msg != "" {
		formErrors = append(formErrors, msg)
	}
	if msg, _ := s.geoServerPublishDatasourceError(r, checkJRXML, dataSource); msg != "" {
		formErrors = append(formErrors, msg)
	}
	if s.isCassandraDatasource(r, dataSource) && strings.TrimSpace(querySQL) == "" {
		formErrors = append(formErrors, "Cassandra reports need a CQL query in the JRXML (from Report Designer). For imported reports without one, use Advanced to provide CQL or project.json.")
	}

	mapDef := mapreport.MapDefinitionFromForm(form)
	if mapDef == nil {
		mapDef = map[string]any{}
	}
	if reportType == "map" {
		mapDef["enabled"] = true
	} else if !formTruthy(form, "map[enabled]") {
		mapDef["enabled"] = false
	}

	emb := embeddedmap.FromForm(form)
	if s.qcartaProfile() {
		// QCarta edit is published-report configuration, not a map designer.
		// Preserve existing map/embedded metadata so a save without the Map tab
		// cannot disable Designer-authored map images.
		if raw, ok := def.Raw["mapDefinition"].(map[string]any); ok {
			mapDef = raw
		} else if raw, ok := def.Raw["map_definition"].(map[string]any); ok {
			mapDef = raw
		}
		emb = embeddedmap.FromDefinition(def)
		reportType = def.Type()
		if reportType == "" {
			reportType = "jasper"
		}
	}
	if emb.Enabled {
		var ds *datasources.Datasource
		if dataSource != "" && s.deps.Datasources != nil {
			if found, err := s.deps.Datasources.Find(r.Context(), dataSource); err == nil {
				ds = found
			}
		}
		provider := embeddedmap.ResolveProvider(emb.Renderer, ds)
		emb.Renderer = embeddedmap.NormalizeRenderer(emb.Renderer)
		if err := s.requireEmbeddedMapProvider(provider); err != nil {
			formErrors = append(formErrors, errFeatureUnavailable.Message)
		}
		if emb.Renderer == embeddedmap.RendererGeoServer {
			if strings.TrimSpace(emb.GeoServerWorkspace) == "" || strings.TrimSpace(emb.GeoServerLayer) == "" {
				formErrors = append(formErrors, "GeoServer Embedded Map requires a workspace and layer.")
			}
			embeddedmap.SnapshotGeoServerBaseURL(r.Context(), s.deps.GeoServer, &emb)
			if strings.TrimSpace(emb.GeoServerBaseURL) == "" && strings.TrimSpace(emb.GeoServerConnection) == "" {
				formErrors = append(formErrors, "Select a GeoServer connection for the Embedded Map.")
			}
		} else if emb.Renderer == embeddedmap.RendererQGIS {
			if strings.TrimSpace(emb.QGISLayer) == "" {
				formErrors = append(formErrors, "QGIS Embedded Map requires a layer name.")
			}
			embeddedmap.SnapshotQGISBaseURL(r.Context(), s.deps.QGIS, &emb)
			if strings.TrimSpace(emb.QGISBaseURL) == "" && strings.TrimSpace(emb.QGISConnection) == "" {
				formErrors = append(formErrors, "Select a QGIS Server connection for the Embedded Map.")
			}
		} else {
			if strings.TrimSpace(stringify(mapDef["geometryColumn"])) == "" {
				mapDef["geometryColumn"] = "geom"
			}
			if ds != nil {
				switch {
				case emb.Renderer == embeddedmap.RendererMySQL && !ds.IsMySQL():
					formErrors = append(formErrors, "MySQL Embedded Map requires a MySQL datasource.")
				case emb.Renderer == embeddedmap.RendererPostGIS && !ds.IsPostgreSQL():
					formErrors = append(formErrors, "PostGIS Embedded Map requires a PostgreSQL/PostGIS datasource.")
				case !ds.IsPostgreSQL() && !ds.IsMySQL():
					formErrors = append(formErrors, "Embedded Map requires a PostgreSQL/PostGIS or MySQL datasource.")
				}
			}
		}
	}

	raw := applyEditDefinition(def.Raw, def.Key, reportName, reportDescription, dataSource, reportType, groupsCSV, parameters, mapDef, emb.ToMap(), querySQL)

	jrxmlUploaded := false
	if len(formErrors) == 0 {
		if err := s.deps.Reports.SaveJSON(def.Key, raw); err != nil {
			formErrors = append(formErrors, "Failed to save report JSON.")
		}
	}
	if len(formErrors) == 0 {
		if _, data, present, uerr := readUpload(multipart, "jrxml_file"); present {
			jrxmlUploaded = true
			if uerr != nil || len(data) == 0 || s.deps.Reports.SaveJRXML(def.Key, data) != nil {
				formErrors = append(formErrors, "Failed to save uploaded JRXML.")
			}
		}
	}
	if len(formErrors) == 0 {
		if _, data, present, uerr := readUpload(multipart, "jasper_file"); present {
			if uerr != nil || len(data) == 0 || s.deps.Reports.SaveJasper(def.Key, data) != nil {
				formErrors = append(formErrors, "Failed to save uploaded .jasper file.")
			}
		}
	}
	if len(formErrors) == 0 {
		if _, data, present, uerr := readUpload(multipart, "thumbnail_file"); present {
			if uerr != nil || len(data) == 0 || s.deps.Reports.SaveThumbnail(def.Key, data) != nil {
				formErrors = append(formErrors, "Failed to save thumbnail.")
			}
		}
	}

	if len(formErrors) == 0 && s.deps.Reports != nil && !s.qcartaProfile() {
		jrxmlPath := filepath.Join(s.deps.Reports.JRXMLDir(), def.Key+".jrxml")
		sync := embeddedmap.SyncJRXML(jrxmlPath, emb)
		if !sync.OK {
			msg := strings.TrimSpace(sync.Message)
			if msg == "" {
				msg = "Unable to update the report layout. Please contact your administrator."
			}
			formErrors = append(formErrors, msg)
			s.logger.Error("embedded map JRXML sync failed", "report", def.Key, "message", sync.Message)
		} else if sync.JRXMLChanged || jrxmlUploaded {
			// Always drop .jasper after JRXML layout changes so Tomcat cannot keep
			// serving a pre-map binary (it prefers .jasper when equally fresh).
			if err := s.deps.Reports.RemoveJasper(def.Key); err != nil {
				s.logger.Error("failed to invalidate .jasper after JRXML sync",
					"report", def.Key, "error", err)
			}
			compiled := s.deps.Reports.CompileJRXMLResult(def.Key)
			if !compiled.OK {
				s.logger.Warn("compile skipped after embedded map sync; Tomcat will compile JRXML",
					"report", def.Key, "note", compiled.Message, "detail", compiled.Detail)
			}
		}
	} else if len(formErrors) == 0 && s.qcartaProfile() && jrxmlUploaded && s.deps.Reports != nil {
		if _, data, present, uerr := readUpload(multipart, "jasper_file"); !present || uerr != nil || len(data) == 0 {
			if err := s.deps.Reports.RemoveJasper(def.Key); err != nil {
				s.logger.Error("failed to invalidate .jasper after JRXML replace",
					"report", def.Key, "error", err)
			}
			compiled := s.deps.Reports.CompileJRXMLResult(def.Key)
			if !compiled.OK {
				s.logger.Warn("compile skipped after JRXML replace; Tomcat will compile JRXML",
					"report", def.Key, "note", compiled.Message, "detail", compiled.Detail)
			}
		}
	}

	if len(formErrors) == 0 {
		disp := stringify(raw["name"])
		_ = s.syncReportACL(r, user, def.Key, disp, selectedGroups)
		if s.deps.Folders != nil {
			if err := s.deps.Folders.UpdateReportDisplayName(r.Context(), def.Key, disp); err != nil {
				s.logger.Warn("sync folder display name after edit", "error", err, "report", def.Key)
			}
		}
		s.recordAudit(r, audit.Event{
			Category:   audit.CategoryContent,
			Action:     audit.ActionUpdate,
			ObjectType: audit.ObjectReport,
			ObjectID:   def.Key,
			ObjectName: disp,
			Status:     audit.StatusSuccess,
		})
		if wantsJSON {
			s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": def.Key})
			return
		}
		http.Redirect(w, r, s.reportsCatalogURL(), http.StatusFound)
		return
	}

	def.Raw = raw
	if wantsJSON {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": strings.Join(formErrors, " "), "errors": formErrors})
		return
	}
	s.renderEdit(w, r, def, formErrors)
}

func (s *Server) renderEdit(w http.ResponseWriter, r *http.Request, def *reports.Definition, formErrors []string) {
	currentMap := mapreport.NormalizeMapDefinition(nil, "geom")
	if raw, ok := def.Raw["mapDefinition"].(map[string]any); ok {
		currentMap = mapreport.NormalizeMapDefinition(raw, stringify(raw["geometryColumn"]))
	} else if raw, ok := def.Raw["map_definition"].(map[string]any); ok {
		currentMap = mapreport.NormalizeMapDefinition(raw, stringify(raw["geometryColumn"]))
	}
	reportType := def.Type()
	if reportType == "map" {
		currentMap["enabled"] = true
	}

	geomColumns := []string{}
	layerError := ""
	layerTable := ""
	layerGeometryKind := "unknown"
	reportFields := []reportFieldOpt{}
	numericFields := []reportFieldOpt{}
	if s.deps.Map != nil {
		layer, _ := s.deps.Map.ResolveLayer(r.Context(), def)
		geomColumns = append(geomColumns, layer.GeometryColumns...)
		layerError = layer.Error
		layerGeometryKind = geometryKindFromPostGIS(layer.GeometryType)
		if layer.Schema != "" && layer.Table != "" {
			layerTable = layer.Schema + "." + layer.Table
		}
		for _, f := range layer.ReportFields {
			name := stringify(f["name"])
			if name == "" {
				continue
			}
			label := stringify(f["label"])
			if label == "" {
				label = name
			}
			opt := reportFieldOpt{Name: name, Label: label}
			reportFields = append(reportFields, opt)
			ft := strings.ToLower(stringify(f["type"]))
			if strings.Contains(ft, "int") || strings.Contains(ft, "num") || strings.Contains(ft, "float") || strings.Contains(ft, "double") || strings.Contains(ft, "decimal") || strings.Contains(ft, "real") {
				numericFields = append(numericFields, opt)
			}
		}
	}
	if len(numericFields) == 0 {
		numericFields = reportFields
	}
	geom := stringify(currentMap["geometryColumn"])
	found := false
	for _, g := range geomColumns {
		if g == geom {
			found = true
			break
		}
	}
	if geom != "" && !found {
		geomColumns = append(geomColumns, geom)
	}

	groupsCSV := stringify(def.Raw["access_groups"])
	selected := map[string]bool{}
	var currentGroups []string
	if groupsCSV != "" {
		for _, g := range strings.Split(groupsCSV, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				selected[g] = true
				currentGroups = append(currentGroups, g)
			}
		}
	}
	accessGroups := s.listAccessGroups(r)
	seen := map[string]bool{}
	mergedGroups := make([]string, 0, len(accessGroups)+len(currentGroups))
	for _, g := range append(accessGroups, currentGroups...) {
		if seen[g] {
			continue
		}
		seen[g] = true
		mergedGroups = append(mergedGroups, g)
	}

	ss, _ := currentMap["singleSymbol"].(map[string]any)
	labels, _ := currentMap["labels"].(map[string]any)
	popups, _ := currentMap["popups"].(map[string]any)
	tooltips, _ := currentMap["tooltips"].(map[string]any)
	sync, _ := currentMap["sync"].(map[string]any)
	if popups == nil {
		popups = map[string]any{}
	}
	if tooltips == nil {
		tooltips = map[string]any{}
	}
	if sync == nil {
		sync = map[string]any{}
	}
	if ss == nil {
		ss = map[string]any{}
	}
	if labels == nil {
		labels = map[string]any{}
	}
	popupSelected := map[string]bool{}
	if popups != nil {
		if fields, ok := popups["fields"].([]any); ok {
			for _, f := range fields {
				popupSelected[stringify(f)] = true
			}
		}
	}
	tooltipSelected := map[string]bool{}
	if tooltips != nil {
		if fields, ok := tooltips["fields"].([]any); ok {
			for _, f := range fields {
				tooltipSelected[stringify(f)] = true
			}
		}
	}
	categories := []mapCategoryRow{}
	if cats, ok := currentMap["categories"].([]any); ok {
		for i, c := range cats {
			m, ok := c.(map[string]any)
			if !ok {
				continue
			}
			categories = append(categories, mapCategoryRow{
				Index:        i,
				Value:        stringify(m["value"]),
				Label:        stringify(m["label"]),
				FillColor:    firstNonEmpty(stringify(m["fillColor"]), "#9e9e9e"),
				OutlineColor: firstNonEmpty(stringify(m["outlineColor"]), "#424242"),
			})
		}
	}

	currentEmbedded := embeddedmap.FromDefinition(def)
	embMap := currentEmbedded.ToMap()
	jrxmlPath := ""
	if s.deps.Reports != nil {
		jrxmlPath = filepath.Join(s.deps.Reports.JRXMLDir(), def.Key+".jrxml")
	}
	jrxmlGroups := embeddedmap.GroupNames(jrxmlPath)

	mapTypes := make([]struct{ Key, Label string }, 0, len(embeddedmap.MapTypes))
	mapReady := map[string]bool{}
	for _, key := range []string{"locator", "choropleth", "categorized", "graduated_symbols", "selected_features", "overview_highlight"} {
		mapTypes = append(mapTypes, struct{ Key, Label string }{Key: key, Label: embeddedmap.MapTypes[key]})
		mapReady[key] = embeddedmap.MapTypeReady(key)
	}
	pointShapes := make([]struct{ Key, Label string }, 0, len(embeddedmap.PointShapes))
	for _, key := range []string{"circle", "square", "triangle", "diamond", "star", "cross"} {
		pointShapes = append(pointShapes, struct{ Key, Label string }{Key: key, Label: embeddedmap.PointShapes[key]})
	}
	bands := make([]struct{ Key, Label string }, 0, len(embeddedmap.Bands))
	for _, key := range []string{"summary", "detail", "groupHeader"} {
		bands = append(bands, struct{ Key, Label string }{Key: key, Label: embeddedmap.Bands[key]})
	}
	classifs := make([]struct{ Key, Label string }, 0, len(embeddedmap.Classifications))
	for _, key := range []string{"quantile", "equal_interval", "natural_breaks"} {
		classifs = append(classifs, struct{ Key, Label string }{Key: key, Label: embeddedmap.Classifications[key]})
	}

	gsConns := []string{}
	if s.deps.GeoServer != nil {
		if list, err := s.deps.GeoServer.ListEnabled(r.Context()); err == nil {
			for _, c := range list {
				if strings.TrimSpace(c.Name) != "" {
					gsConns = append(gsConns, c.Name)
				}
			}
		}
	}
	qgisConns := []string{}
	if s.deps.QGIS != nil {
		if list, err := s.deps.QGIS.ListEnabled(r.Context()); err == nil {
			for _, c := range list {
				if strings.TrimSpace(c.Name) != "" {
					qgisConns = append(qgisConns, c.Name)
				}
			}
		}
	}
	embCaps := s.Edition().Capabilities().Maps.Embedded
	filterJSON := "[]"
	if rows, ok := embMap["geoserverFilterMappings"].([]map[string]any); ok && len(rows) > 0 {
		if b, err := json.Marshal(rows); err == nil {
			filterJSON = string(b)
		}
	} else if rows, ok := embMap["geoserverFilterMappings"].([]any); ok && len(rows) > 0 {
		if b, err := json.Marshal(rows); err == nil {
			filterJSON = string(b)
		}
	}

	params := paramViewModels(def.Parameters())
	querySQL, querySource := resolveReportQueryDisplay(def, s.deps.Reports)
	data := editPageData{
		materialChromeData:         s.materialChrome("Edit Report"),
		ReportKey:                  def.Key,
		CatalogURL:                 s.reportsCatalogURL(),
		DataSources:                s.listDatasourceNames(r),
		CassandraDatasources:       s.cassandraDatasourceSet(r),
		BigQueryDatasources:        s.bigQueryDatasourceSet(r),
		DuckDBDatasources:          s.duckDBDatasourceSet(r),
		ArcGISDatasources:          s.arcGISDatasourceSet(r),
		QuerySQL:                   querySQL,
		QuerySource:                querySource,
		HasReportQuery:             querySQL != "",
		AccessGroups:               mergedGroups,
		SelectedGroups:             selected,
		FormErrors:                 formErrors,
		Name:                       def.DisplayName(),
		Description:                firstNonEmpty(stringify(def.Raw["description"]), stringify(def.Raw["summary"])),
		Datasource:                 def.Datasource(),
		ReportType:                 reportType,
		AllowMapReports:            s.allowMapReports(),
		ShowMapTab:                 !s.qcartaProfile(),
		Params:                     params,
		ParamCount:                 len(params),
		Map:                        currentMap,
		MapEnabled:                 truthyAny(currentMap["enabled"]),
		Basemaps:                   mapreport.MapBasemaps,
		InitialZooms:               mapreport.MapInitialZoom,
		ColorRamps:                 mapreport.MapColorRamps,
		GeomColumns:                geomColumns,
		ReportFields:               reportFields,
		LayerError:                 layerError,
		LayerTable:                 layerTable,
		Categories:                 categories,
		CategoryCount:              len(categories),
		PopupSelected:              popupSelected,
		TooltipSelected:            tooltipSelected,
		Renderer:                   firstNonEmpty(stringify(currentMap["renderer"]), "single"),
		Classification:             firstNonEmpty(stringify(currentMap["classification"]), "quantile"),
		SingleSymbol:               ss,
		Labels:                     labels,
		PopupsEnabled:              truthyAny(popups["enabled"]),
		TooltipsEnabled:            truthyAny(tooltips["enabled"]),
		Sync:                       sync,
		Embedded:                   embMap,
		EmbeddedEnabled:            currentEmbedded.Enabled,
		EmbeddedMapTypes:           mapTypes,
		EmbeddedMapReady:           mapReady,
		EmbeddedPointShapes:        pointShapes,
		EmbeddedBands:              bands,
		EmbeddedClassifs:           classifs,
		EmbeddedRampIDs:            embeddedmap.ColorRampIDs(),
		JRXMLGroups:                jrxmlGroups,
		LayerGeometryKind:          layerGeometryKind,
		NumericFields:              numericFields,
		GeoServerConnections:       gsConns,
		QGISConnections:            qgisConns,
		EmbeddedCaps:               embCaps,
		EmbeddedFilterMappingsJSON: template.JS(filterJSON),
		EmbeddedBasemaps:           append([]struct{ Key, Label string }{}, embeddedmap.BasemapOptions...),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := editTemplate.Execute(w, data); err != nil {
		s.logger.Error("render edit", "error", err)
	}
}

func (s *Server) handleDeleteReport(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceEdit(r, user); err != nil {
		if wantsJSONResponse(r) {
			s.writeAuthzJSON(w, err)
			return
		}
		s.writeAuthzHTML(w, r, err)
		return
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
	}
	key := jasper.NormalizeReportKey(r.URL.Query().Get("report"))
	if key == "" {
		key = jasper.NormalizeReportKey(r.FormValue("report"))
	}
	if key == "" {
		key = jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	}
	wantsJSON := wantsJSONResponse(r) || r.Method == http.MethodDelete
	if s.qcartaProfile() && r.Method == http.MethodGet && !wantsJSON {
		if !jasper.IsValidReportKey(key) {
			http.Redirect(w, r, s.reportsCatalogURL(), http.StatusFound)
			return
		}
		name := key
		if s.deps.Reports != nil {
			if def, err := s.deps.Reports.Get(key); err == nil && def != nil {
				name = def.DisplayName()
			}
		}
		s.renderQCartaDeleteConfirm(w, key, name)
		return
	}

	if jasper.IsValidReportKey(key) {
		ok, err := s.canAdminReport(r, user, key)
		if err != nil {
			if wantsJSON {
				s.writeAuthzJSON(w, err)
				return
			}
			s.writeAuthzHTML(w, r, err)
			return
		}
		if !ok && !s.isPlatformAdmin(user) {
			if wantsJSON {
				s.writeAuthzJSON(w, errPermissionDenied)
				return
			}
			s.writeAuthzHTML(w, r, errPermissionDenied)
			return
		}
		if s.deps.Reports != nil {
			if err := s.deps.Reports.Delete(key); err != nil {
				s.logger.Error("delete report", "error", err, "key", key)
				s.recordAudit(r, audit.Event{
					Category:   audit.CategoryContent,
					Action:     audit.ActionDelete,
					ObjectType: audit.ObjectReport,
					ObjectID:   key,
					ObjectName: key,
					Status:     audit.StatusFailure,
					Details:    map[string]any{"error": audit.SanitizeErrorMessage(err.Error())},
				})
				if wantsJSON {
					s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete report"})
					return
				}
			} else {
				s.recordAudit(r, audit.Event{
					Category:   audit.CategoryContent,
					Action:     audit.ActionDelete,
					ObjectType: audit.ObjectReport,
					ObjectID:   key,
					ObjectName: key,
					Status:     audit.StatusSuccess,
				})
				s.cleanupGovernance(r.Context(), governance.ObjectReport, key)
				if s.qcartaProfile() && s.deps.ACLWriter != nil {
					if err := s.deps.ACLWriter.DeleteResource(r.Context(), acl.ResourceTypeJasperReport, key); err != nil {
						s.logger.Warn("remove report ACL after delete", "error", err, "key", key)
					}
				}
			}
		}
		if s.deps.Folders != nil {
			if err := s.deps.Folders.RemoveReport(r.Context(), key); err != nil {
				s.logger.Warn("remove report from folders after delete", "error", err, "key", key)
			}
		}
	}

	if wantsJSON {
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": key})
		return
	}
	http.Redirect(w, r, s.reportsCatalogURL(), http.StatusFound)
}

func (s *Server) handleAPIReportCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceEdit(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	data := publishPageData{
		materialChromeData:   s.materialChrome("Publish Report"),
		DataSources:          s.listDatasourceNames(r),
		CassandraDatasources: s.cassandraDatasourceSet(r),
		BigQueryDatasources:  s.bigQueryDatasourceSet(r),
		DuckDBDatasources:    s.duckDBDatasourceSet(r),
		ArcGISDatasources:    s.arcGISDatasourceSet(r),
		AccessGroups:         s.listAccessGroups(r),
		AllowMapReports:      s.allowMapReports(),
	}
	// Force JSON responses for the REST create endpoint.
	r.Header.Set("Accept", "application/json")
	s.handlePublishPOST(w, r, user, &data)
}

func (s *Server) handleAPIReportUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireServiceEdit(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	key := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	if !jasper.IsValidReportKey(key) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid report."})
		return
	}
	ok, err := s.canEditReport(r, user, key)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !ok && !s.isPlatformAdmin(user) {
		s.writeAuthzJSON(w, errPermissionDenied)
		return
	}
	if s.deps.Reports == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "report store unavailable"})
		return
	}
	def, err := s.deps.Reports.Get(key)
	if err != nil {
		if errors.Is(err, reports.ErrNotFound) {
			s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "Report not found."})
			return
		}
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load report"})
		return
	}
	r.Header.Set("Accept", "application/json")
	s.handleEditPOST(w, r, user, def)
}

func wantsJSONResponse(r *http.Request) bool {
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(accept, "application/json") || strings.HasPrefix(r.URL.Path, "/api/")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
