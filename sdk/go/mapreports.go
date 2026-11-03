package report

import (
	"context"
	"fmt"
	"net/http"

	"github.com/acugis/acugis-report-go/internal/transport"
)

// MapReportsService covers map report features, extent, columns, and spatial runs.
// Those routes are implemented by Report Server Professional. The Open Source
// server does not register them.
type MapReportsService struct{ c *Client }

// ExtentOptions configures GET /api/v1/map/{key}/extent.
type ExtentOptions struct {
	SRID     int
	Geometry string // GeoJSON geometry string
	XMin     *float64
	YMin     *float64
	XMax     *float64
	YMax     *float64
}

// GetFeatures returns a GeoJSON FeatureCollection. Extra params are forwarded as query params.
func (s *MapReportsService) GetFeatures(ctx context.Context, key string, params map[string]any) (*FeatureCollection, error) {
	var out FeatureCollection
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf("/api/v1/map/%s/features", key),
		Query:   transport.EncodeQuery(params),
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// BuildExtent computes map extent parameters (p_geometry / p_srid).
func (s *MapReportsService) BuildExtent(ctx context.Context, key string, opts ExtentOptions) (*MapExtent, error) {
	params := map[string]any{"srid": opts.SRID}
	if opts.Geometry != "" {
		params["geometry"] = opts.Geometry
	}
	if opts.XMin != nil {
		params["xmin"] = *opts.XMin
	}
	if opts.YMin != nil {
		params["ymin"] = *opts.YMin
	}
	if opts.XMax != nil {
		params["xmax"] = *opts.XMax
	}
	if opts.YMax != nil {
		params["ymax"] = *opts.YMax
	}
	var out MapExtent
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf("/api/v1/map/%s/extent", key),
		Query:   transport.EncodeQuery(params),
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// GetColumns returns map layer column metadata.
func (s *MapReportsService) GetColumns(ctx context.Context, key string) (*MapColumnsResult, error) {
	var out MapColumnsResult
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf("/api/v1/map/%s/columns", key),
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Run wraps Reports.Run, injecting p_geometry and p_srid.
func (s *MapReportsService) Run(ctx context.Context, key string, geometry string, srid int, opts *RunOptions) (*BinaryResult, error) {
	runOpts := &RunOptions{}
	if opts != nil {
		*runOpts = *opts
	}
	if runOpts.Params == nil {
		runOpts.Params = map[string]any{}
	}
	runOpts.Params["p_geometry"] = geometry
	runOpts.Params["p_srid"] = srid
	return s.c.Reports.Run(ctx, key, runOpts)
}
