using AcuGIS.Report.Internal;
using AcuGIS.Report.Models;

namespace AcuGIS.Report.Services;

/// <summary>Folders, browse, favorites, and content placement.</summary>
public sealed class FoldersService
{
    private readonly HttpTransport _http;

    internal FoldersService(HttpTransport http) => _http = http;

    /// <summary>GET /api/v1/folders/tree</summary>
    public Task<List<FolderTreeNode>> GetTreeAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<List<FolderTreeNode>>(
            HttpMethod.Get,
            "api/v1/folders/tree",
            cancellationToken: cancellationToken);

    /// <summary>GET /api/v1/folders/browse</summary>
    public Task<BrowseResult> BrowseAsync(long folderId = 0, CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            "api/v1/folders/browse",
            new[] { ("folder_id", QueryString.Format(folderId)) });
        return _http.SendJsonAsync<BrowseResult>(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }

    /// <summary>GET /api/v1/folders/search</summary>
    public Task<SearchResult> SearchAsync(string q, CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build("api/v1/folders/search", new[] { ("q", q) });
        return _http.SendJsonAsync<SearchResult>(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }

    /// <summary>GET /api/v1/folders/{id}</summary>
    public Task<Folder> GetAsync(long id, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Folder>(HttpMethod.Get, $"api/v1/folders/{id}", cancellationToken: cancellationToken);

    /// <summary>POST /api/v1/folders</summary>
    public Task<Folder> CreateAsync(CreateFolderRequest request, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Folder>(HttpMethod.Post, "api/v1/folders", request, cancellationToken);

    /// <summary>PUT /api/v1/folders/{id}</summary>
    public Task<Folder> UpdateAsync(
        long id,
        UpdateFolderRequest request,
        CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Folder>(HttpMethod.Put, $"api/v1/folders/{id}", request, cancellationToken);

    /// <summary>DELETE /api/v1/folders/{id}</summary>
    public Task<SuccessResponse> DeleteAsync(
        long id,
        bool recursive = false,
        bool confirm = false,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            $"api/v1/folders/{id}",
            new[]
            {
                ("recursive", recursive ? "true" : null),
                ("confirm", confirm ? "true" : null),
            });
        return _http.SendJsonAsync<SuccessResponse>(HttpMethod.Delete, url, cancellationToken: cancellationToken);
    }

    /// <summary>POST /api/v1/folders/{id}/move</summary>
    public Task<Folder> MoveAsync(
        long id,
        MoveFolderRequest request,
        CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Folder>(HttpMethod.Post, $"api/v1/folders/{id}/move", request, cancellationToken);

    /// <summary>POST /api/v1/folders/{id}/duplicate</summary>
    public Task<Folder> DuplicateAsync(long id, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Folder>(
            HttpMethod.Post,
            $"api/v1/folders/{id}/duplicate",
            cancellationToken: cancellationToken);

    /// <summary>GET /api/v1/folders/{id}/permissions</summary>
    public Task<FolderPermissions> GetPermissionsAsync(long id, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<FolderPermissions>(
            HttpMethod.Get,
            $"api/v1/folders/{id}/permissions",
            cancellationToken: cancellationToken);

    /// <summary>PUT /api/v1/folders/{id}/permissions</summary>
    public Task<SuccessResponse> SetPermissionsAsync(
        long id,
        SetFolderPermissionsRequest request,
        CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<SuccessResponse>(
            HttpMethod.Put,
            $"api/v1/folders/{id}/permissions",
            request,
            cancellationToken);

    /// <summary>POST /api/v1/content/move</summary>
    public Task<ContentItem> MoveContentAsync(
        MoveContentRequest request,
        CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<ContentItem>(HttpMethod.Post, "api/v1/content/move", request, cancellationToken);

    /// <summary>GET /api/v1/favorites</summary>
    public Task<FavoritesListResponse> ListFavoritesAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<FavoritesListResponse>(
            HttpMethod.Get,
            "api/v1/favorites",
            cancellationToken: cancellationToken);

    /// <summary>POST /api/v1/favorites</summary>
    public Task<FavoriteResponse> SetFavoriteAsync(
        FavoriteRequest request,
        CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<FavoriteResponse>(HttpMethod.Post, "api/v1/favorites", request, cancellationToken);
}
