package schedules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// prepareGeoServerCollection fetches WFS attribute rows when the JRXML is a
// GeoServer collection dataset and returns a POST body with _collectionData.
func (r *Runner) prepareGeoServerCollection(ctx context.Context, reportName, datasourceName, rawQuery string) (ok bool, method, outQuery, contentType string, body io.Reader, err error) {
	if r == nil || r.Reports == nil {
		return false, "", "", "", nil, nil
	}
	jrxmlPath := filepath.Join(r.Reports.JRXMLDir(), reportName+".jrxml")
	jrInfo := reports.LoadCollectionQueryInfoFromJRXMLFile(jrxmlPath)
	if !jrInfo.IsGeoServerCollection() {
		return false, "", "", "", nil, nil
	}
	if r.GeoServer == nil {
		return true, "", "", "", nil, fmt.Errorf("GeoServer connection catalog is unavailable")
	}
	def, gerr := r.Reports.Get(reportName)
	if gerr != nil || def == nil {
		return true, "", "", "", nil, fmt.Errorf("GeoServer report definition not found")
	}
	name := geoserver.ResolveConnectionName(jrInfo.Connection, rawQuery, datasourceName)
	if name == "" {
		name = geoserver.ResolveConnectionName("", "", def.Datasource())
	}
	if name == "" {
		return true, "", "", "", nil, fmt.Errorf("GeoServer report has no connection name (expected JRXML acugis.datasource.connection)")
	}
	cred, ferr := r.GeoServer.FindGeoServer(ctx, name)
	if ferr != nil {
		if errors.Is(ferr, geoserver.ErrNotFound) {
			return true, "", "", "", nil, fmt.Errorf("GeoServer connection %q was not found — add a GeoServer datasource with that name", name)
		}
		return true, "", "", "", nil, ferr
	}
	if strings.TrimSpace(cred.URL) == "" {
		return true, "", "", "", nil, fmt.Errorf("GeoServer connection %q has no base URL", name)
	}
	sql := strings.TrimSpace(jrInfo.Query)
	if sql == "" {
		sql = strings.TrimSpace(def.QuerySQL())
	}
	if sql == "" {
		return true, "", "", "", nil, fmt.Errorf("GeoServer report has no query (expected JRXML language=collection with acugis.datasource.provider=geoserver)")
	}
	params := geoserver.FillParams(rawQuery, jrxmlPath, def.Parameters())
	rows, qerr := geoserver.FetchTypedRows(cred, sql, params, 0)
	if qerr != nil {
		return true, "", "", "", nil, qerr
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	vals, _ := url.ParseQuery(rawQuery)
	if vals.Get("_dataSource") == "" {
		vals.Set("_dataSource", name)
		rawQuery = vals.Encode()
	}
	method, outQuery, contentType, body, aerr := jasper.AttachCollectionData(rawQuery, rows)
	if aerr != nil {
		return true, "", "", "", nil, aerr
	}
	return true, method, outQuery, contentType, body, nil
}
