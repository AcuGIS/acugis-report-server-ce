package server

import (
	"context"
	"io"

	"github.com/acugis/acugis-report-server-go/internal/xmlfill"
)

// prepareXMLCollectionFill loads a packaged XML document and returns an
// ExecuteReport body that includes _collectionData.
// Interactive execution uses the same preparation as scheduled execution.
//
// When the report is not an XML collection dataset, ok is false.
func (s *Server) prepareXMLCollectionFill(
	ctx context.Context,
	reportName, rawQuery string,
) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	_ = ctx
	if s == nil || s.deps.Reports == nil {
		return false, "", "", "", nil, nil
	}
	return xmlfill.Prepare(s.deps.Reports.JRXMLDir(), s.cfg.Paths.ResourcePathsDir, reportName, rawQuery)
}
