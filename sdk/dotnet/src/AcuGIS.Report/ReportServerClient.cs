using AcuGIS.Report.Internal;
using AcuGIS.Report.Services;

namespace AcuGIS.Report;

/// <summary>
/// Official AcuGIS Report Server client.
/// </summary>
/// <example>
/// <code>
/// var client = new ReportServerClient("https://reports.example.com", "jrs_…");
/// var me = await client.Auth.MeAsync();
/// var reports = await client.Reports.ListAsync();
/// await client.Reports.RunToFileAsync("Invoice", parameters, "invoice.pdf");
/// </code>
/// </example>
public sealed class ReportServerClient : IDisposable
{
    private readonly HttpTransport _transport;
    private readonly bool _ownsTransport;

    /// <summary>Create a client with base URL and personal access token.</summary>
    public ReportServerClient(string baseUrl, string? token = null)
        : this(new ClientOptions { BaseUrl = baseUrl, Token = token })
    {
    }

    /// <summary>Create a client from <see cref="ClientOptions"/> (owns the HttpClient).</summary>
    public ReportServerClient(ClientOptions options)
        : this(CreateHttpClient(options), options, ownsClient: true)
    {
    }

    /// <summary>
    /// Create a client with an externally managed <see cref="HttpClient"/>
    /// (e.g. from <c>IHttpClientFactory</c>). The client is not disposed.
    /// </summary>
    public ReportServerClient(HttpClient httpClient, ClientOptions? options = null)
        : this(httpClient, options ?? new ClientOptions(), ownsClient: false)
    {
        if (string.IsNullOrWhiteSpace(options?.BaseUrl) && httpClient.BaseAddress is not null)
        {
            // Base address already configured on the factory client.
        }
        else if (httpClient.BaseAddress is null && options is not null && !string.IsNullOrWhiteSpace(options.BaseUrl))
        {
            httpClient.BaseAddress = options.GetBaseAddress();
        }
    }

    private ReportServerClient(HttpClient httpClient, ClientOptions options, bool ownsClient)
    {
        Options = options;
        _transport = new HttpTransport(httpClient, options, ownsClient);
        _ownsTransport = true;

        Auth = new AuthService(_transport);
        Reports = new ReportsService(_transport);
        Folders = new FoldersService(_transport);
        Resources = new ResourcesService(_transport);
        Datasources = new DatasourcesService(_transport);
        MapReports = new MapReportsService(_transport);
        EmbeddedMaps = new EmbeddedMapsService(_transport);
        Health = new HealthService(_transport);
        Users = new UsersService();
        Groups = new GroupsService();
        Schedules = new SchedulesService();
        Jobs = new JobsService();
        Recipients = new RecipientsService();
    }

    public ClientOptions Options { get; }

    public AuthService Auth { get; }
    public ReportsService Reports { get; }
    public FoldersService Folders { get; }
    public ResourcesService Resources { get; }
    public DatasourcesService Datasources { get; }
    public MapReportsService MapReports { get; }
    public EmbeddedMapsService EmbeddedMaps { get; }
    public HealthService Health { get; }

    /// <summary>Reserved — throws <see cref="Errors.NotImplementedByServerException"/>.</summary>
    public UsersService Users { get; }

    /// <summary>Reserved — throws <see cref="Errors.NotImplementedByServerException"/>.</summary>
    public GroupsService Groups { get; }

    /// <summary>Reserved — throws <see cref="Errors.NotImplementedByServerException"/>.</summary>
    public SchedulesService Schedules { get; }

    /// <summary>Reserved — throws <see cref="Errors.NotImplementedByServerException"/>.</summary>
    public JobsService Jobs { get; }

    /// <summary>Reserved — throws <see cref="Errors.NotImplementedByServerException"/>.</summary>
    public RecipientsService Recipients { get; }

    /// <summary>Create a client from <c>ACUGIS_REPORT_URL</c> and <c>ACUGIS_REPORT_TOKEN</c>.</summary>
    public static ReportServerClient FromEnvironment() =>
        new(ClientOptions.FromEnvironment());

    private static HttpClient CreateHttpClient(ClientOptions options)
    {
        HttpMessageHandler handler = options.VerifyTls
            ? new HttpClientHandler()
            : new HttpClientHandler
            {
                ServerCertificateCustomValidationCallback =
                    HttpClientHandler.DangerousAcceptAnyServerCertificateValidator,
            };

        return new HttpClient(handler)
        {
            BaseAddress = options.GetBaseAddress(),
            Timeout = options.Timeout,
        };
    }

    public void Dispose()
    {
        if (_ownsTransport)
        {
            _transport.Dispose();
        }
    }
}
