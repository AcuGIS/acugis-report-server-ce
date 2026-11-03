package server

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

func (s *Server) qcartaProfile() bool {
	return s != nil && s.cfg.IsQCartaProfile()
}

func (s *Server) federationAppID() string {
	if s.qcartaProfile() {
		return "report-runtime"
	}
	return "jasper"
}

func (s *Server) qcartaDBTypeKeys() []string {
	if s.qcartaProfile() {
		return datasources.QCartaDBTypeKeys
	}
	return nil
}

func (s *Server) handleCentralLoginRedirect(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(s.cfg.CentralLoginURL)
	if target == "" {
		target = "/login"
	}
	next := safeLoginNext(r.URL.Query().Get("next"), s.defaultLoginNext())
	u, err := url.Parse(target)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	q := u.Query()
	if q.Get("next") == "" {
		q.Set("next", next)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

func (s *Server) handleCentralLogoutRedirect(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(s.cfg.CentralLoginURL)
	if target == "" {
		target = "/logout"
	} else {
		target = strings.TrimSuffix(target, "/login")
		if target == "" {
			target = "/"
		}
		if !strings.HasSuffix(target, "/logout") {
			target = strings.TrimRight(target, "/") + "/logout"
		}
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) handleQCartaDiscovery(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	items := []map[string]any{}
	if s.deps.Reports != nil {
		listed, err := s.deps.Reports.List()
		if err != nil {
			s.logger.Warn("qcarta discovery list", "error", err)
		}
		for _, item := range listed {
			if item.Type == "map" {
				continue
			}
			if s.deps.ACL != nil {
				ok, err := s.deps.ACL.CanViewReport(r.Context(), user, item.Key)
				if err != nil || !ok {
					continue
				}
			}
			items = append(items, map[string]any{
				"source_service":     "report-runtime",
				"source_resource_id": item.Key,
				"title":              item.Name,
				"description":        item.Description,
				"url":                s.goURL("/reports/" + item.Key),
				"card_type":          "report",
				"icon":               "assessment",
				"category":           "Reports",
			})
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"service":       "report-runtime",
		"resource_type": "report",
		"items":         items,
		"total":         len(items),
	})
}

func (s *Server) qcartaRequireService(w http.ResponseWriter, r *http.Request) bool {
	user := auth.FromContext(r.Context())
	if s.deps.ACL == nil {
		return true
	}
	ok, err := s.deps.ACL.CanAccessReportService(r.Context(), user)
	if err == nil && ok {
		return true
	}
	if !auth.IsAuthenticated(r.Context()) {
		s.handleCentralLoginRedirect(w, r)
		return false
	}
	http.Error(w, "forbidden", http.StatusForbidden)
	return false
}

func (s *Server) writeQCartaChrome(w http.ResponseWriter, title string, mainInner string) {
	chrome := s.materialChrome(title)
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">`)
	b.WriteString(`<title>` + html.EscapeString(title) + `</title>`)
	b.WriteString(`<link rel="stylesheet" href="` + html.EscapeString(chrome.AssetsURL) + `/shell.css">`)
	b.WriteString(`<link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">`)
	b.WriteString(`<script>window.FEDERATION_SHELL_MOUNT=` + string(chrome.ShellMount) + `;window.FEDERATION_SHELL_ASSETS=` + string(chrome.AssetsBase) + `;</script>`)
	b.WriteString(`<script src="` + html.EscapeString(chrome.AssetsURL) + `/shell-bootstrap.js?v=20261011b" defer></script>`)
	b.WriteString(`</head><body data-federation-app="` + html.EscapeString(s.federationAppID()) + `" class="fed-shell-body">`)
	b.WriteString(`<aside id="fed-shell-sidebar" class="fed-shell-sidebar"><div class="fed-shell-nav-scroll" data-fed-shell-nav-mount></div></aside>`)
	b.WriteString(mainInner)
	b.WriteString(`</body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) handleQCartaWorkspace(w http.ResponseWriter, r *http.Request) {
	if !s.qcartaRequireService(w, r) {
		return
	}
	s.writeQCartaChrome(w, "Reports", `<main class="fed-shell-main" data-federation-workspace></main>`)
}

func (s *Server) reportsCatalogURL() string {
	if s.qcartaProfile() {
		return s.goURL("/reports")
	}
	return s.goURL("/")
}

func (s *Server) handleQCartaCatalog(w http.ResponseWriter, r *http.Request) {
	if !s.qcartaRequireService(w, r) {
		return
	}
	user := auth.FromContext(r.Context())
	type row struct {
		Key, Name, Description, ViewURL, ExecuteURL, EditURL, DeleteURL string
		CanEdit, CanDelete                                              bool
	}
	var rows []row
	if s.deps.Reports != nil {
		listed, err := s.deps.Reports.List()
		if err != nil {
			s.logger.Warn("qcarta catalog list", "error", err)
		}
		for _, item := range listed {
			if item.Type == "map" {
				continue
			}
			if s.deps.ACL != nil {
				ok, err := s.deps.ACL.CanViewReport(r.Context(), user, item.Key)
				if err != nil || !ok {
					continue
				}
			}
			canEdit := false
			canDelete := false
			if ok, err := s.canEditReport(r, user, item.Key); err == nil {
				canEdit = ok || s.isPlatformAdmin(user)
			}
			if ok, err := s.canAdminReport(r, user, item.Key); err == nil {
				canDelete = ok || s.isPlatformAdmin(user)
			}
			rows = append(rows, row{
				Key:         item.Key,
				Name:        item.Name,
				Description: item.Description,
				ViewURL:     s.goURL("/reports/" + item.Key),
				ExecuteURL:  s.goURL("/reports/execute?_repName=" + url.QueryEscape(item.Key) + "&_dataSource=" + url.QueryEscape(item.Datasource) + "&_repFormat=html"),
				EditURL:     s.goURL("/reports/edit_report?report=" + url.QueryEscape(item.Key)),
				DeleteURL:   s.goURL("/reports/delete_report?report=" + url.QueryEscape(item.Key)),
				CanEdit:     canEdit,
				CanDelete:   canDelete,
			})
		}
	}
	var b strings.Builder
	b.WriteString(`<main class="fed-shell-main" data-federation-workspace-static="true"><div class="main-content" style="padding:32px 40px">`)
	b.WriteString(`<style>
.qcarta-reports-table{width:100%;border-collapse:collapse;margin-top:20px}
.qcarta-reports-table th,.qcarta-reports-table td{text-align:left;padding:10px 12px;border-bottom:1px solid #e0e4e8;vertical-align:top}
.qcarta-reports-table th{color:#44515c;font-weight:500}
.qcarta-reports-table .actions{white-space:nowrap}
.qcarta-reports-table .actions a{margin-right:12px}
.qcarta-reports-table .actions a.danger{color:#b71c1c}
</style>`)
	b.WriteString(`<h1>Reports</h1><p>Publish and run reports. Use a stable execute URL to call a report from another QCarta page.</p>`)
	b.WriteString(`<p><a href="` + html.EscapeString(s.goURL("/reports/publish_report")) + `">Publish</a>`)
	if ok, err := s.canEditReportService(r, user); err == nil && (ok || s.isPlatformAdmin(user)) && s.designerConfigured() {
		b.WriteString(` · <a href="` + html.EscapeString(s.goURL("/reports/designer/new")) + `">Create Designer Report</a> · <a href="` + html.EscapeString(s.goURL("/reports/designer/import")) + `">Import Designer Project</a>`)
	}
	b.WriteString(` · <a href="` + html.EscapeString(s.goURL("/reports/datasources")) + `">Datasources</a> · <a href="` + html.EscapeString(s.goURL("/reports/report_runner")) + `">URL builder</a></p>`)
	if len(rows) == 0 {
		b.WriteString(`<p>No reports are visible yet.</p>`)
	} else {
		b.WriteString(`<table class="qcarta-reports-table"><thead><tr><th>Report</th><th>Description</th><th>Actions</th></tr></thead><tbody>`)
		for _, row := range rows {
			b.WriteString(`<tr><td><a href="` + html.EscapeString(row.ViewURL) + `">` + html.EscapeString(row.Name) + `</a></td><td>`)
			b.WriteString(html.EscapeString(row.Description))
			b.WriteString(`</td><td class="actions"><a href="` + html.EscapeString(row.ExecuteURL) + `">Execute</a>`)
			if row.CanEdit {
				b.WriteString(`<a href="` + html.EscapeString(row.EditURL) + `">Edit</a>`)
				if _, design := s.annotateDesignerReport(r, user, row.Key); design {
					b.WriteString(`<a href="` + html.EscapeString(s.goURL("/reports/"+row.Key+"/design")) + `">Edit in Designer</a>`)
				}
			}
			if row.CanDelete {
				b.WriteString(`<a class="danger" href="` + html.EscapeString(row.DeleteURL) + `">Delete</a>`)
			}
			b.WriteString(`</td></tr>`)
		}
		b.WriteString(`</tbody></table>`)
	}
	b.WriteString(`</div></main>`)
	s.writeQCartaChrome(w, "Reports", b.String())
}

func (s *Server) renderQCartaDeleteConfirm(w http.ResponseWriter, key, name string) {
	if name == "" {
		name = key
	}
	action := s.goURL("/reports/delete_report")
	cancel := s.reportsCatalogURL()
	var b strings.Builder
	b.WriteString(`<main class="fed-shell-main" data-federation-workspace-static="true"><div class="main-content" style="padding:32px 40px">`)
	b.WriteString(`<h1>Delete ` + html.EscapeString(`"`+name+`"`) + `?</h1>`)
	b.WriteString(`<p>This removes the published report from QCarta Reports.</p>`)
	b.WriteString(`<p>This action cannot be undone.</p>`)
	b.WriteString(`<form method="post" action="` + html.EscapeString(action) + `">`)
	b.WriteString(`<input type="hidden" name="report" value="` + html.EscapeString(key) + `">`)
	b.WriteString(`<p><button type="submit">Delete</button> · <a href="` + html.EscapeString(cancel) + `">Cancel</a></p>`)
	b.WriteString(`</form></div></main>`)
	s.writeQCartaChrome(w, "Delete Report", b.String())
}

func (s *Server) rejectQCartaMapReport(reportType string) string {
	if s.qcartaProfile() && reportType == "map" {
		return "Map Reports are not available in the QCarta report runtime."
	}
	return ""
}

func (s *Server) reportServiceKey() string {
	return qcartaACLServiceKey(s.qcartaProfile())
}

func qcartaACLServiceKey(qcarta bool) string {
	if qcarta {
		return acl.ServiceKeyReportRuntime
	}
	return acl.ServiceKeyJasperReports
}
