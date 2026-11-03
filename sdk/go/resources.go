package report

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/acugis/acugis-report-go/internal/transport"
)

// ResourcesService covers content resources, images, and sync.
type ResourcesService struct{ c *Client }

// ListResourcesOptions filters resource listings.
type ListResourcesOptions struct {
	FolderID     *int64
	ResourceType string
	Q            string
}

// UploadResourceOptions uploads a resource via multipart.
type UploadResourceOptions struct {
	DisplayName      string
	ResourceType     string
	FolderID         *int64
	MimeType         string
	Extension        string
	OriginalFilename string
	Content          string // text content when File is nil
	File             io.Reader
	FileName         string
}

// ReplaceContentOptions replaces resource file bytes.
type ReplaceContentOptions struct {
	File      io.Reader
	FileName  string
	Extension string
	MimeType  string
}

// DownloadOptions controls Content-Disposition.
type DownloadOptions struct {
	Inline      bool
	Disposition string // "inline" | "attachment"
}

// List lists content resources.
func (s *ResourcesService) List(ctx context.Context, opts *ListResourcesOptions) (*ResourceListResponse, error) {
	params := map[string]any{}
	if opts != nil {
		if opts.FolderID != nil {
			params["folder_id"] = *opts.FolderID
		}
		if opts.ResourceType != "" {
			params["resource_type"] = opts.ResourceType
		}
		if opts.Q != "" {
			params["q"] = opts.Q
		}
	}
	var out ResourceListResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/resources",
		Query:   transport.EncodeQuery(params),
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Get returns resource metadata.
func (s *ResourcesService) Get(ctx context.Context, uuid string) (*Resource, error) {
	var out Resource
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/resources/" + uuid,
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Upload creates a resource (JSON text content or multipart file).
func (s *ResourcesService) Upload(ctx context.Context, opts UploadResourceOptions) (*Resource, error) {
	if opts.File != nil {
		fields := map[string]string{
			"display_name":  opts.DisplayName,
			"resource_type": opts.ResourceType,
		}
		if opts.FolderID != nil {
			fields["folder_id"] = strconv.FormatInt(*opts.FolderID, 10)
		}
		if opts.MimeType != "" {
			fields["mime_type"] = opts.MimeType
		}
		if opts.Extension != "" {
			fields["extension"] = opts.Extension
		}
		if opts.OriginalFilename != "" {
			fields["original_filename"] = opts.OriginalFilename
		}
		if opts.Content != "" {
			fields["content"] = opts.Content
		}
		name := opts.FileName
		if name == "" {
			name = "upload.bin"
		}
		var out Resource
		resp, err := s.c.tr.DoMultipart(ctx, transport.Request{
			Method: http.MethodPost,
			Path:   "/api/v1/resources",
		}, fields, []transport.MultipartFile{{FieldName: "file", FileName: name, Reader: opts.File}}, nil, &out)
		if err != nil {
			return nil, decodeError(err, bodyOf(resp))
		}
		return &out, nil
	}
	body := ResourceCreateJSON{
		DisplayName:      opts.DisplayName,
		ResourceType:     opts.ResourceType,
		Content:          opts.Content,
		MimeType:         opts.MimeType,
		Extension:        opts.Extension,
		OriginalFilename: opts.OriginalFilename,
	}
	if opts.FolderID != nil {
		body.FolderID = *opts.FolderID
	}
	var out Resource
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/resources",
	}, body, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Update updates resource metadata.
func (s *ResourcesService) Update(ctx context.Context, uuid string, req ResourceUpdateRequest) (*Resource, error) {
	var out Resource
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPut,
		Path:   "/api/v1/resources/" + uuid,
	}, req, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// ReplaceContent replaces the stored file.
func (s *ResourcesService) ReplaceContent(ctx context.Context, uuid string, opts ReplaceContentOptions) (*Resource, error) {
	fields := map[string]string{}
	if opts.Extension != "" {
		fields["extension"] = opts.Extension
	}
	if opts.MimeType != "" {
		fields["mime_type"] = opts.MimeType
	}
	name := opts.FileName
	if name == "" {
		name = "content.bin"
	}
	var out Resource
	resp, err := s.c.tr.DoMultipart(ctx, transport.Request{
		Method: http.MethodPut,
		Path:   "/api/v1/resources/" + uuid + "/content",
	}, fields, []transport.MultipartFile{{FieldName: "file", FileName: name, Reader: opts.File}}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Download downloads resource bytes.
func (s *ResourcesService) Download(ctx context.Context, uuid string, opts *DownloadOptions) (*BinaryResult, error) {
	resp, err := s.c.tr.DoBinary(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/resources/" + uuid + "/download",
		Query:   downloadQuery(opts),
		SafeGET: true,
	})
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &BinaryResult{Data: resp.Body, ContentType: resp.ContentType, Header: resp.Header}, nil
}

// DownloadToFile downloads a resource to a local path.
func (s *ResourcesService) DownloadToFile(ctx context.Context, uuid, path string, opts *DownloadOptions) (*BinaryResult, error) {
	res, err := s.Download(ctx, uuid, opts)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, res.Data, 0o644); err != nil {
		return res, wrapTransportError(err)
	}
	return res, nil
}

// DownloadToWriter streams a resource download to w.
func (s *ResourcesService) DownloadToWriter(ctx context.Context, uuid string, w io.Writer, opts *DownloadOptions) (contentType string, err error) {
	resp, err := s.c.tr.DoStream(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/resources/" + uuid + "/download",
		Query:   downloadQuery(opts),
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

// Delete deletes a resource.
func (s *ResourcesService) Delete(ctx context.Context, uuid string) (*SuccessResponse, error) {
	var out SuccessResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodDelete,
		Path:   "/api/v1/resources/" + uuid,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// ListImages lists image resources.
func (s *ResourcesService) ListImages(ctx context.Context, folderID *int64, q string) (*ImageListResponse, error) {
	params := map[string]any{}
	if folderID != nil {
		params["folder_id"] = *folderID
	}
	if q != "" {
		params["q"] = q
	}
	var out ImageListResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/resources/images",
		Query:   transport.EncodeQuery(params),
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// UploadImageOptions uploads an image resource.
type UploadImageOptions struct {
	File        io.Reader
	FileName    string
	DisplayName string
	FolderID    *int64
	MimeType    string
	Extension   string
}

// UploadImage uploads an image resource (multipart).
func (s *ResourcesService) UploadImage(ctx context.Context, opts UploadImageOptions) (*Resource, error) {
	fields := map[string]string{}
	if opts.DisplayName != "" {
		fields["display_name"] = opts.DisplayName
	}
	if opts.FolderID != nil {
		fields["folder_id"] = strconv.FormatInt(*opts.FolderID, 10)
	}
	if opts.MimeType != "" {
		fields["mime_type"] = opts.MimeType
	}
	if opts.Extension != "" {
		fields["extension"] = opts.Extension
	}
	name := opts.FileName
	if name == "" {
		name = "image.png"
	}
	var out Resource
	resp, err := s.c.tr.DoMultipart(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/resources/images",
	}, fields, []transport.MultipartFile{{FieldName: "file", FileName: name, Reader: opts.File}}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// ReplaceImage replaces an image file.
func (s *ResourcesService) ReplaceImage(ctx context.Context, uuid string, opts ReplaceContentOptions) (*Resource, error) {
	fields := map[string]string{}
	if opts.Extension != "" {
		fields["extension"] = opts.Extension
	}
	if opts.MimeType != "" {
		fields["mime_type"] = opts.MimeType
	}
	name := opts.FileName
	if name == "" {
		name = "image.png"
	}
	var out Resource
	resp, err := s.c.tr.DoMultipart(ctx, transport.Request{
		Method: http.MethodPut,
		Path:   "/api/v1/resources/images/" + uuid,
	}, fields, []transport.MultipartFile{{FieldName: "file", FileName: name, Reader: opts.File}}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// DownloadImage downloads image bytes.
func (s *ResourcesService) DownloadImage(ctx context.Context, uuid string, opts *DownloadOptions) (*BinaryResult, error) {
	resp, err := s.c.tr.DoBinary(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/resources/images/" + uuid + "/download",
		Query:   downloadQuery(opts),
		SafeGET: true,
	})
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &BinaryResult{Data: resp.Body, ContentType: resp.ContentType, Header: resp.Header}, nil
}

// GetSyncHint returns how to trigger a resource sync (platform-admin).
func (s *ResourcesService) GetSyncHint(ctx context.Context) (*ResourceSyncHint, error) {
	var out ResourceSyncHint
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/resource-sync",
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// RunSync runs resource sync (platform-admin).
func (s *ResourcesService) RunSync(ctx context.Context) (*ResourceSyncResult, error) {
	var out ResourceSyncResult
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/resource-sync",
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

func downloadQuery(opts *DownloadOptions) map[string][]string {
	params := map[string]any{}
	if opts != nil {
		if opts.Inline {
			params["inline"] = "1"
		}
		if opts.Disposition != "" {
			params["disposition"] = opts.Disposition
		}
	}
	return transport.EncodeQuery(params)
}
