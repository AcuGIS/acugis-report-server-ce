package schedules

import (
	"context"
	"io"

	"github.com/acugis/acugis-report-server-go/internal/excelfill"
)

// prepareExcelCollection loads the published Excel workbook for a scheduled run
// and returns a POST body with _collectionData. It uses the same preparation
// as interactive execution.
func (r *Runner) prepareExcelCollection(ctx context.Context, reportName, datasourceName, rawQuery string) (ok bool, method, outQuery, contentType string, body io.Reader, err error) {
	_ = ctx
	_ = datasourceName
	if r == nil || r.Reports == nil {
		return false, "", "", "", nil, nil
	}
	return excelfill.Prepare(r.Reports.JRXMLDir(), r.ResourcePaths, reportName, rawQuery)
}
