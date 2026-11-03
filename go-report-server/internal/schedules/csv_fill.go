package schedules

import (
	"context"
	"io"

	"github.com/acugis/acugis-report-server-go/internal/csvfill"
)

// prepareCSVCollection loads the published CSV resource for a scheduled run
// and returns a POST body with _collectionData. It uses the same preparation
// as interactive execution.
func (r *Runner) prepareCSVCollection(ctx context.Context, reportName, datasourceName, rawQuery string) (ok bool, method, outQuery, contentType string, body io.Reader, err error) {
	_ = ctx
	_ = datasourceName
	if r == nil || r.Reports == nil {
		return false, "", "", "", nil, nil
	}
	return csvfill.Prepare(r.Reports.JRXMLDir(), r.ResourcePaths, reportName, rawQuery)
}
