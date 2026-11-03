# Quickstart

Run an **Invoice** report end to end with an API Token.

## 1. Create an API Token

Sign in to Report Server in the browser, open **Profile → API Tokens**, and create a token. The plaintext `jrs_…` value is shown **once**.

Alternatively, create one via REST after a session login — see [Authentication](authentication.md).

## 2. Set client environment

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

On Windows PowerShell:

```powershell
$env:ACUGIS_REPORT_URL = "https://reports.example.com"
$env:ACUGIS_REPORT_TOKEN = "jrs_xxxxxxxxx"
```

## 3. Confirm connectivity

```bash
acugis-report doctor
acugis-report whoami
```

## 4. List and run a report

```bash
acugis-report reports list
acugis-report reports run Invoice \
  --param InvoiceID=123 \
  --output invoice.pdf
```

## 5. Same workflow in SDKs

All official SDKs expose a `Reports` service with a `Run` operation. Environment variables are the same.

=== "Python"

    ```python
    import os
    from pathlib import Path
    from acugis_report import ReportServer

    with ReportServer(
        url=os.environ["ACUGIS_REPORT_URL"],
        token=os.environ["ACUGIS_REPORT_TOKEN"],
    ) as client:
        pdf, content_type = client.Reports.Run(
            "Invoice",
            format="pdf",
            params={"InvoiceID": 123},
        )
        Path("invoice.pdf").write_bytes(pdf)
    ```

=== ".NET"

    ```csharp
    using AcuGIS.Report;

    var client = new ReportServerClient(
        Environment.GetEnvironmentVariable("ACUGIS_REPORT_URL")!,
        Environment.GetEnvironmentVariable("ACUGIS_REPORT_TOKEN")!);

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

    var result = client.reports().run(
        "Invoice",
        "pdf",
        Map.of("InvoiceID", 123));

    Files.write(Path.of("invoice.pdf"), result.content());
    ```

=== "Go"

    ```go
    client, err := report.NewClient(
        os.Getenv("ACUGIS_REPORT_URL"),
        os.Getenv("ACUGIS_REPORT_TOKEN"),
    )
    if err != nil {
        log.Fatal(err)
    }

    result, err := client.Reports.Run(ctx, "Invoice", report.RunOptions{
        Format: "pdf",
        Params: map[string]any{"InvoiceID": 123},
    })
    if err != nil {
        log.Fatal(err)
    }
    os.WriteFile("invoice.pdf", result.Content, 0o644)
    ```

!!! tip "Python package today"
    The published Python client also exposes flat helpers such as `client.run_report(...)` that call the same REST execute endpoint. Prefer the `Reports.Run` service style in new integrations so all languages stay aligned.

## 6. Explore OpenAPI

On the running server:

- `/swagger` — try operations with **BearerAuth**
- `/api/openapi.yaml` — download the contract for generators

## Next

- [User Guide](../user-guide/index.md)
- [CLI reports](../developer/cli/reports.md)
- [SDK overview](../developer/sdk/overview.md)
- [Publish from Designer](../user-guide/designer/publishing.md)
