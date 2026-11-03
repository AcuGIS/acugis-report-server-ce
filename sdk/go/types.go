package report

import "encoding/json"

// User is the authenticated user from GET /api/v1/auth/me.
type User struct {
	ID           int64    `json:"id,omitempty"`
	Username     string   `json:"username,omitempty"`
	Email        string   `json:"email,omitempty"`
	IsAdmin      bool     `json:"is_admin,omitempty"`
	AuthDisabled bool     `json:"auth_disabled,omitempty"`
	Groups       []string `json:"groups,omitempty"`
	Permissions  []string `json:"permissions,omitempty"`
}

// Token is returned when creating or rotating a personal access token.
type Token struct {
	ID        int64  `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Token     string `json:"token,omitempty"`
	Message   string `json:"message,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// TokenMetadata describes a token without revealing the secret.
type TokenMetadata struct {
	ID          int64    `json:"id,omitempty"`
	UserID      int64    `json:"user_id,omitempty"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	TokenPrefix string   `json:"token_prefix,omitempty"`
	Active      bool     `json:"active,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	ExpiresAt   *string  `json:"expires_at,omitempty"`
	LastUsedAt  *string  `json:"last_used_at,omitempty"`
	LastUsedIP  string   `json:"last_used_ip,omitempty"`
	CreatedByIP string   `json:"created_by_ip,omitempty"`
	RevokedAt   *string  `json:"revoked_at,omitempty"`
}

// CreateTokenRequest creates a personal access token.
type CreateTokenRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ExpiresIn   string `json:"expires_in,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

// UpdateTokenRequest updates token metadata.
type UpdateTokenRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	ExpiresIn   string `json:"expires_in,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

// ReportSummary is an item from the report list.
type ReportSummary struct {
	Key          string `json:"key,omitempty"`
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
	Type         string `json:"type,omitempty"`
	Format       string `json:"format,omitempty"`
	Datasource   string `json:"datasource,omitempty"`
	HasThumbnail bool   `json:"has_thumbnail,omitempty"`
}

// ReportListResponse is GET /api/v1/reports.
type ReportListResponse struct {
	Count int             `json:"count,omitempty"`
	Items []ReportSummary `json:"items,omitempty"`
}

// Report is the full report definition.
type Report struct {
	Report        string           `json:"report,omitempty"`
	Name          string           `json:"name,omitempty"`
	Description   string           `json:"description,omitempty"`
	Type          string           `json:"type,omitempty"`
	Format        string           `json:"format,omitempty"`
	Datasource    string           `json:"datasource,omitempty"`
	AccessGroups  []string         `json:"access_groups,omitempty"`
	Parameters    []map[string]any `json:"parameters,omitempty"`
	MapDefinition map[string]any   `json:"mapDefinition,omitempty"`
	EmbeddedMap   map[string]any   `json:"embeddedMap,omitempty"`
}

// ReportCreateResponse is returned by Publish.
type ReportCreateResponse struct {
	OK      bool     `json:"ok,omitempty"`
	Key     string   `json:"key,omitempty"`
	Message string   `json:"message,omitempty"`
	JSONURL string   `json:"json_url,omitempty"`
	RunURL  string   `json:"run_url,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

// ReportMutationResponse is returned by Update/Delete.
type ReportMutationResponse struct {
	OK  bool   `json:"ok,omitempty"`
	Key string `json:"key,omitempty"`
}

// ReportConfig is viewer configuration.
type ReportConfig struct {
	Report          string           `json:"report,omitempty"`
	Type            string           `json:"type,omitempty"`
	ReportTypeLabel string           `json:"reportTypeLabel,omitempty"`
	Parameters      []map[string]any `json:"parameters,omitempty"`
	Map             *MapLayerMeta    `json:"map,omitempty"`
	MapDefinition   map[string]any   `json:"mapDefinition,omitempty"`
}

// MapLayerMeta describes a map layer.
type MapLayerMeta struct {
	Type            string   `json:"type,omitempty"`
	Schema          string   `json:"schema,omitempty"`
	Table           string   `json:"table,omitempty"`
	GeometryColumn  string   `json:"geometry_column,omitempty"`
	GeometryColumns []string `json:"geometry_columns,omitempty"`
	GeometryType    string   `json:"geometry_type,omitempty"`
	SRID            int      `json:"srid,omitempty"`
	SpatialCapable  bool     `json:"spatial_capable,omitempty"`
	ReportFields    []string `json:"report_fields,omitempty"`
	Error           string   `json:"error,omitempty"`
}

// ParameterOption is a cascading parameter choice.
type ParameterOption struct {
	Label string `json:"label,omitempty"`
	Value string `json:"value,omitempty"`
}

// ParameterLookupResponse holds lookup options.
type ParameterLookupResponse struct {
	Options []ParameterOption `json:"options,omitempty"`
}

// Folder is a content folder.
type Folder struct {
	ID               int64  `json:"id,omitempty"`
	ParentID         *int64 `json:"parent_id,omitempty"`
	Name             string `json:"name,omitempty"`
	Description      string `json:"description,omitempty"`
	Icon             string `json:"icon,omitempty"`
	Path             string `json:"path,omitempty"`
	SortOrder        int    `json:"sort_order,omitempty"`
	ACLInherit       bool   `json:"acl_inherit,omitempty"`
	ChildFolderCount int    `json:"child_folder_count,omitempty"`
	ChildItemCount   int    `json:"child_item_count,omitempty"`
	CreatedBy        int64  `json:"created_by,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

// FolderTreeNode is a node in GET /api/v1/folders/tree.
type FolderTreeNode struct {
	ID       int64            `json:"id,omitempty"`
	ParentID *int64           `json:"parent_id,omitempty"`
	Name     string           `json:"name,omitempty"`
	Icon     string           `json:"icon,omitempty"`
	Children []FolderTreeNode `json:"children,omitempty"`
}

// Breadcrumb is a browse path segment.
type Breadcrumb struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// ContentItem is a browsable content row.
type ContentItem struct {
	ID               int64  `json:"id,omitempty"`
	FolderID         int64  `json:"folder_id,omitempty"`
	FolderPath       string `json:"folder_path,omitempty"`
	ContentType      string `json:"content_type,omitempty"`
	ContentKey       string `json:"content_key,omitempty"`
	DisplayName      string `json:"display_name,omitempty"`
	ResourceType     string `json:"resource_type,omitempty"`
	MimeType         string `json:"mime_type,omitempty"`
	Extension        string `json:"extension,omitempty"`
	OriginalFilename string `json:"original_filename,omitempty"`
	SizeBytes        int64  `json:"size_bytes,omitempty"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	SortOrder        int    `json:"sort_order,omitempty"`
	ACLInherit       bool   `json:"acl_inherit,omitempty"`
	CreatedBy        int64  `json:"created_by,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

// BrowseResult is GET /api/v1/folders/browse.
type BrowseResult struct {
	Folder      *Folder       `json:"folder,omitempty"`
	Folders     []Folder      `json:"folders,omitempty"`
	Items       []ContentItem `json:"items,omitempty"`
	Breadcrumbs []Breadcrumb  `json:"breadcrumbs,omitempty"`
}

// CreateFolderRequest creates a folder.
type CreateFolderRequest struct {
	Name        string `json:"name"`
	ParentID    int64  `json:"parent_id,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	SortOrder   int    `json:"sort_order,omitempty"`
	ACLInherit  *bool  `json:"acl_inherit,omitempty"`
}

// UpdateFolderRequest updates a folder.
type UpdateFolderRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	SortOrder   *int   `json:"sort_order,omitempty"`
	ACLInherit  *bool  `json:"acl_inherit,omitempty"`
}

// MoveFolderRequest moves a folder.
type MoveFolderRequest struct {
	ParentID  int64 `json:"parent_id"`
	SortOrder *int  `json:"sort_order,omitempty"`
}

// FolderPermissions lists group grants.
type FolderPermissions struct {
	GroupNames []string `json:"group_names,omitempty"`
}

// SetFolderPermissionsRequest replaces folder grants.
type SetFolderPermissionsRequest struct {
	GroupNames []string `json:"group_names"`
	Permission string   `json:"permission,omitempty"`
}

// ViewPermissions is the Access Groups ACL for a saved view or dashboard.
type ViewPermissions struct {
	Ref             string   `json:"ref,omitempty"`
	GroupNames      []string `json:"group_names,omitempty"`
	AvailableGroups []string `json:"available_groups,omitempty"`
	CanEdit         bool     `json:"can_edit,omitempty"`
	ResourceType    string   `json:"resource_type,omitempty"`
}

// SetViewPermissionsRequest replaces view/dashboard Access Groups.
type SetViewPermissionsRequest struct {
	GroupNames []string `json:"group_names"`
}

// FavoriteRequest sets or unsets a favorite.
type FavoriteRequest struct {
	ContentType string `json:"content_type"`
	ContentKey  string `json:"content_key"`
	Favorite    *bool  `json:"favorite,omitempty"`
}

// SearchHit is a folder/search result.
type SearchHit struct {
	Kind        string `json:"kind,omitempty"`
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	FolderID    int64  `json:"folder_id,omitempty"`
	FolderPath  string `json:"folder_path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	ContentKey  string `json:"content_key,omitempty"`
}

// MoveContentRequest moves content between folders.
type MoveContentRequest struct {
	FolderID    int64  `json:"folder_id"`
	ContentType string `json:"content_type"`
	ContentKey  string `json:"content_key"`
	DisplayName string `json:"display_name,omitempty"`
	SortOrder   *int   `json:"sort_order,omitempty"`
	ACLInherit  *bool  `json:"acl_inherit,omitempty"`
}

// SuccessResponse is a generic ok payload.
type SuccessResponse struct {
	OK      bool   `json:"ok,omitempty"`
	Message string `json:"message,omitempty"`
	UUID    string `json:"uuid,omitempty"`
}

// Resource is a content resource.
type Resource struct {
	ID               int64  `json:"id,omitempty"`
	UUID             string `json:"uuid,omitempty"`
	FolderID         int64  `json:"folder_id,omitempty"`
	FolderPath       string `json:"folder_path,omitempty"`
	DisplayName      string `json:"display_name,omitempty"`
	ResourceType     string `json:"resource_type,omitempty"`
	MimeType         string `json:"mime_type,omitempty"`
	Extension        string `json:"extension,omitempty"`
	OriginalFilename string `json:"original_filename,omitempty"`
	StorageFilename  string `json:"storage_filename,omitempty"`
	ResourcePath     string `json:"resource_path,omitempty"`
	SizeBytes        int64  `json:"size_bytes,omitempty"`
	SHA256           string `json:"sha256,omitempty"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	HasAlpha         bool   `json:"has_alpha,omitempty"`
	CreatedBy        int64  `json:"created_by,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

// ResourceCreateJSON creates a resource from pasted text.
type ResourceCreateJSON struct {
	DisplayName      string `json:"display_name"`
	ResourceType     string `json:"resource_type"`
	Content          string `json:"content"`
	FolderID         int64  `json:"folder_id,omitempty"`
	MimeType         string `json:"mime_type,omitempty"`
	Extension        string `json:"extension,omitempty"`
	OriginalFilename string `json:"original_filename,omitempty"`
}

// ResourceUpdateRequest updates resource metadata.
type ResourceUpdateRequest struct {
	DisplayName  string `json:"display_name,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	FolderID     *int64 `json:"folder_id,omitempty"`
}

// ResourceListResponse is GET /api/v1/resources.
type ResourceListResponse struct {
	Items []Resource `json:"items,omitempty"`
}

// ImageListResponse is GET /api/v1/resources/images.
type ImageListResponse struct {
	Items            []Resource `json:"items,omitempty"`
	SupportedFormats []string   `json:"supported_formats,omitempty"`
}

// ResourceSyncHint is GET /api/v1/resource-sync.
type ResourceSyncHint struct {
	OK      bool   `json:"ok,omitempty"`
	Hint    string `json:"hint,omitempty"`
	Service string `json:"service,omitempty"`
}

// ResourceSyncResult is POST /api/v1/resource-sync.
type ResourceSyncResult struct {
	OK            bool   `json:"ok,omitempty"`
	Registered    int    `json:"registered,omitempty"`
	AlreadyExists int    `json:"already_exists,omitempty"`
	Skipped       int    `json:"skipped,omitempty"`
	Errors        int    `json:"errors,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Service       string `json:"service,omitempty"`
}

// DatasourceName is a listed datasource.
type DatasourceName struct {
	Name string `json:"name,omitempty"`
}

// DatasourceCreateRequest upserts a datasource via form fields.
type DatasourceCreateRequest struct {
	Name                   string `json:"name,omitempty"`
	DBType                 string `json:"db_type,omitempty"`
	Host                   string `json:"host,omitempty"`
	Port                   string `json:"port,omitempty"`
	DBName                 string `json:"dbname,omitempty"`
	SchemaName             string `json:"schema_name,omitempty"`
	Username               string `json:"username,omitempty"`
	Password               string `json:"password,omitempty"`
	AuthType               string `json:"auth_type,omitempty"`
	PublicKeyResourceUUID  string `json:"public_key_resource_uuid,omitempty"`
	PrivateKeyResourceUUID string `json:"private_key_resource_uuid,omitempty"`
}

// DatasourceMutationResponse is returned by datasource mutations.
type DatasourceMutationResponse struct {
	OK      bool   `json:"ok,omitempty"`
	Message string `json:"message,omitempty"`
}

// JasperContextResponse is POST /api/v1/datasources/context.
type JasperContextResponse struct {
	OK    bool     `json:"ok,omitempty"`
	Path  string   `json:"path,omitempty"`
	Notes []string `json:"notes,omitempty"`
}

// MapColumnsResult is GET /api/v1/map/{key}/columns.
type MapColumnsResult struct {
	Schema          string           `json:"schema,omitempty"`
	Table           string           `json:"table,omitempty"`
	SRID            int              `json:"srid,omitempty"`
	GeometryColumn  string           `json:"geometry_column,omitempty"`
	GeometryColumns []string         `json:"geometry_columns,omitempty"`
	Columns         []map[string]any `json:"columns,omitempty"`
	ReportFields    []string         `json:"report_fields,omitempty"`
	Error           string           `json:"error,omitempty"`
}

// MapExtent is GET /api/v1/map/{key}/extent.
type MapExtent struct {
	PGeometry string `json:"p_geometry,omitempty"`
	PSRID     int    `json:"p_srid,omitempty"`
}

// FeatureCollection is a GeoJSON FeatureCollection.
type FeatureCollection struct {
	Type          string           `json:"type"`
	Features      []map[string]any `json:"features"`
	CRS           map[string]any   `json:"crs,omitempty"`
	Meta          map[string]any   `json:"meta,omitempty"`
	MapDefinition map[string]any   `json:"mapDefinition,omitempty"`
	Error         string           `json:"error,omitempty"`
	Detail        string           `json:"detail,omitempty"`
}

// HealthResponse is GET /health.
type HealthResponse struct {
	Service string `json:"service,omitempty"`
	Status  string `json:"status,omitempty"`
}

// ReadyResponse is GET /ready.
type ReadyResponse struct {
	Service string         `json:"service,omitempty"`
	Status  string         `json:"status,omitempty"`
	Checks  map[string]any `json:"checks,omitempty"`
}

// Diagnostics is GET /api/v1/diagnostics.
type Diagnostics struct {
	Service     string `json:"service,omitempty"`
	AppVersion  string `json:"app_version,omitempty"`
	GoVersion   string `json:"go_version,omitempty"`
	Environment string `json:"environment,omitempty"`
	BasePath    string `json:"base_path,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
}

// BinaryResult holds bytes and content type for binary endpoints.
type BinaryResult struct {
	Data        []byte
	ContentType string
	Header      map[string][]string
}

// ExplorerQuery is a structured explorer query body.
type ExplorerQuery struct {
	Datasource     string              `json:"datasource,omitempty"`
	Schema         string              `json:"schema,omitempty"`
	Table          string              `json:"table,omitempty"`
	SourceMode     string              `json:"source_mode,omitempty"`
	ReportKey      string              `json:"report_key,omitempty"`
	ReportCacheKey string              `json:"report_cache_key,omitempty"`
	Columns        []string            `json:"columns,omitempty"`
	Filters        []ExplorerFilter    `json:"filters,omitempty"`
	Sort           []ExplorerSort      `json:"sort,omitempty"`
	Aggregates     []ExplorerAggregate `json:"aggregates,omitempty"`
	GroupBy        []string            `json:"group_by,omitempty"`
	Limit          int                 `json:"limit,omitempty"`
	Offset         int                 `json:"offset,omitempty"`
	Page           int                 `json:"page,omitempty"`
	PageSize       int                 `json:"page_size,omitempty"`
}

// ExplorerFilter is a query filter.
type ExplorerFilter struct {
	Column   string `json:"column,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    any    `json:"value,omitempty"`
	ValueTo  any    `json:"value_to,omitempty"`
	Values   []any  `json:"values,omitempty"`
}

// ExplorerSort is a sort clause.
type ExplorerSort struct {
	Column    string `json:"column,omitempty"`
	Direction string `json:"direction,omitempty"`
}

// ExplorerAggregate is an aggregate clause.
type ExplorerAggregate struct {
	Func   string `json:"func,omitempty"`
	Column string `json:"column,omitempty"`
	Alias  string `json:"alias,omitempty"`
}

// ExplorerResult is a tabular explorer response.
type ExplorerResult struct {
	Columns    []string         `json:"columns,omitempty"`
	Rows       []map[string]any `json:"rows,omitempty"`
	TotalRows  int64            `json:"total_rows,omitempty"`
	Page       int              `json:"page,omitempty"`
	PageSize   int              `json:"page_size,omitempty"`
	Truncated  bool             `json:"truncated,omitempty"`
	Engine     string           `json:"engine,omitempty"`
	ExecutedAt string           `json:"executed_at,omitempty"`
}

// ExplorerColumn describes a table column.
type ExplorerColumn struct {
	Name       string `json:"name,omitempty"`
	Type       string `json:"type,omitempty"`
	UDTName    string `json:"udt_name,omitempty"`
	Nullable   bool   `json:"nullable,omitempty"`
	IsPrimary  bool   `json:"is_primary,omitempty"`
	IsGeometry bool   `json:"is_geometry,omitempty"`
	Ordinal    int    `json:"ordinal,omitempty"`
}

// TableMeta is explorer table metadata.
type TableMeta struct {
	Datasource     string           `json:"datasource,omitempty"`
	Schema         string           `json:"schema,omitempty"`
	Table          string           `json:"table,omitempty"`
	Engine         string           `json:"engine,omitempty"`
	Columns        []ExplorerColumn `json:"columns,omitempty"`
	PrimaryKey     []string         `json:"primary_key,omitempty"`
	GeometryColumn string           `json:"geometry_column,omitempty"`
	SRID           int              `json:"srid,omitempty"`
	SpatialCapable bool             `json:"spatial_capable,omitempty"`
}

// SavedView is an explorer saved view.
type SavedView struct {
	Key             string              `json:"key,omitempty"`
	Name            string              `json:"name,omitempty"`
	OwnerUsername   string              `json:"owner_username,omitempty"`
	Datasource      string              `json:"datasource,omitempty"`
	Schema          string              `json:"schema,omitempty"`
	Table           string              `json:"table,omitempty"`
	SourceMode      string              `json:"source_mode,omitempty"`
	ReportKey       string              `json:"report_key,omitempty"`
	ReportName      string              `json:"report_name,omitempty"`
	Columns         []string            `json:"columns,omitempty"`
	Filters         []ExplorerFilter    `json:"filters,omitempty"`
	Sort            []ExplorerSort      `json:"sort,omitempty"`
	Aggregates      []ExplorerAggregate `json:"aggregates,omitempty"`
	GroupBy         []string            `json:"group_by,omitempty"`
	Limit           int                 `json:"limit,omitempty"`
	Chart           map[string]any      `json:"chart,omitempty"`
	Map             map[string]any      `json:"map,omitempty"`
	PanelLayout     map[string]any      `json:"panel_layout,omitempty"`
	Widgets         []map[string]any    `json:"widgets,omitempty"`
	DashboardWidget bool                `json:"dashboard_widget,omitempty"`
	CreatedAt       string              `json:"created_at,omitempty"`
	UpdatedAt       string              `json:"updated_at,omitempty"`
}

// JSONObject is a loose JSON object for flexible explorer responses.
type JSONObject = map[string]any

// RawJSON is an unparsed JSON payload.
type RawJSON = json.RawMessage
