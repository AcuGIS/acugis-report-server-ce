package jasper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ReportQueryRequest asks Tomcat to execute a report's main dataset query.
type ReportQueryRequest struct {
	ReportName string
	Datasource string
	MaxRows    int
	// RawQuery is the full query string forwarded to /reportQuery (includes
	// _repName, _dataSource, report parameters). When empty, it is built from
	// ReportName / Datasource / MaxRows / Params.
	RawQuery string
	Params   url.Values
	Header   http.Header
}

// ReportQueryColumn is one column from the JDBC result set.
type ReportQueryColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	JavaType   string `json:"java_type"`
	Nullable   bool   `json:"nullable"`
	IsGeometry bool   `json:"is_geometry"`
	Ordinal    int    `json:"ordinal"`
}

// ReportQueryResult is the JSON payload returned by /reportQuery.
type ReportQueryResult struct {
	Report          string              `json:"report"`
	Datasource      string              `json:"datasource"`
	Engine          string              `json:"engine"`
	Columns         []ReportQueryColumn `json:"columns"`
	Rows            []map[string]any    `json:"rows"`
	TotalRows       int                 `json:"total_rows"`
	Truncated       bool                `json:"truncated"`
	GeometryColumn  string              `json:"geometry_column,omitempty"`
	SpatialCapable  bool                `json:"spatial_capable"`
	Error           string              `json:"error,omitempty"`
	ExceptionMsg    string              `json:"exceptionMessage,omitempty"`
}

// QueryURL derives the report-query endpoint from the configured report URL.
func (c *Client) QueryURL() string {
	if c == nil {
		return ""
	}
	base := strings.TrimRight(c.reportURL, "/")
	if strings.HasSuffix(strings.ToLower(base), "/report") {
		return base[:len(base)-len("/report")] + "/reportQuery"
	}
	return base + "/reportQuery"
}

// ExecuteReportQuery runs the report main query via Tomcat and returns tabular JSON.
func (g *gateway) ExecuteReportQuery(ctx context.Context, req ReportQueryRequest) (*ReportQueryResult, error) {
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
	ds := strings.TrimSpace(req.Datasource)
	if ds == "" {
		err := ErrBadRequest("Datasource is required.")
		g.recordError(err)
		return nil, err
	}

	if g.reportExists != nil && !g.reportExists(reportName) {
		err := ErrNotFound("Report not found.")
		g.recordError(err)
		return nil, err
	}

	rawQuery := strings.TrimSpace(req.RawQuery)
	if rawQuery == "" {
		q := url.Values{}
		if req.Params != nil {
			for k, vals := range req.Params {
				for _, v := range vals {
					q.Add(k, v)
				}
			}
		}
		q.Set("_repName", reportName)
		q.Set("_dataSource", ds)
		if req.MaxRows > 0 {
			q.Set("_maxRows", strconv.Itoa(req.MaxRows))
		}
		rawQuery = q.Encode()
	}

	target := buildURL(g.client.QueryURL(), rawQuery)
	outReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		ge := wrapf(CodeInternal, "build jasper report-query request", err)
		g.recordError(ge)
		return nil, ge
	}
	copyForwardHeaders(outReq.Header, req.Header)
	outReq.Header.Set("Accept", "application/json")

	resp, err := g.client.DoStream(ctx, outReq)
	if err != nil {
		ge := Translate(err)
		g.recordError(ge)
		return nil, ge
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		ge := wrapf(CodeInternal, "read jasper report-query response", err)
		g.recordError(ge)
		return nil, ge
	}

	var result ReportQueryResult
	if err := json.Unmarshal(body, &result); err != nil {
		ge := wrapf(CodeInternal, fmt.Sprintf("decode jasper report-query JSON (HTTP %d)", resp.StatusCode), err)
		g.recordError(ge)
		return nil, ge
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := result.Error
		if msg == "" {
			msg = result.ExceptionMsg
		}
		if msg == "" {
			msg = fmt.Sprintf("jasper report-query returned HTTP %d", resp.StatusCode)
		}
		ge := newError(CodeInternal, msg, nil)
		g.recordError(ge)
		g.setLastMeta(reportName, "query", resp.StatusCode)
		return nil, ge
	}

	g.recordSuccess(reportName, "query", resp.StatusCode)
	return &result, nil
}
