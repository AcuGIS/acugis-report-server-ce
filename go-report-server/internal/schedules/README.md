# Schedules package

Go port of PHP scheduling (`lib/schedule_*.php`, `lib/recipient_repository.php`,
`lib/execution_repository.php`, `lib/email_template_repository.php`).

## Responsibilities

- **Config / normalize** — repeat modes, validation, summaries, legacy cron derivation, systemd timer specs
- **Repository** — `jasper_schedule` CRUD with optional repeat columns
- **Recipients** — `jasper_recipient` CRUD
- **Executions** — `jasper_schedule_execution` create/finish/list
- **Paths** — `{schedule_dir}`, `output/{id}`, `logs/`, env files
- **Env files** — `{id}_env.sh` KEY=value (shell-quoted when needed)
- **Systemd** — service + timer unit files; `SyncAfterSave` / `Remove` (native Ubuntu)
- **Dispatcher** — container timer replacement (`jrs-schedule-dispatcher`) that execs existing `jrs-run-schedule <id>`
- **Runner** — HTTP execute against Jasper with `X-JRS-Scheduled-Run: 1`
- **Mail** — SMTP when `JRS_SMTP_HOST` is set; otherwise local sendmail (PHP `mail()` parity)
- **Email templates** — list/load HTML from `email_tmpl` directory

## Wiring (server)

```go
paths := schedules.NewPaths(cfg.Paths.ScheduleDir, cfg.Paths.ScheduleOutputDir, cfg.Paths.EmailTmplDir, cfg.Paths.AppRoot)

opts := schedules.Options{
    Pool:    pool,
    Schema:  cfg.Database.Schema,
    Paths:   paths,
    Jasper:  schedules.DefaultRunnerConfig(cfg.Jasper.ReportURL, cfg.Jasper.Timeout),
    Mail:    schedules.MailConfigFromEnv(),
    Systemd: schedules.DefaultSystemdOptions(cfg.Paths.AppRoot),
    AppRoot: cfg.Paths.AppRoot,
}

repo, recipients, executions, runner, systemd, templates := schedules.NewFromOptions(opts)
```

### Interfaces the server must satisfy for form normalize

Implement and pass into `NormalizePost`:

| Interface | Method | Typical adapter |
|-----------|--------|-----------------|
| `schedules.ReportChecker` | `ReadReport(ctx, key) (datasource, error)` | `reports.Store.Read` + `Definition.Datasource()` |
| `schedules.DatasourceChecker` | `FindDatasource(ctx, name) error` | `datasources.Repository.Find` |

Example adapters:

```go
type reportChecker struct{ store *reports.Store }

func (a reportChecker) ReadReport(ctx context.Context, key string) (string, error) {
    def, err := a.store.Read(key)
    if err != nil {
        return "", err
    }
    return def.Datasource(), nil
}

type datasourceChecker struct{ repo *datasources.Repository }

func (a datasourceChecker) FindDatasource(ctx context.Context, name string) error {
    _, err := a.repo.Find(ctx, name)
    return err
}
```

## Systemd behavior

- **Linux + systemctl** — writes env, unit files, symlinks, `daemon-reload`, `enable --now`
- **Non-Linux (e.g. Windows dev)** — writes env file (and unit files when possible); returns `ok=true` with `LastError` noting timers are Linux-only
- **Timer not required** (`never` / disabled / `now`) — best-effort cleanup, `ok=true`

Environment variables (same as PHP `config.php`):

| Variable | Default |
|----------|---------|
| `JRS_JASPER_SYSTEMD_UNIT_DIR` | `/opt/acugis-report-server/systemd` |
| `JRS_JASPER_SYSTEMD_SYSTEM_DIR` | `/etc/systemd/system` |
| `JRS_JASPER_SYSTEMCTL` | `/bin/systemctl` |
| `JRS_JASPER_SYSTEMD_USE_SUDO` | `true` (matches PHP `config.php`; native installer writes sudoers for `ln`/`rm`/`systemctl`) |
| `JRS_SCHEDULE_RUNTIME` | `systemd` (default, native) or `dispatcher` (containers). Does not change native timer units. |

### Container dispatcher (`jrs-schedule-dispatcher`)

Used by Docker / future Kubernetes instead of systemd timers. Same runner command as systemd's oneshot:

```bash
jrs-run-schedule <schedule_id>
```

| Variable | Default |
|----------|---------|
| `JRS_SCHEDULE_RUNTIME` | `systemd` |
| `JRS_SCHEDULER_POLL_INTERVAL` | `15s` |
| `JRS_SCHEDULER_LISTEN_ADDR` | `:9089` (`GET /health`) |
| `JRS_SCHEDULE_RUNNER` | `jrs-run-schedule` (beside this binary, or PATH) |
| `TZ` | process local timezone for calendar / `once` wall-clock times |

SMTP: `JRS_SMTP_HOST`, `JRS_SMTP_PORT`, `JRS_SMTP_USER`, `JRS_SMTP_PASS`, `JRS_SMTP_FROM`

## Filesystem layout

| Path | Purpose |
|------|---------|
| `{schedule_dir}/{id}_env.sh` | systemd EnvironmentFile |
| `{schedule_dir}/output/{id}/` | generated reports |
| `{schedule_dir}/logs/schedule-{id}.log` | run log |

## Tests

```bash
go test ./internal/schedules/...
```

Config/normalize tests run without a database using injected checker stubs.
