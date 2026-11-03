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

// formURLEncodedMax is the max collection JSON size for application/x-www-form-urlencoded.
// Larger payloads use multipart/form-data.
const formURLEncodedMax = 48 << 10 // 48 KiB

// AttachCollectionData builds a POST body that includes query parameters plus
// _collectionData (JSON array of row objects) for Tomcat collection fill.
// Returns method, rawQuery (usually empty — params are in the body), content type, and body.
func AttachCollectionData(rawQuery string, rows []map[string]any) (method, outRawQuery, contentType string, body io.Reader, err error) {
	vals, err := url.ParseQuery(rawQuery)
	if err != nil {
		vals = url.Values{}
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return "", "", "", nil, err
	}
	vals.Del("_collectionData")

	if len(payload) <= formURLEncodedMax {
		vals.Set("_collectionData", string(payload))
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
	if err := w.WriteField("_collectionData", string(payload)); err != nil {
		_ = w.Close()
		return "", "", "", nil, err
	}
	if err := w.Close(); err != nil {
		return "", "", "", nil, err
	}
	return http.MethodPost, "", w.FormDataContentType(), &buf, nil
}

// ReportFillParams extracts report parameters from a query string, excluding _rep* / system keys.
func ReportFillParams(rawQuery string) map[string]any {
	vals, err := url.ParseQuery(rawQuery)
	if err != nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(vals))
	for k, list := range vals {
		if strings.HasPrefix(k, "_rep") || k == "_dataSource" || k == "_outFilename" ||
			k == "_collectionData" || k == "_duckdbSession" || k == "_acugisResourceBase" {
			continue
		}
		if len(list) == 0 {
			continue
		}
		if len(list) == 1 {
			out[k] = list[0]
		} else {
			cp := make([]string, len(list))
			copy(cp, list)
			out[k] = cp
		}
	}
	return out
}
