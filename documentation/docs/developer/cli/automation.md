# CLI automation

Use the CLI in scripts and CI with environment variables or profiles.

## Environment

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## JSON output

Place `--output json` **before** the subcommand:

```bash
acugis-report --output json reports list | jq '.items[].key'
```

## Doctor in pipelines

```bash
acugis-report doctor
acugis-report whoami
```

## Related

- [Profiles](profiles.md)
- [GitHub Actions tutorial](../tutorials/github-actions.md)
- [CLI reference](../../reference/cli.md)
