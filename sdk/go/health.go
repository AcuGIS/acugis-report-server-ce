package report

import (
	"context"
	"net/http"

	"github.com/acugis/acugis-report-go/internal/transport"
)

// HealthService covers liveness, readiness, diagnostics, and Jasper status.
type HealthService struct{ c *Client }

// Check returns process liveness (GET /health).
func (s *HealthService) Check(ctx context.Context) (*HealthResponse, error) {
	var out HealthResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/health",
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Ready returns readiness (GET /ready). A 503 is mapped to ServerError but the
// body may still decode as ReadyResponse via errors — callers should check err.
func (s *HealthService) Ready(ctx context.Context) (*ReadyResponse, error) {
	var out ReadyResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/ready",
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		// Still return decoded body when present (not_ready).
		if resp != nil && len(resp.Body) > 0 && out.Status != "" {
			return &out, decodeError(err, bodyOf(resp))
		}
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Diagnostics returns authenticated diagnostics (GET /api/v1/diagnostics).
func (s *HealthService) Diagnostics(ctx context.Context) (*Diagnostics, error) {
	var out Diagnostics
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/diagnostics",
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// JasperStatus returns Jasper gateway status (GET /dev/jasper-status).
func (s *HealthService) JasperStatus(ctx context.Context) (JSONObject, error) {
	var out JSONObject
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/dev/jasper-status",
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return out, nil
}
