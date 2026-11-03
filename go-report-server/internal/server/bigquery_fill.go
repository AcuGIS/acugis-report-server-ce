package server

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/bigquery"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// prepareBigQueryCollectionFill resolves a BigQuery datasource, loads the Service
// Account JSON Resource server-side, runs the JRXML GoogleSQL query, and returns
// an ExecuteReport body that includes _collectionData for Tomcat collection fill.
func (s *Server) prepareBigQueryCollectionFill(
	ctx context.Context,
	reportName, rawQuery string,
) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	if s == nil || s.deps.Datasources == nil || s.deps.Reports == nil {
		return false, "", "", "", nil, nil
	}

	dsName := strings.TrimSpace(datasourceNameFromQuery(rawQuery))
	var defQuery string
	if def, gerr := s.deps.Reports.Get(reportName); gerr == nil && def != nil {
		if dsName == "" {
			dsName = strings.TrimSpace(def.Datasource())
		}
		defQuery = strings.TrimSpace(def.QuerySQL())
	}
	if dsName == "" {
		return false, "", "", "", nil, nil
	}

	jrxmlPath := filepath.Join(s.deps.Reports.JRXMLDir(), reportName+".jrxml")
	jrInfo := reports.LoadCollectionQueryInfoFromJRXMLFile(jrxmlPath)
	jrxmlSaysBQ := jrInfo.IsBigQueryCollection()

	ds, ferr := s.deps.Datasources.Find(ctx, dsName)
	if ferr != nil || ds == nil {
		if jrxmlSaysBQ {
			return true, "", "", "", nil, fmt.Errorf(
				"BigQuery report datasource %q was not found in the Server catalog — add it under Datasources as Google BigQuery",
				dsName,
			)
		}
		return false, "", "", "", nil, nil
	}

	if !ds.IsBigQuery() {
		if jrxmlSaysBQ {
			return true, "", "", "", nil, fmt.Errorf(
				"datasource %q is type %q but this report requires Google BigQuery — edit the datasource and set Database Type to Google BigQuery. BigQuery is not written to context.xml",
				ds.Name, datasources.NormalizeDBType(ds.DBType),
			)
		}
		return false, "", "", "", nil, nil
	}

	sql := strings.TrimSpace(jrInfo.Query)
	if sql == "" {
		sql = defQuery
	}
	if sql == "" {
		return true, "", "", "", nil, fmt.Errorf("BigQuery report has no GoogleSQL query (expected JRXML language=collection with acugis.datasource.provider=bigquery)")
	}

	credJSON, cerr := s.resolveBigQueryCredentials(ctx, *ds)
	if cerr != nil {
		return true, "", "", "", nil, cerr
	}

	params := jasper.ReportFillParams(rawQuery)
	cfg := datasources.BigQueryConfig(*ds, credJSON)
	s.logger.Info("bigquery fill",
		"datasource", ds.Name,
		"project", cfg.ProjectID,
		"dataset", cfg.DefaultDataset,
		"report", reportName,
	)

	start := time.Now()
	rows, rerr := bigquery.FetchTypedRows(cfg, sql, params, 0)
	if rerr != nil {
		s.logger.Error("bigquery fill failed",
			"datasource", ds.Name,
			"project", cfg.ProjectID,
			"report", reportName,
			"error", rerr,
		)
		return true, "", "", "", nil, rerr
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	s.logger.Info("bigquery fill ok",
		"datasource", ds.Name,
		"project", cfg.ProjectID,
		"report", reportName,
		"rows", len(rows),
		"duration", time.Since(start).String(),
	)

	vals, _ := url.ParseQuery(rawQuery)
	if vals.Get("_dataSource") == "" {
		vals.Set("_dataSource", dsName)
		rawQuery = vals.Encode()
	}

	method, outRawQuery, contentType, body, aerr := jasper.AttachCollectionData(rawQuery, rows)
	if aerr != nil {
		return true, "", "", "", nil, aerr
	}
	return true, method, outRawQuery, contentType, body, nil
}

// resolveBigQueryCredentials loads Service Account JSON bytes from the catalog Resource UUID.
// Never returns JSON contents to the caller for browser/API exposure — only for in-process use.
func (s *Server) resolveBigQueryCredentials(ctx context.Context, ds datasources.Datasource) ([]byte, error) {
	uuid := strings.TrimSpace(ds.CredentialsResourceUUID())
	if uuid == "" {
		return nil, fmt.Errorf("BigQuery datasource %q has no Service Account JSON resource configured", ds.Name)
	}
	if s.deps.Resources == nil || !s.deps.Resources.Configured() {
		return nil, fmt.Errorf("content resources unavailable — cannot resolve BigQuery credentials for %q", ds.Name)
	}
	_, data, err := s.deps.Resources.ReadBytes(ctx, uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to load Service Account JSON resource for datasource %q", ds.Name)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("Service Account JSON resource for datasource %q is empty", ds.Name)
	}
	return data, nil
}
