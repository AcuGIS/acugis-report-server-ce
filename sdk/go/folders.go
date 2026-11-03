package report

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/acugis/acugis-report-go/internal/transport"
)

// FoldersService covers folders, browse, search, permissions, favorites.
type FoldersService struct{ c *Client }

// Tree returns the ACL-filtered folder tree.
func (s *FoldersService) Tree(ctx context.Context) ([]FolderTreeNode, error) {
	var out []FolderTreeNode
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/folders/tree",
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return out, nil
}

// Browse lists folder contents. folderID 0 is Home/root.
func (s *FoldersService) Browse(ctx context.Context, folderID int64) (*BrowseResult, error) {
	var out BrowseResult
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/folders/browse",
		Query:   transport.EncodeQuery(map[string]any{"folder_id": folderID}),
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Search searches folders and items (capped at 50).
func (s *FoldersService) Search(ctx context.Context, q string) ([]SearchHit, error) {
	var wrap struct {
		Count int         `json:"count"`
		Items []SearchHit `json:"items"`
	}
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/folders/search",
		Query:   transport.EncodeQuery(map[string]any{"q": q}),
		SafeGET: true,
	}, nil, &wrap)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return wrap.Items, nil
}

// Get returns a folder by id.
func (s *FoldersService) Get(ctx context.Context, id int64) (*Folder, error) {
	var out Folder
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf("/api/v1/folders/%d", id),
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Create creates a folder.
func (s *FoldersService) Create(ctx context.Context, req CreateFolderRequest) (*Folder, error) {
	var out Folder
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/folders",
	}, req, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Update updates a folder.
func (s *FoldersService) Update(ctx context.Context, id int64, req UpdateFolderRequest) (*Folder, error) {
	var out Folder
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPut,
		Path:   fmt.Sprintf("/api/v1/folders/%d", id),
	}, req, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// DeleteOptions configures folder deletion.
type DeleteOptions struct {
	Recursive *bool
	Confirm   *bool
}

// Delete deletes a folder.
func (s *FoldersService) Delete(ctx context.Context, id int64, opts *DeleteOptions) (*SuccessResponse, error) {
	params := map[string]any{}
	if opts != nil {
		if opts.Recursive != nil {
			params["recursive"] = strconv.FormatBool(*opts.Recursive)
		}
		if opts.Confirm != nil {
			params["confirm"] = strconv.FormatBool(*opts.Confirm)
		}
	}
	var out SuccessResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodDelete,
		Path:   fmt.Sprintf("/api/v1/folders/%d", id),
		Query:  transport.EncodeQuery(params),
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Move moves a folder under a new parent.
func (s *FoldersService) Move(ctx context.Context, id int64, req MoveFolderRequest) (*Folder, error) {
	var out Folder
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("/api/v1/folders/%d/move", id),
	}, req, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// Duplicate duplicates a folder.
func (s *FoldersService) Duplicate(ctx context.Context, id int64) (*Folder, error) {
	var out Folder
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("/api/v1/folders/%d/duplicate", id),
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// GetPermissions returns group grants for a folder.
func (s *FoldersService) GetPermissions(ctx context.Context, id int64) (*FolderPermissions, error) {
	var out FolderPermissions
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    fmt.Sprintf("/api/v1/folders/%d/permissions", id),
		SafeGET: true,
	}, nil, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// ReplacePermissions replaces folder group grants.
func (s *FoldersService) ReplacePermissions(ctx context.Context, id int64, req SetFolderPermissionsRequest) (*SuccessResponse, error) {
	var out SuccessResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPut,
		Path:   fmt.Sprintf("/api/v1/folders/%d/permissions", id),
	}, req, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// MoveContent moves a content item into a folder (POST /api/v1/content/move).
func (s *FoldersService) MoveContent(ctx context.Context, req MoveContentRequest) (*SuccessResponse, error) {
	var out SuccessResponse
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/content/move",
	}, req, &out)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return &out, nil
}

// ListFavorites returns favorite items.
func (s *FoldersService) ListFavorites(ctx context.Context) ([]ContentItem, error) {
	var wrap struct {
		Items []ContentItem `json:"items"`
	}
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method:  http.MethodGet,
		Path:    "/api/v1/favorites",
		SafeGET: true,
	}, nil, &wrap)
	if err != nil {
		return nil, decodeError(err, bodyOf(resp))
	}
	return wrap.Items, nil
}

// SetFavorite sets or unsets a favorite.
func (s *FoldersService) SetFavorite(ctx context.Context, req FavoriteRequest) (favorite bool, err error) {
	var wrap struct {
		OK       bool `json:"ok"`
		Favorite bool `json:"favorite"`
	}
	resp, err := s.c.tr.DoJSON(ctx, transport.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/favorites",
	}, req, &wrap)
	if err != nil {
		return false, decodeError(err, bodyOf(resp))
	}
	return wrap.Favorite, nil
}
