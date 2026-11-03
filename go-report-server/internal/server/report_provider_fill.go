package server

import (
	"context"
	"io"
)

// prepareReportProviderFill runs the same provider preparation Report Run uses
// before Tomcat fill. The first matching provider wins. Collection providers
// (Cassandra, BigQuery, ArcGIS, GeoServer, CSV, JSON, Excel, XML) attach
// _collectionData. DuckDB attaches a JDBC session instead of collection rows.
//
// ok is false for ordinary JDBC/SQL reports, which Report Run sends to Tomcat
// unchanged and Report Query sends to /reportQuery. Collection languages must
// not be sent to ReportQueryRunner.
func (s *Server) prepareReportProviderFill(
	ctx context.Context,
	reportName, rawQuery string,
) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	if used, method, outRawQuery, contentType, body, err := s.prepareCassandraCollectionFill(ctx, reportName, rawQuery); used {
		return true, method, outRawQuery, contentType, body, err
	}
	if used, method, outRawQuery, contentType, body, err := s.prepareBigQueryCollectionFill(ctx, reportName, rawQuery); used {
		return true, method, outRawQuery, contentType, body, err
	}
	if used, method, outRawQuery, contentType, body, err := s.prepareArcGISCollectionFill(ctx, reportName, rawQuery); used {
		return true, method, outRawQuery, contentType, body, err
	}
	if used, method, outRawQuery, contentType, body, err := s.prepareGeoServerCollectionFill(ctx, reportName, rawQuery); used {
		return true, method, outRawQuery, contentType, body, err
	}
	if used, method, outRawQuery, contentType, body, err := s.prepareCSVCollectionFill(ctx, reportName, rawQuery); used {
		return true, method, outRawQuery, contentType, body, err
	}
	if used, method, outRawQuery, contentType, body, err := s.prepareJSONCollectionFill(ctx, reportName, rawQuery); used {
		return true, method, outRawQuery, contentType, body, err
	}
	if used, method, outRawQuery, contentType, body, err := s.prepareExcelCollectionFill(ctx, reportName, rawQuery); used {
		return true, method, outRawQuery, contentType, body, err
	}
	if used, method, outRawQuery, contentType, body, err := s.prepareXMLCollectionFill(ctx, reportName, rawQuery); used {
		return true, method, outRawQuery, contentType, body, err
	}
	if used, method, outRawQuery, contentType, body, err := s.prepareDuckDBJDBCFill(ctx, reportName, rawQuery); used {
		return true, method, outRawQuery, contentType, body, err
	}
	return false, "", "", "", nil, nil
}
