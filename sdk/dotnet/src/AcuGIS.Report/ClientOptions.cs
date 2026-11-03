namespace AcuGIS.Report;

/// <summary>
/// Configuration for <see cref="ReportServerClient"/>.
/// </summary>
public sealed class ClientOptions
{
    /// <summary>Environment variable for the report server base URL.</summary>
    public const string UrlEnvironmentVariable = "ACUGIS_REPORT_URL";

    /// <summary>Environment variable for the personal access token.</summary>
    public const string TokenEnvironmentVariable = "ACUGIS_REPORT_TOKEN";

    /// <summary>SDK User-Agent header value.</summary>
    public const string DefaultUserAgent = "AcuGIS.Report/0.1.0";

    /// <summary>Base URL of the AcuGIS Report Server (scheme + host[+port][+path prefix]).</summary>
    public string BaseUrl { get; set; } = "";

    /// <summary>Personal access token (<c>jrs_…</c>). Sent as <c>Authorization: Bearer</c>.</summary>
    public string? Token { get; set; }

    /// <summary>Request timeout. Default 100 seconds.</summary>
    public TimeSpan Timeout { get; set; } = TimeSpan.FromSeconds(100);

    /// <summary>
    /// When <c>false</c>, TLS certificate validation is skipped (dev only).
    /// Ignored when an external <see cref="HttpClient"/> or handler is supplied.
    /// </summary>
    public bool VerifyTls { get; set; } = true;

    /// <summary>Optional extra headers applied to every request.</summary>
    public IDictionary<string, string> DefaultHeaders { get; set; } =
        new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);

    /// <summary>User-Agent header. Defaults to <see cref="DefaultUserAgent"/>.</summary>
    public string UserAgent { get; set; } = DefaultUserAgent;

    /// <summary>
    /// When greater than 0, failed GET requests are retried this many times
    /// (exponential backoff). Mutations are never retried.
    /// </summary>
    public int MaxGetRetries { get; set; }

    /// <summary>Initial backoff for GET retries. Default 200ms.</summary>
    public TimeSpan RetryBackoff { get; set; } = TimeSpan.FromMilliseconds(200);

    /// <summary>
    /// Load <see cref="BaseUrl"/> and <see cref="Token"/> from
    /// <c>ACUGIS_REPORT_URL</c> / <c>ACUGIS_REPORT_TOKEN</c>.
    /// </summary>
    public static ClientOptions FromEnvironment()
    {
        var url = Environment.GetEnvironmentVariable(UrlEnvironmentVariable);
        var token = Environment.GetEnvironmentVariable(TokenEnvironmentVariable);
        if (string.IsNullOrWhiteSpace(url))
        {
            throw new InvalidOperationException(
                $"Environment variable {UrlEnvironmentVariable} is required.");
        }

        return new ClientOptions
        {
            BaseUrl = url.Trim().TrimEnd('/'),
            Token = string.IsNullOrWhiteSpace(token) ? null : token.Trim(),
        };
    }

    internal Uri GetBaseAddress()
    {
        if (string.IsNullOrWhiteSpace(BaseUrl))
        {
            throw new InvalidOperationException("BaseUrl is required.");
        }

        var trimmed = BaseUrl.Trim().TrimEnd('/') + "/";
        if (!Uri.TryCreate(trimmed, UriKind.Absolute, out var uri) ||
            (uri.Scheme != Uri.UriSchemeHttp && uri.Scheme != Uri.UriSchemeHttps))
        {
            throw new InvalidOperationException($"Invalid BaseUrl: {BaseUrl}");
        }

        return uri;
    }
}
