package jasper

import (
	"context"
	"net/http"
)

func (g *gateway) FetchReportImage(ctx context.Context, req ImageRequest) (*ProxyResponse, error) {
	if g == nil || g.client == nil || !g.client.Configured() {
		err := ErrUnavailable("Jasper image URL is not configured", nil)
		g.recordError(err)
		return nil, err
	}

	reportName := NormalizeReportKey(req.ReportName)
	if !IsValidReportKey(reportName) {
		// Match PHP report_image.php: invalid key → 403.
		err := ErrForbidden("Invalid report.")
		g.recordError(err)
		return nil, err
	}

	target := buildURL(g.client.ImageURL(), req.RawQuery)
	outReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		ge := wrapf(CodeInternal, "build jasper image request", err)
		g.recordError(ge)
		return nil, ge
	}
	copyForwardHeaders(outReq.Header, req.Header)

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

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		g.recordSuccess(reportName, "image", resp.StatusCode)
	} else {
		g.recordError(newError(CodeInternal, "jasper image returned error status", nil))
		g.setLastMeta(reportName, "image", resp.StatusCode)
	}

	return proxy, nil
}
