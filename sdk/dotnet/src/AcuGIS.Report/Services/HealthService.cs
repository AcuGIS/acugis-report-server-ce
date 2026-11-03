using AcuGIS.Report.Internal;
using AcuGIS.Report.Models;

namespace AcuGIS.Report.Services;

/// <summary>Liveness, readiness, and diagnostics.</summary>
public sealed class HealthService
{
    private readonly HttpTransport _http;

    internal HealthService(HttpTransport http) => _http = http;

    /// <summary>GET /health</summary>
    public Task<HealthResponse> GetHealthAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<HealthResponse>(HttpMethod.Get, "health", cancellationToken: cancellationToken);

    /// <summary>
    /// GET /ready — returns the body for both 200 and 503 (not ready).
    /// </summary>
    public async Task<ReadyResponse> GetReadyAsync(CancellationToken cancellationToken = default)
    {
        using var response = await _http.SendRawAsync(
                HttpMethod.Get,
                "ready",
                ensureSuccess: false,
                cancellationToken: cancellationToken)
            .ConfigureAwait(false);
        if ((int)response.StatusCode is not (200 or 503))
        {
            await HttpTransport.EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
        }

        return await HttpTransport.ReadJsonAsync<ReadyResponse>(response, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>GET /api/v1/diagnostics</summary>
    public Task<Diagnostics> GetDiagnosticsAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Diagnostics>(HttpMethod.Get, "api/v1/diagnostics", cancellationToken: cancellationToken);
}
