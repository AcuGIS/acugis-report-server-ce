# Building a Go HTTP integration

Minimal `net/http` handler that runs Invoice via `client.Reports.Run`.

## Code

```go
package main

import (
    "log"
    "net/http"
    "os"
    "strconv"
    "strings"

    report "github.com/acugis/acugis-report-go"
)

func main() {
    client, err := report.NewClient(
        os.Getenv("ACUGIS_REPORT_URL"),
        os.Getenv("ACUGIS_REPORT_TOKEN"),
    )
    if err != nil {
        log.Fatal(err)
    }

    http.HandleFunc("/invoices/", func(w http.ResponseWriter, r *http.Request) {
        idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/invoices/"), ".pdf")
        id, err := strconv.Atoi(idStr)
        if err != nil {
            http.Error(w, "invalid id", http.StatusBadRequest)
            return
        }
        result, err := client.Reports.Run(r.Context(), "Invoice", report.RunOptions{
            Format: "pdf",
            Params: map[string]any{"InvoiceID": id},
        })
        if err != nil {
            http.Error(w, err.Error(), http.StatusBadGateway)
            return
        }
        w.Header().Set("Content-Type", "application/pdf")
        _, _ = w.Write(result.Content)
    })

    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## Direct REST alternative

If you prefer raw HTTP without an SDK:

```bash
curl -H "Authorization: Bearer $ACUGIS_REPORT_TOKEN" \
  -o invoice.pdf \
  "$ACUGIS_REPORT_URL/reports/execute?_repName=Invoice&_repFormat=pdf&InvoiceID=123"
```
