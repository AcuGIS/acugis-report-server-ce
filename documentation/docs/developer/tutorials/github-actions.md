# Automating reports with GitHub Actions

Automate publish and verification with the CLI and API Tokens.

## Secrets

| Name | Value |
|------|-------|
| `ACUGIS_REPORT_URL` | Server base URL (include `/go` if using `JRS_BASE_PATH=/go`) |
| `ACUGIS_REPORT_TOKEN` | API Token (`jrs_…`) stored as a secret |

## Shell

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=${ACUGIS_REPORT_TOKEN}

acugis-report doctor
acugis-report --output json reports list
acugis-report reports run Invoice --param InvoiceID=42 --output invoice.pdf
```

## GitHub Actions

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

## Tips

- Create a dedicated automation user with least-privilege groups
- Rotate tokens with `POST /api/v1/auth/tokens/{id}/rotate`
- Fail the pipeline on non-zero CLI exit codes (auth=2, validation=3, …)
- Prefer `--output json` for assertions in tests
