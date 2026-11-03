using Xunit;

namespace AcuGIS.Report.Tests;

/// <summary>
/// Live server tests. Skipped unless both
/// <c>ACUGIS_REPORT_TEST_URL</c> and <c>ACUGIS_REPORT_TEST_TOKEN</c> are set.
/// </summary>
public class IntegrationTests
{
    private static bool IsConfigured =>
        !string.IsNullOrWhiteSpace(Environment.GetEnvironmentVariable("ACUGIS_REPORT_TEST_URL")) &&
        !string.IsNullOrWhiteSpace(Environment.GetEnvironmentVariable("ACUGIS_REPORT_TEST_TOKEN"));

    private static ReportServerClient CreateClient()
    {
        var url = Environment.GetEnvironmentVariable("ACUGIS_REPORT_TEST_URL")!;
        var token = Environment.GetEnvironmentVariable("ACUGIS_REPORT_TEST_TOKEN")!;
        return new ReportServerClient(url, token);
    }

    [Fact]
    public async Task Health_And_Me()
    {
        if (!IsConfigured)
        {
            return; // skip silently when unset
        }

        using var client = CreateClient();
        var health = await client.Health.GetHealthAsync();
        Assert.Equal("ok", health.Status);

        var me = await client.Auth.MeAsync();
        Assert.False(string.IsNullOrWhiteSpace(me.Username));
    }

    [Fact]
    public async Task ListReports()
    {
        if (!IsConfigured)
        {
            return;
        }

        using var client = CreateClient();
        var list = await client.Reports.ListAsync();
        Assert.NotNull(list.Items);
    }
}
