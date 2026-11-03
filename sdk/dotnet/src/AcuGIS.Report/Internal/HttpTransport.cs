using System.Net;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using AcuGIS.Report.Errors;

namespace AcuGIS.Report.Internal;

/// <summary>Low-level HTTP transport matching the OpenAPI contract.</summary>
internal sealed class HttpTransport : IDisposable
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true,
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.WhenWritingNull,
    };

    private readonly HttpClient _http;
    private readonly ClientOptions _options;
    private readonly bool _ownsClient;

    public HttpTransport(HttpClient http, ClientOptions options, bool ownsClient)
    {
        _http = http ?? throw new ArgumentNullException(nameof(http));
        _options = options ?? throw new ArgumentNullException(nameof(options));
        _ownsClient = ownsClient;

        if (_http.BaseAddress is null)
        {
            _http.BaseAddress = _options.GetBaseAddress();
        }

        if (_http.Timeout == Timeout.InfiniteTimeSpan || _http.Timeout == TimeSpan.FromSeconds(100))
        {
            _http.Timeout = _options.Timeout;
        }

        if (!_http.DefaultRequestHeaders.UserAgent.Any())
        {
            _http.DefaultRequestHeaders.TryAddWithoutValidation("User-Agent", _options.UserAgent);
        }

        if (!_http.DefaultRequestHeaders.Accept.Any())
        {
            _http.DefaultRequestHeaders.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));
        }

        if (!string.IsNullOrWhiteSpace(_options.Token) &&
            _http.DefaultRequestHeaders.Authorization is null)
        {
            _http.DefaultRequestHeaders.Authorization =
                new AuthenticationHeaderValue("Bearer", _options.Token);
        }

        foreach (var header in _options.DefaultHeaders)
        {
            if (!_http.DefaultRequestHeaders.Contains(header.Key))
            {
                _http.DefaultRequestHeaders.TryAddWithoutValidation(header.Key, header.Value);
            }
        }
    }

    public static JsonSerializerOptions SerializerOptions => JsonOptions;

    public async Task<T> SendJsonAsync<T>(
        HttpMethod method,
        string relativeUrl,
        object? body = null,
        CancellationToken cancellationToken = default)
    {
        HttpContent? content = body is null
            ? null
            : new StringContent(JsonSerializer.Serialize(body, body.GetType(), JsonOptions), Encoding.UTF8, "application/json");
        using var response = await SendCoreAsync(method, relativeUrl, content, disposeContent: true, cancellationToken)
            .ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
        return await ReadJsonAsync<T>(response, cancellationToken).ConfigureAwait(false);
    }

    public async Task SendNoContentAsync(
        HttpMethod method,
        string relativeUrl,
        HttpContent? content = null,
        CancellationToken cancellationToken = default)
    {
        using var response = await SendCoreAsync(method, relativeUrl, content, disposeContent: true, cancellationToken)
            .ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
    }

    public async Task<HttpResponseMessage> SendRawAsync(
        HttpMethod method,
        string relativeUrl,
        HttpContent? content = null,
        bool ensureSuccess = true,
        CancellationToken cancellationToken = default)
    {
        var response = await SendCoreAsync(method, relativeUrl, content, disposeContent: true, cancellationToken)
            .ConfigureAwait(false);
        if (ensureSuccess)
        {
            await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
        }

        return response;
    }

    public async Task<Stream> SendStreamAsync(
        HttpMethod method,
        string relativeUrl,
        HttpContent? content = null,
        CancellationToken cancellationToken = default)
    {
        var response = await SendCoreAsync(method, relativeUrl, content, disposeContent: true, cancellationToken)
            .ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
        var stream = await response.Content.ReadAsStreamAsync(cancellationToken).ConfigureAwait(false);
        return new ResponseStream(stream, response);
    }

    public async Task DownloadToFileAsync(
        HttpMethod method,
        string relativeUrl,
        string filePath,
        HttpContent? content = null,
        CancellationToken cancellationToken = default)
    {
        await using var stream = await SendStreamAsync(method, relativeUrl, content, cancellationToken)
            .ConfigureAwait(false);
        var dir = Path.GetDirectoryName(filePath);
        if (!string.IsNullOrEmpty(dir))
        {
            Directory.CreateDirectory(dir);
        }

        await using var file = new FileStream(
            filePath,
            FileMode.Create,
            FileAccess.Write,
            FileShare.None,
            81920,
            FileOptions.Asynchronous);
        await stream.CopyToAsync(file, cancellationToken).ConfigureAwait(false);
    }

    private async Task<HttpResponseMessage> SendCoreAsync(
        HttpMethod method,
        string relativeUrl,
        HttpContent? content,
        bool disposeContent,
        CancellationToken cancellationToken)
    {
        var isGet = method == HttpMethod.Get;
        var attempts = isGet ? Math.Max(0, _options.MaxGetRetries) + 1 : 1;
        Exception? last = null;

        for (var attempt = 0; attempt < attempts; attempt++)
        {
            if (attempt > 0)
            {
                var delay = TimeSpan.FromMilliseconds(
                    _options.RetryBackoff.TotalMilliseconds * Math.Pow(2, attempt - 1));
                await Task.Delay(delay, cancellationToken).ConfigureAwait(false);
            }

            var request = new HttpRequestMessage(method, relativeUrl);
            if (content is not null)
            {
                // Content can only be sent once; retries are GET-only (no body).
                request.Content = content;
            }

            try
            {
                var response = await _http.SendAsync(
                        request,
                        HttpCompletionOption.ResponseHeadersRead,
                        cancellationToken)
                    .ConfigureAwait(false);

                if (isGet && attempt < attempts - 1 && IsRetryableStatus(response.StatusCode))
                {
                    response.Dispose();
                    continue;
                }

                return response;
            }
            catch (TaskCanceledException ex) when (!cancellationToken.IsCancellationRequested)
            {
                last = new Errors.TimeoutException(
                    $"Request timed out after {_options.Timeout.TotalSeconds:0}s: {method} {relativeUrl}",
                    ex)
                {
                    ResponseBody = null,
                };
                if (!isGet || attempt >= attempts - 1)
                {
                    throw last;
                }
            }
            catch (HttpRequestException ex)
            {
                last = new NetworkException(
                    $"Network error calling {method} {relativeUrl}: {ex.Message}",
                    ex);
                if (!isGet || attempt >= attempts - 1)
                {
                    throw last;
                }
            }
            finally
            {
                // Always detach content before disposing the request message so we
                // control disposal exactly once via disposeContent / caller.
                var owned = request.Content;
                request.Content = null;
                request.Dispose();
                if (disposeContent)
                {
                    owned?.Dispose();
                    content = null;
                }
            }
        }

        throw last ?? new NetworkException($"Request failed: {method} {relativeUrl}");
    }

    private static bool IsRetryableStatus(HttpStatusCode status) =>
        status is HttpStatusCode.RequestTimeout
            or HttpStatusCode.TooManyRequests
            or HttpStatusCode.BadGateway
            or HttpStatusCode.ServiceUnavailable
            or HttpStatusCode.GatewayTimeout
            || (int)status >= 500;

    internal static async Task EnsureSuccessAsync(
        HttpResponseMessage response,
        CancellationToken cancellationToken)
    {
        if (response.IsSuccessStatusCode)
        {
            return;
        }

        var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
        throw MapError(response.StatusCode, body);
    }

    internal static ReportServerException MapError(HttpStatusCode status, string? body)
    {
        string message = "Request failed";
        string? code = null;
        string? hint = null;
        IReadOnlyList<string> errors = Array.Empty<string>();

        if (!string.IsNullOrWhiteSpace(body))
        {
            try
            {
                using var doc = JsonDocument.Parse(body);
                var root = doc.RootElement;
                if (root.TryGetProperty("error", out var errEl) && errEl.ValueKind == JsonValueKind.String)
                {
                    message = errEl.GetString() ?? message;
                }
                else
                {
                    message = body.Length > 500 ? body[..500] : body;
                }

                if (root.TryGetProperty("code", out var codeEl) && codeEl.ValueKind == JsonValueKind.String)
                {
                    code = codeEl.GetString();
                }

                if (root.TryGetProperty("hint", out var hintEl) && hintEl.ValueKind == JsonValueKind.String)
                {
                    hint = hintEl.GetString();
                }

                if (root.TryGetProperty("errors", out var errorsEl) && errorsEl.ValueKind == JsonValueKind.Array)
                {
                    var list = new List<string>();
                    foreach (var item in errorsEl.EnumerateArray())
                    {
                        if (item.ValueKind == JsonValueKind.String)
                        {
                            list.Add(item.GetString() ?? "");
                        }
                    }

                    errors = list;
                    if (errors.Count > 0)
                    {
                        message = $"{message}: {string.Join("; ", errors)}";
                    }
                }
            }
            catch (JsonException)
            {
                message = body.Length > 500 ? body[..500] : body;
            }
        }
        else
        {
            message = $"HTTP {(int)status} {status}";
        }

        ReportServerException ex = status switch
        {
            HttpStatusCode.Unauthorized => new AuthenticationException(message),
            HttpStatusCode.Forbidden => new PermissionDeniedException(message),
            HttpStatusCode.NotFound => new NotFoundException(message),
            HttpStatusCode.Conflict => new ConflictException(message),
            HttpStatusCode.BadRequest or HttpStatusCode.UnprocessableEntity =>
                new ValidationException(message) { Errors = errors },
            HttpStatusCode.RequestTimeout => new Errors.TimeoutException(message),
            >= HttpStatusCode.InternalServerError => new ServerException(message),
            _ => new ReportServerException(message),
        };

        ex.StatusCode = status;
        ex.ResponseBody = body;
        ex.ErrorCode = code;
        ex.Hint = hint;
        return ex;
    }

    internal static async Task<T> ReadJsonAsync<T>(
        HttpResponseMessage response,
        CancellationToken cancellationToken)
    {
        await using var stream = await response.Content.ReadAsStreamAsync(cancellationToken)
            .ConfigureAwait(false);
        try
        {
            var value = await JsonSerializer.DeserializeAsync<T>(stream, JsonOptions, cancellationToken)
                .ConfigureAwait(false);
            if (value is null)
            {
                throw new InvalidResponseException(
                    $"Empty or null JSON response for {typeof(T).Name}.")
                {
                    StatusCode = response.StatusCode,
                };
            }

            return value;
        }
        catch (JsonException ex)
        {
            throw new InvalidResponseException(
                $"Failed to deserialize response as {typeof(T).Name}: {ex.Message}",
                ex)
            {
                StatusCode = response.StatusCode,
            };
        }
    }

    public void Dispose()
    {
        if (_ownsClient)
        {
            _http.Dispose();
        }
    }

    private sealed class ResponseStream : Stream
    {
        private readonly Stream _inner;
        private readonly HttpResponseMessage _response;

        public ResponseStream(Stream inner, HttpResponseMessage response)
        {
            _inner = inner;
            _response = response;
        }

        public override bool CanRead => _inner.CanRead;
        public override bool CanSeek => _inner.CanSeek;
        public override bool CanWrite => _inner.CanWrite;
        public override long Length => _inner.Length;
        public override long Position
        {
            get => _inner.Position;
            set => _inner.Position = value;
        }

        public override void Flush() => _inner.Flush();
        public override int Read(byte[] buffer, int offset, int count) => _inner.Read(buffer, offset, count);
        public override long Seek(long offset, SeekOrigin origin) => _inner.Seek(offset, origin);
        public override void SetLength(long value) => _inner.SetLength(value);
        public override void Write(byte[] buffer, int offset, int count) => _inner.Write(buffer, offset, count);
        public override Task<int> ReadAsync(byte[] buffer, int offset, int count, CancellationToken cancellationToken) =>
            _inner.ReadAsync(buffer, offset, count, cancellationToken);
        public override ValueTask<int> ReadAsync(Memory<byte> buffer, CancellationToken cancellationToken = default) =>
            _inner.ReadAsync(buffer, cancellationToken);
        public override Task CopyToAsync(Stream destination, int bufferSize, CancellationToken cancellationToken) =>
            _inner.CopyToAsync(destination, bufferSize, cancellationToken);

        protected override void Dispose(bool disposing)
        {
            if (disposing)
            {
                _inner.Dispose();
                _response.Dispose();
            }

            base.Dispose(disposing);
        }

        public override async ValueTask DisposeAsync()
        {
            await _inner.DisposeAsync().ConfigureAwait(false);
            _response.Dispose();
            await base.DisposeAsync().ConfigureAwait(false);
        }
    }
}
