# SDK overview

Official SDKs wrap the Report Server OpenAPI contract. All languages share:

- Base URL from `ACUGIS_REPORT_URL`
- Bearer API Token from `ACUGIS_REPORT_TOKEN`
- Service namespaces such as `Reports`, `Folders`, and `Resources`
- The same Invoice run workflow

## Service namespaces

| Area | Typical calls |
|------|----------------|
| Auth | `Auth.Me`, token helpers |
| Reports | `Reports.List`, `Reports.Get`, `Reports.Run`, `Reports.Publish` |
| Folders | `Folders.Tree`, `Folders.Browse` |
| Resources | `Resources.List`, `Resources.Upload` |
| Datasources | `Datasources.List`, `Datasources.Create` |
| Map | `MapReports` feature, extent, and run helpers. These call Report Server Professional. The Open Source server does not implement them. |
| Explorer | Data Explorer clients, where a language SDK still includes them, require Report Server Professional. The Open Source server does not implement them. |

Language surface spelling differs slightly (`client.Reports.Run` vs `client.reports().run`) but operation names stay aligned with OpenAPI `operationId`s such as `runReport` and `listReports`.

## Invoice workflow (all languages)

=== "Python"

    ```python
    import os
    from acugis_report import ReportServer

    with ReportServer(
        url=os.environ["ACUGIS_REPORT_URL"],
        token=os.environ["ACUGIS_REPORT_TOKEN"],
    ) as client:
        me = client.auth.me()
        items = client.reports.list()
        client.reports.run_to_file(
            "Invoice",
            "invoice.pdf",
            format="pdf",
            params={"InvoiceID": 123},
        )
    ```

=== ".NET"

    ```csharp
    var client = new ReportServerClient(
        Environment.GetEnvironmentVariable("ACUGIS_REPORT_URL")!,
        Environment.GetEnvironmentVariable("ACUGIS_REPORT_TOKEN")!);

    var me = await client.Auth.Me();
    var reports = await client.Reports.List();
    var result = await client.Reports.Run(
        "Invoice",
        format: "pdf",
        parameters: new Dictionary<string, object?> { ["InvoiceID"] = 123 });
    await File.WriteAllBytesAsync("invoice.pdf", result.Content);
    ```

=== "Java"

    ```java
    var client = ReportServerClient.builder()
        .url(System.getenv("ACUGIS_REPORT_URL"))
        .token(System.getenv("ACUGIS_REPORT_TOKEN"))
        .build();

    var me = client.auth().me();
    var reports = client.reports().list();
    var result = client.reports().run(
        "Invoice", "pdf", Map.of("InvoiceID", 123));
    Files.write(Path.of("invoice.pdf"), result.content());
    ```

=== "Go"

    ```go
    client, err := report.NewClient(
        os.Getenv("ACUGIS_REPORT_URL"),
        os.Getenv("ACUGIS_REPORT_TOKEN"),
    )
    me, err := client.Auth.Me(ctx)
    list, err := client.Reports.List(ctx)
    result, err := client.Reports.Run(ctx, "Invoice", report.RunOptions{
        Format: "pdf",
        Params: map[string]any{"InvoiceID": 123},
    })
    os.WriteFile("invoice.pdf", result.Content, 0o644)
    ```

## Generate from OpenAPI

Export the live contract from a running server:

```bash
curl -o openapi.yaml "$ACUGIS_REPORT_URL/api/openapi.yaml"
```

Or use the repository bundle `sdk/openapi.bundle.yaml`. Stable `operationId` values must not be renamed by generators.

## Language guides

- [Python](python.md)
- [.NET](dotnet.md)
- [Java](java.md)
- [Go](go.md)
