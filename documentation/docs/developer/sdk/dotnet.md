# .NET SDK

The .NET client targets the same OpenAPI contract as the other SDKs. Configure with `ACUGIS_REPORT_URL` and `ACUGIS_REPORT_TOKEN`. Repository path: [`sdk/dotnet/`](https://github.com/AcuGIS/acugis-report-server-ce/tree/main/sdk/dotnet).

## Installation

Add the `AcuGIS.Report` project/package from `sdk/dotnet` (see the SDK README for NuGet/publish status).

## Authentication

Pass a Bearer API Token into `ReportServerClient`. Token lifecycle (create/rotate/revoke) is available via `client.Auth` and the REST token APIs. See [Developer authentication](../authentication.md).

## Creating a client

```csharp
using AcuGIS.Report;

var url = Environment.GetEnvironmentVariable("ACUGIS_REPORT_URL")
    ?? throw new InvalidOperationException("ACUGIS_REPORT_URL is required");
var token = Environment.GetEnvironmentVariable("ACUGIS_REPORT_TOKEN")
    ?? throw new InvalidOperationException("ACUGIS_REPORT_TOKEN is required");

var client = new ReportServerClient(url, token);

var me = await client.Auth.Me();
Console.WriteLine($"{me.Username} groups={string.Join(",", me.Groups)}");
```

### Service namespaces

| Service | Examples |
|---------|----------|
| `client.Auth` | `Me()`, token management |
| `client.Reports` | `List()`, `Get(key)`, `Run(...)`, `Publish(...)` |
| `client.Folders` | `Tree()`, `Browse(folderId)` |
| `client.Resources` | `List()`, `Upload(...)`, `Download(uuid)` |
| `client.Datasources` | `List()`, `Create(...)`, `Delete(id)`, `GenerateContext()` |
| `client.MapReports` | `Features`, `Extent`, `Run`. Requires Report Server Professional. The Open Source server does not implement these routes. |
| Explorer | Not exposed by this .NET client. Data Explorer requires Report Server Professional. |

## Listing reports

```csharp
var reports = await client.Reports.List();
foreach (var item in reports.Items)
{
    Console.WriteLine($"{item.Key} {item.Name}");
}
```

## Running reports

```csharp
var result = await client.Reports.Run(
    "Invoice",
    format: "pdf",
    parameters: new Dictionary<string, object?>
    {
        ["InvoiceID"] = 123,
    });

await File.WriteAllBytesAsync("invoice.pdf", result.Content);
Console.WriteLine(result.ContentType);
```

## Uploading reports

Use `client.Reports.Publish(...)` (multipart JRXML) to create catalog entries. Update with the corresponding update API on the Reports service.

## Downloading reports

`Reports.Run` returns binary `Content`. Resource downloads use `client.Resources.Download(uuid)`. Save bytes to disk — see [Binary responses](../rest-api/binary-responses.md).

## Error handling

Handle HTTP and typed `ReportServerException` failures explicitly in application code. Invalid/revoked tokens share the same auth error path.

## Environment variables

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## Common patterns

- Construct the client from `ACUGIS_REPORT_URL` and `ACUGIS_REPORT_TOKEN`
- List reports, then `Run` with a format and parameter map
- Write binary `result.Content` to disk
- Prefer DI registration helpers when embedding in ASP.NET

## OpenAPI generation

```bash
curl -o openapi.yaml "$ACUGIS_REPORT_URL/api/openapi.yaml"
openapi-generator-cli generate -i openapi.yaml -g csharp -o ./sdk-dotnet
```

Keep `operationId` values stable (`runReport`, `listReports`, …).

## Related

- [ASP.NET tutorial](../tutorials/aspnet-integration.md)
- [REST API overview](../rest-api/overview.md)
