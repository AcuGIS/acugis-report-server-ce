package report

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/acugis/acugis-report-go/internal/transport"
)

// DatasourcesService covers datasource list/create/delete and Jasper context generation.
//
// Note: Get and Update endpoints are not available in the current API.
// Calling Get/Update returns NotImplementedError (or maps 404/405 if a future
// server exposes those routes without documenting them yet).
type DatasourcesService struct{ c *Client }

// List returns datasource names.
func (s *DatasourcesService) List(ctx context.Context) ([]DatasourceName, error) {
	var out []DatasourceName
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/datasources",
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return out, nil
}

// Create upserts a datasource (application/x-www-form-urlencoded).
func (s *DatasourcesService) Create(ctx context.Context, req DatasourceCreateRequest) (*DatasourceMutationResponse, error) {
	form := url.Values{}
	set := func(k, v string) {
		if v != "" {
			form.Set(k, v)
		}
	}
	set("name", req.Name)
	set("db_type", req.DBType)
	set("host", req.Host)
	set("port", req.Port)
	set("dbname", req.DBName)
	set("schema_name", req.SchemaName)
	set("username", req.Username)
	set("password", req.Password)
	set("auth_type", req.AuthType)
	set("public_key_resource_uuid", req.PublicKeyResourceUUID)
	set("private_key_resource_uuid", req.PrivateKeyResourceUUID)

	var out DatasourceMutationResponse
	resp, err := s.c.tr.DoFormURLEncoded(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/datasources",
	}, form, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Delete deletes a datasource by numeric id.
func (s *DatasourcesService) Delete(ctx context.Context, id int64) (*DatasourceMutationResponse, error) {
	var out DatasourceMutationResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodDelete,
		Path:   "/api/v1/datasources/" + strconv.FormatInt(id, 10),
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// GenerateContext generates Jasper context.xml (POST /api/v1/datasources/context).
func (s *DatasourcesService) GenerateContext(ctx context.Context) (*JasperContextResponse, error) {
	var out JasperContextResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/datasources/context",
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Get is not implemented by the server yet.
func (s *DatasourcesService) Get(ctx context.Context, id int64) (*DatasourceName, error) {
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/datasources/" + strconv.FormatInt(id, 10),
		SafeGET: true,
	}, nil, &DatasourceName{})
	if err != nil {
		if _, ok := err.(*NotFoundError); ok {
			return nil, &NotImplementedError{baseError{
				status:  http.StatusNotImplemented,
				message: "datasources Get is not available yet",
				body:    bodyOf(resp),
			}}
		}
		if resp != nil && resp.StatusCode == http.StatusMethodNotAllowed {
			return nil, &NotImplementedError{baseError{
				status:  http.StatusNotImplemented,
				message: "datasources Get is not available yet",
				body:    bodyOf(resp),
			}}
		}
		return nil, decodeError(err, bodyOf(resp))
	}
	return nil, &NotImplementedError{baseError{
		status:  http.StatusNotImplemented,
		message: "datasources Get is not available yet",
	}}
}

// Update is not implemented by the server yet.
func (s *DatasourcesService) Update(ctx context.Context, id int64, req DatasourceCreateRequest) (*DatasourceMutationResponse, error) {
	_ = ctx
	_ = id
	_ = req
	return nil, &NotImplementedError{baseError{
		status:  http.StatusNotImplemented,
		message: "datasources Update is not available yet",
	}}
}
