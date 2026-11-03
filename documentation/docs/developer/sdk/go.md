# Go SDK

The Go client exposes exported service fields (`client.Reports`, `client.Auth`) consistent with Go naming conventions. Repository path: [`sdk/go/`](https://github.com/AcuGIS/acugis-report-server-ce/tree/main/sdk/go). The module path remains `github.com/acugis/acugis-report-go`.

## Installation

```bash
go get github.com/acugis/acugis-report-go@latest
```

(Use the module path published in `sdk/go/go.mod` for your checkout.)

## Authentication

Pass a Bearer API Token to `report.NewClient(url, token)`. Auth helpers live on `client.Auth`. See [Developer authentication](../authentication.md).

## Creating a client

```go
package main

import (
    "context"
    "log"
    "os"

    report "github.com/acugis/acugis-report-go"
)

func main() {
    ctx := context.Background()
    client, err := report.NewClient(
        os.Getenv("ACUGIS_REPORT_URL"),
        os.Getenv("ACUGIS_REPORT_TOKEN"),
    )
    if err != nil {
        log.Fatal(err)
    }

    me, err := client.Auth.Me(ctx)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("user=%s", me.Username)
}
```

### Service namespaces

| Field | Examples |
|-------|----------|
| `client.Auth` | `Me`, token APIs |
| `client.Reports` | `List`, `Get`, `Run`, `Publish` |
| `client.Folders` | `Tree`, `Browse` |
| `client.Resources` | `List`, `Upload`, `Download` |
| `client.Datasources` | `List`, `Create`, `Delete`, `Context` |
| `client.MapReports` | `GetFeatures`, `BuildExtent`, `GetColumns`, `Run`. Requires Report Server Professional. The Open Source server does not implement these routes. |
| Explorer | Not exposed by this Go client. Data Explorer requires Report Server Professional. |

## Listing reports

```go
list, err := client.Reports.List(ctx)
if err != nil {
    log.Fatal(err)
}
for _, item := range list.Items {
    log.Printf("%s %s", item.Key, item.Name)
}
```

## Running reports

```go
result, err := client.Reports.Run(ctx, "Invoice", report.RunOptions{
    Format: "pdf",
    Params: map[string]any{"InvoiceID": 123},
})
if err != nil {
    log.Fatal(err)
}
if err := os.WriteFile("invoice.pdf", result.Content, 0o644); err != nil {
    log.Fatal(err)
}
```

## Uploading reports

Use `client.Reports.Publish` for multipart JRXML create. Update with the Reports update API.

## Downloading reports

`Reports.Run` returns binary `Content`. Resource downloads use `client.Resources.Download`. Save bytes to disk — see [Binary responses](../rest-api/binary-responses.md).

## Error handling

Check `error` returns from each call. Transport and API errors should be logged without dumping secrets; treat auth failures distinctly from not-found / permission errors where the SDK surfaces them.

## Environment variables

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## Common patterns

- Construct the client from `ACUGIS_REPORT_URL` and `ACUGIS_REPORT_TOKEN`
- List reports, then `Run` with a format and parameter map
- Write binary `result.Content` to disk
- Pass `context.Context` for cancellation in HTTP handlers

## OpenAPI generation

```bash
curl -o openapi.yaml "$ACUGIS_REPORT_URL/api/openapi.yaml"
openapi-generator-cli generate -i openapi.yaml -g go -o ./sdk-go
```

## Related

- [Go HTTP tutorial](../tutorials/go-http-integration.md)
- [REST API overview](../rest-api/overview.md)
