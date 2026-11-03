# AcuGIS Report Go SDK

Idiomatic Go client for [AcuGIS Report Server](https://github.com/AcuGIS/acugis-report-server-ce) (`github.com/acugis/acugis-report-go`).

Version **0.1.0** · Go **1.26+** · User-Agent `AcuGIS-Report-Go/0.1.0`

## Install

```bash
go get github.com/acugis/acugis-report-go@v0.1.0
```

## Quick start

```go
package main

import (
    "context"
    "fmt"
    "log"

    report "github.com/acugis/acugis-report-go"
)

func main() {
    client, err := report.NewClientFromEnv()
    // or: report.NewClient("https://reports.example.com", report.WithToken(token))
    if err != nil {
        log.Fatal(err)
    }

    me, err := client.Auth.Me(context.Background())
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("hello", me.Username)

    list, err := client.Reports.List(context.Background())
    if err != nil {
        log.Fatal(err)
    }
    if len(list.Items) == 0 {
        return
    }
    pdf, err := client.Reports.Run(context.Background(), list.Items[0].Key, &report.RunOptions{
        Format: "pdf",
        Params: map[string]any{"city": "NYC", "tags": []string{"a", "b"}},
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("got %d bytes (%s)\n", len(pdf.Data), pdf.ContentType)
}
```

### Environment

| Variable | Purpose |
|----------|---------|
| `ACUGIS_REPORT_URL` | Base URL (required for `NewClientFromEnv`) |
| `ACUGIS_REPORT_TOKEN` | Bearer token |

### Options

```go
report.NewClient(baseURL,
    report.WithToken(token),
    report.WithTimeout(60*time.Second),
    report.WithTLSVerify(false),      // local/dev only
    report.WithHTTPClient(custom),
    report.WithHeader("X-Custom", "1"),
    report.WithRetries(2, 200*time.Millisecond), // safe GET only
)
```

## Service namespaces

| Field | Coverage |
|-------|----------|
| `client.Auth` | `Me`, tokens CRUD / rotate |
| `client.Reports` | List, Get, Publish, Update, Delete, GetConfig, GetThumbnail, Run / RunToFile / RunStream |
| `client.Folders` | Tree, Browse, Search, CRUD, Move, Duplicate, permissions, favorites, MoveContent |
| `client.Resources` | CRUD, upload/download, images, resource sync |
| `client.Datasources` | List, Create, Delete, GenerateContext (`Get`/`Update` → NotImplemented) |
| `client.MapReports` | GetFeatures, BuildExtent, GetColumns, Run. These methods call Report Server Professional. The Open Source server does not implement them. |
| `client.EmbeddedMaps` | Debug |
| Explorer | Not a field on this client. Data Explorer is a Professional server API and is not exposed here. |
| `client.Health` | Check, Ready, Diagnostics, JasperStatus |
| `client.Users` / `Groups` / `Schedules` / `Jobs` / `Recipients` | Reserved stubs → `NotImplementedError` |

Authentication for SDKs is **Bearer only**. Optional retries apply only to safe GET requests — never to publish/update/delete.

## Errors

Typed errors map from HTTP status so callers do not check status codes:

`AuthenticationError`, `PermissionDeniedError`, `ValidationError`, `NotFoundError`, `ConflictError`, `ServerError`, `NetworkError`, `TimeoutError`, `InvalidResponseError`, `NotImplementedError`

```go
var authErr *report.AuthenticationError
if errors.As(err, &authErr) {
    log.Println(authErr.Message(), authErr.Hint())
}
```

## Examples

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_...

go run ./examples/console
go run ./examples/http-server
```

## Tests

```bash
go test ./...

# optional live integration
ACUGIS_REPORT_TEST_URL=... ACUGIS_REPORT_TEST_TOKEN=... go test ./... -run Integration
```

## License

MIT — see [LICENSE](LICENSE).
