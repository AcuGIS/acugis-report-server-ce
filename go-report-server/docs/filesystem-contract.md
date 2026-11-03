# Filesystem contract

Install root = `JRS_JASPER_HOME` when set, otherwise the PHP service directory
(`dirname(config.php)`). Go resolves the same root via `JRS_JASPER_HOME` or by
detecting the parent of `go-report-server/` during development.

The Go foundation originally only **inspected** these paths. Publish/edit/delete
now write the same report JSON, JRXML, `.jasper`, and thumbnail files PHP does.
Schedule save/run under `/go` also writes env files, output, and (on Linux) unit files.
Startup still must not rewrite `context.xml` or schedule env files.

| Path | Default location | Readers (PHP) | Writers |
|------|------------------|---------------|---------|
| Report JSON | `{root}/reports/json/*.report.json` | `jrs_read_report`, catalog, APIs, execute ACL precheck | PHP + Go publish/edit/delete |
| JRXML | `{root}/docker/report-server/reports/*.jrxml` | Tomcat/Jasper volume, publish validation | PHP + Go publish/edit |
| Compiled Jasper | `{root}/docker/report-server/reports/*.jasper` | Tomcat | Jasper compile / PHP+Go upload |
| Thumbnails | `{root}/assets/reports/*.png` | `report_thumbnail.php`, catalog UI | PHP + Go publish/edit |
| Content resources | `JRS_RESOURCES_DIR` or `{root}/resources/{uuid}{ext}` | Resource download APIs (non-image) | Go contentresource |
| Image resource thumbs | `JRS_RESOURCE_THUMBNAILS_DIR` or `{root}/thumbnails/{uuid}.png` | Resource thumbnail API, catalog UI | Go contentresource |
| Image resource paths | `JRS_RESOURCE_PATHS_DIR` or sibling of JRXML dir `resource-paths/{folder}/…/{file}` | Tomcat Jasper fill (JRXML image paths) | Go contentresource |
| Jasper context.xml | `JRS_JASPER_CONTEXT_PATH` or `{root}/docker/report-server/context.xml` | Tomcat | PHP + Go generate context |
| Schedule dir | `JRS_JASPER_SCHEDULE_DIR` or `{root}/schedules` | runner, systemd helpers | PHP + Go schedule create/update |
| Schedule env files | `{schedule_dir}/{id}_env.sh` | systemd / runner | PHP + Go `SyncAfterSave` |
| Schedule output | `{schedule_dir}/output/{id}/` | history UI, email attach | PHP + Go runner |
| Schedule logs | `{schedule_dir}/logs/` | ops | PHP + Go runner / systemd helpers |
| Email templates | `JRS_JASPER_EMAIL_TMPL_DIR` or `{root}/email_tmpl/` | `lib/email_template_repository.php`, mailer | admins (files) |
| Shared UI assets | `/shared/ui/` (referenced as absolute web path) | shells | external / shared |
| App assets | `{root}/assets/` | pages | various |
| systemd unit dir | `JRS_JASPER_SYSTEMD_UNIT_DIR` (default `/opt/acugis-report-server/systemd`) | systemctl | schedule install |
| Docker Tomcat reports volume | `docker/report-server/reports` → container jasper_reports/reports | Tomcat | PHP publish |

## Safety rules for Go

- Use `SafeJoin` (or equivalent) to reject `..` traversal outside configured roots
- Publish/edit/delete may write report JSON, JRXML, `.jasper`, and thumbnails only
- Content resource upload/replace/delete may write `{root}/resources/` and `{root}/thumbnails/` only
- Datasource admin may write `jasper_datasource` rows and regenerate Tomcat `context.xml`
- Schedule save/run under `/go` may write env files, output, logs, and (Linux) systemd units
- Startup diagnostics remain inspect-only
- Do not enable production Apache cutover or new production timers until approved
