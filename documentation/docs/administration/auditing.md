# Auditing

Administrators can review a searchable audit trail of meaningful business and
security events: who did what, to which resource, when, whether it succeeded,
and selected performance fields such as report execution duration.

The audit subsystem lives in the **Go Report Server**. It does not parse Tomcat
or Jasper log files. Application logs remain separate for debugging and stack
traces.

## Who can record events, and who can read them

Recording and the administration UI are different gates.

| Capability | Open Source package | Professional package, no valid license | Licensed Professional |
|------------|---------------------|----------------------------------------|------------------------|
| Write audit events when `JRS_AUDIT_ENABLED` is on (the default) | Yes | Yes | Yes |
| **Administration → Auditing** and `/api/v1/admin/audit*` | Routes are not in the binary | Denied (`audit`) | Yes |
| Usage analytics | Routes are not in the binary | Denied (`usage_analytics`) | Yes |
| Audit export | Routes are not in the binary | Denied (`audit_export`) | Yes |

Do not describe auditing as Professional-only. Event recording follows `JRS_AUDIT_*` on both packages. The UI, usage analytics, and export each have their own feature check, and those routes exist only in the Professional binary. Feature denials on an unlicensed Professional process say Community Edition. That is the unlicensed state, not a third product.

## Overview

| Concept | Purpose |
|---------|---------|
| **Application logs** | Errors, stack traces, diagnostics |
| **Audit log** | Who did what, when, to what |
| **Performance views** | Aggregations from report execution audit events (slow reports) |

Open **Administration → Auditing** (`/admin/audit.php`). Platform administrators
only. Menu visibility is not sufficient — every audit API enforces admin checks
on the server.

Aggregated operational usage of Reports, Dashboards, Views, APIs, and embeds is Usage Analytics. That capability is available in Professional and is not part of this Open Source documentation.

Forwarding selected Audit events continuously to Syslog, Webhook, Splunk HEC, or Sentinel is Audit Export. That capability is available in Professional and is not part of this Open Source documentation.

## Audit Events

Events use a small set of categories and more specific actions:

| Category | Example actions |
|----------|-----------------|
| `authentication` | `login_success`, `login_failed`, `logout` |
| `report` | `view`, `execute`, `export` |
| `datasource` | `create`, `update`, `delete`, `test` |
| `content` | report/view/dashboard/folder/resource create, update, delete, move |
| `security` | user/group CRUD, permission change, token create/revoke |
| `analysis` | Data Explorer query, export, save view/dashboard |
| `system` | reserved for system-level events |

Each row stores object type/id/name, status (`success` / `failure`), optional
duration, output format/size, IP, user agent, and a sanitized `details` JSON
document.

**Not recorded:** passwords, bearer tokens, datasource credentials, private
keys, OAuth secrets, API keys, session cookie values, report binary output, or
full query result sets. Raw SQL is off by default.

## Searching and Filtering

The Auditing page and `GET /api/v1/admin/audit` support server-side:

- Date range (`from` / `to`)
- Username, category, action, object type, status
- Text search across username, object name, object ID, event ID
- Pagination (`limit`, `offset`) and sorting

Click a row to open the event detail view (pretty-printed JSON details).

Export the current filter as CSV via **Export CSV** or
`GET /api/v1/admin/audit/export`.

## Report Execution Monitoring

Report execute/export events capture report id/name, user, duration, status,
output format, and size when available. Parameter values may be stored in
`details.parameters` when `JRS_AUDIT_LOG_PARAMETERS=true` (default), after
secret-name sanitization.

The **Slow Reports** tab aggregates executions in the database:

- Report name
- Execution count
- Average and maximum duration
- Failure count

Summary cards at the top of the page (default **Last 24 Hours**) show report
executions, failed executions, active users, exports, and average execution
time. Periods: 24 hours, 7 days, 30 days, or custom.

## Audit Retention

| Setting | Default |
|---------|---------|
| Retention window | 90 days |

Events older than the retention window are deleted by a periodic cleanup inside
`jrs-server`. Recent events are never deleted by this job. There is no
application API to edit or delete individual audit rows — the trail is
append-only from the application’s perspective.

## Configuration

Prefer `JRS_AUDIT_*`. Product-facing `AUDIT_*` aliases are also accepted.

| Variable | Default | Meaning |
|----------|---------|---------|
| `JRS_AUDIT_ENABLED` / `AUDIT_ENABLED` | `true` | Master switch |
| `JRS_AUDIT_RETENTION_DAYS` / `AUDIT_RETENTION_DAYS` | `90` | Days to keep |
| `JRS_AUDIT_LOG_PARAMETERS` / `AUDIT_LOG_PARAMETERS` | `true` | Store sanitized report parameters |
| `JRS_AUDIT_LOG_SQL` / `AUDIT_LOG_SQL` | `false` | Allow raw SQL in details (keep off unless required) |

Schema: `database/migrations/010_audit_log.sql` (also ensured at startup when
auditing is enabled).

### Administrator APIs

All require platform administrator:

| Method | Path |
|--------|------|
| `GET` | `/api/v1/admin/audit` |
| `GET` | `/api/v1/admin/audit/{event_id}` |
| `GET` | `/api/v1/admin/audit/summary` |
| `GET` | `/api/v1/admin/audit/slow-reports` |
| `GET` | `/api/v1/admin/audit/export` |

There are no create/update/delete endpoints for audit records.

## Security Considerations

Audit records may contain:

- Usernames
- IP addresses
- Report parameter values (when enabled)
- Resource names and identifiers
- Activity timestamps

Protect the application database accordingly. Do not grant ordinary users read
access to `audit_log`. Treat audit exports as sensitive.

Failed audit writes are logged to the application log and **do not** break the
original user operation.

## Related

- [Logging](logging.md)
- [Security](security.md)
- [Users](users.md)
- [Configuration](configuration.md)
