# AcuGIS.Report — .NET SDK

Official .NET 8 client for [AcuGIS Report Server](https://acugis.com).

Package: **AcuGIS.Report** · Namespace: **AcuGIS.Report** · Version: **0.1.0**

API contract: [`../openapi.bundle.yaml`](../openapi.bundle.yaml)

Product docs: [https://acugis.com](https://acugis.com) · MkDocs site under [`documentation/`](../../documentation/).

## Install

```bash
dotnet add package AcuGIS.Report
```

Or project reference this folder while developing:

```bash
dotnet add reference path/to/sdk/dotnet/src/AcuGIS.Report/AcuGIS.Report.csproj
```

## Quick start

```csharp
using AcuGIS.Report;

// From constructor
var client = new ReportServerClient("https://reports.example.com", "jrs_…");

// Or environment: ACUGIS_REPORT_URL + ACUGIS_REPORT_TOKEN
using var envClient = ReportServerClient.FromEnvironment();

var me = await client.Auth.MeAsync();
var reports = await client.Reports.ListAsync();

var parameters = new Dictionary<string, object?>
{
    ["city"] = new[] { "NYC", "LA" }, // multi-value → repeated query keys
    ["year"] = 2024,
};

await client.Reports.RunToFileAsync("Invoice", parameters, "invoice.pdf");
```

### ASP.NET Core / DI

```csharp
builder.Services.AddAcuGISReport(options =>
{
    options.BaseUrl = "https://reports.example.com";
    options.Token = builder.Configuration["AcuGIS:Token"];
    options.Timeout = TimeSpan.FromMinutes(2);
    options.VerifyTls = true;
    options.MaxGetRetries = 2;
    options.DefaultHeaders["X-Request-Id"] = Guid.NewGuid().ToString("N");
});

app.MapGet("/reports/{key}/pdf", async (string key, ReportServerClient client, CancellationToken ct) =>
{
    var stream = await client.Reports.RunStreamAsync(key, "pdf", cancellationToken: ct);
    return Results.File(stream, "application/pdf", $"{key}.pdf");
});
```

## Services

| Property | Coverage |
|----------|----------|
| `Auth` | Me, login/logout, PAT list/create/update/delete/rotate |
| `Reports` | List/get/publish/update/delete/config/thumbnail, parameter lookup, execute (`Run` / `RunStream` / `RunToFile`) |
| `Folders` | Tree, browse, search, CRUD, move, duplicate, permissions, move content, favorites |
| `Resources` | CRUD, content replace, download/stream-to-file, thumbnails, images, resource sync |
| `Datasources` | List names, form-urlencoded create/upsert, delete, Jasper context |
| `MapReports` | Columns, extent, GeoJSON features |
| `EmbeddedMaps` | Debug |
| `Explorer` | Datasources, tables, metadata, query, export, features, handoff, map helpers, spatial, stats, views, dashboard widgets, view permissions |
| `Health` | `/health`, `/ready`, diagnostics |
| `Users` / `Groups` / `Schedules` / `Jobs` / `Recipients` | Reserved → `NotImplementedByServerException` |

## Configuration

| Option | Default | Notes |
|--------|---------|--------|
| `BaseUrl` | required | Server origin |
| `Token` | optional | `Authorization: Bearer` |
| `Timeout` | 100s | `HttpClient.Timeout` |
| `VerifyTls` | `true` | Set `false` only for local/dev |
| `DefaultHeaders` | empty | Applied to every request |
| `UserAgent` | `AcuGIS.Report/0.1.0` | |
| `MaxGetRetries` | `0` | GET-only retries with exponential backoff |

Environment variables: `ACUGIS_REPORT_URL`, `ACUGIS_REPORT_TOKEN`.

## Errors

All failures derive from `AcuGIS.Report.Errors.ReportServerException` and preserve `ResponseBody`, `StatusCode`, `ErrorCode`, and `Hint` when present:

- `AuthenticationException` (401)
- `PermissionDeniedException` (403)
- `ValidationException` (400 / 422)
- `NotFoundException` (404)
- `ConflictException` (409)
- `ServerException` (5xx)
- `NetworkException`
- `TimeoutException`
- `InvalidResponseException`
- `NotImplementedByServerException`

## Build & test

Requires the [.NET 8 SDK](https://dotnet.microsoft.com/download).

```bash
cd sdk/dotnet
dotnet build AcuGIS.Report.sln
dotnet test
dotnet pack src/AcuGIS.Report/AcuGIS.Report.csproj -c Release
```

Integration tests run only when both are set:

- `ACUGIS_REPORT_TEST_URL`
- `ACUGIS_REPORT_TEST_TOKEN`

## Examples

- [`examples/console`](examples/console) — env vars, Me, list reports, save PDF
- [`examples/aspnet-core`](examples/aspnet-core) — DI + PDF endpoint

## License

MIT — see [LICENSE](LICENSE).
