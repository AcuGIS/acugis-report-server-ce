package server

import (
	"context"
	"io"

	"github.com/acugis/acugis-report-server-go/internal/jsonfill"
)

// prepareJSONCollectionFill loads a packaged JSON file or fetches a JSON URL
// and returns an ExecuteReport body that includes _collectionData.
// Interactive execution uses the same preparation as scheduled execution.
//
// When the report is not a JSON collection dataset, ok is false.
func (s *Server) prepareJSONCollectionFill(
	ctx context.Context,
	reportName, rawQuery string,
) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	if s == nil || s.deps.Reports == nil {
		return false, "", "", "", nil, nil
	}
	return jsonfill.Prepare(ctx, s.deps.Reports.JRXMLDir(), s.cfg.Paths.ResourcePathsDir, reportName, rawQuery)
}
