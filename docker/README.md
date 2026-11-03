# AcuGIS Report Server — Docker deployment (packaging only)

Official Compose packaging lives under `docker/` with the application image
built from `docker/report-server/Dockerfile`.

This package runs the **existing** AcuGIS Report Server using Docker Compose:

- existing `jrs-server` (Go)
- existing standalone Tomcat **10** + Jakarta WAR
- published JDBC drivers, plus customer-supplied IBM Db2
- Compose PostgreSQL (or your own external PostgreSQL)

This source tree builds the Open Source image only (`-tags oss`).
Professional modules are not included. `ACUGIS_EDITION` is not a license.

## Architecture

```text
Internet / LAN
      |
      v
report-server :9088   (Go UI/API)
      |
      +--> Tomcat 10 :8080 (Compose network; not published to the host)
      |
      +--> postgres :5432 (Compose internal, or external)
      |
report-scheduler       (jrs-schedule-dispatcher -> jrs-run-schedule)
      |
      +--> postgres (same JRS_DB_*)
      +--> Tomcat execute URL (JRS_JASPER_INTERNAL_REPORT_URL)
```

## Quick start

Edit `.env` before the first start. Replace the example passwords. There is no password-generation step.

```bash
git clone https://github.com/AcuGIS/report-server-docker
cd report-server-docker

nano .env
docker compose up -d
```

Set strong, unique values for:

- `JRS_DB_PASS` — PostgreSQL password used by Report Server. The Compose PostgreSQL service receives that same value as `POSTGRES_PASSWORD`.
- `JRS_ADMIN_PASSWORD` — initial Report Server administrator password.

`JRS_ADMIN_USER` defaults to `admin`. The supplied `.env` uses `change-me-strong-password` and `change-me-admin-password`. Those example values cannot be used to start Report Server. A later `docker compose up -d`, `docker compose down`, `docker compose restart`, and an image upgrade leave the passwords you set in `.env` unchanged. Sign in with `JRS_ADMIN_USER` and `JRS_ADMIN_PASSWORD` from `.env`.

The standard installation is [https://github.com/AcuGIS/report-server-docker](https://github.com/AcuGIS/report-server-docker). In this source repository, run the same Compose commands from `docker/`.

`docker compose up` pulls the image named in `compose.yml` when it is not already present. It does not build the application.
A local source build is `docker compose -f compose.yml -f compose.build.yml up -d --build`.

Open: http://localhost:9088/

Administrator (after seed):

- user: value of `JRS_ADMIN_USER` (default `admin`)
- password: `JRS_ADMIN_PASSWORD` in `.env`

## Docker Hub

The supported deployment is Docker Compose. It includes PostgreSQL, Report Server, the scheduler, persistent volumes, networking, and health checks.

Do not use `docker run acugis/report-server` as the normal deployment method. That command does not start the rest of the stack.

Installation is the Quick start above. `docker compose up` pulls `acugis/report-server` from Docker Hub when the image is not already present. The Compose files for that installation are in [https://github.com/AcuGIS/report-server-docker](https://github.com/AcuGIS/report-server-docker).

## Runtime versions (pinned in Dockerfile / Compose)

| Component | Version |
|-----------|---------|
| Go toolchain (build) | 1.26.3 |
| JDK | Eclipse Temurin 17 |
| Tomcat | 10.1.57 |
| Jasper WAR | `stand-alone-tomcat-10/dist/acugis-report-server.war` (JR 7.0.1 Jakarta) |
| PostgreSQL | 16.9-alpine |
| DuckDB JDBC | 1.5.5.1 linux_amd64 |

## Ports

| Service | Port | Exposure |
|---------|-----:|----------|
| Go Report Server | 9088 | Host (`JRS_HOST_PORT`) |
| Scheduler health | 9089 | Internal only |
| Tomcat / Jasper | 8080 | Internal only (reachable by `report-scheduler`) |
| PostgreSQL | 5432 | Internal only |

## Volumes

| Volume / mount | Purpose |
|----------------|---------|
| `postgres-data` | Application database |
| `report-resources` | `/opt/acugis-report-server` — shared by `report-server` and `report-scheduler` (report JSON, resources, schedule env/output/logs, DuckDB home) |
| `report-tomcat-acugis` | Tomcat `acugis/` (JRXML, uploads, fonts, resource-paths) — shared so the scheduler can resolve report files |
| `report-tomcat-conf` | Persisted `context.xml` and map connection properties |
| `./data` → `/data` | Local DuckDB / SQLite / GeoPackage files (shared) |
| `./qgis/projects` → `/qgis/projects` | Optional QGIS Desktop projects (`--profile qgis`) |
| `./qgis/data` → `/qgis/data` | Optional QGIS file datasources (`--profile qgis`) |

## Database bootstrap

On first start the entrypoint:

1. Waits for PostgreSQL
2. Applies existing `database/schema.sql`
3. Applies existing `database/migrations/*.sql`
4. Applies existing `database/migrate_*.sql`
5. Records marker table `acugis_docker_schema_applied`

## Admin bootstrap

Uses existing `scripts/seed_admin.php` unchanged (PHP CLI in the image):

```bash
JRS_ADMIN_USER=admin
JRS_ADMIN_PASSWORD=...
```

## Editions

This source tree publishes the Open Source image:

```text
acugis/report-server:1.0.21-oss
acugis/report-server:oss
```

```bash
make docker-build-oss VERSION=1.0.21
```

`make docker-push-oss` pushes those tags. Package builds do not push.
Customer Compose pulls `acugis/report-server:1.0.21-oss`.

The image compiles with `-tags oss`. Professional modules are not in this tree.
Installation ID and `license.lic` are stored at `ACUGIS_LICENSE_STATE_DIR`
(default in Compose: `/opt/acugis-report-server/var/license` on the
`report-resources` volume, shared with the scheduler). `ACUGIS_EDITION` does
not unlock Professional features.

## External PostgreSQL

1. Set `JRS_DB_HOST`, `JRS_DB_PORT`, `JRS_DB_NAME`, `JRS_DB_USER`, `JRS_DB_PASS` (and SSL if needed).
2. Remove or disable the `postgres` service in `compose.yml` (or merge `compose.external-postgres.example.yml`).
3. Keep the same `report-server` / `report-scheduler` image. Both services use `JRS_DB_*` only.

## Scheduling

Scheduled reports are supported in Docker. Timing is handled automatically by
the `report-scheduler` service. **No host cron or systemd configuration is
required**, and you should not invoke the scheduler by hand.

The scheduler container runs `jrs-schedule-dispatcher`, which polls due
schedules and executes the existing oneshot command:

```text
jrs-run-schedule <schedule_id>
```

That is the same Go runner used for **Run Now**. Native Ubuntu installs are
unchanged: they continue to use systemd timers that call
`php scripts/run_schedule.php <schedule_id>`.

Configuration comes from the normal Docker environment (`.env` plus Compose
`environment:`). Important values:

| Variable | Purpose |
|----------|---------|
| `JRS_DB_*` | Application PostgreSQL (same as report-server) |
| `JRS_SMTP_*` | Scheduled email delivery |
| `TZ` | IANA timezone for calendar / `once` wall-clock times (set on **both** `report-server` and `report-scheduler`; default `UTC`) |
| `JRS_SCHEDULER_POLL_INTERVAL` | How often due schedules are checked (default `15s`) |
| `JRS_JASPER_INTERNAL_REPORT_URL` | Set automatically for the scheduler to `http://report-server:8080/report` |

Inspect scheduler logs:

```bash
docker compose logs report-scheduler
```

Per-schedule run logs also land on the shared volume:

```text
/opt/acugis-report-server/schedules/logs/schedule-<id>.log
```

The same scheduler image/command is intended for a future Kubernetes workload
(environment variables + shared/external PostgreSQL, Jasper URL, and volumes).

## Demo fixtures (optional)

With `ACUGIS_DOCKER_SEED_DEMO=1` (default), first boot seeds:

- PostgreSQL datasource `DemoPG` + report `hello` (PDF)
- DuckDB file `/data/demo.duckdb` + datasource `DemoDuckDB` + report `hello_duckdb`

UI assets (`reports/portal.css`, `shared/ui/*`, `assets/js/*`) are packaged in
the image and refreshed into the `report-resources` volume on every container
start.

Demo boot upserts only the `jdbc/DemoPG` resource in `context.xml` (it does not
replace other JNDI resources). If credentials drift, use **Generate Context
XML** in Datasources to rebuild from the catalog.

## Known limitations (Docker v1)

### IBM Db2 and Oracle JDBC

IBM Db2 (`db2jcc4.jar`) is not in the image. The jar in this repository has no
license text, and the repository does not record redistribution permission.
Put a customer-supplied jar in `docker/jdbc-drivers/` (or set
`JRS_JDBC_DRIVERS_PATH`). The entrypoint copies `*.jar` from
`/usr/local/tomcat/acugis/drivers` into Tomcat's shared lib before Tomcat starts.

Oracle JDBC stays in the Professional image. The unmodified `ojdbc11.jar`
includes Oracle Free Distribution, Hosting, and Use Terms
(`META-INF/license.txt`), which permit redistributing the unmodified jar when
that license accompanies it. The Open Source image does not include it. The
same customer drivers directory can supply it there. Oracle and Db2 application
support remains either way.

### MariaDB JDBC jar

Standalone `INSTALL.md` mentions MariaDB, but `stand-alone-tomcat-10/lib` does
not currently vendor a MariaDB driver jar. Docker packages only the jars present
there. Add the jar to Tomcat `lib` in a future packaging update if/when standalone vendors it.

### Reverse proxy / HTTPS

Do not bundle nginx/Caddy/Traefik. Terminate TLS at your own reverse proxy and
forward to `:9088`. The application already honors `X-Forwarded-Proto` /
`X-Forwarded-Host` for OIDC/SAML absolute URLs.

## Optional QGIS Server

QGIS Server is a map source, not part of the default stack. Native OS installs
do not install QGIS.

```bash
docker compose --profile qgis up -d
```

Copy QGIS Desktop `.qgz` / `.qgs` files into `docker/qgis/projects/` and file
datasources into `docker/qgis/data/`. Designer uses `http://localhost:9088/qgis/`
(or HTTPS `/qgis/` behind your reverse proxy). Do not publish the QGIS container
port or FastCGI. See `docker/qgis/README.md`.

## Useful commands

```bash
# Logs
docker compose logs -f report-server
docker compose logs -f report-scheduler

# Health (process)
curl -sS http://127.0.0.1:9088/health

# Ready (Postgres + required paths; Jasper reported but not gating — unchanged app behavior)
curl -sS http://127.0.0.1:9088/ready

# Stop
docker compose down

# Stop and remove volumes (destructive)
docker compose down -v
```

## Backup

Back up:

1. PostgreSQL volume / dump
2. `report-resources` volume
3. `report-tomcat-acugis` volume
4. `report-tomcat-conf` volume
5. `./data` bind mount (local datasource files)
6. `.env` (secrets)

## Relationship to standalone OS install

Bare-metal / systemd installs are unchanged. This `docker/` tree is an additive
deployment option only.
