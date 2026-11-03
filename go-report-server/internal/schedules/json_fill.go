package schedules

import (
	"context"
	"io"

	"github.com/acugis/acugis-report-server-go/internal/jsonfill"
)

// prepareJSONCollection loads a packaged JSON file or fetches a JSON URL for a
// scheduled run and returns a POST body with _collectionData. It uses the same
// preparation as interactive execution.
func (r *Runner) prepareJSONCollection(ctx context.Context, reportName, datasourceName, rawQuery string) (ok bool, method, outQuery, contentType string, body io.Reader, err error) {
	_ = datasourceName
	if r == nil || r.Reports == nil {
		return false, "", "", "", nil, nil
	}
	return jsonfill.Prepare(ctx, r.Reports.JRXMLDir(), r.ResourcePaths, reportName, rawQuery)
}
