# AcuGIS Report Server — Standalone Apache Tomcat 10 Installation Guide

This guide deploys **acugis-report-server.war** to a normal Tomcat 10.x installation.
No Docker is required.

The existing Docker / Tomcat 9 deployment under `docker/report-server` is unchanged.

## Requirements

- JDK 11 or newer
- Apache Tomcat **10.0** or **10.1** (Jakarta Servlet 5/6)
- Network access from Tomcat to your databases (JNDI datasources)

> Tomcat 9 will **not** run this WAR (it still uses `javax.servlet`). Use the Docker build for Tomcat 9.

## 1. Install Tomcat 10

1. Download Tomcat 10.x from https://tomcat.apache.org/
2. Unpack to a directory, e.g. `C:\Tomcat` or `/opt/tomcat`
3. Set (optional but recommended):
   - `CATALINA_HOME` — Tomcat install directory
   - `CATALINA_BASE` — instance directory (often same as `CATALINA_HOME`)
4. Confirm `java -version` is 11+.

## 2. Build the WAR (developers)

From `stand-alone-tomcat-10`:

```powershell
powershell -File .\build.ps1
```

Or with Maven:

```bash
mvn -Pstandalone package
```

Output:

```
dist/
  acugis-report-server.war
  README.txt
  sample-acugis.properties
```

## 3. Copy the WAR

Copy `dist/acugis-report-server.war` to:

```
$CATALINA_BASE/webapps/acugis-report-server.war
```

Or for context path `/`:

```
$CATALINA_BASE/webapps/ROOT.war
```

The application never hard-codes the context path; URLs use `request.getContextPath()`.

## 4. Create the AcuGIS data directory

```
$CATALINA_BASE/acugis/
  reports/
  uploads/
  fonts/
  drivers/
  licenses/
  temp/
```

Windows example (`C:\Tomcat`):

```powershell
New-Item -ItemType Directory -Force -Path `
  C:\Tomcat\acugis\reports,
  C:\Tomcat\acugis\uploads,
  C:\Tomcat\acugis\fonts,
  C:\Tomcat\acugis\drivers,
  C:\Tomcat\acugis\licenses,
  C:\Tomcat\acugis\temp | Out-Null
```

Linux example:

```bash
mkdir -p /opt/tomcat/acugis/{reports,uploads,fonts,drivers,licenses,temp}
chown -R tomcat:tomcat /opt/tomcat/acugis
```

## 5. Configure `acugis.properties`

Copy `dist/sample-acugis.properties` to:

```
$CATALINA_BASE/conf/acugis.properties
```

Priority:

1. Environment variables (`ACUGIS_REPORTS_DIR`, `REPORTS_DIR`, …)
2. `$CATALINA_BASE/conf/acugis.properties`
3. `WEB-INF/classes/application.properties` (bundled defaults)

Common keys:

| Key | Default |
|-----|---------|
| `reports.dir` | `${catalina.base}/acugis/reports` |
| `uploads.dir` | `${catalina.base}/acugis/uploads` |
| `logs.dir` | `${catalina.base}/logs` |
| `temp.dir` | `${catalina.base}/temp` |
| `fonts.dir` | `${catalina.base}/acugis/fonts` |
| `drivers.dir` | `${catalina.base}/acugis/drivers` |
| `licenses.dir` | `${catalina.base}/acugis/licenses` |

## 6. Install JDBC drivers

JDBC drivers are **not** bundled in the WAR. Install them into Tomcat shared lib:

```
$CATALINA_HOME/lib/   (or $CATALINA_BASE/lib/)
  postgresql-*.jar
  mysql-connector-j-*.jar
  mariadb-java-client-*.jar
  mssql-jdbc-*.jar
  ojdbc11.jar
  sqlite-jdbc-*.jar
  snowflake-jdbc-*.jar
  duckdb_jdbc-*-windows_amd64.jar   # or linux_amd64 / osx_* matching the host
                                    # (run stand-alone-tomcat-10/sync-duckdb-jdbc.ps1)
```

Operator copies are available under `stand-alone-tomcat-10/lib/` in the source tree. Restart Tomcat after adding or replacing drivers.

`snowflake-jdbc.jar` is the unmodified Maven Central artifact `net.snowflake:snowflake-jdbc:3.22.0`. Its published POM names the Apache License 2.0. The license text from that version's source tag is `snowflake-jdbc-3.22.0-LICENSE.txt`. `mssql-jdbc.jar` is `com.microsoft.sqlserver:mssql-jdbc:12.8.1.jre11`. Its manifest and published POM name the MIT License, and `mssql-jdbc-12.8.1-LICENSE.txt` is the upstream license text. MySQL Connector/J keeps its license inside the JAR. Coordinates and checksums are in `JDBC-DRIVER-LICENSES.txt`.

> Note: `$CATALINA_BASE/acugis/drivers/` is scanned for startup diagnostics only — jars there are **not** on the classpath. Prefer `CATALINA_HOME/lib`.

Supported vendors: PostgreSQL, MySQL, MariaDB, SQL Server, Oracle, SQLite, Snowflake, DuckDB.

DuckDB reports do **not** use `context.xml` JNDI. The Go Report Server attaches `_duckdbSession`; Tomcat opens `jdbc:duckdb:` directly and fills via Jasper `REPORT_CONNECTION` (same architecture as AcuGIS Report Designer).

## 7. Configure JNDI datasources

Edit `$CATALINA_BASE/conf/context.xml` (or a per-host context file) and add `<Resource>` entries.
See `docker/report-server/context.xml.example` for PostgreSQL / MySQL / Oracle samples.

The report URL parameter `_dataSource` must match the JNDI resource name under `java:comp/env/jdbc/`.

## 8. Deploy reports

Place files in `$CATALINA_BASE/acugis/reports/` (or your configured `reports.dir`):

- `*.jrxml` / `*.jasper`
- images, subreports, and other resources referenced by relative path

No WAR rebuild is required when reports change.

## 9. Fonts

The WAR ships a **built-in JasperReports font extension** under `WEB-INF/classes/fonts/` so reports render the same on Linux, Windows, Docker, and Kubernetes without installing OS fonts.

Bundled families (PDF-embedded): **DejaVu** (Sans / Serif / Sans Mono), **Liberation** (Sans / Serif / Mono), **Nimbus** (Sans / Roman / Mono PS), **Carlito**, **Caladea**, **Cousine**, **Noto Sans**, **Noto Serif**.

Common legacy names are remapped automatically (JRXML unchanged), for example:

| Report `fontName` | Bundled family |
|-------------------|----------------|
| Arial, Tahoma | Liberation Sans |
| Helvetica, Helvetica Neue | Nimbus Sans |
| Times New Roman, Georgia | Liberation Serif |
| Times | Nimbus Roman |
| Calibri, Aptos | Carlito |
| Cambria | Caladea |
| Courier, Courier New | Liberation Mono |
| Consolas | Cousine |
| Verdana, SansSerif, Dialog | DejaVu Sans |
| Serif | DejaVu Serif |
| Monospaced, DialogInput | DejaVu Sans Mono |

Startup logs list loaded families at INFO. Missing individual font files log a warning and do not prevent startup.

Optional operator fonts (in addition to the WAR bundle):

- Drop files under `$CATALINA_BASE/acugis/fonts/`
- Optional Jasper family XML: `$CATALINA_BASE/acugis/fonts/fonts.xml`
- `ignore.missing.font=true` by default (unknown names do not crash the JVM)

## 10. License

Lookup order:

1. `ACUGIS_LICENSE_FILE` / `LICENSE_FILE` / `license.file`
2. `$CATALINA_BASE/acugis/licenses/`
3. `WEB-INF/license` inside the WAR
4. Evaluation mode (startup log notes this)

## 11. First startup

```bash
# Linux/macOS
$CATALINA_HOME/bin/catalina.sh run

# Windows
%CATALINA_HOME%\bin\catalina.bat run
```

Look in `$CATALINA_BASE/logs/catalina.*.log` for:

```
======== AcuGIS Report Server startup ========
```

Checks include writable reports/temp dirs, uploads/fonts/drivers presence, JDBC detection, fonts, license status, and Jasper compiler availability.

> **JRXML compile:** JasperReports 7 needs `jasperreports-jdt` in the WAR (included by the standalone `pom.xml`). Without it, compiling a `.jrxml` at runtime fails with `package net.sf.jasperreports.engine does not exist` / missing `JREvaluator`. Prefer deploying a matching `.jasper`, or rebuild/redeploy the WAR after pulling this dependency.

Smoke test (adjust host/port/context):

```
http://localhost:8080/acugis-report-server/report?_repName=demo&_repFormat=PDF&_dataSource=MainPG
```

For `ROOT.war`:

```
http://localhost:8080/report?_repName=demo&_repFormat=PDF&_dataSource=MainPG
```

## 12. Suggested layout

```
Tomcat/
├── webapps/
│     └── acugis-report-server.war
├── lib/                  ← JDBC drivers (required)
├── conf/
│     ├── context.xml     ← JNDI datasources
│     └── acugis.properties
├── logs/
├── temp/
└── acugis/
      ├── reports/
      ├── uploads/
      ├── fonts/
      ├── drivers/
      ├── licenses/
      └── temp/
```

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `javax/servlet` / `ClassNotFoundException: jakarta.servlet` | Wrong Tomcat major version | Use Tomcat 10.x |
| Report not found | Wrong `reports.dir` or missing JRXML | Check startup summary path; place files there |
| DataSource lookup failed | Missing JNDI `Resource` | Add `jdbc/<name>` to `context.xml` |
| JDBC driver failed to load | Driver jar not on classpath | Put jar in `CATALINA_HOME/lib` (or `CATALINA_BASE/lib`) and restart |
| Blank/missing glyphs in PDF | Font not in WAR bundle / unknown name | Use a bundled family or alias (see §9); or add custom fonts under `acugis/fonts` |
| HTML images 404 | Context path mismatch | Confirm `html.image.uri.pattern` uses `{contextPath}` |
| Cannot write temp files | Read-only filesystem / wrong temp | Set `temp.dir` to `$CATALINA_BASE/temp` |
| Map elements fail | `acugis-report-map.jar` missing from the WAR | Use a WAR that includes the current map renderer |
| `AcuGISFunctions cannot be resolved` | WAR has an older map jar (pre–Expression Builder) | Redeploy a WAR built with the current map renderer |
| `barcode4j:QRCode` / known type ids = `[iconLabel, spiderChart]` | `jasperreports-barcode4j` missing from WAR | Run `docker/report-server/sync-barcode-lib.ps1`, rebuild/redeploy WAR |

## Backward compatibility

| Target | Location | Servlet API | Notes |
|--------|----------|-------------|-------|
| Docker Tomcat 9 | `docker/report-server` | `javax.servlet` | Unchanged |
| Standalone Tomcat 10 | `stand-alone-tomcat-10` | `jakarta.servlet` | This guide |
