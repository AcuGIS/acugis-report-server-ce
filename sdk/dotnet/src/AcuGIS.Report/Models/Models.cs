using System.Text.Json;
using System.Text.Json.Serialization;

namespace AcuGIS.Report.Models;

public sealed class ErrorBody
{
    [JsonPropertyName("error")]
    public string? Error { get; set; }

    [JsonPropertyName("code")]
    public string? Code { get; set; }

    [JsonPropertyName("hint")]
    public string? Hint { get; set; }

    [JsonPropertyName("service")]
    public string? Service { get; set; }
}

public sealed class ValidationErrorBody
{
    [JsonPropertyName("error")]
    public string? Error { get; set; }

    [JsonPropertyName("errors")]
    public List<string>? Errors { get; set; }
}

public sealed class SuccessResponse
{
    [JsonPropertyName("ok")]
    public bool? Ok { get; set; }

    [JsonPropertyName("message")]
    public string? Message { get; set; }
}

public sealed class HealthResponse
{
    [JsonPropertyName("service")]
    public string? Service { get; set; }

    [JsonPropertyName("status")]
    public string? Status { get; set; }
}

public sealed class ReadyResponse
{
    [JsonPropertyName("service")]
    public string? Service { get; set; }

    [JsonPropertyName("status")]
    public string? Status { get; set; }

    [JsonPropertyName("checks")]
    public Dictionary<string, JsonElement>? Checks { get; set; }
}

public sealed class User
{
    [JsonPropertyName("id")]
    public long? Id { get; set; }

    [JsonPropertyName("username")]
    public string? Username { get; set; }

    [JsonPropertyName("email")]
    public string? Email { get; set; }

    [JsonPropertyName("is_admin")]
    public bool? IsAdmin { get; set; }

    [JsonPropertyName("auth_disabled")]
    public bool? AuthDisabled { get; set; }

    [JsonPropertyName("groups")]
    public List<string>? Groups { get; set; }

    [JsonPropertyName("permissions")]
    public List<string>? Permissions { get; set; }
}

public sealed class LoginRequest
{
    [JsonPropertyName("username")]
    public string Username { get; set; } = "";

    [JsonPropertyName("password")]
    public string Password { get; set; } = "";
}

public sealed class CreateTokenRequest
{
    [JsonPropertyName("name")]
    public string Name { get; set; } = "";

    [JsonPropertyName("description")]
    public string? Description { get; set; }

    [JsonPropertyName("expires_in")]
    public string? ExpiresIn { get; set; }

    [JsonPropertyName("expires_at")]
    public string? ExpiresAt { get; set; }
}

public sealed class UpdateTokenRequest
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("description")]
    public string? Description { get; set; }

    [JsonPropertyName("expires_in")]
    public string? ExpiresIn { get; set; }

    [JsonPropertyName("expires_at")]
    public string? ExpiresAt { get; set; }
}

public sealed class Token
{
    [JsonPropertyName("id")]
    public long? Id { get; set; }

    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("token")]
    public string? Value { get; set; }

    [JsonPropertyName("message")]
    public string? Message { get; set; }

    [JsonPropertyName("created_at")]
    public DateTimeOffset? CreatedAt { get; set; }
}

public sealed class TokenMetadata
{
    [JsonPropertyName("id")]
    public long? Id { get; set; }

    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("description")]
    public string? Description { get; set; }

    [JsonPropertyName("active")]
    public bool? Active { get; set; }

    [JsonPropertyName("token_prefix")]
    public string? TokenPrefix { get; set; }

    [JsonPropertyName("created_at")]
    public DateTimeOffset? CreatedAt { get; set; }

    [JsonPropertyName("updated_at")]
    public DateTimeOffset? UpdatedAt { get; set; }

    [JsonPropertyName("expires_at")]
    public DateTimeOffset? ExpiresAt { get; set; }

    [JsonPropertyName("revoked_at")]
    public DateTimeOffset? RevokedAt { get; set; }

    [JsonPropertyName("last_used_at")]
    public DateTimeOffset? LastUsedAt { get; set; }

    [JsonPropertyName("last_used_ip")]
    public string? LastUsedIp { get; set; }

    [JsonPropertyName("created_by_ip")]
    public string? CreatedByIp { get; set; }

    [JsonPropertyName("user_id")]
    public long? UserId { get; set; }

    [JsonPropertyName("scopes")]
    public List<string>? Scopes { get; set; }
}

public sealed class TokenListResponse
{
    [JsonPropertyName("tokens")]
    public List<TokenMetadata>? Tokens { get; set; }
}

public sealed class ReportSummary
{
    [JsonPropertyName("key")]
    public string? Key { get; set; }

    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("description")]
    public string? Description { get; set; }

    [JsonPropertyName("datasource")]
    public string? Datasource { get; set; }

    [JsonPropertyName("format")]
    public string? Format { get; set; }

    [JsonPropertyName("type")]
    public string? Type { get; set; }

    [JsonPropertyName("has_thumbnail")]
    public bool? HasThumbnail { get; set; }
}

public sealed class ReportListResponse
{
    [JsonPropertyName("count")]
    public int? Count { get; set; }

    [JsonPropertyName("items")]
    public List<ReportSummary>? Items { get; set; }
}

public sealed class Report
{
    [JsonPropertyName("report")]
    public string? ReportKey { get; set; }

    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("description")]
    public string? Description { get; set; }

    [JsonPropertyName("datasource")]
    public string? Datasource { get; set; }

    [JsonPropertyName("format")]
    public string? Format { get; set; }

    [JsonPropertyName("type")]
    public string? Type { get; set; }

    [JsonPropertyName("access_groups")]
    public List<string>? AccessGroups { get; set; }

    [JsonPropertyName("parameters")]
    public List<Dictionary<string, JsonElement>>? Parameters { get; set; }

    [JsonPropertyName("mapDefinition")]
    public Dictionary<string, JsonElement>? MapDefinition { get; set; }

    [JsonPropertyName("embeddedMap")]
    public Dictionary<string, JsonElement>? EmbeddedMap { get; set; }

    [JsonExtensionData]
    public Dictionary<string, JsonElement>? ExtensionData { get; set; }
}

public sealed class ReportConfig
{
    [JsonPropertyName("report")]
    public string? ReportKey { get; set; }

    [JsonPropertyName("type")]
    public string? Type { get; set; }

    [JsonPropertyName("reportTypeLabel")]
    public string? ReportTypeLabel { get; set; }

    [JsonPropertyName("parameters")]
    public List<Dictionary<string, JsonElement>>? Parameters { get; set; }

    [JsonPropertyName("map")]
    public MapLayerMeta? Map { get; set; }

    [JsonPropertyName("mapDefinition")]
    public Dictionary<string, JsonElement>? MapDefinition { get; set; }

    [JsonExtensionData]
    public Dictionary<string, JsonElement>? ExtensionData { get; set; }
}

public sealed class ReportCreateResponse
{
    [JsonPropertyName("ok")]
    public bool? Ok { get; set; }

    [JsonPropertyName("key")]
    public string? Key { get; set; }

    [JsonPropertyName("message")]
    public string? Message { get; set; }

    [JsonPropertyName("json_url")]
    public string? JsonUrl { get; set; }

    [JsonPropertyName("run_url")]
    public string? RunUrl { get; set; }

    [JsonPropertyName("notes")]
    public List<string>? Notes { get; set; }
}

public sealed class ReportMutationResponse
{
    [JsonPropertyName("ok")]
    public bool? Ok { get; set; }

    [JsonPropertyName("key")]
    public string? Key { get; set; }
}

public sealed class ParameterOption
{
    [JsonPropertyName("label")]
    public string? Label { get; set; }

    [JsonPropertyName("value")]
    public string? Value { get; set; }
}

public sealed class ParameterLookupResponse
{
    [JsonPropertyName("options")]
    public List<ParameterOption>? Options { get; set; }
}

public sealed class PublishReportRequest
{
    /// <summary>Required for create; optional for update.</summary>
    public Stream? JrxmlFile { get; set; }
    public string JrxmlFileName { get; set; } = "report.jrxml";
    /// <summary>Required for create; optional for update.</summary>
    public string? DataSource { get; set; }
    public Stream? JasperFile { get; set; }
    public string? JasperFileName { get; set; }
    public Stream? ThumbnailFile { get; set; }
    public string? ThumbnailFileName { get; set; }
    public string? ReportName { get; set; }
    public string? ReportDescription { get; set; }
    /// <summary><c>jasper</c> or <c>map</c>.</summary>
    public string? ReportType { get; set; }
    public IEnumerable<string>? AccessGroups { get; set; }
}

public sealed class Breadcrumb
{
    [JsonPropertyName("id")]
    public long? Id { get; set; }

    [JsonPropertyName("name")]
    public string? Name { get; set; }
}

public sealed class Folder
{
    [JsonPropertyName("id")]
    public long? Id { get; set; }

    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("description")]
    public string? Description { get; set; }

    [JsonPropertyName("icon")]
    public string? Icon { get; set; }

    [JsonPropertyName("parent_id")]
    public long? ParentId { get; set; }

    [JsonPropertyName("path")]
    public string? Path { get; set; }

    [JsonPropertyName("sort_order")]
    public int? SortOrder { get; set; }

    [JsonPropertyName("acl_inherit")]
    public bool? AclInherit { get; set; }

    [JsonPropertyName("child_folder_count")]
    public int? ChildFolderCount { get; set; }

    [JsonPropertyName("child_item_count")]
    public int? ChildItemCount { get; set; }

    [JsonPropertyName("created_at")]
    public DateTimeOffset? CreatedAt { get; set; }

    [JsonPropertyName("updated_at")]
    public DateTimeOffset? UpdatedAt { get; set; }

    [JsonPropertyName("created_by")]
    public long? CreatedBy { get; set; }
}

public sealed class FolderTreeNode
{
    [JsonPropertyName("id")]
    public long? Id { get; set; }

    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("icon")]
    public string? Icon { get; set; }

    [JsonPropertyName("parent_id")]
    public long? ParentId { get; set; }

    [JsonPropertyName("children")]
    public List<FolderTreeNode>? Children { get; set; }
}

public sealed class ContentItem
{
    [JsonPropertyName("id")]
    public long? Id { get; set; }

    [JsonPropertyName("content_key")]
    public string? ContentKey { get; set; }

    [JsonPropertyName("content_type")]
    public string? ContentType { get; set; }

    [JsonPropertyName("display_name")]
    public string? DisplayName { get; set; }

    [JsonPropertyName("folder_id")]
    public long? FolderId { get; set; }

    [JsonPropertyName("folder_path")]
    public string? FolderPath { get; set; }

    [JsonPropertyName("acl_inherit")]
    public bool? AclInherit { get; set; }

    [JsonPropertyName("extension")]
    public string? Extension { get; set; }

    [JsonPropertyName("mime_type")]
    public string? MimeType { get; set; }

    [JsonPropertyName("original_filename")]
    public string? OriginalFilename { get; set; }

    [JsonPropertyName("resource_type")]
    public string? ResourceType { get; set; }

    [JsonPropertyName("size_bytes")]
    public long? SizeBytes { get; set; }

    [JsonPropertyName("sort_order")]
    public int? SortOrder { get; set; }

    [JsonPropertyName("width")]
    public int? Width { get; set; }

    [JsonPropertyName("height")]
    public int? Height { get; set; }

    [JsonPropertyName("created_at")]
    public DateTimeOffset? CreatedAt { get; set; }

    [JsonPropertyName("updated_at")]
    public DateTimeOffset? UpdatedAt { get; set; }

    [JsonPropertyName("created_by")]
    public long? CreatedBy { get; set; }
}

public sealed class BrowseResult
{
    [JsonPropertyName("folder")]
    public Folder? Folder { get; set; }

    [JsonPropertyName("folders")]
    public List<Folder>? Folders { get; set; }

    [JsonPropertyName("items")]
    public List<ContentItem>? Items { get; set; }

    [JsonPropertyName("breadcrumbs")]
    public List<Breadcrumb>? Breadcrumbs { get; set; }
}

public sealed class SearchHit
{
    [JsonPropertyName("id")]
    public long? Id { get; set; }

    [JsonPropertyName("kind")]
    public string? Kind { get; set; }

    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("description")]
    public string? Description { get; set; }

    [JsonPropertyName("icon")]
    public string? Icon { get; set; }

    [JsonPropertyName("content_key")]
    public string? ContentKey { get; set; }

    [JsonPropertyName("content_type")]
    public string? ContentType { get; set; }

    [JsonPropertyName("folder_id")]
    public long? FolderId { get; set; }

    [JsonPropertyName("folder_path")]
    public string? FolderPath { get; set; }
}

public sealed class SearchResult
{
    [JsonPropertyName("count")]
    public int? Count { get; set; }

    [JsonPropertyName("items")]
    public List<SearchHit>? Items { get; set; }
}

public sealed class CreateFolderRequest
{
    [JsonPropertyName("name")]
    public string Name { get; set; } = "";

    [JsonPropertyName("description")]
    public string? Description { get; set; }

    [JsonPropertyName("icon")]
    public string? Icon { get; set; }

    [JsonPropertyName("parent_id")]
    public long? ParentId { get; set; }

    [JsonPropertyName("sort_order")]
    public int? SortOrder { get; set; }

    [JsonPropertyName("acl_inherit")]
    public bool? AclInherit { get; set; }
}

public sealed class UpdateFolderRequest
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("description")]
    public string? Description { get; set; }

    [JsonPropertyName("icon")]
    public string? Icon { get; set; }

    [JsonPropertyName("sort_order")]
    public int? SortOrder { get; set; }

    [JsonPropertyName("acl_inherit")]
    public bool? AclInherit { get; set; }
}

public sealed class MoveFolderRequest
{
    [JsonPropertyName("parent_id")]
    public long ParentId { get; set; }

    [JsonPropertyName("sort_order")]
    public int? SortOrder { get; set; }
}

public sealed class MoveContentRequest
{
    [JsonPropertyName("folder_id")]
    public long FolderId { get; set; }

    [JsonPropertyName("content_type")]
    public string ContentType { get; set; } = "";

    [JsonPropertyName("content_key")]
    public string ContentKey { get; set; } = "";

    [JsonPropertyName("display_name")]
    public string? DisplayName { get; set; }

    [JsonPropertyName("sort_order")]
    public int? SortOrder { get; set; }

    [JsonPropertyName("acl_inherit")]
    public bool? AclInherit { get; set; }
}

public sealed class FolderPermissions
{
    [JsonPropertyName("group_names")]
    public List<string>? GroupNames { get; set; }
}

public sealed class SetFolderPermissionsRequest
{
    [JsonPropertyName("group_names")]
    public List<string> GroupNames { get; set; } = new();

    [JsonPropertyName("permission")]
    public string? Permission { get; set; }
}

/// <summary>Access Groups ACL for a saved explorer view or dashboard.</summary>
public sealed class ViewPermissions
{
    [JsonPropertyName("ref")]
    public string? Ref { get; set; }

    [JsonPropertyName("group_names")]
    public List<string>? GroupNames { get; set; }

    [JsonPropertyName("available_groups")]
    public List<string>? AvailableGroups { get; set; }

    [JsonPropertyName("can_edit")]
    public bool? CanEdit { get; set; }

    [JsonPropertyName("resource_type")]
    public string? ResourceType { get; set; }
}

public sealed class SetViewPermissionsRequest
{
    [JsonPropertyName("group_names")]
    public List<string> GroupNames { get; set; } = new();
}

public sealed class FavoriteRequest
{
    [JsonPropertyName("content_type")]
    public string ContentType { get; set; } = "";

    [JsonPropertyName("content_key")]
    public string ContentKey { get; set; } = "";

    [JsonPropertyName("favorite")]
    public bool Favorite { get; set; } = true;
}

public sealed class FavoriteResponse
{
    [JsonPropertyName("ok")]
    public bool? Ok { get; set; }

    [JsonPropertyName("favorite")]
    public bool? Favorite { get; set; }
}

public sealed class FavoritesListResponse
{
    [JsonPropertyName("items")]
    public List<ContentItem>? Items { get; set; }
}

public sealed class Resource
{
    [JsonPropertyName("id")]
    public long? Id { get; set; }

    [JsonPropertyName("uuid")]
    public Guid? Uuid { get; set; }

    [JsonPropertyName("display_name")]
    public string? DisplayName { get; set; }

    [JsonPropertyName("resource_type")]
    public string? ResourceType { get; set; }

    [JsonPropertyName("folder_id")]
    public long? FolderId { get; set; }

    [JsonPropertyName("folder_path")]
    public string? FolderPath { get; set; }

    [JsonPropertyName("mime_type")]
    public string? MimeType { get; set; }

    [JsonPropertyName("extension")]
    public string? Extension { get; set; }

    [JsonPropertyName("original_filename")]
    public string? OriginalFilename { get; set; }

    [JsonPropertyName("storage_filename")]
    public string? StorageFilename { get; set; }

    [JsonPropertyName("resource_path")]
    public string? ResourcePath { get; set; }

    [JsonPropertyName("size_bytes")]
    public long? SizeBytes { get; set; }

    [JsonPropertyName("sha256")]
    public string? Sha256 { get; set; }

    [JsonPropertyName("width")]
    public int? Width { get; set; }

    [JsonPropertyName("height")]
    public int? Height { get; set; }

    [JsonPropertyName("has_alpha")]
    public bool? HasAlpha { get; set; }

    [JsonPropertyName("created_at")]
    public DateTimeOffset? CreatedAt { get; set; }

    [JsonPropertyName("updated_at")]
    public DateTimeOffset? UpdatedAt { get; set; }

    [JsonPropertyName("created_by")]
    public long? CreatedBy { get; set; }
}

public sealed class ResourceListResponse
{
    [JsonPropertyName("items")]
    public List<Resource>? Items { get; set; }
}

public sealed class ImageResourceListResponse
{
    [JsonPropertyName("items")]
    public List<Resource>? Items { get; set; }

    [JsonPropertyName("supported_formats")]
    public List<string>? SupportedFormats { get; set; }
}

public sealed class ResourceCreateJson
{
    [JsonPropertyName("display_name")]
    public string DisplayName { get; set; } = "";

    [JsonPropertyName("resource_type")]
    public string ResourceType { get; set; } = "";

    [JsonPropertyName("content")]
    public string Content { get; set; } = "";

    [JsonPropertyName("folder_id")]
    public long? FolderId { get; set; }

    [JsonPropertyName("mime_type")]
    public string? MimeType { get; set; }

    [JsonPropertyName("extension")]
    public string? Extension { get; set; }

    [JsonPropertyName("original_filename")]
    public string? OriginalFilename { get; set; }
}

public sealed class ResourceUpdateRequest
{
    [JsonPropertyName("display_name")]
    public string? DisplayName { get; set; }

    [JsonPropertyName("folder_id")]
    public long? FolderId { get; set; }

    [JsonPropertyName("resource_type")]
    public string? ResourceType { get; set; }
}

public sealed class ResourceSyncHint
{
    [JsonPropertyName("ok")]
    public bool? Ok { get; set; }

    [JsonPropertyName("hint")]
    public string? Hint { get; set; }

    [JsonPropertyName("service")]
    public string? Service { get; set; }
}

public sealed class ResourceSyncResult
{
    [JsonPropertyName("ok")]
    public bool? Ok { get; set; }

    [JsonPropertyName("service")]
    public string? Service { get; set; }

    [JsonPropertyName("registered")]
    public int? Registered { get; set; }

    [JsonPropertyName("already_exists")]
    public int? AlreadyExists { get; set; }

    [JsonPropertyName("skipped")]
    public int? Skipped { get; set; }

    [JsonPropertyName("errors")]
    public int? Errors { get; set; }

    [JsonPropertyName("reason")]
    public string? Reason { get; set; }
}

public sealed class DatasourceName
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }
}

public sealed class DatasourceUpsertRequest
{
    public string? Name { get; set; }
    public string? DbType { get; set; }
    public string? Host { get; set; }
    public string? Port { get; set; }
    public string? DbName { get; set; }
    public string? SchemaName { get; set; }
    public string? Username { get; set; }
    public string? Password { get; set; }
    public string? AuthType { get; set; }
    public Guid? PrivateKeyResourceUuid { get; set; }
    public Guid? PublicKeyResourceUuid { get; set; }
}

public sealed class DatasourceMutationResponse
{
    [JsonPropertyName("ok")]
    public bool? Ok { get; set; }

    [JsonPropertyName("message")]
    public string? Message { get; set; }
}

public sealed class JasperContextResponse
{
    [JsonPropertyName("ok")]
    public bool? Ok { get; set; }

    [JsonPropertyName("path")]
    public string? Path { get; set; }

    [JsonPropertyName("notes")]
    public List<string>? Notes { get; set; }
}

public sealed class MapLayerMeta
{
    [JsonPropertyName("type")]
    public string? Type { get; set; }

    [JsonPropertyName("table")]
    public string? Table { get; set; }

    [JsonPropertyName("schema")]
    public string? Schema { get; set; }

    [JsonPropertyName("geometry_column")]
    public string? GeometryColumn { get; set; }

    [JsonPropertyName("geometry_columns")]
    public List<string>? GeometryColumns { get; set; }

    [JsonPropertyName("geometry_type")]
    public string? GeometryType { get; set; }

    [JsonPropertyName("srid")]
    public int? Srid { get; set; }

    [JsonPropertyName("spatial_capable")]
    public bool? SpatialCapable { get; set; }

    [JsonPropertyName("report_fields")]
    public List<string>? ReportFields { get; set; }

    [JsonPropertyName("error")]
    public string? Error { get; set; }
}

public sealed class MapColumn
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("type")]
    public string? Type { get; set; }
}

public sealed class MapColumnsResult
{
    [JsonPropertyName("columns")]
    public List<MapColumn>? Columns { get; set; }

    [JsonPropertyName("geometry_column")]
    public string? GeometryColumn { get; set; }

    [JsonPropertyName("geometry_columns")]
    public List<string>? GeometryColumns { get; set; }

    [JsonPropertyName("report_fields")]
    public List<string>? ReportFields { get; set; }

    [JsonPropertyName("schema")]
    public string? Schema { get; set; }

    [JsonPropertyName("table")]
    public string? Table { get; set; }

    [JsonPropertyName("srid")]
    public int? Srid { get; set; }

    [JsonPropertyName("error")]
    public string? Error { get; set; }
}

public sealed class MapExtent
{
    [JsonPropertyName("p_geometry")]
    public string? PGeometry { get; set; }

    [JsonPropertyName("p_srid")]
    public int? PSrid { get; set; }
}

public sealed class FeatureCollection
{
    [JsonPropertyName("type")]
    public string Type { get; set; } = "FeatureCollection";

    [JsonPropertyName("features")]
    public List<Dictionary<string, JsonElement>>? Features { get; set; }

    [JsonPropertyName("meta")]
    public Dictionary<string, JsonElement>? Meta { get; set; }

    [JsonPropertyName("crs")]
    public Dictionary<string, JsonElement>? Crs { get; set; }

    [JsonPropertyName("mapDefinition")]
    public Dictionary<string, JsonElement>? MapDefinition { get; set; }

    [JsonPropertyName("error")]
    public string? Error { get; set; }

    [JsonPropertyName("detail")]
    public string? Detail { get; set; }
}

public sealed class ExplorerColumn
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("type")]
    public string? Type { get; set; }

    [JsonPropertyName("udt_name")]
    public string? UdtName { get; set; }

    [JsonPropertyName("nullable")]
    public bool? Nullable { get; set; }

    [JsonPropertyName("ordinal")]
    public int? Ordinal { get; set; }

    [JsonPropertyName("is_primary")]
    public bool? IsPrimary { get; set; }

    [JsonPropertyName("is_geometry")]
    public bool? IsGeometry { get; set; }
}

public sealed class ExplorerFilter
{
    [JsonPropertyName("column")]
    public string? Column { get; set; }

    [JsonPropertyName("operator")]
    public string? Operator { get; set; }

    [JsonPropertyName("value")]
    public JsonElement? Value { get; set; }

    [JsonPropertyName("value_to")]
    public JsonElement? ValueTo { get; set; }

    [JsonPropertyName("values")]
    public List<JsonElement>? Values { get; set; }
}

public sealed class ExplorerSort
{
    [JsonPropertyName("column")]
    public string? Column { get; set; }

    [JsonPropertyName("direction")]
    public string? Direction { get; set; }
}

public sealed class ExplorerAggregate
{
    [JsonPropertyName("alias")]
    public string? Alias { get; set; }

    [JsonPropertyName("column")]
    public string? Column { get; set; }

    [JsonPropertyName("func")]
    public string? Func { get; set; }
}

public sealed class ExplorerQuery
{
    [JsonPropertyName("datasource")]
    public string? Datasource { get; set; }

    [JsonPropertyName("schema")]
    public string? Schema { get; set; }

    [JsonPropertyName("table")]
    public string? Table { get; set; }

    [JsonPropertyName("columns")]
    public List<string>? Columns { get; set; }

    [JsonPropertyName("filters")]
    public List<ExplorerFilter>? Filters { get; set; }

    [JsonPropertyName("sort")]
    public List<ExplorerSort>? Sort { get; set; }

    [JsonPropertyName("group_by")]
    public List<string>? GroupBy { get; set; }

    [JsonPropertyName("aggregates")]
    public List<ExplorerAggregate>? Aggregates { get; set; }

    [JsonPropertyName("limit")]
    public int? Limit { get; set; }

    [JsonPropertyName("offset")]
    public int? Offset { get; set; }

    [JsonPropertyName("page")]
    public int? Page { get; set; }

    [JsonPropertyName("page_size")]
    public int? PageSize { get; set; }

    [JsonPropertyName("source_mode")]
    public string? SourceMode { get; set; }

    [JsonPropertyName("report_key")]
    public string? ReportKey { get; set; }

    [JsonPropertyName("report_cache_key")]
    public string? ReportCacheKey { get; set; }
}

public sealed class ExplorerResult
{
    [JsonPropertyName("columns")]
    public List<string>? Columns { get; set; }

    [JsonPropertyName("rows")]
    public List<Dictionary<string, JsonElement>>? Rows { get; set; }

    [JsonPropertyName("engine")]
    public string? Engine { get; set; }

    [JsonPropertyName("executed_at")]
    public DateTimeOffset? ExecutedAt { get; set; }

    [JsonPropertyName("page")]
    public int? Page { get; set; }

    [JsonPropertyName("page_size")]
    public int? PageSize { get; set; }

    [JsonPropertyName("total_rows")]
    public long? TotalRows { get; set; }

    [JsonPropertyName("truncated")]
    public bool? Truncated { get; set; }
}

public sealed class TableMeta
{
    [JsonPropertyName("datasource")]
    public string? Datasource { get; set; }

    [JsonPropertyName("schema")]
    public string? Schema { get; set; }

    [JsonPropertyName("table")]
    public string? Table { get; set; }

    [JsonPropertyName("engine")]
    public string? Engine { get; set; }

    [JsonPropertyName("columns")]
    public List<ExplorerColumn>? Columns { get; set; }

    [JsonPropertyName("geometry_column")]
    public string? GeometryColumn { get; set; }

    [JsonPropertyName("primary_key")]
    public List<string>? PrimaryKey { get; set; }

    [JsonPropertyName("spatial_capable")]
    public bool? SpatialCapable { get; set; }

    [JsonPropertyName("srid")]
    public int? Srid { get; set; }
}

public sealed class SavedView
{
    [JsonPropertyName("key")]
    public string? Key { get; set; }

    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("datasource")]
    public string? Datasource { get; set; }

    [JsonPropertyName("schema")]
    public string? Schema { get; set; }

    [JsonPropertyName("table")]
    public string? Table { get; set; }

    [JsonPropertyName("columns")]
    public List<string>? Columns { get; set; }

    [JsonPropertyName("filters")]
    public List<ExplorerFilter>? Filters { get; set; }

    [JsonPropertyName("sort")]
    public List<ExplorerSort>? Sort { get; set; }

    [JsonPropertyName("group_by")]
    public List<string>? GroupBy { get; set; }

    [JsonPropertyName("aggregates")]
    public List<ExplorerAggregate>? Aggregates { get; set; }

    [JsonPropertyName("limit")]
    public int? Limit { get; set; }

    [JsonPropertyName("source_mode")]
    public string? SourceMode { get; set; }

    [JsonPropertyName("report_key")]
    public string? ReportKey { get; set; }

    [JsonPropertyName("report_name")]
    public string? ReportName { get; set; }

    [JsonPropertyName("owner_username")]
    public string? OwnerUsername { get; set; }

    [JsonPropertyName("dashboard_widget")]
    public bool? DashboardWidget { get; set; }

    [JsonPropertyName("chart")]
    public Dictionary<string, JsonElement>? Chart { get; set; }

    [JsonPropertyName("map")]
    public Dictionary<string, JsonElement>? Map { get; set; }

    [JsonPropertyName("panel_layout")]
    public Dictionary<string, JsonElement>? PanelLayout { get; set; }

    [JsonPropertyName("widgets")]
    public List<Dictionary<string, JsonElement>>? Widgets { get; set; }

    [JsonPropertyName("created_at")]
    public DateTimeOffset? CreatedAt { get; set; }

    [JsonPropertyName("updated_at")]
    public DateTimeOffset? UpdatedAt { get; set; }
}

public sealed class ExplorerDatasourceInfo
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("db_type")]
    public string? DbType { get; set; }

    [JsonPropertyName("engine")]
    public string? Engine { get; set; }

    [JsonPropertyName("interactive_ok")]
    public bool? InteractiveOk { get; set; }

    [JsonPropertyName("unsupported_message")]
    public string? UnsupportedMessage { get; set; }
}

public sealed class ExplorerDatasourcesResponse
{
    [JsonPropertyName("datasources")]
    public List<ExplorerDatasourceInfo>? Datasources { get; set; }
}

public sealed class ExplorerTableInfo
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("schema")]
    public string? Schema { get; set; }

    [JsonPropertyName("type")]
    public string? Type { get; set; }
}

public sealed class ExplorerTablesResponse
{
    [JsonPropertyName("engine")]
    public string? Engine { get; set; }

    [JsonPropertyName("tables")]
    public List<ExplorerTableInfo>? Tables { get; set; }
}

public sealed class ExplorerViewsResponse
{
    [JsonPropertyName("views")]
    public List<SavedView>? Views { get; set; }
}

public sealed class DashboardWidgetsResponse
{
    [JsonPropertyName("widgets")]
    public List<SavedView>? Widgets { get; set; }
}

public sealed class ExplorerHandoffResponse
{
    [JsonPropertyName("handoff")]
    public Dictionary<string, JsonElement>? Handoff { get; set; }

    [JsonPropertyName("note")]
    public string? Note { get; set; }

    [JsonPropertyName("publish_url")]
    public string? PublishUrl { get; set; }
}

public sealed class ExplorerReportQueryRequest
{
    [JsonPropertyName("report_key")]
    public string ReportKey { get; set; } = "";

    [JsonPropertyName("datasource")]
    public string? Datasource { get; set; }

    [JsonPropertyName("max_rows")]
    public int? MaxRows { get; set; }

    [JsonPropertyName("parameters")]
    public Dictionary<string, List<string>>? Parameters { get; set; }
}

public sealed class ExplorerReportQueryResponse
{
    [JsonPropertyName("cache_key")]
    public string? CacheKey { get; set; }

    [JsonPropertyName("meta")]
    public TableMeta? Meta { get; set; }

    [JsonPropertyName("total_rows")]
    public long? TotalRows { get; set; }

    [JsonPropertyName("truncated")]
    public bool? Truncated { get; set; }
}

public sealed class ExplorerMapSource
{
    [JsonPropertyName("id")]
    public string? Id { get; set; }

    [JsonPropertyName("kind")]
    public string? Kind { get; set; }

    [JsonPropertyName("label")]
    public string? Label { get; set; }

    [JsonPropertyName("description")]
    public string? Description { get; set; }

    [JsonPropertyName("feature_ops")]
    public bool? FeatureOps { get; set; }

    [JsonPropertyName("needs_geometry")]
    public bool? NeedsGeometry { get; set; }
}

public sealed class ExplorerMapSourcesResponse
{
    [JsonPropertyName("sources")]
    public List<ExplorerMapSource>? Sources { get; set; }

    [JsonPropertyName("connections")]
    public Dictionary<string, JsonElement>? Connections { get; set; }

    [JsonPropertyName("spatial")]
    public List<Dictionary<string, JsonElement>>? Spatial { get; set; }
}

public sealed class ExplorerMapTablesResponse
{
    [JsonPropertyName("engine")]
    public string? Engine { get; set; }

    [JsonPropertyName("tables")]
    public List<Dictionary<string, JsonElement>>? Tables { get; set; }
}

public sealed class SpatialCapabilitiesResponse
{
    [JsonPropertyName("engine")]
    public string? Engine { get; set; }

    [JsonPropertyName("provider")]
    public string? Provider { get; set; }

    [JsonPropertyName("source_type")]
    public string? SourceType { get; set; }

    [JsonPropertyName("source_types")]
    public List<string>? SourceTypes { get; set; }

    [JsonPropertyName("supported_analyses")]
    public List<string>? SupportedAnalyses { get; set; }
}

public sealed class SpatialValidateResponse
{
    [JsonPropertyName("ok")]
    public bool? Ok { get; set; }

    [JsonPropertyName("engine")]
    public string? Engine { get; set; }

    [JsonPropertyName("provider")]
    public string? Provider { get; set; }

    [JsonPropertyName("geometry_column")]
    public string? GeometryColumn { get; set; }

    [JsonPropertyName("srid")]
    public int? Srid { get; set; }

    [JsonPropertyName("error")]
    public string? Error { get; set; }

    [JsonPropertyName("supported_analyses")]
    public List<string>? SupportedAnalyses { get; set; }

    [JsonPropertyName("warnings")]
    public List<string>? Warnings { get; set; }
}

public sealed class DashboardWidgetFlagResponse
{
    [JsonPropertyName("status")]
    public string? Status { get; set; }

    [JsonPropertyName("widget")]
    public DashboardWidgetRef? Widget { get; set; }
}

public sealed class DashboardWidgetRef
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("owner_username")]
    public string? OwnerUsername { get; set; }

    [JsonPropertyName("ref")]
    public string? Ref { get; set; }

    [JsonPropertyName("type")]
    public string? Type { get; set; }

    [JsonPropertyName("view_key")]
    public string? ViewKey { get; set; }
}

public sealed class DeleteStatusResponse
{
    [JsonPropertyName("status")]
    public string? Status { get; set; }
}

public sealed class Diagnostics
{
    [JsonPropertyName("app_version")]
    public string? AppVersion { get; set; }

    [JsonPropertyName("base_path")]
    public string? BasePath { get; set; }

    [JsonPropertyName("environment")]
    public string? Environment { get; set; }

    [JsonPropertyName("go_version")]
    public string? GoVersion { get; set; }

    [JsonPropertyName("service")]
    public string? Service { get; set; }

    [JsonPropertyName("timestamp")]
    public DateTimeOffset? Timestamp { get; set; }

    [JsonExtensionData]
    public Dictionary<string, JsonElement>? ExtensionData { get; set; }
}
