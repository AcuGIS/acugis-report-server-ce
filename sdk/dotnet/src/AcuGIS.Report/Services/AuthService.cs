using AcuGIS.Report.Internal;
using AcuGIS.Report.Models;

namespace AcuGIS.Report.Services;

/// <summary>Authentication and personal access tokens (<c>/api/v1/auth/*</c>).</summary>
public sealed class AuthService
{
    private readonly HttpTransport _http;

    internal AuthService(HttpTransport http) => _http = http;

    /// <summary>GET /api/v1/auth/me</summary>
    public Task<User> MeAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<User>(HttpMethod.Get, "api/v1/auth/me", cancellationToken: cancellationToken);

    /// <summary>POST /api/v1/auth/login</summary>
    public Task<SuccessResponse> LoginAsync(LoginRequest request, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<SuccessResponse>(HttpMethod.Post, "api/v1/auth/login", request, cancellationToken);

    /// <summary>POST /api/v1/auth/logout</summary>
    public Task<SuccessResponse> LogoutAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<SuccessResponse>(HttpMethod.Post, "api/v1/auth/logout", cancellationToken: cancellationToken);

    /// <summary>GET /api/v1/auth/tokens</summary>
    public Task<TokenListResponse> ListTokensAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<TokenListResponse>(HttpMethod.Get, "api/v1/auth/tokens", cancellationToken: cancellationToken);

    /// <summary>POST /api/v1/auth/tokens — plaintext token returned once.</summary>
    public Task<Token> CreateTokenAsync(CreateTokenRequest request, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Token>(HttpMethod.Post, "api/v1/auth/tokens", request, cancellationToken);

    /// <summary>PUT /api/v1/auth/tokens/{id}</summary>
    public Task<TokenMetadata> UpdateTokenAsync(
        long id,
        UpdateTokenRequest request,
        CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<TokenMetadata>(
            HttpMethod.Put,
            $"api/v1/auth/tokens/{id}",
            request,
            cancellationToken);

    /// <summary>DELETE /api/v1/auth/tokens/{id}</summary>
    public Task DeleteTokenAsync(long id, bool purge = false, CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            $"api/v1/auth/tokens/{id}",
            new[] { ("purge", purge ? "1" : null) });
        return _http.SendNoContentAsync(HttpMethod.Delete, url, cancellationToken: cancellationToken);
    }

    /// <summary>POST /api/v1/auth/tokens/{id}/rotate</summary>
    public Task<Token> RotateTokenAsync(long id, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Token>(
            HttpMethod.Post,
            $"api/v1/auth/tokens/{id}/rotate",
            cancellationToken: cancellationToken);
}
