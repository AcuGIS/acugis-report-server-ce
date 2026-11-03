# AcuGIS Report Server — Standalone Tomcat 10

Second deployment option: a standard **Jakarta EE WAR** for Apache Tomcat **10.x**.
No Docker required.

The Docker / Tomcat 9 build under [`docker/report-server`](../docker/report-server) is **not** modified by this tree.

## Build profiles

| Profile | Location | Output |
|---------|----------|--------|
| `docker` | `docker/report-server` | Existing image + `acugis-report-server.war` (Tomcat 9 / `javax.*`) |
| `standalone` | `stand-alone-tomcat-10` | `dist/acugis-report-server.war` (Tomcat 10 / `jakarta.*`) |

```powershell
cd stand-alone-tomcat-10
powershell -File .\build.ps1
```

```bash
cd stand-alone-tomcat-10
mvn -Pstandalone package
```

Produces:

```
dist/
  acugis-report-server.war
  README.txt
  sample-acugis.properties
```

## Highlights vs Docker

- External config: env → `CATALINA_BASE/conf/acugis.properties` → bundled defaults
- Data dirs under `${catalina.base}/acugis/` (reports, uploads, fonts, drivers, licenses)
- Built-in JasperReports font extension (DejaVu, Liberation, Nimbus, Croscore, Noto) with legacy font-name aliases — no OS fonts required
- JDBC drivers from `CATALINA_HOME/lib` (or `CATALINA_BASE/lib`) — not bundled in the WAR
- Context-path aware URLs (`request.getContextPath()`)
- Startup verification summary in Tomcat logs
- Standard Tomcat / JUL logging to `CATALINA_BASE/logs`

See **[INSTALL.md](INSTALL.md)** for the full deployment guide.
