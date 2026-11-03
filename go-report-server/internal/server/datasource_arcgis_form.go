package server

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// errArcGISProps means the jasper_arcgis row was saved or deleted, but the
// Java map properties file was not regenerated.
var errArcGISProps = errors.New("saved the ArcGIS connection, but failed to regenerate arcgis-connections.properties")

func (s *Server) datasourceTypeMetas() []datasources.DBTypeMeta {
	metas := datasources.DBTypeMetasFor(s.qcartaDBTypeKeys())
	if s.arcgisDatasourcesEnabled() && s.geoServerDatasourcesEnabled() {
		return metas
	}
	out := make([]datasources.DBTypeMeta, 0, len(metas))
	for _, meta := range metas {
		if meta.Key == "arcgis" && !s.arcgisDatasourcesEnabled() {
			continue
		}
		if meta.Key == "geoserver" && !s.geoServerDatasourcesEnabled() {
			continue
		}
		out = append(out, meta)
	}
	return out
}

func (s *Server) lookupDatasourceRow(r *http.Request, store datasources.Store, name, dbType string) (dsRowView, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return dsRowView{}, false
	}
	if datasources.NormalizeDBType(dbType) == "arcgis" {
		ds, ok := s.findArcGISDatasource(r.Context(), name)
		if !ok {
			return dsRowView{}, false
		}
		return dsToRowView(ds), true
	}
	if datasources.NormalizeDBType(dbType) == "geoserver" {
		ds, ok := s.findGeoServerDatasource(r.Context(), name)
		if !ok {
			return dsRowView{}, false
		}
		return dsToRowView(ds), true
	}
	if store != nil {
		if existing, err := store.Find(r.Context(), name); err == nil && existing != nil {
			return dsToRowView(*existing), true
		}
	}
	if strings.TrimSpace(dbType) == "" {
		if ds, ok := s.findArcGISDatasource(r.Context(), name); ok {
			return dsToRowView(ds), true
		}
		if ds, ok := s.findGeoServerDatasource(r.Context(), name); ok {
			return dsToRowView(ds), true
		}
	}
	return dsRowView{}, false
}

func (s *Server) saveArcGISDatasourceForm(r *http.Request, ds datasources.Datasource, data *editDatasourcePageData) {
	saveAction := audit.ActionCreate
	objectID := ""
	if existing, ok := s.findArcGISDatasource(r.Context(), ds.Name); ok && existing.ID > 0 {
		saveAction = audit.ActionUpdate
		objectID = strconv.FormatInt(existing.ID, 10)
	}
	err := s.saveArcGISDatasource(r, ds)
	propsWarn := errors.Is(err, errArcGISProps)
	if err != nil && !propsWarn {
		data.Errors = append(data.Errors, err.Error())
		s.logger.Warn("save arcgis datasource", "error", err)
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
	data.Message = "Datasource saved. ArcGIS Feature Server is stored in jasper_arcgis and shared with ArcGIS maps. Credentials stay on the server and are not written to JRXML."
	if propsWarn {
		data.Message += " The connection was saved, but arcgis-connections.properties was not regenerated."
	}
	if objectID == "" {
		if existing, ok := s.findArcGISDatasource(r.Context(), ds.Name); ok && existing.ID > 0 {
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

func (s *Server) deleteArcGISDatasourceForm(r *http.Request, id int64, data *datasourcesPageData) {
	err := s.deleteArcGISDatasource(r, id)
	propsWarn := errors.Is(err, errArcGISProps)
	if err != nil && !propsWarn {
		data.Errors = append(data.Errors, err.Error())
		s.recordAudit(r, audit.Event{
			Category:   audit.CategoryDatasource,
			Action:     audit.ActionDelete,
			ObjectType: audit.ObjectDatasource,
			ObjectID:   strconv.FormatInt(id, 10),
			Status:     audit.StatusFailure,
			Details:    map[string]any{"type": "arcgis", "error": audit.SanitizeErrorMessage(err.Error())},
		})
		return
	}
	data.Message = "Datasource deleted."
	if propsWarn {
		data.Message = "Datasource deleted, but arcgis-connections.properties was not regenerated."
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryDatasource,
		Action:     audit.ActionDelete,
		ObjectType: audit.ObjectDatasource,
		ObjectID:   strconv.FormatInt(id, 10),
		Status:     audit.StatusSuccess,
		Details:    map[string]any{"type": "arcgis"},
	})
}

func (s *Server) arcGISDatasourceSet(r *http.Request) map[string]bool {
	out := map[string]bool{}
	for _, name := range s.arcgisDatasourceNames(r.Context()) {
		out[name] = true
	}
	return out
}

// arcGISPublishDatasourceError keeps a published ArcGIS report bound to the
// jasper_arcgis name in acugis.datasource.connection. It does not copy
// Designer credentials.
func (s *Server) arcGISPublishDatasourceError(r *http.Request, jrxml []byte, dataSource string) (string, string) {
	if len(jrxml) == 0 {
		return "", ""
	}
	info := reports.ExtractCollectionQueryInfo(jrxml)
	if !info.IsArcGISCollection() {
		return "", ""
	}
	note := "ArcGIS query and connection name come from the JRXML. Credentials stay on the Report Server datasource."
	conn := strings.TrimSpace(info.Connection)
	if conn == "" {
		if strings.TrimSpace(dataSource) != "" && !s.arcgisDatasourceExists(r.Context(), dataSource) {
			return "Select an ArcGIS Feature Server datasource. This report has no acugis.datasource.connection name.", ""
		}
		return "", note
	}
	if !s.arcgisDatasourceExists(r.Context(), conn) {
		return "ArcGIS connection " + strconv.Quote(conn) + " is not configured. Add an ArcGIS Feature Server datasource with that name. Designer credentials are not copied.", ""
	}
	if dataSource != "" && dataSource != conn {
		return "This report uses ArcGIS connection " + strconv.Quote(conn) + ". Select that datasource.", ""
	}
	return "", note
}

func mergeDatasourceNames(primary, extra []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(primary)+len(extra))
	for _, name := range append(append([]string{}, primary...), extra...) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
