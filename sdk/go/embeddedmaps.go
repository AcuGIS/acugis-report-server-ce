package report

import (
	"context"
	"net/http"

	"github.com/acugis/acugis-report-go/internal/transport"
)

// EmbeddedMapsService covers embedded map diagnostics.
type EmbeddedMapsService struct{ c *Client }

// DebugOptions configures GET /api/v1/embedded-map/debug.
type DebugOptions struct {
	Format  string // e.g. "json"
	Compile string // any value triggers compile attempt
}

// Debug returns embedded map diagnostics for a report.
func (s *EmbeddedMapsService) Debug(ctx context.Context, reportKey string, opts *DebugOptions) (JSONObject, error) {
	params := map[string]any{"report": reportKey}
	if opts != nil {
		if opts.Format != "" {
			params["format"] = opts.Format
		}
		if opts.Compile != "" {
			params["compile"] = opts.Compile
		}
	}
	var out JSONObject
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/embedded-map/debug",
		Query:   transport.EncodeQuery(params),
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return out, nil
}
