package server

import (
	"context"
	"io"

	"github.com/acugis/acugis-report-server-go/internal/csvfill"
)

// prepareCSVCollectionFill loads a report-owned CSV and returns an
// ExecuteReport body that includes _collectionData.
//
// When the report is not a CSV collection dataset, ok is false.
func (s *Server) prepareCSVCollectionFill(
	ctx context.Context,
	reportName, rawQuery string,
) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	_ = ctx
	if s == nil || s.deps.Reports == nil {
		return false, "", "", "", nil, nil
	}
	return csvfill.Prepare(s.deps.Reports.JRXMLDir(), s.cfg.Paths.ResourcePathsDir, reportName, rawQuery)
}
