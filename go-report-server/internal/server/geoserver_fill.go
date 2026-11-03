package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// prepareGeoServerCollectionFill loads a GeoServer WFS attribute query and
// returns an ExecuteReport body that includes _collectionData.
// Map rendering stays on the existing WMS path and is not used here.
//
// When the report is not a GeoServer collection dataset, ok is false.
func (s *Server) prepareGeoServerCollectionFill(
	ctx context.Context,
	reportName, rawQuery string,
) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	if s == nil || s.deps.Reports == nil {
		return false, "", "", "", nil, nil
	}
	jrxmlPath := filepath.Join(s.deps.Reports.JRXMLDir(), reportName+".jrxml")
	info := reports.LoadCollectionQueryInfoFromJRXMLFile(jrxmlPath)
	if !info.IsGeoServerCollection() {
		return false, "", "", "", nil, nil
	}
	if s.deps.GeoServer == nil {
		return true, "", "", "", nil, fmt.Errorf("GeoServer connection catalog is unavailable")
	}

	var catalogName, defQuery string
	var reportParams []map[string]any
	if def, gerr := s.deps.Reports.Get(reportName); gerr == nil && def != nil {
		catalogName = strings.TrimSpace(def.Datasource())
		defQuery = strings.TrimSpace(def.QuerySQL())
		reportParams = def.Parameters()
	}
	name := geoserver.ResolveConnectionName(info.Connection, rawQuery, catalogName)
	if name == "" {
		return true, "", "", "", nil, fmt.Errorf("GeoServer report has no connection name (expected JRXML acugis.datasource.connection)")
	}
	cred, ferr := s.deps.GeoServer.FindGeoServer(ctx, name)
	if ferr != nil {
		if errors.Is(ferr, geoserver.ErrNotFound) {
			return true, "", "", "", nil, fmt.Errorf("GeoServer connection %q was not found — add a GeoServer datasource with that name", name)
		}
		return true, "", "", "", nil, ferr
	}
	if strings.TrimSpace(cred.URL) == "" {
		return true, "", "", "", nil, fmt.Errorf("GeoServer connection %q has no base URL", name)
	}

	sql := strings.TrimSpace(info.Query)
	if sql == "" {
		sql = defQuery
	}
	if sql == "" {
		return true, "", "", "", nil, fmt.Errorf("GeoServer report has no query (expected JRXML language=collection with acugis.datasource.provider=geoserver)")
	}

	params := geoserver.FillParams(rawQuery, jrxmlPath, reportParams)
	if s.logger != nil {
		s.logger.Info("geoserver fill",
			"connection", name,
			"report", reportName,
			"anonymous", strings.TrimSpace(cred.Username) == "",
		)
	}
	start := time.Now()
	rows, rerr := geoserver.FetchTypedRows(cred, sql, params, 0)
	if rerr != nil {
		if s.logger != nil {
			s.logger.Error("geoserver fill failed", "connection", name, "report", reportName, "error", rerr)
		}
		return true, "", "", "", nil, rerr
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	if s.logger != nil {
		s.logger.Info("geoserver fill ok",
			"connection", name,
			"report", reportName,
			"rows", len(rows),
			"duration", time.Since(start).String(),
		)
	}

	vals, _ := url.ParseQuery(rawQuery)
	if vals.Get("_dataSource") == "" {
		vals.Set("_dataSource", name)
		rawQuery = vals.Encode()
	}
	method, outRawQuery, contentType, body, aerr := jasper.AttachCollectionData(rawQuery, rows)
	if aerr != nil {
		return true, "", "", "", nil, aerr
	}
	return true, method, outRawQuery, contentType, body, nil
}
