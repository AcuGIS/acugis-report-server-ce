using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Options;

namespace AcuGIS.Report;

/// <summary>DI extensions for <see cref="ReportServerClient"/>.</summary>
public static class DependencyInjection
{
    public const string HttpClientName = "AcuGIS.Report";

    /// <summary>
    /// Register <see cref="ReportServerClient"/> with <c>IHttpClientFactory</c>.
    /// </summary>
    public static IServiceCollection AddAcuGISReport(
        this IServiceCollection services,
        Action<ClientOptions> configure)
    {
        ArgumentNullException.ThrowIfNull(services);
        ArgumentNullException.ThrowIfNull(configure);

        services.AddOptions<ClientOptions>().Configure(configure);
        services.AddHttpClient(HttpClientName).ConfigurePrimaryHttpMessageHandler(sp =>
        {
            var options = sp.GetRequiredService<IOptions<ClientOptions>>().Value;
            if (options.VerifyTls)
            {
                return new HttpClientHandler();
            }

            return new HttpClientHandler
            {
                ServerCertificateCustomValidationCallback =
                    HttpClientHandler.DangerousAcceptAnyServerCertificateValidator,
            };
        }).ConfigureHttpClient((sp, client) =>
        {
            var options = sp.GetRequiredService<IOptions<ClientOptions>>().Value;
            client.BaseAddress = options.GetBaseAddress();
            client.Timeout = options.Timeout;
            client.DefaultRequestHeaders.TryAddWithoutValidation("User-Agent", options.UserAgent);
            if (!string.IsNullOrWhiteSpace(options.Token))
            {
                client.DefaultRequestHeaders.Authorization =
                    new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", options.Token);
            }

            foreach (var header in options.DefaultHeaders)
            {
                client.DefaultRequestHeaders.TryAddWithoutValidation(header.Key, header.Value);
            }
        });

        services.AddTransient(sp =>
        {
            var factory = sp.GetRequiredService<IHttpClientFactory>();
            var options = sp.GetRequiredService<IOptions<ClientOptions>>().Value;
            var http = factory.CreateClient(HttpClientName);
            return new ReportServerClient(http, options);
        });

        return services;
    }

    /// <summary>
    /// Register using environment variables <c>ACUGIS_REPORT_URL</c> / <c>ACUGIS_REPORT_TOKEN</c>.
    /// </summary>
    public static IServiceCollection AddAcuGISReportFromEnvironment(this IServiceCollection services) =>
        services.AddAcuGISReport(opts =>
        {
            var env = ClientOptions.FromEnvironment();
            opts.BaseUrl = env.BaseUrl;
            opts.Token = env.Token;
        });
}
