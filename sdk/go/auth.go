package report

import (
	"context"
	"fmt"
	"net/http"

	"github.com/acugis/acugis-report-go/internal/transport"
)

// AuthService covers /api/v1/auth/* (Bearer token APIs).
type AuthService struct{ c *Client }

// Me returns the current authenticated user.
func (s *AuthService) Me(ctx context.Context) (*User, error) {
	var out User
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/auth/me",
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// ListTokens returns personal access token metadata.
func (s *AuthService) ListTokens(ctx context.Context) ([]TokenMetadata, error) {
	var wrap struct {
		Tokens []TokenMetadata `json:"tokens"`
	}
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/auth/tokens",
		SafeGET: true,
	}, nil, &wrap)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return wrap.Tokens, nil
}

// CreateToken creates a personal access token.
func (s *AuthService) CreateToken(ctx context.Context, req CreateTokenRequest) (*Token, error) {
	if req.Name == "" {
		return nil, &ValidationError{baseError{message: "name is required"}}
	}
	var out Token
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/auth/tokens",
	}, req, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// UpdateToken updates token metadata.
func (s *AuthService) UpdateToken(ctx context.Context, id int64, req UpdateTokenRequest) (*TokenMetadata, error) {
	var out TokenMetadata
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPut,
		Path:   fmt.Sprintf("/api/v1/auth/tokens/%d", id),
	}, req, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// DeleteToken revokes/deletes a token.
func (s *AuthService) DeleteToken(ctx context.Context, id int64) error {
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodDelete,
		Path:   fmt.Sprintf("/api/v1/auth/tokens/%d", id),
	}, nil, nil)
	if err != nil {
		return decodeError(err, bodyOf(resp))
	}
	return nil
}

// RotateToken rotates a token secret.
func (s *AuthService) RotateToken(ctx context.Context, id int64) (*Token, error) {
	var out Token
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("/api/v1/auth/tokens/%d/rotate", id),
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

func bodyOf(resp *transport.Response) []byte {
	if resp == nil {
		return nil
	}
	return resp.Body
}
