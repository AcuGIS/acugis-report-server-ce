using System.Net.Http.Headers;
using AcuGIS.Report.Internal;
using AcuGIS.Report.Models;

namespace AcuGIS.Report.Services;

/// <summary>Report catalog, publish, and execution.</summary>
public sealed class ReportsService
{
    private readonly HttpTransport _http;

    internal ReportsService(HttpTransport http) => _http = http;

    /// <summary>GET /api/v1/reports</summary>
    public Task<ReportListResponse> ListAsync(CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<ReportListResponse>(HttpMethod.Get, "api/v1/reports", cancellationToken: cancellationToken);

    /// <summary>GET /api/v1/reports/{key}</summary>
    public Task<Report> GetAsync(string key, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<Report>(
            HttpMethod.Get,
            $"api/v1/reports/{Uri.EscapeDataString(key)}",
            cancellationToken: cancellationToken);

    /// <summary>DELETE /api/v1/reports/{key}</summary>
    public Task<ReportMutationResponse> DeleteAsync(string key, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<ReportMutationResponse>(
            HttpMethod.Delete,
            $"api/v1/reports/{Uri.EscapeDataString(key)}",
            cancellationToken: cancellationToken);

    /// <summary>GET /api/v1/reports/{key}/config</summary>
    public Task<ReportConfig> GetConfigAsync(string key, CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<ReportConfig>(
            HttpMethod.Get,
            $"api/v1/reports/{Uri.EscapeDataString(key)}/config",
            cancellationToken: cancellationToken);

    /// <summary>GET /api/v1/reports/{key}/thumbnail</summary>
    public Task<Stream> GetThumbnailAsync(string key, CancellationToken cancellationToken = default) =>
        _http.SendStreamAsync(
            HttpMethod.Get,
            $"api/v1/reports/{Uri.EscapeDataString(key)}/thumbnail",
            cancellationToken: cancellationToken);

    /// <summary>POST /api/v1/reports — multipart publish.</summary>
    public async Task<ReportCreateResponse> PublishAsync(
        PublishReportRequest request,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        var content = BuildPublishContent(request);
        using var response = await _http.SendRawAsync(HttpMethod.Post, "api/v1/reports", content, cancellationToken: cancellationToken)
            .ConfigureAwait(false);
        return await HttpTransport.ReadJsonAsync<ReportCreateResponse>(response, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>PUT /api/v1/reports/{key} — multipart update.</summary>
    public async Task<ReportMutationResponse> UpdateAsync(
        string key,
        PublishReportRequest request,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        var content = BuildPublishContent(request, requireJrxml: false);
        using var response = await _http.SendRawAsync(
                HttpMethod.Put,
                $"api/v1/reports/{Uri.EscapeDataString(key)}",
                content,
                cancellationToken: cancellationToken)
            .ConfigureAwait(false);
        return await HttpTransport.ReadJsonAsync<ReportMutationResponse>(response, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>GET /api/v1/reports/{key}/parameters/{name}/lookup</summary>
    public Task<ParameterLookupResponse> LookupParameterAsync(
        string key,
        string name,
        IEnumerable<KeyValuePair<string, object?>>? parentValues = null,
        CancellationToken cancellationToken = default)
    {
        var url = QueryString.Build(
            $"api/v1/reports/{Uri.EscapeDataString(key)}/parameters/{Uri.EscapeDataString(name)}/lookup",
            QueryString.FromReportParameters(parentValues));
        return _http.SendJsonAsync<ParameterLookupResponse>(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }

    /// <summary>POST /api/v1/reports/{key}/parameters/{name}/lookup</summary>
    public Task<ParameterLookupResponse> LookupParameterPostAsync(
        string key,
        string name,
        object? body = null,
        CancellationToken cancellationToken = default) =>
        _http.SendJsonAsync<ParameterLookupResponse>(
            HttpMethod.Post,
            $"api/v1/reports/{Uri.EscapeDataString(key)}/parameters/{Uri.EscapeDataString(name)}/lookup",
            body,
            cancellationToken);

    /// <summary>
    /// GET /reports/execute — render report bytes as a stream.
    /// Multi-value parameters are sent as repeated query keys.
    /// </summary>
    public Task<Stream> RunStreamAsync(
        string reportName,
        string format = "pdf",
        IEnumerable<KeyValuePair<string, object?>>? parameters = null,
        string? datasource = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildExecuteUrl(reportName, format, parameters, datasource);
        return _http.SendStreamAsync(HttpMethod.Get, url, cancellationToken: cancellationToken);
    }

    /// <summary>GET /reports/execute and write the result to <paramref name="filePath"/>.</summary>
    public Task RunToFileAsync(
        string reportName,
        IEnumerable<KeyValuePair<string, object?>>? parameters,
        string filePath,
        string format = "pdf",
        string? datasource = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildExecuteUrl(reportName, format, parameters, datasource);
        return _http.DownloadToFileAsync(HttpMethod.Get, url, filePath, cancellationToken: cancellationToken);
    }

    /// <summary>Convenience overload matching common SDK usage.</summary>
    public Task RunToFileAsync(
        string reportName,
        IDictionary<string, object?>? parameters,
        string filePath,
        CancellationToken cancellationToken = default) =>
        RunToFileAsync(reportName, parameters, filePath, "pdf", null, cancellationToken);

    /// <summary>GET /reports/execute — returns the full byte array.</summary>
    public async Task<byte[]> RunAsync(
        string reportName,
        string format = "pdf",
        IEnumerable<KeyValuePair<string, object?>>? parameters = null,
        string? datasource = null,
        CancellationToken cancellationToken = default)
    {
        await using var stream = await RunStreamAsync(reportName, format, parameters, datasource, cancellationToken)
            .ConfigureAwait(false);
        using var ms = new MemoryStream();
        await stream.CopyToAsync(ms, cancellationToken).ConfigureAwait(false);
        return ms.ToArray();
    }

    /// <summary>POST /reports/execute — same as GET; body forwarded to Jasper.</summary>
    public Task<Stream> RunPostStreamAsync(
        string reportName,
        string format = "pdf",
        object? body = null,
        IEnumerable<KeyValuePair<string, object?>>? queryParameters = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildExecuteUrl(reportName, format, queryParameters, datasource: null);
        HttpContent? content = body is null
            ? null
            : new StringContent(
                System.Text.Json.JsonSerializer.Serialize(body, HttpTransport.SerializerOptions),
                System.Text.Encoding.UTF8,
                "application/json");
        return _http.SendStreamAsync(HttpMethod.Post, url, content, cancellationToken);
    }

    private static string BuildExecuteUrl(
        string reportName,
        string format,
        IEnumerable<KeyValuePair<string, object?>>? parameters,
        string? datasource)
    {
        var list = new List<(string Key, string? Value)>
        {
            ("_repName", reportName),
            ("_repFormat", format),
        };
        if (!string.IsNullOrEmpty(datasource))
        {
            list.Add(("datasource", datasource));
        }

        list.AddRange(QueryString.FromReportParameters(parameters));
        return QueryString.Build("reports/execute", list);
    }

    private static MultipartFormDataContent BuildPublishContent(
        PublishReportRequest request,
        bool requireJrxml = true)
    {
        var content = new MultipartFormDataContent();
        if (requireJrxml)
        {
            if (request.JrxmlFile is null)
            {
                throw new ArgumentException("JrxmlFile is required when publishing.", nameof(request));
            }

            if (string.IsNullOrWhiteSpace(request.DataSource))
            {
                throw new ArgumentException("DataSource is required when publishing.", nameof(request));
            }
        }

        if (request.JrxmlFile is not null)
        {
            var jrxml = new StreamContent(request.JrxmlFile);
            jrxml.Headers.ContentType = new MediaTypeHeaderValue("application/xml");
            content.Add(jrxml, "jrxml_file", request.JrxmlFileName);
        }

        if (!string.IsNullOrWhiteSpace(request.DataSource))
        {
            content.Add(new StringContent(request.DataSource), "data_source");
        }

        if (request.JasperFile is not null)
        {
            content.Add(
                new StreamContent(request.JasperFile),
                "jasper_file",
                request.JasperFileName ?? "report.jasper");
        }

        if (request.ThumbnailFile is not null)
        {
            content.Add(
                new StreamContent(request.ThumbnailFile),
                "thumbnail_file",
                request.ThumbnailFileName ?? "thumbnail.png");
        }

        if (!string.IsNullOrEmpty(request.ReportName))
        {
            content.Add(new StringContent(request.ReportName), "report_name");
        }

        if (!string.IsNullOrEmpty(request.ReportDescription))
        {
            content.Add(new StringContent(request.ReportDescription), "report_description");
        }

        if (!string.IsNullOrEmpty(request.ReportType))
        {
            content.Add(new StringContent(request.ReportType), "report_type");
        }

        if (request.AccessGroups is not null)
        {
            foreach (var group in request.AccessGroups)
            {
                content.Add(new StringContent(group), "access_groups[]");
            }
        }

        return content;
    }
}
