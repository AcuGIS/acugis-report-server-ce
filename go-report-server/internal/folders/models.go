// Package folders implements hierarchical content folders for the report catalog.
package folders

import "time"

// Content type constants for the polymorphic content_items table.
const (
	ContentTypeReport       = "jasper_report"
	ContentTypeDashboard    = "dashboard" // legacy / display alias; prefer explorer_view + is_dashboard
	ContentTypeExplorerView = "explorer_view"
	ContentTypeTemplate     = "template"
	ContentTypeResource     = "content_resource"
	ContentTypeFolder       = "content_folder"
)

// Folder is a row from content_folders.
type Folder struct {
	ID          int64     `json:"id"`
	ParentID    *int64    `json:"parent_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	SortOrder   int       `json:"sort_order"`
	ACLInherit  bool      `json:"acl_inherit"`
	CreatedBy   *int64    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Path is populated by service/list helpers (Home > …).
	Path string `json:"path,omitempty"`
	// ChildCounts optional.
	ChildFolderCount int `json:"child_folder_count,omitempty"`
	ChildItemCount   int `json:"child_item_count,omitempty"`
}

// Item is a polymorphic content membership row.
type Item struct {
	ID          int64     `json:"id"`
	FolderID    int64     `json:"folder_id"`
	ContentType string    `json:"content_type"`
	ContentKey  string    `json:"content_key"`
	DisplayName string    `json:"display_name"`
	SortOrder   int       `json:"sort_order"`
	ACLInherit  bool      `json:"acl_inherit"`
	CreatedBy   *int64    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	FolderPath  string    `json:"folder_path,omitempty"`
	// Optional enrichment for content_resource rows (from jasper_content_resource).
	ResourceType     string `json:"resource_type,omitempty"`
	OriginalFilename string `json:"original_filename,omitempty"`
	SizeBytes        int64  `json:"size_bytes,omitempty"`
	MimeType         string `json:"mime_type,omitempty"`
	Extension        string `json:"extension,omitempty"`
	Width            *int   `json:"width,omitempty"`
	Height           *int   `json:"height,omitempty"`
	// DesignerNative and CanDesign are filled by the catalog handler.
	// They are not stored in content_items.
	DesignerNative bool `json:"designer_native,omitempty"`
	CanDesign      bool `json:"can_design,omitempty"`
}

// Breadcrumb is one crumb in a folder path.
type Breadcrumb struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// TreeNode is a folder node for the navigation tree.
type TreeNode struct {
	ID       int64      `json:"id"`
	ParentID *int64     `json:"parent_id"`
	Name     string     `json:"name"`
	Icon     string     `json:"icon"`
	Children []TreeNode `json:"children,omitempty"`
}

// BrowseResult is the contents of one folder (folders first, then items).
type BrowseResult struct {
	Folder      *Folder      `json:"folder"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs"`
	Folders     []Folder     `json:"folders"`
	Items       []Item       `json:"items"`
}

// SearchHit is a search result with full folder path.
type SearchHit struct {
	Kind           string `json:"kind"` // folder | item
	ID             int64  `json:"id,omitempty"`
	FolderID       int64  `json:"folder_id,omitempty"`
	ContentType    string `json:"content_type,omitempty"`
	ContentKey     string `json:"content_key,omitempty"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	FolderPath     string `json:"folder_path"`
	Icon           string `json:"icon,omitempty"`
	ResourceType   string `json:"resource_type,omitempty"` // e.g. view | dashboard for explorer_view
	DesignerNative bool   `json:"designer_native,omitempty"`
	CanDesign      bool   `json:"can_design,omitempty"`
}

// CreateFolderInput creates a child folder.
type CreateFolderInput struct {
	ParentID    int64  `json:"parent_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	SortOrder   int    `json:"sort_order"`
	ACLInherit  *bool  `json:"acl_inherit"`
	CreatedBy   int64  `json:"-"`
}

// UpdateFolderInput patches folder metadata.
type UpdateFolderInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	SortOrder   *int    `json:"sort_order"`
	ACLInherit  *bool   `json:"acl_inherit"`
}

// MoveFolderInput relocates a folder under a new parent.
type MoveFolderInput struct {
	ParentID  int64 `json:"parent_id"`
	SortOrder *int  `json:"sort_order"`
}

// MoveItemInput relocates content into a folder.
type MoveItemInput struct {
	FolderID    int64  `json:"folder_id"`
	ContentType string `json:"content_type"`
	ContentKey  string `json:"content_key"`
	DisplayName string `json:"display_name"`
	SortOrder   *int   `json:"sort_order"`
	ACLInherit  *bool  `json:"acl_inherit"`
	CreatedBy   int64  `json:"-"`
}

// DeleteOptions controls recursive folder deletion.
type DeleteOptions struct {
	Recursive bool
	Confirm   bool
	// AllowRecursive comes from server config (JRS_FOLDER_ALLOW_RECURSIVE_DELETE).
	AllowRecursive bool
}

// SetPermissionsInput replaces group grants on a folder resource.
type SetPermissionsInput struct {
	GroupNames []string `json:"group_names"`
	Permission string   `json:"permission"` // default view
}
