package excelfill

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/collectionfile"
	"github.com/acugis/acugis-report-server-go/excelfile"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/resourcefs"
)

// Prepare reads the packaged .xlsx workbook and returns a collection-fill body.
// ok is false when the JRXML is not an Excel report, so the caller can try the
// next provider. An Excel report that cannot be prepared returns ok true and an
// error so it does not fall through to JDBC.
func Prepare(jrxmlDir, resourcePathsDir, reportName, rawQuery string) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	jrxmlPath := filepath.Join(strings.TrimSpace(jrxmlDir), reportName+".jrxml")
	info := reports.LoadCollectionQueryInfoFromJRXMLFile(jrxmlPath)
	if !info.IsExcelCollection() {
		return false, "", "", "", nil, nil
	}
	logical := strings.TrimSpace(info.Resource)
	if logical == "" {
		return true, "", "", "", nil, fmt.Errorf("Excel report has no resource file")
	}
	store := resourcefs.NewStore(jrxmlDir, resourcePathsDir)
	f, stat, oerr := store.Open(reportName, logical)
	if oerr != nil {
		if errors.Is(oerr, resourcefs.ErrInvalidPath) || errors.Is(oerr, resourcefs.ErrInvalidKey) {
			return true, "", "", "", nil, fmt.Errorf("invalid resource path")
		}
		return true, "", "", "", nil, fmt.Errorf("Resource unavailable: %s", logical)
	}
	defer f.Close()
	wb, err := excelfile.Open(f, stat.Size)
	if err != nil {
		return true, "", "", "", nil, err
	}
	defer wb.Close()
	sheets, err := wb.Sheets()
	if err != nil {
		return true, "", "", "", nil, err
	}
	selected, err := excelfile.Select(sheets, info.ExcelSheet, info.Query)
	if err != nil {
		return true, "", "", "", nil, err
	}
	table, err := wb.Table(selected)
	if err != nil {
		return true, "", "", "", nil, err
	}
	sql := strings.TrimSpace(info.Query)
	if sql == "" {
		sql = `SELECT * FROM "Sheet1"`
	}
	params := jasper.ReportFillParams(rawQuery)
	queried, qerr := collectionfile.Query(table, sql, params, 0)
	if qerr != nil {
		return true, "", "", "", nil, rewriteQueryError(qerr)
	}
	rows := queried.Rows
	if rows == nil {
		rows = []map[string]any{}
	}
	vals, _ := url.ParseQuery(rawQuery)
	vals.Del("_dataSource")
	method, outRawQuery, contentType, body, aerr := jasper.AttachCollectionData(vals.Encode(), rows)
	if aerr != nil {
		return true, "", "", "", nil, aerr
	}
	return true, method, outRawQuery, contentType, body, nil
}

func rewriteQueryError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ReplaceAll(err.Error(), "CSV queries", "Excel queries")
	msg = strings.ReplaceAll(msg, "CSV query", "Excel query")
	msg = strings.ReplaceAll(msg, "CSV file has no header row", "worksheet has no tabular data")
	if msg == err.Error() {
		return err
	}
	return fmt.Errorf("%s", msg)
}
