package jasper

import (
	"io"
	"net/http"
	"net/url"
	"time"
)

// Allowed export formats matching reports/execute_report.php.
var AllowedFormats = map[string]struct{}{
	"html": {},
	"pdf":  {},
	"xlsx": {},
	"xls":  {},
	"csv":  {},
	"docx": {},
	"rtf":  {},
	"odt":  {},
	"ods":  {},
	"pptx": {},
	"xml":  {},
}

// ExecuteRequest is an outbound report execution request.
type ExecuteRequest struct {
	Method      string
	ReportName  string
	Format      string
	RawQuery    string
	Header      http.Header
	Body        io.Reader
	ContentType string
	// ImageProxyBase is the public base path used when rewriting HTML image URLs
	// (for example "/dev/report-image"). Empty skips rewriting.
	ImageProxyBase string
	// EmbedComponent asks Tomcat to export one filled component. ExecuteReport
	// sets X-Acugis-Embed-Component only when this is true. Client headers are
	// not copied onto that name, and ordinary /reports/execute leaves it false.
	EmbedComponent bool
}

const (
	// EmbedComponentHeader arms Tomcat component export. It is not a
	// forwarded client header. The value must match EmbedComponentExporter.HEADER.
	EmbedComponentHeader = "X-Acugis-Embed-Component"
	// EmbedComponentHeaderValue is the only marker Tomcat accepts.
	EmbedComponentHeaderValue = "1"
)

// ImageRequest is an outbound chart/image fetch.
type ImageRequest struct {
	ReportName string
	RawQuery   string
	Header     http.Header
}

// ProxyResponse is a streaming Jasper response. Callers must Close Body.
type ProxyResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
	// BytesHint is Content-Length when known; -1 if unknown.
	BytesHint int64
}

// Close closes the response body if present.
func (r *ProxyResponse) Close() error {
	if r == nil || r.Body == nil {
		return nil
	}
	return r.Body.Close()
}

// Status describes gateway configuration and recent activity for diagnostics.
type Status struct {
	Configured       bool       `json:"configured"`
	ReportURL        string     `json:"report_url"`
	ImageURL         string     `json:"image_url"`
	Timeout          string     `json:"timeout"`
	Connectivity     string     `json:"connectivity"`
	LastSuccessfulAt *time.Time `json:"last_successful_at,omitempty"`
	LastErrorAt      *time.Time `json:"last_error_at,omitempty"`
	LastErrorSummary string     `json:"last_error_summary,omitempty"`
	LastReportName   string     `json:"last_report_name,omitempty"`
	LastFormat       string     `json:"last_format,omitempty"`
	LastStatusCode   int        `json:"last_status_code,omitempty"`
}

// QueryValues returns the parsed query without mutating the original raw string.
func QueryValues(rawQuery string) (url.Values, error) {
	return url.ParseQuery(rawQuery)
}
