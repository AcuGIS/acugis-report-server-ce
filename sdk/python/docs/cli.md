# AcuGIS Report Server CLI

Modern cross-platform CLI and Python SDK for [AcuGIS Report Server](https://github.com/AcuGIS/acugis-report-server-ce).

The CLI talks **only** to the REST API through the Python SDK — it never accesses the database directly.

```text
acugis-report (CLI)
        │
        ▼
Python Client Library (acugis_report)
        │
        ▼
REST API
        │
        ▼
Report Server
```

## Installation

Requires Python 3.10+.

```bash
cd sdk/python
pip install -e ".[dev]"
```


Or from the package directory:

```bash
pip install .
```

Verify:

```bash
acugis-report --version
acugis-report --help
```

## Authentication

### Interactive login

```bash
acugis-report login
```

Prompts for:

* Server URL
* Username
* Password

The CLI authenticates, creates a personal access token (`jrs_…`), and stores the token locally. **Passwords are never stored.**

### Token login

```bash
acugis-report login --token
```

Prompts for an existing API token (create one in the Report Server UI under Profile → API Tokens).

### Environment variables (CI)

```bash
export ACUGIS_REPORT_URL=https://reports.example.com/go
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

### Who am I?

```bash
acugis-report whoami
acugis-report --output json whoami
```

Shows username, groups, authentication type, server, token name, and expiration.

### Logout

```bash
acugis-report logout
```

## Configuration & profiles

Config file locations:

| Platform | Path |
|----------|------|
| Windows | `%APPDATA%\AcuGIS\Report\config.toml` |
| Linux / macOS | `~/.config/acugis-report/config.toml` |

Stored fields: server URL, bearer token, active profile, TLS verify flag. Never passwords.

### Multiple servers

```bash
acugis-report profile add production --server https://reports.example.com/go
acugis-report profile add staging --server https://staging.example.com/go
acugis-report profile use production
acugis-report profile list
```

Override per command:

```bash
acugis-report --profile staging reports list
```

## Global options

```bash
acugis-report --profile NAME --output table|json|yaml --verbose --debug --no-color <command>
```

Place `--output json` **before** the subcommand:

```bash
acugis-report --output json reports list
```

## Health

```bash
acugis-report doctor
```

Aggregates `/health`, `/ready`, diagnostics, and Jasper status.

## Reports

```bash
acugis-report reports list
acugis-report reports show Invoice
acugis-report reports publish report.jrxml --data-source postgres
acugis-report reports update Invoice --jrxml report.jrxml
acugis-report reports delete Invoice --yes
acugis-report reports config Invoice
acugis-report reports thumbnail Invoice --output thumb.png
```

### Run a report

```bash
acugis-report reports run Invoice \
  --param InvoiceID=123 \
  --output invoice.pdf
```

Multi-value parameters:

```bash
acugis-report reports run Sales \
  --param region=West \
  --param region=North \
  --format xlsx \
  --output sales.xlsx
```

Parameter file:

```bash
acugis-report reports run Invoice --params params.json --output invoice.pdf
```

Supported formats: `html`, `pdf`, `xlsx`, `xls`, `csv`, `docx`, `rtf`, `odt`, `ods`, `pptx`, `xml`.

Binary results are written to a file (or `--stdout`). They are never dumped to the terminal by default.

## Views

Saved Data Explorer views (excludes dashboards). ACL filtering is server-side.

```bash
acugis-report views list
acugis-report views show 61
acugis-report views show admin/environment-view
acugis-report views permissions 61
acugis-report views permissions 61 --set-group Analysts
```

Lookup accepts a content item id, a view key (current user), or `owner/key`.
Use `--verbose` on `show` to include the full saved configuration.

## Dashboards

Dashboards are the same explorer_view records with `is_dashboard=true` (no separate storage).

```bash
acugis-report dashboards list
acugis-report dashboards show 58
acugis-report dashboards show admin/ops-board
acugis-report dashboards permissions 58 --set-group Ops
```

`folders browse` still returns mixed Folder content (reports, views, and dashboards).

## Folders

```bash
acugis-report folders tree
acugis-report folders browse --folder-id 0
acugis-report folders create Finance
acugis-report folders update 12 --name Billing
acugis-report folders move 12 --parent-id 3
acugis-report folders delete 12 --yes
acugis-report folders permissions 12
```

## Resources

```bash
acugis-report resources list
acugis-report resources upload logo.png --folder-id 3
acugis-report resources download <uuid> --output logo.png
acugis-report resources replace <uuid> new-logo.png
acugis-report resources delete <uuid> --yes
acugis-report resources images
```

## Datasources

```bash
acugis-report datasources list
acugis-report datasources create -n postgres --db-type postgres --host db --dbname reports -u jrs -p secret
acugis-report datasources delete 3 --yes
acugis-report datasources context
```

`show` / `update` activate automatically when the corresponding REST endpoints exist.

## Map reports

These commands call Report Server Professional. The Open Source server does not implement Map Report routes.

```bash
acugis-report map features Parcels
acugis-report map extent Parcels --srid 2193 --geometry-file geometry.wkt
acugis-report map columns Parcels

acugis-report map-report run Parcels \
  --geometry-file geometry.wkt \
  --srid 2193 \
  --output report.pdf
```

## Embedded maps

```bash
acugis-report embedded-map debug Invoice
acugis-report embedded-map debug Invoice --compile --output json
```

## Explorer

Data Explorer commands are not part of this Open Source CLI. Data Explorer is available in Report Server Professional.

## Reserved commands

These groups are reserved for upcoming APIs and currently print:

```text
Not yet implemented by Report Server.
```

* `users`
* `groups`
* `schedules`
* `jobs`

They will activate when REST endpoints ship — no breaking CLI changes required.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Authentication |
| 3 | Validation |
| 4 | Not found |
| 5 | Permissions |
| 6 | Server error |
| 7 | Network error |

Stack traces appear only with `--debug`.

## Automation & CI/CD

### Shell

```bash
export ACUGIS_REPORT_URL=https://reports.example.com/go
export ACUGIS_REPORT_TOKEN=${{ secrets.ACUGIS_REPORT_TOKEN }}

acugis-report doctor
acugis-report --output json reports list
acugis-report reports run Invoice --param InvoiceID=42 --output invoice.pdf
```

### GitHub Actions

```yaml
name: Publish report
on:
  push:
    paths: ["reports/**/*.jrxml"]

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"

      - name: Install CLI
        run: pip install ./sdk/python

      - name: Publish JRXML
        env:
          ACUGIS_REPORT_URL: ${{ vars.ACUGIS_REPORT_URL }}
          ACUGIS_REPORT_TOKEN: ${{ secrets.ACUGIS_REPORT_TOKEN }}
        run: |
          acugis-report doctor
          acugis-report reports publish reports/Invoice.jrxml \
            --data-source postgres \
            --name Invoice
```

### JSON for scripting

```bash
acugis-report --output json reports list | jq '.items[].key'
acugis-report --output yaml folders tree > folders.yaml
```

## Python SDK integration

```python
from acugis_report import ReportServer

with ReportServer(
    url="https://reports.example.com/go",
    token="jrs_xxxxxxxxx",
) as client:
    me = client.me()
    print(me.username, me.groups)

    reports = client.list_reports()
    for item in reports.items:
        print(item.key, item.name)

    pdf, content_type = client.run_report(
        "Invoice",
        format="pdf",
        params={"InvoiceID": 123},
    )
    Path("invoice.pdf").write_bytes(pdf)
```

Every CLI command is a thin wrapper around this SDK, so the CLI also acts as an integration test surface for the client library.

## Related documentation

* [API inventory](../../go-report-server/docs/api-inventory.md)
* [Authentication](../../go-report-server/docs/authentication.md)
