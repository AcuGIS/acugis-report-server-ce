package csvfill

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
)

// Prepare reads the report-owned CSV and returns a collection-fill body.
// ok is false when the JRXML is not a CSV report, so the caller can try the
// next provider. A CSV report that cannot be prepared returns ok true and an
// error so it does not fall through to JDBC.
func Prepare(jrxmlDir, resourcePathsDir, reportName, rawQuery string) (ok bool, method, outRawQuery, contentType string, body io.Reader, err error) {
	jrxmlPath := filepath.Join(strings.TrimSpace(jrxmlDir), reportName+".jrxml")
	info := reports.LoadCollectionQueryInfoFromJRXMLFile(jrxmlPath)
	if !info.IsCSVCollection() {
		return false, "", "", "", nil, nil
	}
	logical := strings.TrimSpace(info.Resource)
	if logical == "" {
		return true, "", "", "", nil, fmt.Errorf("CSV report has no resource file")
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

	opt, oerr := optionsFor(info.Delimiter, logical)
	if oerr != nil {
		return true, "", "", "", nil, oerr
	}
	table, perr := collectionfile.ParseCSV(f, opt)
	if perr != nil {
		return true, "", "", "", nil, perr
	}
	sql := strings.TrimSpace(info.Query)
	if sql == "" {
		sql = "SELECT * FROM csv"
	}
	params := jasper.ReportFillParams(rawQuery)
	queried, qerr := collectionfile.Query(table, sql, params, 0)
	if qerr != nil {
		return true, "", "", "", nil, qerr
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

func optionsFor(name, logical string) (collectionfile.Options, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		switch strings.ToLower(filepath.Ext(logical)) {
		case ".tsv":
			name = "tab"
		case ".txt":
			name = "auto"
		default:
			name = "comma"
		}
	}
	delim, auto, err := collectionfile.ParseDelimiter(name)
	if err != nil {
		return collectionfile.Options{}, err
	}
	return collectionfile.Options{Delimiter: delim, Auto: auto}, nil
}
