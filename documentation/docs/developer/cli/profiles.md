# CLI profiles

Manage multiple Report Server endpoints without re-entering credentials each time.

## Config file locations

| Platform | Path |
|----------|------|
| Windows | `%APPDATA%\AcuGIS\Report\config.toml` |
| Linux / macOS | `~/.config/acugis-report/config.toml` |

Stored fields: server URL, bearer token, active profile, TLS verify flag. Never passwords.

## Commands

```bash
acugis-report profile add production --server https://reports.example.com
acugis-report profile add staging --server https://staging.example.com
acugis-report profile use production
acugis-report profile list
```

## Per-command override

```bash
acugis-report --profile staging reports list
```

## Environment vs profile

If `ACUGIS_REPORT_URL` and `ACUGIS_REPORT_TOKEN` are set, they are ideal for CI. Local interactive work typically uses profiles created by `login`.
