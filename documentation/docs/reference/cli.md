# CLI reference

Command lookup for `acugis-report`. For workflows see the [CLI overview](../developer/cli/overview.md).

## Global options

```bash
acugis-report --profile NAME --output table|json|yaml --verbose --debug --no-color <command>
```

Place `--output json` **before** the subcommand.

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
| `map` / `map-report` | Map Report helpers. Requires Report Server Professional. The Open Source server does not implement these routes. |
| `embedded-map` | Embedded Map debug |
| `explorer` | Data Explorer APIs. Requires Report Server Professional. Not included in this Open Source CLI. |

## Reserved

`users`, `groups`, `schedules`, `jobs` — not yet implemented by Report Server.

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

## Environment

```bash
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## Related

- [CLI overview](../developer/cli/overview.md)
- [CLI reports](../developer/cli/reports.md)
