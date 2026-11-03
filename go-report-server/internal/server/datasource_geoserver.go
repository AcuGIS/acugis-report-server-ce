package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

const (
	geoServerNameMax = 80
	geoServerURLMax  = 500
)

// errGeoServerProps means the jasper_geoserver row was saved or deleted, but the
// Java map properties file was not regenerated.
var errGeoServerProps = errors.New("saved the GeoServer connection, but failed to regenerate geoserver-connections.properties")

func (s *Server) geoServerDatasourceNames(ctx context.Context) []string {
	if s == nil || s.deps.GeoServer == nil {
		return nil
	}
	rows, err := s.deps.GeoServer.ListEnabled(ctx)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func (s *Server) geoServerDatasourceExists(ctx context.Context, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || s == nil || s.deps.GeoServer == nil {
		return false
	}
	_, err := s.deps.GeoServer.FindGeoServer(ctx, name)
	return err == nil
}

// geoServerPublishDatasourceError keeps a published GeoServer report bound to
// the jasper_geoserver name in acugis.datasource.connection. It does not copy
// Designer credentials.
func (s *Server) geoServerPublishDatasourceError(r *http.Request, jrxml []byte, dataSource string) (string, string) {
	if len(jrxml) == 0 {
		return "", ""
	}
	info := reports.ExtractCollectionQueryInfo(jrxml)
	if !info.IsGeoServerCollection() {
		return "", ""
	}
	note := "GeoServer query and connection name come from the JRXML. Credentials stay on the Report Server GeoServer connection."
	conn := strings.TrimSpace(info.Connection)
	if conn == "" {
		if strings.TrimSpace(dataSource) != "" && !s.geoServerDatasourceExists(r.Context(), dataSource) {
			return "Select a GeoServer datasource. This report has no acugis.datasource.connection name.", ""
		}
		return "", note
	}
	if !s.geoServerDatasourceExists(r.Context(), conn) {
		return "GeoServer connection " + strconv.Quote(conn) + " is not configured. Add a GeoServer connection with that name. Designer credentials are not copied.", ""
	}
	if dataSource != "" && dataSource != conn {
		return "This report uses GeoServer connection " + strconv.Quote(conn) + ". Select that datasource.", ""
	}
	return "", note
}

func (s *Server) geoServerDatasourcesEnabled() bool {
	return s != nil && s.deps.GeoServer != nil
}

func (s *Server) listGeoServerDatasources(ctx context.Context) []datasources.Datasource {
	if s == nil || s.deps.GeoServer == nil {
		return nil
	}
	rows, err := s.deps.GeoServer.ListEnabled(ctx)
	if err != nil {
		return nil
	}
	out := make([]datasources.Datasource, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Name) == "" {
			continue
		}
		out = append(out, geoServerConnectionToDatasource(row))
	}
	return out
}

func (s *Server) findGeoServerDatasource(ctx context.Context, name string) (datasources.Datasource, bool) {
	name = strings.TrimSpace(name)
	if name == "" || s == nil || s.deps.GeoServer == nil {
		return datasources.Datasource{}, false
	}
	cred, err := s.deps.GeoServer.FindGeoServer(ctx, name)
	if err != nil {
		return datasources.Datasource{}, false
	}
	rows, listErr := s.deps.GeoServer.ListEnabled(ctx)
	id := int64(0)
	if listErr == nil {
		for _, row := range rows {
			if row.Name == cred.Name {
				id = row.ID
				break
			}
		}
	}
	return geoServerConnectionToDatasource(geoserver.Connection{
		ID:       id,
		Name:     cred.Name,
		URL:      cred.URL,
		Username: cred.Username,
		Password: cred.Password,
	}), true
}

func geoServerConnectionToDatasource(c geoserver.Connection) datasources.Datasource {
	baseURL := strings.TrimRight(strings.TrimSpace(c.URL), "/")
	return datasources.Datasource{
		ID:       c.ID,
		Name:     strings.TrimSpace(c.Name),
		DBType:   "geoserver",
		Host:     baseURL,
		DBName:   baseURL,
		Username: strings.TrimSpace(c.Username),
		Password: c.Password,
		AuthType: datasources.AuthPassword,
	}
}

func (s *Server) saveGeoServerDatasource(r *http.Request, ds datasources.Datasource) error {
	if s == nil || s.deps.GeoServer == nil {
		return errors.New("GeoServer connection catalog is unavailable")
	}
	if !s.deps.GeoServer.TableAvailable(r.Context()) {
		return errors.New("Missing public.jasper_geoserver. Load database/schema.sql or database/migrate_geoserver.sql into the application database.")
	}
	name := strings.TrimSpace(ds.Name)
	if name == "" {
		return errors.New("Name and GeoServer URL are required.")
	}
	if strings.Contains(name, ".") {
		return errors.New("Datasource name must not contain a period.")
	}
	if len(name) > geoServerNameMax {
		return fmt.Errorf("Datasource name must be %d characters or fewer.", geoServerNameMax)
	}
	baseURL, err := geoserver.NormalizeBaseURL(ds.Host)
	if err != nil {
		return err
	}
	if len(baseURL) > geoServerURLMax {
		return fmt.Errorf("GeoServer URL must be %d characters or fewer.", geoServerURLMax)
	}
	username := strings.TrimSpace(ds.Username)
	password := ds.Password
	if username == "" {
		password = ""
	} else if strings.TrimSpace(password) == "" {
		if existing, ferr := s.deps.GeoServer.FindGeoServer(r.Context(), name); ferr == nil {
			password = existing.Password
		}
	}
	if store := s.dsStore(); store != nil {
		if existing, ferr := store.Find(r.Context(), name); ferr == nil && existing != nil && !existing.IsGeoServer() {
			return fmt.Errorf("A datasource named %q already exists.", name)
		}
	}
	if s.arcgisDatasourceExists(r.Context(), name) {
		return fmt.Errorf("An ArcGIS Feature Server datasource named %q already exists.", name)
	}
	if err := s.deps.GeoServer.Upsert(r.Context(), geoserver.Connection{
		Name:     name,
		URL:      baseURL,
		Username: username,
		Password: password,
	}); err != nil {
		if errors.Is(err, geoserver.ErrInvalidInput) {
			return errors.New("Name and GeoServer URL are required, and the name must not contain a period.")
		}
		return errors.New("Failed to save GeoServer datasource.")
	}
	if err := s.writeGeoServerProps(r, s.deps.GeoServer); err != nil {
		s.logger.Warn("geoserver properties", "error", err)
		return errGeoServerProps
	}
	return nil
}

func (s *Server) deleteGeoServerDatasource(r *http.Request, id int64) error {
	if s == nil || s.deps.GeoServer == nil {
		return errors.New("GeoServer connection catalog is unavailable")
	}
	if err := s.deps.GeoServer.DeleteByID(r.Context(), id); err != nil {
		return errors.New("Failed to delete datasource.")
	}
	if err := s.writeGeoServerProps(r, s.deps.GeoServer); err != nil {
		s.logger.Warn("geoserver properties", "error", err)
		return errGeoServerProps
	}
	return nil
}

func (s *Server) testGeoServerDatasource(ctx context.Context, ds datasources.Datasource) (string, error) {
	username := strings.TrimSpace(ds.Username)
	password := ds.Password
	if username != "" && strings.TrimSpace(password) == "" && strings.TrimSpace(ds.Name) != "" && s != nil && s.deps.GeoServer != nil {
		if existing, err := s.deps.GeoServer.FindGeoServer(ctx, ds.Name); err == nil {
			password = existing.Password
		}
	}
	if username == "" {
		password = ""
	}
	return geoserver.TestConnection(ctx, geoserver.Credentials{
		Name:     strings.TrimSpace(ds.Name),
		URL:      ds.Host,
		Username: username,
		Password: password,
	})
}

func (s *Server) handleAPIGeoServerTest(w http.ResponseWriter, r *http.Request, ds datasources.Datasource) {
	if !s.geoServerDatasourcesEnabled() {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "GeoServer is not available."})
		return
	}
	start := time.Now()
	msg, err := s.testGeoServerDatasource(r.Context(), ds)
	if err != nil {
		s.recordDatasourceTest(r, ds, false, start, err.Error())
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	if strings.TrimSpace(msg) == "" {
		msg = "Connected"
	}
	s.recordDatasourceTest(r, ds, true, start, "")
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (s *Server) saveGeoServerDatasourceForm(r *http.Request, ds datasources.Datasource, data *editDatasourcePageData) {
	saveAction := audit.ActionCreate
	objectID := ""
	if existing, ok := s.findGeoServerDatasource(r.Context(), ds.Name); ok && existing.ID > 0 {
		saveAction = audit.ActionUpdate
		objectID = strconv.FormatInt(existing.ID, 10)
	}
	err := s.saveGeoServerDatasource(r, ds)
	propsWarn := errors.Is(err, errGeoServerProps)
	if err != nil && !propsWarn {
		data.Errors = append(data.Errors, err.Error())
		s.logger.Warn("save geoserver datasource", "error", err)
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
		return
	}
	data.Message = "Datasource saved. GeoServer is stored in jasper_geoserver and shared with GeoServer maps. Credentials stay on the server and are not written to JRXML."
	if propsWarn {
		data.Message += " The connection was saved, but geoserver-connections.properties was not regenerated."
	}
	if objectID == "" {
		if existing, ok := s.findGeoServerDatasource(r.Context(), ds.Name); ok && existing.ID > 0 {
			objectID = strconv.FormatInt(existing.ID, 10)
		}
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

func (s *Server) deleteGeoServerDatasourceForm(r *http.Request, id int64, data *datasourcesPageData) {
	err := s.deleteGeoServerDatasource(r, id)
	propsWarn := errors.Is(err, errGeoServerProps)
	if err != nil && !propsWarn {
		data.Errors = append(data.Errors, err.Error())
		s.recordAudit(r, audit.Event{
			Category:   audit.CategoryDatasource,
			Action:     audit.ActionDelete,
			ObjectType: audit.ObjectDatasource,
			ObjectID:   strconv.FormatInt(id, 10),
			Status:     audit.StatusFailure,
			Details:    map[string]any{"type": "geoserver", "error": audit.SanitizeErrorMessage(err.Error())},
		})
		return
	}
	data.Message = "Datasource deleted."
	if propsWarn {
		data.Message = "Datasource deleted, but geoserver-connections.properties was not regenerated."
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryDatasource,
		Action:     audit.ActionDelete,
		ObjectType: audit.ObjectDatasource,
		ObjectID:   strconv.FormatInt(id, 10),
		Status:     audit.StatusSuccess,
		Details:    map[string]any{"type": "geoserver"},
	})
}
