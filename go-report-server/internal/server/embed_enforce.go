package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/embedtoken"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

// errEmbedScope is returned when an emb_ token is used against report content
// other than the content_id it was minted for.
var errEmbedScope = &httpError{Status: http.StatusForbidden, Message: "embed token is not scoped to this report"}

func embedGrant(r *http.Request) *auth.EmbedGrant {
	if r == nil {
		return nil
	}
	u := auth.FromContext(r.Context())
	if u == nil {
		return nil
	}
	return u.Embed
}

// embedParameters returns bound parameters for an embed-token request.
// Nil for sessions, PATs, anonymous callers, and embed tokens minted without parameters.
func embedParameters(r *http.Request) map[string][]string {
	g := embedGrant(r)
	if g == nil || len(g.Parameters) == 0 {
		return nil
	}
	return g.Parameters
}

// enforceEmbedContentScope limits embed-token requests to the signed report.
// Session and PAT requests are unchanged. Dashboard and view tokens cannot execute reports.
func (s *Server) enforceEmbedContentScope(r *http.Request, reportKey string) error {
	g := embedGrant(r)
	if g == nil {
		return nil
	}
	switch embedtoken.ContentType(g.ContentType) {
	case embedtoken.ContentReport, embedtoken.ContentMapReport:
		if jasper.NormalizeReportKey(g.ContentID) != jasper.NormalizeReportKey(reportKey) {
			return errEmbedScope
		}
		return nil
	default:
		return errEmbedScope
	}
}

// prepareEmbedReportQuery applies content scope and bound parameters to a
// report data query string. Callers that only need scope can ignore the
// returned query when they do not forward parameters.
func (s *Server) prepareEmbedReportQuery(r *http.Request, reportKey, rawQuery string) (string, error) {
	if err := s.enforceEmbedContentScope(r, reportKey); err != nil {
		return "", err
	}
	return applyBoundToRawQuery(rawQuery, embedParameters(r)), nil
}

func applyBoundToRawQuery(raw string, bound map[string][]string) string {
	if len(bound) == 0 {
		return raw
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		q = url.Values{}
	}
	applyBoundValues(q, bound)
	return q.Encode()
}

func applyBoundValues(q url.Values, bound map[string][]string) {
	if q == nil || len(bound) == 0 {
		return
	}
	for name, vals := range bound {
		q.Del(name)
		if len(vals) == 0 {
			q.Set(name, "")
			continue
		}
		for _, v := range vals {
			q.Add(name, v)
		}
	}
}

// applyBoundStringMap overwrites dst with bound parameter values.
// Used by report-data JSON bodies such as explorer report-query.
func applyBoundStringMap(dst map[string][]string, bound map[string][]string) map[string][]string {
	if len(bound) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[string][]string{}
	}
	for name, vals := range bound {
		dst[name] = append([]string(nil), vals...)
	}
	return dst
}

// overlayEmbedBound forces bound parameter values onto a lookup/filter map.
func overlayEmbedBound(values map[string]any, bound map[string][]string) map[string]any {
	if len(bound) == 0 {
		return values
	}
	if values == nil {
		values = map[string]any{}
	}
	for name, vals := range bound {
		if len(vals) == 1 {
			values[name] = vals[0]
			continue
		}
		values[name] = append([]string(nil), vals...)
	}
	return values
}

// rewriteEmbedExecute forces bound parameters onto the query string and, for
// POST, onto the body before those bytes are forwarded to Jasper.
// It also pins _repName and _repFormat to the report already authorized from
// the URL, so a POST body cannot switch reports.
// GET requests leave the body untouched. A POST body that is not a form or a
// JSON object is rejected when bound parameters must be enforced, so a client
// cannot hide an override in an unread body.
func rewriteEmbedExecute(r *http.Request, reportName, format string, bound map[string][]string) (rawQuery string, body io.Reader, contentType string, err error) {
	if r == nil {
		return "", nil, "", nil
	}
	if r.Method != http.MethodPost {
		q := applyBoundToRawQuery(r.URL.RawQuery, bound)
		return pinReportIdentity(q, reportName, format), nil, "", nil
	}

	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	media := ct
	if i := strings.Index(media, ";"); i >= 0 {
		media = strings.TrimSpace(media[:i])
	}
	switch {
	case media == "application/json":
		return rewriteEmbedJSONExecute(r, reportName, format, bound)
	default:
		// urlencoded, multipart, or missing Content-Type: form fields.
		if err := r.ParseForm(); err != nil {
			return "", nil, "", &httpError{Status: http.StatusBadRequest, Message: "invalid form body"}
		}
		merged := url.Values{}
		for k, vs := range r.Form {
			for _, v := range vs {
				merged.Add(k, v)
			}
		}
		applyBoundValues(merged, bound)
		// _componentId is an internal control. Drop it from embed form bodies
		// after bound parameters are applied. The component route is GET and
		// seals the path id onto the query after this rewrite.
		merged.Del("_componentId")
		encoded := pinReportIdentity(merged.Encode(), reportName, format)
		return encoded, strings.NewReader(encoded), "application/x-www-form-urlencoded", nil
	}
}

func pinReportIdentity(raw, reportName, format string) string {
	q, err := url.ParseQuery(raw)
	if err != nil {
		q = url.Values{}
	}
	if reportName != "" {
		q.Set("_repName", reportName)
		q.Del("report")
	}
	if format != "" {
		q.Set("_repFormat", format)
	}
	// Same rule as _repName: a browser or token value is not authoritative.
	// The embed-component handler seals the path id after this rewrite.
	q.Del("_componentId")
	return q.Encode()
}

func rewriteEmbedJSONExecute(r *http.Request, reportName, format string, bound map[string][]string) (string, io.Reader, string, error) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return "", nil, "", &httpError{Status: http.StatusBadRequest, Message: "invalid JSON body"}
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return "", nil, "", &httpError{Status: http.StatusBadRequest, Message: "invalid JSON body"}
	}
	forceJSONBound(obj, bound)
	delete(obj, "_componentId")
	if nested, ok := obj["parameters"].(map[string]any); ok {
		forceJSONBound(nested, bound)
		delete(nested, "_componentId")
	}
	if nested, ok := obj["params"].(map[string]any); ok {
		forceJSONBound(nested, bound)
		delete(nested, "_componentId")
	}
	if reportName != "" {
		obj["_repName"] = reportName
		delete(obj, "report")
	}
	if format != "" {
		obj["_repFormat"] = format
	}
	encoded, err := json.Marshal(obj)
	if err != nil {
		return "", nil, "", &httpError{Status: http.StatusBadRequest, Message: "invalid JSON body"}
	}
	return pinReportIdentity(applyBoundToRawQuery(r.URL.RawQuery, bound), reportName, format), bytes.NewReader(encoded), "application/json", nil
}

func forceJSONBound(obj map[string]any, bound map[string][]string) {
	if obj == nil {
		return
	}
	for name, vals := range bound {
		if len(vals) == 1 {
			obj[name] = vals[0]
			continue
		}
		arr := make([]any, len(vals))
		for i, v := range vals {
			arr[i] = v
		}
		obj[name] = arr
	}
}

func boundParameterNames(parameters map[string][]string) []string {
	if len(parameters) == 0 {
		return []string{}
	}
	names := make([]string, 0, len(parameters))
	for name := range parameters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
