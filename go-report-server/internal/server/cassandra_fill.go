package server

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/cassandra"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// prepareCassandraCollectionFill resolves a Cassandra/Astra datasource, runs the
// JRXML (or report JSON) query via gocql / Astra Data API, and returns an
// ExecuteReport body that includes _collectionData for Tomcat collection fill.
//
// When the datasource is not Cassandra, ok is false and other return values are empty.
// Cassandra must NEVER fall through to Tomcat JDBC/JNDI (no context.xml Resource).
func (s *Server) prepareCassandraCollectionFill(
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
	jrxmlSaysCassandra := jrInfo.IsCassandraCollection()

	ds, ferr := s.deps.Datasources.Find(ctx, dsName)
	if ferr != nil || ds == nil {
		if jrxmlSaysCassandra {
			return true, "", "", "", nil, fmt.Errorf(
				"Cassandra report datasource %q was not found in the Server catalog — add it under Datasources as Apache Cassandra (do not use Generate Context XML for Cassandra)",
				dsName,
			)
		}
		return false, "", "", "", nil, nil
	}

	// Heuristic: Astra HTTPS endpoint saved under a wrong db_type still needs collection fill.
	astraHost := looksLikeAstraEndpoint(ds.Host)
	isCassandra := ds.IsCassandra() || astraHost || jrxmlSaysCassandra
	if !isCassandra {
		return false, "", "", "", nil, nil
	}
	if !ds.IsCassandra() {
		// JRXML / Astra host indicates Cassandra, but catalog type is wrong — fail clearly
		// rather than Tomcat JNDI NameNotFoundException for jdbc/{name}.
		if !astraHost && jrxmlSaysCassandra {
			return true, "", "", "", nil, fmt.Errorf(
				"datasource %q is type %q but this report requires Apache Cassandra — edit the datasource and set Database Type to Apache Cassandra (Standard or DataStax Astra). Cassandra is not written to context.xml",
				ds.Name, datasources.NormalizeDBType(ds.DBType),
			)
		}
		// Astra URL with wrong db_type: coerce for this fill only.
		dsCopy := *ds
		dsCopy.DBType = "cassandra"
		if dsCopy.Options == nil {
			dsCopy.Options = map[string]string{}
		}
		dsCopy.Options[datasources.OptionConnectionMethod] = datasources.ConnectionMethodAstra
		dsCopy.Port = 0
		dsCopy.Username = ""
		ds = &dsCopy
	}

	cql := strings.TrimSpace(jrInfo.Query)
	if cql == "" {
		cql = defQuery
	}
	if cql == "" {
		return true, "", "", "", nil, fmt.Errorf("Cassandra report has no CQL query (expected JRXML language=cassandra query or report JSON query)")
	}

	params := jasper.ReportFillParams(rawQuery)
	cfg := datasources.CassandraConfig(*ds)
	methodLabel := ds.CassandraConnectionMethod()
	if ds.IsAstra() {
		s.logger.Info("cassandra fill",
			"datasource", ds.Name,
			"method", methodLabel,
			"endpoint", ds.Host,
			"keyspace", ds.DBName,
			"report", reportName,
		)
	} else {
		s.logger.Info("cassandra fill",
			"datasource", ds.Name,
			"method", methodLabel,
			"hosts", ds.Host,
			"port", ds.Port,
			"keyspace", ds.DBName,
			"report", reportName,
		)
	}

	start := time.Now()
	rows, rerr := cassandra.FetchTypedRows(cfg, cql, params, 0)
	if rerr != nil {
		s.logger.Error("cassandra fill failed",
			"datasource", ds.Name,
			"method", methodLabel,
			"keyspace", ds.DBName,
			"report", reportName,
			"error", rerr,
		)
		return true, "", "", "", nil, rerr
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	s.logger.Info("cassandra fill ok",
		"datasource", ds.Name,
		"method", methodLabel,
		"keyspace", ds.DBName,
		"report", reportName,
		"rows", len(rows),
		"duration", time.Since(start).String(),
	)

	// Ensure Tomcat still receives _dataSource (required by ReportServlet).
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

func datasourceNameFromQuery(rawQuery string) string {
	vals, err := url.ParseQuery(rawQuery)
	if err != nil {
		return ""
	}
	if v := strings.TrimSpace(vals.Get("_dataSource")); v != "" {
		return v
	}
	return strings.TrimSpace(vals.Get("datasource"))
}

func looksLikeAstraEndpoint(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	return strings.Contains(h, "astra.datastax.com") ||
		(strings.HasPrefix(strings.ToLower(strings.TrimSpace(host)), "https://") && strings.Contains(h, ".apps.astra."))
}
