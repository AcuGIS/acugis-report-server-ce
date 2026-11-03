# Building an ASP.NET integration

Return Invoice PDFs from an ASP.NET Minimal API using the .NET SDK service namespace.

## Environment

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## Program.cs

```csharp
using AcuGIS.Report;

var builder = WebApplication.CreateBuilder(args);
builder.Services.AddSingleton(_ =>
{
    var url = Environment.GetEnvironmentVariable("ACUGIS_REPORT_URL")
        ?? throw new InvalidOperationException("ACUGIS_REPORT_URL required");
    var token = Environment.GetEnvironmentVariable("ACUGIS_REPORT_TOKEN")
        ?? throw new InvalidOperationException("ACUGIS_REPORT_TOKEN required");
    return new ReportServerClient(url, token);
});

var app = builder.Build();

app.MapGet("/invoices/{invoiceId:int}.pdf", async (
    int invoiceId,
    ReportServerClient client) =>
{
    var result = await client.Reports.Run(
        "Invoice",
        format: "pdf",
        parameters: new Dictionary<string, object?> { ["InvoiceID"] = invoiceId });

    return Results.File(
        result.Content,
        result.ContentType ?? "application/pdf",
        fileDownloadName: $"invoice-{invoiceId}.pdf");
});

app.Run();
```

## Notes

- Register a single client instance when thread-safe; otherwise create per request
- Stream large exports instead of buffering when your SDK supports streams
- Pair with your own authorization layer before calling Report Server
