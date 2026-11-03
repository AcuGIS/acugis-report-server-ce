using System.Text.Json;
using AcuGIS.Report.Internal;

namespace AcuGIS.Report.Services;

/// <summary>Embedded map debug (<c>/api/v1/embedded-map/debug</c>).</summary>
public sealed class EmbeddedMapsService
{
    private readonly HttpTransport _http;

    internal EmbeddedMapsService(HttpTransport http) => _http = http;

    /// <summary>
    /// GET /api/v1/embedded-map/debug — JSON debug payload when <paramref name="format"/> is <c>json</c>.
    /// </summary>
    public Task<JsonElement> DebugAsync(
        string report,
        string? format = "json",
        string? compile = null,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            "api/v1/embedded-map/debug",
            new[]
            {
                ("report", report),
                ("format", format),
                ("compile", compile),
            });
        return _http.SendJsonAsync<JsonElement>(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }

    /// <summary>GET /api/v1/embedded-map/debug — raw HTML or JSON stream.</summary>
    public Task<Stream> DebugStreamAsync(
        string report,
        string? format = null,
        string? compile = null,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            "api/v1/embedded-map/debug",
            new[]
            {
                ("report", report),
                ("format", format),
                ("compile", compile),
            });
        return _http.SendStreamAsync(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }
}
