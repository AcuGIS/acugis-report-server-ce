package schedules

import (
	"context"
	"io"

	"github.com/acugis/acugis-report-server-go/internal/xmlfill"
)

// prepareXMLCollection loads the published XML document for a scheduled run
// and returns a POST body with _collectionData. It uses the same preparation
// as interactive execution.
func (r *Runner) prepareXMLCollection(ctx context.Context, reportName, datasourceName, rawQuery string) (ok bool, method, outQuery, contentType string, body io.Reader, err error) {
	_ = ctx
	_ = datasourceName
	if r == nil || r.Reports == nil {
		return false, "", "", "", nil, nil
	}
	return xmlfill.Prepare(r.Reports.JRXMLDir(), r.ResourcePaths, reportName, rawQuery)
}
