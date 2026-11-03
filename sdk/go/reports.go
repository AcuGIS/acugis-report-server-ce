package report

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/acugis/acugis-report-go/internal/transport"
)

// ReportsService covers report catalog and execution.
type ReportsService struct{ c *Client }

// PublishOptions configures multipart report publish/update.
type PublishOptions struct {
	JRXMLFile         io.Reader
	JRXMLFileName     string
	JasperFile        io.Reader
	JasperFileName    string
	ThumbnailFile     io.Reader
	ThumbnailFileName string
	DataSource        string
	ReportName        string
	ReportDescription string
	ReportType        string
	AccessGroups      []string
	// ExtraFields are additional form fields (e.g. params[0][name]).
	ExtraFields map[string]string
}

// RunOptions configures /reports/execute.
type RunOptions struct {
	Format     string
	Datasource string
	// Params are forwarded as query parameters (scalars or []string collections).
	Params map[string]any
}

// List returns ACL-filtered report summaries.
func (s *ReportsService) List(ctx context.Context) (*ReportListResponse, error) {
	var out ReportListResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/reports",
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Get returns a full report definition.
func (s *ReportsService) Get(ctx context.Context, key string) (*Report, error) {
	var out Report
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/reports/" + key,
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Publish creates a report via multipart upload.
func (s *ReportsService) Publish(ctx context.Context, opts PublishOptions) (*ReportCreateResponse, error) {
	fields, files, multi := publishParts(opts)
	var out ReportCreateResponse
	resp, err := s.c.tr.DoMultipart(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/reports",
	}, fields, files, multi, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Update updates a report via multipart upload.
func (s *ReportsService) Update(ctx context.Context, key string, opts PublishOptions) (*ReportMutationResponse, error) {
	fields, files, multi := publishParts(opts)
	var out ReportMutationResponse
	resp, err := s.c.tr.DoMultipart(ctx, transport.Request{
		Method: http.MethodPut,
		Path:   "/api/v1/reports/" + key,
	}, fields, files, multi, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Delete removes a report.
func (s *ReportsService) Delete(ctx context.Context, key string) (*ReportMutationResponse, error) {
	var out ReportMutationResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodDelete,
		Path:   "/api/v1/reports/" + key,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// GetConfig returns viewer configuration.
func (s *ReportsService) GetConfig(ctx context.Context, key string) (*ReportConfig, error) {
	var out ReportConfig
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/reports/" + key + "/config",
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// GetThumbnail downloads the report PNG thumbnail.
func (s *ReportsService) GetThumbnail(ctx context.Context, key string) (*BinaryResult, error) {
	resp, err := s.c.tr.DoBinary(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/reports/" + key + "/thumbnail",
		SafeGET: true,
	})
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &BinaryResult{Data: resp.Body, ContentType: resp.ContentType, Header: resp.Header}, nil
}

// LookupParameter looks up cascading parameter options (GET).
func (s *ReportsService) LookupParameter(ctx context.Context, key, name string, parentParams map[string]any) (*ParameterLookupResponse, error) {
	var out ParameterLookupResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf("/api/v1/reports/%s/parameters/%s/lookup", key, name),
		Query:   transport.EncodeQuery(parentParams),
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Run executes a report and returns bytes + content-type.
func (s *ReportsService) Run(ctx context.Context, key string, opts *RunOptions) (*BinaryResult, error) {
	q := runQuery(key, opts)
	resp, err := s.c.tr.DoBinary(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/reports/execute",
		Query:   q,
		SafeGET: true,
	})
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &BinaryResult{Data: resp.Body, ContentType: resp.ContentType, Header: resp.Header}, nil
}

// RunToFile executes a report and writes it to path.
func (s *ReportsService) RunToFile(ctx context.Context, key, path string, opts *RunOptions) (*BinaryResult, error) {
	res, err := s.Run(ctx, key, opts)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, res.Data, 0o644); err != nil {
		return res, wrapTransportError(err)
	}
	return res, nil
}

// RunStream executes a report and copies the body to w.
func (s *ReportsService) RunStream(ctx context.Context, key string, w io.Writer, opts *RunOptions) (contentType string, err error) {
	q := runQuery(key, opts)
	resp, err := s.c.tr.DoStream(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/reports/execute",
		Query:   q,
		SafeGET: true,
	})
	if err != nil {
		return "", decodeError(err, nil)
	}
	defer resp.StreamBody.Close()
	if _, err := io.Copy(w, resp.StreamBody); err != nil {
		return resp.ContentType, wrapTransportError(err)
	}
	return resp.ContentType, nil
}

func runQuery(key string, opts *RunOptions) map[string][]string {
	params := map[string]any{"_repName": key}
	if opts != nil {
		if opts.Format != "" {
			params["_repFormat"] = opts.Format
		}
		if opts.Datasource != "" {
			params["datasource"] = opts.Datasource
		}
		for k, v := range opts.Params {
			params[k] = v
		}
	}
	return transport.EncodeQuery(params)
}

func publishParts(opts PublishOptions) (map[string]string, []transport.MultipartFile, map[string][]string) {
	fields := map[string]string{}
	if opts.DataSource != "" {
		fields["data_source"] = opts.DataSource
	}
	if opts.ReportName != "" {
		fields["report_name"] = opts.ReportName
	}
	if opts.ReportDescription != "" {
		fields["report_description"] = opts.ReportDescription
	}
	if opts.ReportType != "" {
		fields["report_type"] = opts.ReportType
	}
	for k, v := range opts.ExtraFields {
		fields[k] = v
	}
	multi := map[string][]string{}
	if len(opts.AccessGroups) > 0 {
		multi["access_groups"] = opts.AccessGroups
	}
	var files []transport.MultipartFile
	if opts.JRXMLFile != nil {
		name := opts.JRXMLFileName
		if name == "" {
			name = "report.jrxml"
		}
		files = append(files, transport.MultipartFile{FieldName: "jrxml_file", FileName: name, Reader: opts.JRXMLFile})
	}
	if opts.JasperFile != nil {
		name := opts.JasperFileName
		if name == "" {
			name = "report.jasper"
		}
		files = append(files, transport.MultipartFile{FieldName: "jasper_file", FileName: name, Reader: opts.JasperFile})
	}
	if opts.ThumbnailFile != nil {
		name := opts.ThumbnailFileName
		if name == "" {
			name = "thumbnail.png"
		}
		files = append(files, transport.MultipartFile{FieldName: "thumbnail_file", FileName: name, Reader: opts.ThumbnailFile})
	}
	return fields, files, multi
}
