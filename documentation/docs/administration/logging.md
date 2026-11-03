# Logging

Process logs and the audit trail are different. Process logs are stdout and stderr. The audit trail is rows in PostgreSQL. See [Auditing](auditing.md).

## Where process logs go

| Installation | Where |
|--------------|--------|
| Native systemd `acugis-report-server.service` | The unit does not set `StandardOutput=`. `jrs-server` stdout and stderr go to the journal: `journalctl -u acugis-report-server`. |
| Native `acugis-tomcat.service` | Journal for the Tomcat process, plus Tomcat's own logs under `/opt/tomcat`. |
| Per-schedule systemd units | `StandardOutput=` and `StandardError=` append to the schedule log file under the schedule directory. |
| Docker Compose | `docker compose logs report-server` and `docker compose logs report-scheduler`. |

Request lines include method, path, status, duration, and the authenticated user name when a session or token was present. They are not a substitute for the audit UI.

## Do not log

- Personal access tokens or `Authorization` header values
- Database or datasource passwords
- Session cookie values
- Embed-token secrets, OIDC client secrets, SAML private keys, or `JRS_DESIGNER_INTERNAL_TOKEN`
- License tokens or activation keys
- Full environment dumps

`JRS_AUDIT_LOG_SQL` defaults to off. Leave it off unless you have a reason to store query text in the audit table.

## CLI

`acugis-report --verbose` and `--debug` affect the client. `--debug` is what adds stack traces. That output stays on the machine running the CLI.

## Related

- [Auditing](auditing.md)
- [Diagnostics](diagnostics.md)
- [Security](security.md)
