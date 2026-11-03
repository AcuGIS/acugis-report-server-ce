# Designer Web

Designer Web is the browser report editor that Report Server can proxy. It is not Report Designer Desktop, and it is not included in the Report Server package as a licensed feature.

Report Designer Desktop is the separate authoring application described in the [Report Designer](../user-guide/designer/overview.md) user guide. Designer Web is a second product (`report-designer-web`) that you run beside Report Server and attach with the variables below.

## What Report Server proxies

When Designer Web is configured, Report Server serves:

| Path | Role |
|------|------|
| `/reports/designer/new` | Create a report in the browser |
| `/reports/designer/import` | Import a desktop Designer project |
| `/reports/{key}/design` | Open a published report in Designer Web |
| `/api/designer/*` | Authenticated proxy to the Designer process |
| `/designer-assets/*` | Designer Web files from `JRS_DESIGNER_ASSETS_DIR`. Requires a signed-in Designer user and a valid Designer Web license. Not a public file directory. |

These routes are registered in both packages. They stay inactive until configuration is complete. The hub links for Designer Web are hidden until then. `/api/v1/edition` reports `report_designer: true` when the proxy is configured. That flag means configured, not licensed.

The upstream must be loopback: `http` or `https` on `127.0.0.1`, `localhost`, or `::1`, with no userinfo and no path. Report Server adds its own internal paths. A public Designer URL is rejected at startup.

Expected shape:

```text
Browser  →  Report Server (public HTTPS)
              →  http://127.0.0.1:<designer-port>   (same host, not published)
```

On Docker, the Designer process has to be reachable at a loopback address inside the `report-server` container. A Designer URL on another compose service name is not a loopback URL and will not pass configuration.

## Configuration

Set both of these, or neither:

| Variable | Required | Default |
|----------|----------|---------|
| `JRS_DESIGNER_UPSTREAM` | With the token | empty (Designer off) |
| `JRS_DESIGNER_INTERNAL_TOKEN` | With the upstream | empty; there is no generated default |
| `JRS_DESIGNER_PROXY_TIMEOUT` | no | `120s` |
| `JRS_DESIGNER_PROXY_MAX_BODY` | no | 64 MiB |
| `JRS_DESIGNER_ASSETS_DIR` | no | `{app root}/designer-web` |
| `ACUGIS_DESIGNER_WEB_VERSION` | no | `1.0.0` (sent only on Designer license calls) |

The app root is `JRS_JASPER_HOME` when that variable is set. Otherwise it is the install root detected at startup. `JRS_DESIGNER_INTERNAL_TOKEN` is a shared secret between `jrs-server` and the Designer process. Report Server sends it on proxy requests. It is not a user password, not a personal access token, and not written to diagnostics. Put it in the env file or a secret store and keep that file restricted.

Requests whose `Content-Length` exceeds the body limit are rejected. The proxy uses the configured timeout.

## License

Designer Web has its own license file, `report-designer-web.lic`, in the [license state directory](licensing.md). A Report Server Professional license does not enable Designer Web. An Open Source Report Server package can still use Designer Web when the proxy is configured and a Designer Web license is valid. The two products do not unlock each other.

**Administration → License** shows Report Designer Web as **Not installed** until the upstream and token are set. After that, a platform administrator can activate, refresh, or start a trial with a Designer Web key. A normal trial offer uses the day count the license service returns. The same recovery-label exception described in [Licensing](licensing.md) applies. The same installation ID is used. Designer Web does not create a new one.

Until a Designer Web license is valid, opening a Designer page shows the activation gate for an administrator and tells other users to contact an administrator. API callers receive `403` with “Report Designer Web is not licensed for this installation.”

## Related

- [Licensing](licensing.md)
- [Report Designer Desktop](../user-guide/designer/overview.md)
- [Environment variables](../reference/environment-variables.md)
- [Security](security.md)
