package server

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/duckdb"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// prepareDuckDBJDBCFill resolves a DuckDB datasource and returns an ExecuteReport
// body that includes _duckdbSession so Tomcat opens DuckDB JDBC and fills via
// REPORT_CONNECTION (same architecture as AcuGIS Report Designer).
//
// Does not fetch rows or attach _collectionData.
func (s *Server) prepareDuckDBJDBCFill(
	ctx context.Context,
	reportName, rawQuery string,
) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	if s == nil || s.deps.Datasources == nil || s.deps.Reports == nil {
		return false, "", "", "", nil, nil
	}

	dsName := strings.TrimSpace(datasourceNameFromQuery(rawQuery))
	if def, gerr := s.deps.Reports.Get(reportName); gerr == nil && def != nil {
		if dsName == "" {
			dsName = strings.TrimSpace(def.Datasource())
		}
	}
	if dsName == "" {
		return false, "", "", "", nil, nil
	}

	jrxmlPath := filepath.Join(s.deps.Reports.JRXMLDir(), reportName+".jrxml")
	jrInfo := reports.LoadCollectionQueryInfoFromJRXMLFile(jrxmlPath)
	jrxmlSaysDuck := jrInfo.IsDuckDB()

	ds, ferr := s.deps.Datasources.Find(ctx, dsName)
	if ferr != nil || ds == nil {
		if jrxmlSaysDuck {
			return true, "", "", "", nil, fmt.Errorf(
				"DuckDB report datasource %q was not found in the Server catalog — add it under Datasources as DuckDB",
				dsName,
			)
		}
		return false, "", "", "", nil, nil
	}

	if !ds.IsDuckDB() {
		if jrxmlSaysDuck {
			return true, "", "", "", nil, fmt.Errorf(
				"datasource %q is type %q but this report requires DuckDB — edit the datasource and set Database Type to DuckDB. DuckDB is not written to context.xml",
				ds.Name, datasources.NormalizeDBType(ds.DBType),
			)
		}
		return false, "", "", "", nil, nil
	}

	cfg := datasources.DuckDBConfig(*ds)
	mode := duckdb.ConnectionMode(cfg)
	s.logger.Info("duckdb jdbc fill",
		"datasource", ds.Name,
		"mode", mode,
		"auth", duckdb.AuthType(cfg),
		"report", reportName,
	)

	start := time.Now()
	session, serr := duckdb.BuildJDBCSession(cfg)
	if serr != nil {
		s.logger.Error("duckdb jdbc session failed",
			"datasource", ds.Name,
			"mode", mode,
			"report", reportName,
			"error", serr,
		)
		return true, "", "", "", nil, serr
	}

	vals, _ := url.ParseQuery(rawQuery)
	if vals.Get("_dataSource") == "" {
		vals.Set("_dataSource", dsName)
		rawQuery = vals.Encode()
	}

	method, outRawQuery, contentType, body, aerr := jasper.AttachDuckDBSession(rawQuery, session)
	if aerr != nil {
		return true, "", "", "", nil, aerr
	}
	s.logger.Info("duckdb jdbc fill ready",
		"datasource", ds.Name,
		"mode", mode,
		"report", reportName,
		"duration", time.Since(start).String(),
	)
	return true, method, outRawQuery, contentType, body, nil
}

// prepareDuckDBCollectionFill is kept as a deprecated alias name used by older call sites.
// Prefer prepareDuckDBJDBCFill.
func (s *Server) prepareDuckDBCollectionFill(
	ctx context.Context,
	reportName, rawQuery string,
) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	return s.prepareDuckDBJDBCFill(ctx, reportName, rawQuery)
}
