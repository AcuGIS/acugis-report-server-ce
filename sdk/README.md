# AcuGIS Report Server SDKs

Official client libraries for [AcuGIS Report Server](https://acugis.com).

| SDK | Path | Package |
|-----|------|---------|
| Python | [`python/`](python/) | `acugis-report` (`acugis_report`) |
| .NET | [`dotnet/`](dotnet/) | `AcuGIS.Report` |
| Java | [`java/`](java/) | `com.acugis:acugis-report` |
| Go | [`go/`](go/) | `github.com/acugis/acugis-report-go` |
| **JavaScript Embedding** | [`javascript/`](javascript/) | `@acugis/report-embed` |

The JavaScript package is the **browser Embedding SDK** (iframes, events, parameters). It is not a generated REST client. **Embed tokens (`emb_…`) are required** — mint them via `POST /api/v1/embed/token`.

API contract (shared): [`openapi.bundle.yaml`](openapi.bundle.yaml)

Product documentation (MkDocs): [`../documentation/`](../documentation/)

## OpenAPI bundle

`openapi.bundle.yaml` is generated from `go-report-server/openapi/`. Regenerate after contract changes:

```bash
# Unix
./sdk/scripts/regenerate-openapi.sh

# Windows PowerShell
./sdk/scripts/regenerate-openapi.ps1
```

CI fails PRs when the checked-in bundle drifts (see `.github/workflows/sdk-openapi-check.yml`).

## Docs site

```bash
cd documentation
pip install -r requirements.txt
mkdocs serve
```
