using System.Net.Http.Headers;
using AcuGIS.Report.Internal;
using AcuGIS.Report.Models;

namespace AcuGIS.Report.Services;

/// <summary>
/// Datasources (<c>/api/v1/datasources</c>).
/// List returns names only; create is form-urlencoded upsert; no GET-by-id.
/// </summary>
public sealed class DatasourcesService
{
    private readonly HttpTransport _http;

    internal DatasourcesService(HttpTransport http) => _http = http;

    /// <summary>GET /api/v1/datasources</summary>
    public Task<List<DatasourceName>> ListAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<List<DatasourceName>>(
            HttpMethod.Get,
            "api/v1/datasources",
            cancellationToken: cancellationToken);

    /// <summary>POST /api/v1/datasources — application/x-www-form-urlencoded upsert.</summary>
    public async Task<DatasourceMutationResponse> CreateAsync(
        DatasourceUpsertRequest request,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        var content = BuildForm(request);
        using var response = await _http.SendRawAsync(
                HttpMethod.Post,
                "api/v1/datasources",
                content,
                cancellationToken: cancellationToken)
            .ConfigureAwait(false);
        return await HttpTransport.ReadJsonAsync<DatasourceMutationResponse>(response, cancellationToken)
            .ConfigureAwait(false);
    }

    /// <summary>DELETE /api/v1/datasources/{id}</summary>
    public Task<DatasourceMutationResponse> DeleteAsync(long id, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<DatasourceMutationResponse>(
            HttpMethod.Delete,
            $"api/v1/datasources/{id}",
            cancellationToken: cancellationToken);

    /// <summary>POST /api/v1/datasources/context</summary>
    public Task<JasperContextResponse> GenerateContextAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<JasperContextResponse>(
            HttpMethod.Post,
            "api/v1/datasources/context",
            cancellationToken: cancellationToken);

    private static FormUrlEncodedContent BuildForm(DatasourceUpsertRequest request)
    {
        var pairs = new List<KeyValuePair<string, string>>();
        void Add(string key, string? value)
        {
            if (!string.IsNullOrEmpty(value))
            {
                pairs.Add(new KeyValuePair<string, string>(key, value));
            }
        }

        Add("name", request.Name);
        Add("db_type", request.DbType);
        Add("host", request.Host);
        Add("port", request.Port);
        Add("dbname", request.DbName);
        Add("schema_name", request.SchemaName);
        Add("username", request.Username);
        Add("password", request.Password);
        Add("auth_type", request.AuthType);
        if (request.PrivateKeyResourceUuid is Guid priv)
        {
            Add("private_key_resource_uuid", priv.ToString("D"));
        }

        if (request.PublicKeyResourceUuid is Guid pub)
        {
            Add("public_key_resource_uuid", pub.ToString("D"));
        }

        var content = new FormUrlEncodedContent(pairs);
        content.Headers.ContentType = new MediaTypeHeaderValue("application/x-www-form-urlencoded");
        return content;
    }
}
