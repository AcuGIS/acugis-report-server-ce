# SDK reference

Official SDKs wrap the Report Server OpenAPI contract. Shared environment:

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## Languages

| Language | Package / path | Guide |
|----------|----------------|-------|
| Python | `acugis-report` / `sdk/python` | [Python](../developer/sdk/python.md) |
| .NET | `AcuGIS.Report` / `sdk/dotnet` | [.NET](../developer/sdk/dotnet.md) |
| Java | `com.acugis.report` / `sdk/java` | [Java](../developer/sdk/java.md) |
| Go | `sdk/go` | [Go](../developer/sdk/go.md) |

## Service namespaces

| Area | Typical calls |
|------|----------------|
| Auth | `Auth.Me`, token helpers |
| Reports | `Reports.List`, `Reports.Get`, `Reports.Run`, `Reports.Publish` |
| Folders | `Folders.Tree`, `Folders.Browse` |
| Resources | `Resources.List`, `Resources.Upload` |
| Datasources | `Datasources.List`, `Datasources.Create` |
| Map | `Map.Features`, `Map.Extent`, `MapReports.Run` |
| Explorer | `Explorer.Query`, `Explorer.Export` |

Language surface spelling differs slightly (`client.Reports.Run` vs `client.reports().run`) but operation names stay aligned with OpenAPI `operationId`s.

## Related

- [SDK overview](../developer/sdk/overview.md)
- [OpenAPI](openapi.md)
