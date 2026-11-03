package server

import (
	"encoding/json"
	"html"
	"net/http"
	"strconv"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
)

func resourceSyncBrandHTML(chrome materialChromeData) string {
	if !chrome.ShowSidebarBrand {
		return ""
	}
	return `    <a class="fed-shell-sidebar-brand" href="` + html.EscapeString(chrome.BrandHome) + `"><i class="material-icons">assessment</i><span>` + html.EscapeString(chrome.ProductName) + `</span></a>
`
}

func (s *Server) resourceSyncer() acl.ResourceSyncer {
	if syncer, ok := s.deps.ACLWriter.(acl.ResourceSyncer); ok {
		return syncer
	}
	return nil
}

func (s *Server) handleResourceSyncPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	message := ""
	messageClass := ""
	var stats *acl.SyncStats

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if r.FormValue("sync_reports") != "" {
			syncer := s.resourceSyncer()
			if syncer == nil || s.deps.Reports == nil {
				message = "Application database is not configured or unreachable. Check JRS_DB_* / config.php."
				messageClass = "amber lighten-4 amber-text text-darken-4"
			} else {
				result := syncer.SyncAllReportResources(r.Context(), s.deps.Reports)
				stats = &result
				if result.Skipped {
					message = "Database is not connected; sync was skipped."
					messageClass = "amber lighten-4 amber-text text-darken-4"
				} else if result.OK {
					message = sprintfSyncOK(result)
					messageClass = "green lighten-4 green-text text-darken-4"
				} else {
					message = sprintfSyncErr(result)
					messageClass = "red lighten-4 red-text text-darken-4"
				}
			}
		}
	}

	apiPath := s.goURL("/api/resource-sync/index.php")
	statsJSON := ""
	if stats != nil && !stats.Skipped {
		if b, err := json.MarshalIndent(stats, "", "  "); err == nil {
			statsJSON = string(b)
		}
	}

	chrome := s.materialChrome("Resource sync")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderResourceSyncHTML(chrome, message, messageClass, apiPath, statsJSON, s.goURL("/reports/"))))
}

func (s *Server) handleAPIResourceSync(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}

	if r.Method == http.MethodGet {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"service": "acugis-report-server-go",
			"hint":    "POST to run a full Jasper report resource sync into the local resources table (idempotent).",
		})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	syncer := s.resourceSyncer()
	if syncer == nil || s.deps.Reports == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":      false,
			"service": "acugis-report-server-go",
			"error":   "application database unavailable (check JRS_DB_* / config.php)",
		})
		return
	}
	stats := syncer.SyncAllReportResources(r.Context(), s.deps.Reports)
	payload := map[string]any{
		"ok":             true,
		"service":        "acugis-report-server-go",
		"registered":     stats.Registered,
		"already_exists": stats.AlreadyExists,
		"errors":         stats.Errors,
		"skipped":        stats.Skipped,
	}
	if stats.Reason != "" {
		payload["reason"] = stats.Reason
	}
	if !stats.OK {
		payload["ok"] = false
	}
	s.writeJSON(w, http.StatusOK, payload)
}

func sprintfSyncOK(s acl.SyncStats) string {
	return "Sync finished: " + strconv.Itoa(s.Registered) + " newly registered, " +
		strconv.Itoa(s.AlreadyExists) + " already in catalog, " + strconv.Itoa(s.Errors) + " errors."
}

func sprintfSyncErr(s acl.SyncStats) string {
	return "Sync finished with errors: " + strconv.Itoa(s.Registered) + " registered, " +
		strconv.Itoa(s.AlreadyExists) + " already existed, " + strconv.Itoa(s.Errors) + " errors."
}

func renderResourceSyncHTML(chrome materialChromeData, message, messageClass, apiPath, statsJSON, catalogURL string) string {
	msgBlock := ""
	if message != "" {
		msgBlock = `<div class="card-panel ` + html.EscapeString(messageClass) + `" role="status">` +
			html.EscapeString(message) + `</div>`
	}
	statsBlock := ""
	if statsJSON != "" {
		statsBlock = `<pre class="jrs-url-output" style="margin-top:16px;">` + html.EscapeString(statsJSON) + `</pre>`
	}

	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>` + html.EscapeString(chrome.Title) + ` - ` + html.EscapeString(chrome.ProductName) + `</title>
<style id="fed-shell-critical">
.fed-shell-menu-btn{display:none!important}
.fed-shell-backdrop{display:none!important}
body.fed-shell-body{margin:0;padding-left:var(--fed-shell-sidebar-width,250px);box-sizing:border-box}
.fed-shell-sidebar{position:fixed;top:0;left:0;bottom:0;width:var(--fed-shell-sidebar-width,250px);z-index:1000;overflow:hidden;background:#f0f4f9}
@media (max-width:992px){
body.fed-shell-body{padding-left:0}
.fed-shell-sidebar{transform:translateX(-100%)}
body.fed-shell-open .fed-shell-sidebar{transform:translateX(0)}
body.fed-shell-open .fed-shell-backdrop{display:block!important}
.fed-shell-menu-btn{display:flex!important;align-items:center;justify-content:center;position:fixed;top:12px;left:12px;z-index:1002;width:44px;height:44px;border:none;border-radius:50%;background:#fff;cursor:pointer}
}
</style>
    <link rel="stylesheet" href="` + html.EscapeString(chrome.AssetsURL) + `/shell.css?v=20260727f" id="fed-shell-css">
    <link href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" rel="stylesheet">
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
    <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap" rel="stylesheet">
    <script>
      window.FEDERATION_SHELL_MOUNT = ` + string(chrome.ShellMount) + `;
      window.BASE_PATH = ` + string(chrome.ShellMount) + `;
      window.FEDERATION_PHP_MOUNT = ` + string(chrome.PHPMount) + `;
      window.FEDERATION_SHELL_ASSETS = ` + string(chrome.AssetsBase) + `;
    </script>
    <script src="` + html.EscapeString(chrome.AssetsURL) + `/shell-bootstrap.js?v=20261011b"></script>
    <style>` + string(chrome.CSS) + `</style>
</head>
<body data-federation-app="` + html.EscapeString(chrome.FederationApp) + `" class="fed-shell-body">
<aside id="fed-shell-sidebar" class="fed-shell-sidebar" aria-label="Application navigation">
` + resourceSyncBrandHTML(chrome) + `    <div class="fed-shell-nav-scroll" data-fed-shell-nav-mount></div>
</aside>
<button type="button" class="fed-shell-menu-btn" aria-label="Open menu" hidden><i class="material-icons">menu</i></button>
<div class="fed-shell-backdrop"></div>
<main class="fed-shell-main">
<div class="main-content">
<header class="page-header">
    <div>
        <h1 class="page-title">Resource catalog sync</h1>
        <p class="page-subtitle">Backfill Jasper reports into local <code>resources</code> (idempotent). Requires administrator.</p>
    </div>
    <div class="header-actions">
        <a href="` + html.EscapeString(catalogURL) + `" class="btn waves-effect waves-light grey">Back to reports</a>
    </div>
</header>

<div class="card">
    <div class="card-content">
        <span class="card-title">Jasper reports</span>
        <p class="muted-small">Upsert all <code>*.report.json</code> definitions and grant default permissions only for new rows.</p>
        ` + msgBlock + `
        <form method="post" class="stack-sm" style="margin-top:16px;">
            <input type="hidden" name="sync_reports" value="1">
            <button type="submit" class="btn waves-effect waves-light teal">
                <i class="material-icons left">sync</i>Sync reports to catalog
            </button>
        </form>
        ` + statsBlock + `
    </div>
</div>

<div class="card">
    <div class="card-content">
        <span class="card-title">API</span>
        <p>Programmatic trigger (session cookie required):</p>
        <pre class="jrs-url-output">POST ` + html.EscapeString(apiPath) + `</pre>
    </div>
</div>
</div>
</main>
</body>
</html>`
}
