using AcuGIS.Report;

// Requires:
//   ACUGIS_REPORT_URL=https://reports.example.com
//   ACUGIS_REPORT_TOKEN=jrs_...

using var client = ReportServerClient.FromEnvironment();

var me = await client.Auth.MeAsync();
Console.WriteLine($"Authenticated as {me.Username} (admin={me.IsAdmin})");

var reports = await client.Reports.ListAsync();
Console.WriteLine($"Reports ({reports.Count}):");
foreach (var item in reports.Items ?? [])
{
    Console.WriteLine($"  - {item.Key}: {item.Name} ({item.Type})");
}

if (reports.Items is { Count: > 0 })
{
    var key = reports.Items[0].Key!;
    var outPath = Path.Combine(Environment.CurrentDirectory, $"{key}.pdf");
    var parameters = new Dictionary<string, object?>
    {
        // Multi-value parameters are repeated in the query string:
        // ["city"] = new[] { "NYC", "LA" },
    };

    await client.Reports.RunToFileAsync(key, parameters, outPath);
    Console.WriteLine($"Wrote {outPath}");
}

var health = await client.Health.GetHealthAsync();
Console.WriteLine($"Health: {health.Status} ({health.Service})");
