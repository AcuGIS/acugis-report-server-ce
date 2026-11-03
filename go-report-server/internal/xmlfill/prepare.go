package xmlfill

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/collectionfile"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/resourcefs"
	"github.com/acugis/acugis-report-server-go/xmlfile"
)

// Prepare reads the packaged XML document and returns a collection-fill body.
// ok is false when the JRXML is not an XML report, so the caller can try the
// next provider. An XML report that cannot be prepared returns ok true and an
// error so it does not fall through to JDBC.
func Prepare(jrxmlDir, resourcePathsDir, reportName, rawQuery string) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	jrxmlPath := filepath.Join(strings.TrimSpace(jrxmlDir), reportName+".jrxml")
	info := reports.LoadCollectionQueryInfoFromJRXMLFile(jrxmlPath)
	if !info.IsXMLCollection() {
		return false, "", "", "", nil, nil
	}
	logical := strings.TrimSpace(info.Resource)
	if logical == "" {
		return true, "", "", "", nil, fmt.Errorf("XML report has no resource file")
	}
	store := resourcefs.NewStore(jrxmlDir, resourcePathsDir)
	f, _, oerr := store.Open(reportName, logical)
	if oerr != nil {
		if errors.Is(oerr, resourcefs.ErrInvalidPath) || errors.Is(oerr, resourcefs.ErrInvalidKey) {
			return true, "", "", "", nil, fmt.Errorf("invalid resource path")
		}
		return true, "", "", "", nil, fmt.Errorf("Resource unavailable: %s", logical)
	}
	defer f.Close()
	doc, err := xmlfile.Parse(f)
	if err != nil {
		return true, "", "", "", nil, err
	}
	selected, err := doc.Select(info.XMLCollection, info.Query)
	if err != nil {
		return true, "", "", "", nil, err
	}
	table, err := doc.Table(selected)
	if err != nil {
		return true, "", "", "", nil, err
	}
	sql := strings.TrimSpace(info.Query)
	if sql == "" {
		sql = `SELECT * FROM "data".` + quoteIdent(selected)
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

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func rewriteQueryError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ReplaceAll(err.Error(), "CSV queries", "XML queries")
	msg = strings.ReplaceAll(msg, "CSV query", "XML query")
	msg = strings.ReplaceAll(msg, "CSV file has no header row", "XML document has no records")
	if msg == err.Error() {
		return err
	}
	return fmt.Errorf("%s", msg)
}
