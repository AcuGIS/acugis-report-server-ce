package jasper

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

// AttachDuckDBSession builds a POST body that includes query parameters plus
// _duckdbSession (JSON) so Tomcat opens DuckDB JDBC and fills via REPORT_CONNECTION.
func AttachDuckDBSession(rawQuery string, session map[string]any) (method, outRawQuery, contentType string, body io.Reader, err error) {
	vals, err := url.ParseQuery(rawQuery)
	if err != nil {
		vals = url.Values{}
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return "", "", "", nil, err
	}
	vals.Del("_duckdbSession")
	vals.Del("_collectionData")

	if len(payload) <= formURLEncodedMax {
		vals.Set("_duckdbSession", string(payload))
		encoded := vals.Encode()
		return http.MethodPost, "", "application/x-www-form-urlencoded", strings.NewReader(encoded), nil
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for key, list := range vals {
		for _, v := range list {
			if err := w.WriteField(key, v); err != nil {
				_ = w.Close()
				return "", "", "", nil, err
			}
		}
	}
	if err := w.WriteField("_duckdbSession", string(payload)); err != nil {
		_ = w.Close()
		return "", "", "", nil, err
	}
	if err := w.Close(); err != nil {
		return "", "", "", nil, err
	}
	return http.MethodPost, "", w.FormDataContentType(), &buf, nil
}
