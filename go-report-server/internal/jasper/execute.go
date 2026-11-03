package jasper

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var reportKeyPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// NormalizeReportKey mirrors PHP jrs_report_key.
func NormalizeReportKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	b.Grow(len(value))
	prevDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
			prevDash = r == '-' || r == '_'
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			// drop
		}
	}
	out := strings.Trim(b.String(), "-_")
	return out
}

// IsValidReportKey mirrors PHP jrs_is_valid_report_key.
func IsValidReportKey(value string) bool {
	return value != "" && reportKeyPattern.MatchString(value)
}

// IsAllowedFormat reports whether format is a supported export type.
func IsAllowedFormat(format string) bool {
	_, ok := AllowedFormats[strings.ToLower(strings.TrimSpace(format))]
	return ok
}

func (g *gateway) ExecuteReport(ctx context.Context, req ExecuteRequest) (*ProxyResponse, error) {
	if g == nil || g.client == nil || !g.client.Configured() {
		err := ErrUnavailable("Jasper engine URL is not configured", nil)
		g.recordError(err)
		return nil, err
	}

	reportName := NormalizeReportKey(req.ReportName)
	if !IsValidReportKey(reportName) {
		err := ErrBadRequest("Invalid report.")
		g.recordError(err)
		return nil, err
	}

	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "html"
	}
	if !IsAllowedFormat(format) {
		err := ErrBadRequest("Unsupported export format.")
		g.recordError(err)
		return nil, err
	}

	if g.reportExists != nil && !g.reportExists(reportName) {
		err := ErrNotFound("Report not found.")
		g.recordError(err)
		return nil, err
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	rawQuery := req.RawQuery
	// Ensure _repName / _repFormat in query are consistent when present, but do
	// not rewrite other parameters — forward the raw query as provided.
	target := buildURL(g.client.ReportURL(), rawQuery)

	var body io.Reader
	if method != http.MethodGet && method != http.MethodHead {
		body = req.Body
	}

	outReq, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		ge := wrapf(CodeInternal, "build jasper request", err)
		g.recordError(ge)
		return nil, ge
	}

	copyForwardHeaders(outReq.Header, req.Header)
	if req.ContentType != "" {
		outReq.Header.Set("Content-Type", req.ContentType)
	}
	// Set after the allow-list copy so a client cannot supply or clear it.
	if req.EmbedComponent {
		outReq.Header.Set(EmbedComponentHeader, EmbedComponentHeaderValue)
	}

	resp, err := g.client.DoStream(ctx, outReq)
	if err != nil {
		ge := Translate(err)
		g.recordError(ge)
		return nil, ge
	}

	proxy := &ProxyResponse{
		StatusCode: resp.StatusCode,
		Header:     filterResponseHeaders(resp.Header),
		Body:       resp.Body,
		BytesHint:  resp.ContentLength,
	}

	// Rewrite HTML chart image URLs to the Go development image proxy.
	ct := strings.ToLower(proxy.Header.Get("Content-Type"))
	if format == "html" && strings.Contains(ct, "text/html") && req.ImageProxyBase != "" {
		proxy.Body = newHTMLImageRewriter(resp.Body, reportName, req.ImageProxyBase)
		proxy.Header.Del("Content-Length")
		proxy.BytesHint = -1
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		g.recordSuccess(reportName, format, resp.StatusCode)
	} else {
		g.recordError(newError(CodeInternal, "jasper returned error status", nil))
		g.setLastMeta(reportName, format, resp.StatusCode)
	}

	return proxy, nil
}

// ForwardHeaders lists request headers that may be forwarded to Jasper.
var forwardRequestHeaders = map[string]struct{}{
	"Accept":          {},
	"Accept-Language": {},
	"Content-Type":    {},
	"User-Agent":      {},
}

// ForwardResponseHeaders lists response headers preserved from Jasper.
var forwardResponseHeaders = map[string]struct{}{
	"Content-Type":        {},
	"Content-Disposition": {},
	"Content-Length":      {},
	"Cache-Control":       {},
	"Pragma":              {},
	"Expires":             {},
}

func copyForwardHeaders(dst, src http.Header) {
	if src == nil {
		return
	}
	for k, vals := range src {
		canon := http.CanonicalHeaderKey(k)
		if _, ok := forwardRequestHeaders[canon]; !ok {
			continue
		}
		for _, v := range vals {
			dst.Add(canon, v)
		}
	}
}

func filterResponseHeaders(src http.Header) http.Header {
	out := make(http.Header)
	if src == nil {
		return out
	}
	for k, vals := range src {
		canon := http.CanonicalHeaderKey(k)
		if _, ok := forwardResponseHeaders[canon]; !ok {
			continue
		}
		for _, v := range vals {
			out.Add(canon, v)
		}
	}
	return out
}

// ExtractReportName pulls _repName (or report) from query values.
func ExtractReportName(q url.Values) string {
	if q == nil {
		return ""
	}
	if v := q.Get("_repName"); v != "" {
		return NormalizeReportKey(v)
	}
	if v := q.Get("report"); v != "" {
		return NormalizeReportKey(v)
	}
	return ""
}

// ExtractFormat pulls _repFormat from query values.
func ExtractFormat(q url.Values) string {
	if q == nil {
		return "html"
	}
	f := strings.ToLower(strings.TrimSpace(q.Get("_repFormat")))
	if f == "" {
		return "html"
	}
	return f
}
