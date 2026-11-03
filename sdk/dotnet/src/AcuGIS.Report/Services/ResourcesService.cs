using AcuGIS.Report.Internal;
using AcuGIS.Report.Models;

namespace AcuGIS.Report.Services;

/// <summary>Content resources, images, and resource sync.</summary>
public sealed class ResourcesService
{
    private readonly HttpTransport _http;

    internal ResourcesService(HttpTransport http) => _http = http;

    /// <summary>GET /api/v1/resources</summary>
    public Task<ResourceListResponse> ListAsync(
        long? folderId = null,
        string? resourceType = null,
        string? q = null,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            "api/v1/resources",
            new[]
            {
                ("folder_id", folderId is null ? null : QueryString.Format(folderId)),
                ("resource_type", resourceType),
                ("q", q),
            });
        return _http.SendJsonAsync<ResourceListResponse>(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }

    /// <summary>GET /api/v1/resources/{uuid}</summary>
    public Task<Resource> GetAsync(Guid uuid, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Resource>(
            HttpMethod.Get,
            $"api/v1/resources/{uuid:D}",
            cancellationToken: cancellationToken);

    /// <summary>POST /api/v1/resources (JSON)</summary>
    public Task<Resource> CreateAsync(ResourceCreateJson request, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Resource>(HttpMethod.Post, "api/v1/resources", request, cancellationToken);

    /// <summary>POST /api/v1/resources (multipart file)</summary>
    public async Task<Resource> CreateFromFileAsync(
        Stream file,
        string fileName,
        string displayName,
        string resourceType,
        long? folderId = null,
        string? mimeType = null,
        string? extension = null,
        CancellationToken cancellationToken = default)
    {
        var content = new MultipartFormDataContent();
        content.Add(new StreamContent(file), "file", fileName);
        content.Add(new StringContent(displayName), "display_name");
        content.Add(new StringContent(resourceType), "resource_type");
        if (folderId is not null)
        {
            content.Add(new StringContent(QueryString.Format(folderId)), "folder_id");
        }

        if (!string.IsNullOrEmpty(mimeType))
        {
            content.Add(new StringContent(mimeType), "mime_type");
        }

        if (!string.IsNullOrEmpty(extension))
        {
            content.Add(new StringContent(extension), "extension");
        }

        using var response = await _http.SendRawAsync(
                HttpMethod.Post,
                "api/v1/resources",
                content,
                cancellationToken: cancellationToken)
            .ConfigureAwait(false);
        return await HttpTransport.ReadJsonAsync<Resource>(response, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>PUT /api/v1/resources/{uuid}</summary>
    public Task<Resource> UpdateAsync(
        Guid uuid,
        ResourceUpdateRequest request,
        CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Resource>(
            HttpMethod.Put,
            $"api/v1/resources/{uuid:D}",
            request,
            cancellationToken);

    /// <summary>DELETE /api/v1/resources/{uuid}</summary>
    public Task<SuccessResponse> DeleteAsync(Guid uuid, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<SuccessResponse>(
            HttpMethod.Delete,
            $"api/v1/resources/{uuid:D}",
            cancellationToken: cancellationToken);

    /// <summary>PUT /api/v1/resources/{uuid}/content</summary>
    public async Task<Resource> ReplaceContentAsync(
        Guid uuid,
        Stream file,
        string fileName,
        string? mimeType = null,
        string? extension = null,
        CancellationToken cancellationToken = default)
    {
        var content = new MultipartFormDataContent();
        content.Add(new StreamContent(file), "file", fileName);
        if (!string.IsNullOrEmpty(mimeType))
        {
            content.Add(new StringContent(mimeType), "mime_type");
        }

        if (!string.IsNullOrEmpty(extension))
        {
            content.Add(new StringContent(extension), "extension");
        }

        using var response = await _http.SendRawAsync(
                HttpMethod.Put,
                $"api/v1/resources/{uuid:D}/content",
                content,
                cancellationToken: cancellationToken)
            .ConfigureAwait(false);
        return await HttpTransport.ReadJsonAsync<Resource>(response, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>GET /api/v1/resources/{uuid}/download</summary>
    public Task<Stream> DownloadAsync(
        Guid uuid,
        bool inline = false,
        string? disposition = null,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            $"api/v1/resources/{uuid:D}/download",
            new[]
            {
                ("inline", inline ? "1" : null),
                ("disposition", disposition),
            });
        return _http.SendStreamAsync(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }

    /// <summary>Download resource bytes to a local file.</summary>
    public Task DownloadToFileAsync(
        Guid uuid,
        string filePath,
        bool inline = false,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            $"api/v1/resources/{uuid:D}/download",
            new[] { ("inline", inline ? "1" : null) });
        return _http.DownloadToFileAsync(HttpMethod.Get, url, filePath, cancellationToken: cancellationToken);
    }

    /// <summary>GET /api/v1/resources/{uuid}/thumbnail</summary>
    public Task<Stream> GetThumbnailAsync(Guid uuid, CancellationToken cancellationToken = default) =>
        _http.SendStreamAsync(
            HttpMethod.Get,
            $"api/v1/resources/{uuid:D}/thumbnail",
            cancellationToken: cancellationToken);

    /// <summary>GET /api/v1/resources/images</summary>
    public Task<ImageResourceListResponse> ListImagesAsync(
        long? folderId = null,
        string? q = null,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            "api/v1/resources/images",
            new[]
            {
                ("folder_id", folderId is null ? null : QueryString.Format(folderId)),
                ("q", q),
            });
        return _http.SendJsonAsync<ImageResourceListResponse>(
            HttpMethod.Get,
            url,
            cancellationToken: cancellationToken);
    }

    /// <summary>POST /api/v1/resources/images</summary>
    public async Task<Resource> UploadImageAsync(
        Stream file,
        string fileName,
        string? displayName = null,
        long? folderId = null,
        string? mimeType = null,
        string? extension = null,
        CancellationToken cancellationToken = default)
    {
        var content = new MultipartFormDataContent();
        content.Add(new StreamContent(file), "file", fileName);
        if (!string.IsNullOrEmpty(displayName))
        {
            content.Add(new StringContent(displayName), "display_name");
        }

        if (folderId is not null)
        {
            content.Add(new StringContent(QueryString.Format(folderId)), "folder_id");
        }

        if (!string.IsNullOrEmpty(mimeType))
        {
            content.Add(new StringContent(mimeType), "mime_type");
        }

        if (!string.IsNullOrEmpty(extension))
        {
            content.Add(new StringContent(extension), "extension");
        }

        using var response = await _http.SendRawAsync(
                HttpMethod.Post,
                "api/v1/resources/images",
                content,
                cancellationToken: cancellationToken)
            .ConfigureAwait(false);
        return await HttpTransport.ReadJsonAsync<Resource>(response, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>GET /api/v1/resources/images/{uuid}</summary>
    public Task<Resource> GetImageAsync(Guid uuid, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Resource>(
            HttpMethod.Get,
            $"api/v1/resources/images/{uuid:D}",
            cancellationToken: cancellationToken);

    /// <summary>PUT /api/v1/resources/images/{uuid}</summary>
    public async Task<Resource> ReplaceImageAsync(
        Guid uuid,
        Stream file,
        string fileName,
        string? mimeType = null,
        string? extension = null,
        CancellationToken cancellationToken = default)
    {
        var content = new MultipartFormDataContent();
        content.Add(new StreamContent(file), "file", fileName);
        if (!string.IsNullOrEmpty(mimeType))
        {
            content.Add(new StringContent(mimeType), "mime_type");
        }

        if (!string.IsNullOrEmpty(extension))
        {
            content.Add(new StringContent(extension), "extension");
        }

        using var response = await _http.SendRawAsync(
                HttpMethod.Put,
                $"api/v1/resources/images/{uuid:D}",
                content,
                cancellationToken: cancellationToken)
            .ConfigureAwait(false);
        return await HttpTransport.ReadJsonAsync<Resource>(response, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>DELETE /api/v1/resources/images/{uuid}</summary>
    public Task<SuccessResponse> DeleteImageAsync(Guid uuid, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<SuccessResponse>(
            HttpMethod.Delete,
            $"api/v1/resources/images/{uuid:D}",
            cancellationToken: cancellationToken);

    /// <summary>GET /api/v1/resources/images/{uuid}/download</summary>
    public Task<Stream> DownloadImageAsync(
        Guid uuid,
        bool inline = false,
        string? disposition = null,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            $"api/v1/resources/images/{uuid:D}/download",
            new[]
            {
                ("inline", inline ? "1" : null),
                ("disposition", disposition),
            });
        return _http.SendStreamAsync(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }

    /// <summary>Download image bytes to a local file.</summary>
    public Task DownloadImageToFileAsync(
        Guid uuid,
        string filePath,
        CancellationToken cancellationToken = default) =>
        _http.DownloadToFileAsync(
            HttpMethod.Get,
            $"api/v1/resources/images/{uuid:D}/download",
            filePath,
            cancellationToken: cancellationToken);

    /// <summary>GET /api/v1/resource-sync</summary>
    public Task<ResourceSyncHint> GetSyncHintAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<ResourceSyncHint>(
            HttpMethod.Get,
            "api/v1/resource-sync",
            cancellationToken: cancellationToken);

    /// <summary>POST /api/v1/resource-sync</summary>
    public Task<ResourceSyncResult> RunSyncAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<ResourceSyncResult>(
            HttpMethod.Post,
            "api/v1/resource-sync",
            cancellationToken: cancellationToken);
}
