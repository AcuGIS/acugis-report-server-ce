using AcuGIS.Report;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddAcuGISReport(options =>
{
    options.BaseUrl = builder.Configuration["AcuGIS:BaseUrl"]
        ?? Environment.GetEnvironmentVariable("ACUGIS_REPORT_URL")
        ?? throw new InvalidOperationException("Set AcuGIS:BaseUrl or ACUGIS_REPORT_URL");
    options.Token = builder.Configuration["AcuGIS:Token"]
        ?? Environment.GetEnvironmentVariable("ACUGIS_REPORT_TOKEN");
    options.Timeout = TimeSpan.FromMinutes(2);
});

var app = builder.Build();

app.MapGet("/", () => Results.Ok(new
{
    service = "AcuGIS.Report ASP.NET sample",
    endpoints = new[] { "/me", "/reports", "/reports/{key}/pdf" },
}));

app.MapGet("/me", async (ReportServerClient client, CancellationToken ct) =>
{
    var me = await client.Auth.MeAsync(ct);
    return Results.Ok(me);
});

app.MapGet("/reports", async (ReportServerClient client, CancellationToken ct) =>
{
    var list = await client.Reports.ListAsync(ct);
    return Results.Ok(list);
});

app.MapGet("/reports/{key}/pdf", async (string key, ReportServerClient client, CancellationToken ct) =>
{
    var stream = await client.Reports.RunStreamAsync(key, "pdf", cancellationToken: ct);
    return Results.File(stream, "application/pdf", $"{key}.pdf");
});

app.Run();
