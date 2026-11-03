package jsonfill

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/collectionfile"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/jsonurl"
	"github.com/acugis/acugis-report-server-go/resourcefs"
)

// fetchJSON is the document GET used for URL mode. Tests in this package
// replace it. Production uses the SSRF-checked client.
var fetchJSON = jsonurl.Get

// Prepare reads a packaged JSON file or fetches a JSON URL and returns a
// collection-fill body. ok is false when the JRXML is not a JSON report.
// A JSON report that cannot be prepared returns ok true and an error so it
// does not fall through to JDBC.
func Prepare(ctx context.Context, jrxmlDir, resourcePathsDir, reportName, rawQuery string) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	jrxmlPath := filepath.Join(strings.TrimSpace(jrxmlDir), reportName+".jrxml")
	info := reports.LoadCollectionQueryInfoFromJRXMLFile(jrxmlPath)
	if !info.IsJSONCollection() {
		return false, "", "", "", nil, nil
	}
	doc, err := loadDocument(ctx, jrxmlDir, resourcePathsDir, reportName, info)
	if err != nil {
		return true, "", "", "", nil, err
	}
	selected, err := doc.Select(info.JSONCollection, info.Query)
	if err != nil {
		return true, "", "", "", nil, err
	}
	table, err := doc.Table(selected)
	if err != nil {
		return true, "", "", "", nil, err
	}
	sql := strings.TrimSpace(info.Query)
	if sql == "" {
		sql = "SELECT * FROM json"
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

func loadDocument(ctx context.Context, jrxmlDir, resourcePathsDir, reportName string, info reports.CollectionQueryInfo) (*collectionfile.Document, error) {
	if info.JSONURLMode() {
		raw := strings.TrimSpace(info.JSONURL)
		if raw == "" {
			return nil, fmt.Errorf("JSON URL is required")
		}
		payload, err := fetchJSON(ctx, raw)
		if err != nil {
			return nil, err
		}
		return collectionfile.ParseJSON(bytes.NewReader(payload))
	}
	logical := strings.TrimSpace(info.Resource)
	if logical == "" {
		return nil, fmt.Errorf("JSON report has no resource file")
	}
	store := resourcefs.NewStore(jrxmlDir, resourcePathsDir)
	f, _, oerr := store.Open(reportName, logical)
	if oerr != nil {
		if errors.Is(oerr, resourcefs.ErrInvalidPath) || errors.Is(oerr, resourcefs.ErrInvalidKey) {
			return nil, fmt.Errorf("invalid resource path")
		}
		return nil, fmt.Errorf("Resource unavailable: %s", logical)
	}
	defer f.Close()
	return collectionfile.ParseJSON(f)
}

func rewriteQueryError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ReplaceAll(err.Error(), "CSV queries", "JSON queries")
	msg = strings.ReplaceAll(msg, "CSV query", "JSON query")
	msg = strings.ReplaceAll(msg, "CSV file has no header row", "JSON document has no records")
	if msg == err.Error() {
		return err
	}
	return fmt.Errorf("%s", msg)
}
