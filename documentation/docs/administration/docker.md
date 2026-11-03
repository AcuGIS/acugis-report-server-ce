# Docker

Docker Compose pulls a published Report Server image. It does not build the application. This repository's GitHub Actions do not publish that image. The supported installation is Compose, because the stack includes PostgreSQL, Report Server, the scheduler, persistent volumes, networking, and health checks.

Edit `.env` before the first `docker compose up`. Replace the example passwords. There is no password-generation step.

```bash
git clone https://github.com/AcuGIS/report-server-docker
cd report-server-docker

nano .env
docker compose up -d
```

Set strong, unique values for:

- `JRS_DB_PASS` — PostgreSQL password used by Report Server. Compose passes the same value to PostgreSQL as `POSTGRES_PASSWORD`.
- `JRS_ADMIN_PASSWORD` — initial Report Server administrator password.

`JRS_ADMIN_USER` defaults to `admin`. The example `change-me-*` values cannot be used to start Report Server. Later `docker compose up`, `down`, `restart`, and image upgrades do not replace an existing password and do not reset the administrator password.

The standard installation is [report-server-docker](https://github.com/AcuGIS/report-server-docker). In this source repository the Compose files are under `docker/`.

Open `http://127.0.0.1:9088/`. The packaged base path is `/`. `JRS_HOST_PORT` changes the published host port (default `9088`).

The Compose file pulls the image named in `docker/compose.yml`. Set `REPORT_SERVER_IMAGE` to select another published tag. A local source build is separate: `docker compose -f compose.yml -f compose.build.yml up -d --build`.

## Docker Hub

The image on Docker Hub is not a standalone install. Do not use `docker run acugis/report-server` as the deployment method. Follow the Compose instructions above. `docker compose up` pulls the image when it is not already present. The Compose files are published in [report-server-docker](https://github.com/AcuGIS/report-server-docker).

## Architecture

One `report-server` container runs the Go application and Tomcat 10 with the JasperReports WAR. PostgreSQL and a scheduler container are separate services.

```text
Internet / LAN
      |
      v
report-server :9088   (Go UI/API, published on the host)
      |
      +--> Tomcat 10 :8080 (Compose network only)
      |
      +--> postgres :5432 (Compose network, or your own PostgreSQL)
      |
report-scheduler       (jrs-schedule-dispatcher -> jrs-run-schedule)
      |
      +--> postgres
      +--> http://report-server:8080/report
```

| Component | In this Compose file |
|-----------|----------------------|
| Go Report Server | `jrs-server`, host port `9088` |
| Tomcat / Jasper | Tomcat 10 inside `report-server`, port `8080`, not published |
| PostgreSQL | PostgreSQL 16 — image version is pinned in `docker/compose.yml` and is not published to a host port |
| Scheduler | `report-scheduler`, health port `9089`, not published |
| Java | 17 |

Release images are built from `docker/report-server/Dockerfile`. Customer Compose pulls the published image. Exact patch versions are pinned in that Dockerfile. See `docker/README.md`.

## Jasper URL

| Process | `JRS_JASPER_INTERNAL_REPORT_URL` |
|---------|----------------------------------|
| `report-server` | `http://127.0.0.1:8080/report` |
| `report-scheduler` | `http://report-server:8080/report` |

Port `9099` is the Go process fallback when that variable is unset. Compose sets `8080`. Do not point a Compose deployment at `9099`.

Inside the `report-server` container the Go process listens on `0.0.0.0:9088`. The host reaches it through the published port.

## Edition

Customer Compose pulls `acugis/report-server:1.0.21-oss`. The image compiles the Open Source server. Professional modules are not in this tree. `ACUGIS_EDITION` is not a license. See the [compatibility matrix](../reference/compatibility-matrix.md).

## Scheduling

Scheduled reports are handled by the `report-scheduler` service. No host cron or systemd timers are required, and you should not invoke the scheduler by hand.

The scheduler runs `jrs-schedule-dispatcher`, which polls due schedules and executes `jrs-run-schedule <schedule_id>`.

| Variable | Purpose |
|----------|---------|
| `JRS_DB_*` | Same application database as `report-server` |
| `JRS_SMTP_*` | Scheduled email |
| `TZ` | IANA timezone for calendar and `once` times, on both services (default `UTC`) |
| `JRS_SCHEDULER_POLL_INTERVAL` | Poll interval (default `15s`) |

```bash
docker compose logs report-scheduler
```

Per-schedule logs on the shared volume:

```text
/opt/acugis-report-server/schedules/logs/schedule-<id>.log
```

Native installs keep using systemd timers. They do not use this service.

## Optional QGIS Server

QGIS Server is not in the default stack. Native installs do not install it.

```bash
docker compose --profile qgis up -d
```

Copy QGIS Desktop `.qgz` / `.qgs` files into `docker/qgis/projects/` and file datasources into `docker/qgis/data/`. Designer uses `http://127.0.0.1:9088/qgis/` (or `https://<host>/qgis/` behind your proxy). Do not publish the QGIS container port. See `docker/qgis/README.md`.

## External PostgreSQL

Set `JRS_DB_HOST`, `JRS_DB_PORT`, `JRS_DB_NAME`, `JRS_DB_USER`, and `JRS_DB_PASS`, then remove or disable the `postgres` service. `docker/compose.external-postgres.example.yml` is an example of that override. Keep the same `report-server` and `report-scheduler` image.

## Useful commands

```bash
docker compose logs -f report-server
curl -sS http://127.0.0.1:9088/health
curl -sS http://127.0.0.1:9088/ready
docker compose down
```

`docker compose down -v` deletes the Compose volumes.

## Related

- [Installation](installation.md)
- [Deployment](deployment.md)
- [Configuration](configuration.md)
- Repository guide: `docker/README.md`
