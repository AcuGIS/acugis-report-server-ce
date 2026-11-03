# CLI overview

`acugis-report` is the cross-platform CLI for AcuGIS Report Server. It talks **only** to the REST API through the Python SDK — it never accesses the database directly.

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

## Global options

```bash
acugis-report --profile NAME --output table|json|yaml --verbose --debug --no-color <command>
```

Place `--output json` **before** the subcommand:

```bash
acugis-report --output json reports list
```

## Command groups

| Group | Purpose |
|-------|---------|
| `login` / `logout` / `whoami` | Authentication |
| `profile` | Multi-server profiles |
| `doctor` | Health / readiness / diagnostics |
| `reports` | Publish, update, run |
| `folders` | Tree, browse, ACL |
| `resources` | Upload / download |
| `datasources` | List / create / delete / context |
| `map` / `map-report` | Map Report helpers |
| `embedded-map` | Embedded Map debug |
| `explorer` | Data Explorer APIs |

## Reserved (not yet implemented by Report Server)

These groups print `Not yet implemented by Report Server.` until REST endpoints ship:

- `users`
- `groups`
- `schedules`
- `jobs`

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

## Environment

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```
