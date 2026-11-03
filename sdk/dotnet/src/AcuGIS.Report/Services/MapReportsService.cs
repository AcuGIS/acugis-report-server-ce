using AcuGIS.Report.Internal;
using AcuGIS.Report.Models;

namespace AcuGIS.Report.Services;

/// <summary>Map report APIs (<c>/api/v1/map/{key}/*</c>).</summary>
public sealed class MapReportsService
{
    private readonly HttpTransport _http;

    internal MapReportsService(HttpTransport http) => _http = http;

    /// <summary>GET /api/v1/map/{key}/columns</summary>
    public Task<MapColumnsResult> GetColumnsAsync(string key, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<MapColumnsResult>(
            HttpMethod.Get,
            $"api/v1/map/{Uri.EscapeDataString(key)}/columns",
            cancellationToken: cancellationToken);

    /// <summary>GET /api/v1/map/{key}/extent</summary>
    public Task<MapExtent> GetExtentAsync(
        string key,
        int srid,
        string? geometry = null,
        double? xmin = null,
        double? ymin = null,
        double? xmax = null,
        double? ymax = null,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            $"api/v1/map/{Uri.EscapeDataString(key)}/extent",
            new[]
            {
                ("srid", QueryString.Format(srid)),
                ("geometry", geometry),
                ("xmin", xmin is null ? null : QueryString.Format(xmin)),
                ("ymin", ymin is null ? null : QueryString.Format(ymin)),
                ("xmax", xmax is null ? null : QueryString.Format(xmax)),
                ("ymax", ymax is null ? null : QueryString.Format(ymax)),
            });
        return _http.SendJsonAsync<MapExtent>(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }

    /// <summary>GET /api/v1/map/{key}/features</summary>
    public Task<FeatureCollection> GetFeaturesAsync(
        string key,
        IEnumerable<KeyValuePair<string, object?>>? parameters = null,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            $"api/v1/map/{Uri.EscapeDataString(key)}/features",
            QueryString.FromReportParameters(parameters));
        return _http.SendJsonAsync<FeatureCollection>(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }
}
