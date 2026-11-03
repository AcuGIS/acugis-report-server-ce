package server

import (
	"fmt"
	"html"
	"html/template"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/modules"
)

// shellCriticalCSS is inlined in <head> so first paint matches the desktop shell
// without waiting for shell.css or JS (keep in sync with shared/ui/shell-critical.css).
const shellCriticalCSS = `html,body{background:#f8f9fa;margin:0}
html{scrollbar-gutter:stable}
.fed-shell-menu-btn{display:none!important}
.fed-shell-backdrop{display:none!important}
body.fed-shell-body{margin:0;padding-left:var(--fed-shell-sidebar-width,250px);box-sizing:border-box;background:#f8f9fa;min-height:100vh;font-family:Roboto,system-ui,-apple-system,"Segoe UI","Helvetica Neue",Helvetica,Arial,sans-serif;color:#202124;-webkit-font-smoothing:antialiased}
.fed-shell-sidebar{position:fixed;top:0;left:0;bottom:0;width:var(--fed-shell-sidebar-width,250px);z-index:1000;overflow:hidden;background:#f7f8fc;display:flex;flex-direction:column}
.fed-shell-sidebar-brand{display:block;padding:24px 24px 18px;text-decoration:none;color:#202124;font-size:18px;font-weight:500;line-height:1.3;min-height:calc(18px*1.3)}
.fed-shell-sidebar-brand .material-icons{display:none}
.fed-shell-nav-scroll{flex:1;overflow-y:auto;padding:4px 12px 24px}
.fed-shell-nav-section-label{padding:24px 16px 10px;font-size:11px;font-weight:600;letter-spacing:.08em;text-transform:uppercase;color:#6b7280;line-height:1.2}
.fed-shell-nav-list{list-style:none;margin:0;padding:0}
.fed-shell-nav-list>li{margin:0 0 10px}
.fed-shell-nav-list a{display:flex;align-items:center;gap:14px;padding:8px 16px 8px 10px;border-radius:28px;text-decoration:none;color:#202124;font-size:14px;font-weight:500;line-height:20px;min-height:52px;box-sizing:border-box;background:transparent}
.fed-shell-nav-list li.is-active>a{background:#eef3fd}
.fed-shell-nav-label{flex:1;min-width:0;line-height:20px}
.fed-shell-nav-icon{flex-shrink:0;display:inline-flex;align-items:center;justify-content:center;width:36px;height:36px;border-radius:50%;overflow:hidden}
.fed-shell-nav-icon .material-icons{font-size:18px;width:18px;height:18px;line-height:18px;overflow:hidden}
.fed-shell-nav-icon--blue{background:#d3e3fd;color:#041e49}
.fed-shell-nav-icon--teal{background:#c4e7ed;color:#004d40}
.fed-shell-nav-icon--purple{background:#e8def8;color:#311b92}
.fed-shell-nav-icon--grey{background:#e8eaed;color:#5f6368}
.fed-shell-nav-footer{margin-top:16px;padding-top:12px;border-top:1px solid rgba(60,64,67,.12)}
.fed-shell-main{min-height:100vh;box-sizing:border-box;padding:24px 0 48px 140px;max-width:1200px}
.fed-shell-page-title{font-size:1.75rem;font-weight:400;color:#202124;margin:0 0 8px;line-height:1.3;min-height:calc(1.75rem*1.3)}
.fed-shell-page-subtitle{margin:0 0 32px;color:#5f6368;font-size:.9375rem;line-height:1.5;min-height:calc(.9375rem*1.5)}
.fed-app-nav-section-title{font-size:.75rem;font-weight:500;letter-spacing:.06em;text-transform:uppercase;color:#5f6368;margin:0 0 8px;padding:0 4px}
.fed-app-nav-card{background:#fff;border:1px solid #e8eaed;border-radius:12px;overflow:hidden}
.fed-app-nav-list{list-style:none;margin:0;padding:0}
.fed-app-nav-list a{display:flex;align-items:center;gap:16px;padding:14px 20px;text-decoration:none;color:#202124;font-weight:400;min-height:72px;box-sizing:border-box}
.fed-app-nav-list a>.material-icons{flex-shrink:0;width:24px;height:24px;font-size:24px;line-height:24px;overflow:hidden}
.fed-app-nav-label{font-size:.9375rem;font-weight:500;line-height:1.35}
.fed-app-nav-desc{font-size:.8125rem;color:#5f6368;line-height:1.4}
@media (max-width:992px){
body.fed-shell-body{padding-left:0}
.fed-shell-sidebar{transform:translateX(-100%)}
body.fed-shell-open .fed-shell-sidebar{transform:translateX(0)}
body.fed-shell-open .fed-shell-backdrop{display:block!important}
.fed-shell-menu-btn{display:flex!important;align-items:center;justify-content:center;position:fixed;top:12px;left:12px;z-index:1002;width:44px;height:44px;border:none;border-radius:50%;background:#fff;cursor:pointer}
.fed-shell-main{padding:72px 20px 32px}
}`

func (s *Server) shellCriticalCSS() template.CSS {
	css := shellCriticalCSS
	if s != nil && s.qcartaProfile() {
		css += `.fed-shell-sidebar-brand{display:none!important;padding:0;margin:0;height:0;min-height:0;overflow:hidden}`
	}
	return template.CSS(css)
}

type shellLink struct {
	Label       string
	Icon        string
	Tint        string
	Href        string
	Description string
}

type hubContentMeta struct {
	Description string
	Section     string
	Items       []shellLink
}

// Mirrors shared/ui/shell-config.js hub sections / appNav (labels + targets).
var hubContentByID = map[string]hubContentMeta{
	"reports": {
		Description: "Browse, run, and publish report templates",
		Section:     "Reports",
		Items: []shellLink{
			{Label: "Reports", Icon: "assessment", Href: "reports/", Description: "Browse and run published reports"},
			{Label: "Publish", Icon: "publish", Href: "reports/publish_report", Description: "Upload and publish new report templates"},
			{Label: "Create Designer Report", Icon: "design_services", Href: "reports/designer/new", Description: "Start a new report in Report Designer Web"},
			{Label: "Import Designer Project", Icon: "unarchive", Href: "reports/designer/import", Description: "Upload a desktop Designer project"},
		},
	},
	"data-explorer": {
		Description: "Interactive browse, filter, chart, and map of datasets",
		Section:     "Ad Hoc",
		Items: []shellLink{
			{Label: "Explore Data", Icon: "travel_explore", Href: "explorer/", Description: "Browse, filter, chart, and map datasets"},
			{Label: "My Views", Icon: "bookmark", Href: "explorer/?panel=views", Description: "Saved Data Explorer views"},
			{Label: "Dashboards", Icon: "dashboard", Href: "dashboards/", Description: "Data Explorer views designated as Dashboards"},
			{Label: "Data Quality", Icon: "insights", Href: "dataset-intelligence/", Description: "Dataset structure, quality, spatial profile, and usage"},
		},
	},
	"data": {
		Description: "Configure data sources and parameterized report URLs",
		Section:     "Data Sources",
		Items: []shellLink{
			{Label: "Datasources", Icon: "storage", Href: "reports/datasources", Description: "Configure report data sources"},
			{Label: "GeoServer", Icon: "public", Href: "reports/geoserver_connections", Description: "GeoServer connections for WMS maps and WFS reports"},
			{Label: "QGIS Server", Icon: "map", Href: "reports/qgis_connections", Description: "QGIS Server connections for Designer map publish"},
			{Label: "ArcGIS", Icon: "map", Href: "reports/arcgis_connections", Description: "ArcGIS connections for Designer map publish"},
			{Label: "URL Builder", Icon: "link", Href: "reports/report_runner", Description: "Build parameterized report URLs"},
		},
	},
	"scheduling": {
		Description: "Automate report generation, delivery, and recipients",
		Section:     "Scheduling",
		Items: []shellLink{
			{Label: "Schedules", Icon: "schedule", Href: "schedules", Description: "Automate report generation and delivery"},
			{Label: "Executions", Icon: "play_circle_outline", Href: "executions", Description: "Recent run status per schedule"},
			{Label: "History", Icon: "history", Href: "history", Description: "Execution log and delivery outcomes"},
			{Label: "Recipients", Icon: "people", Href: "recipients", Description: "Email recipient directory"},
		},
	},
	"administration": {
		Description: "Manage users, groups, authentication, embedding, audit, export, usage, and governance",
		Section:     "Administration",
		Items: []shellLink{
			{Label: "Users", Icon: "person", Href: "admin/users", Description: "Manage local user accounts"},
			{Label: "Groups", Icon: "group", Href: "admin/groups", Description: "Manage groups and memberships"},
			{Label: "Authentication", Icon: "vpn_key", Href: "admin/authentication", Description: "Local login and optional OIDC / SAML SSO"},
			{Label: "License", Icon: "verified", Href: "admin/license", Description: "Activate AcuGIS Report Server Professional"},
			{Label: "Embedded Analytics", Icon: "web", Href: "admin/embedded-analytics", Description: "Origins that may embed reports and dashboards"},
			{Label: "Auditing", Icon: "fact_check", Href: "admin/audit", Description: "Searchable audit trail and report performance"},
			{Label: "Audit Export", Icon: "share", Href: "admin/audit-export", Description: "Forward Audit events to SIEM, Syslog, or Webhooks"},
			{Label: "Usage Analytics", Icon: "analytics", Href: "admin/usage", Description: "Aggregated content, API, and embed usage"},
			{Label: "Governance", Icon: "verified", Href: "admin/governance", Description: "Certification status, owners, and review dates"},
		},
	},
}

var appNavLinks = []shellLink{
	{Label: "Reports", Icon: "assessment", Href: "/hubs/reports"},
	{Label: "Ad Hoc", Icon: "travel_explore", Tint: "blue", Href: "/hubs/data-explorer"},
	{Label: "Dashboards", Icon: "dashboard", Tint: "teal", Href: "/dashboards/"},
	{Label: "Data Sources", Icon: "storage", Tint: "teal", Href: "/hubs/data"},
	{Label: "Scheduling", Icon: "schedule", Tint: "purple", Href: "/hubs/scheduling"},
	{Label: "Data Quality", Icon: "insights", Tint: "blue", Href: "/dataset-intelligence/"},
	{Label: "Auditing", Icon: "fact_check", Tint: "grey", Href: "/admin/audit"},
	{Label: "Administration", Icon: "admin_panel_settings", Tint: "grey", Href: "/hubs/administration"},
}

func (s *Server) shellAppURL(href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return s.goURL("/")
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	mount := s.shellMount()
	if strings.HasPrefix(href, "/") {
		if mount != "" && (strings.HasPrefix(href, mount+"/") || href == mount) {
			return href
		}
		return s.goURL(href)
	}
	return s.goURL("/" + href)
}

func (s *Server) appNavLinksForEdition() []shellLink {
	pro := s.Edition().CurrentEdition() == edition.EditionProfessional
	out := make([]shellLink, 0, len(appNavLinks))
	for _, item := range appNavLinks {
		switch sidebarNavItemID(item.Href) {
		case "dashboards":
			if !pro || !s.allowDashboards() {
				continue
			}
		case "data-explorer":
			if !pro || !modules.Has(modules.IDAdHoc) {
				continue
			}
		case "dataset-intelligence":
			if !pro || !modules.Has(modules.IDDataIntel) || !s.Edition().HasFeature(edition.FeatureDataIntelligence) {
				continue
			}
		case "audit":
			if !modules.Has(modules.IDAudit) || !s.Edition().HasFeature(edition.FeatureAudit) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func (s *Server) hubContentForEdition(hubID string) (hubContentMeta, bool) {
	meta, ok := hubContentByID[hubID]
	if !ok {
		return hubContentMeta{}, false
	}
	items := make([]shellLink, 0, len(meta.Items))
	for _, item := range meta.Items {
		href := strings.TrimSpace(item.Href)
		switch {
		case strings.Contains(href, "reports/designer/"):
			if !s.designerConfigured() {
				continue
			}
		case strings.Contains(href, "dashboards"):
			if !s.allowDashboards() {
				continue
			}
		case strings.Contains(href, "dataset-intelligence"):
			if !modules.Has(modules.IDDataIntel) || !s.Edition().HasFeature(edition.FeatureDataIntelligence) {
				continue
			}
		case strings.Contains(href, "admin/audit-export"):
			if !modules.Has(modules.IDAuditExport) || !s.Edition().HasFeature(edition.FeatureAuditExport) {
				continue
			}
		case strings.Contains(href, "admin/usage"):
			if !modules.Has(modules.IDUsage) || !s.Edition().HasFeature(edition.FeatureUsageAnalytics) {
				continue
			}
		case strings.Contains(href, "admin/governance"):
			if !modules.Has(modules.IDGovernance) || !s.Edition().HasFeature(edition.FeatureGovernance) {
				continue
			}
		case strings.Contains(href, "admin/audit"):
			if !modules.Has(modules.IDAudit) || !s.Edition().HasFeature(edition.FeatureAudit) {
				continue
			}
		case strings.Contains(href, "qgis_connections"):
			if !modules.Has(modules.IDQGIS) {
				continue
			}
		case strings.Contains(href, "arcgis_connections"):
			if !modules.Has(modules.IDArcGIS) || !s.Edition().HasDesignerMapProvider("arcgis") {
				continue
			}
		case strings.Contains(href, "explorer"):
			if !modules.Has(modules.IDAdHoc) {
				continue
			}
		}
		items = append(items, item)
	}
	meta.Items = items
	return meta, true
}

func (s *Server) renderSidebarNavHTML(activeHubID string) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="fed-shell-nav-main">`)
	b.WriteString(`<div class="fed-shell-nav-section-label">Menu</div>`)
	b.WriteString(`<ul class="fed-shell-nav-list browser-default">`)
	for _, item := range s.appNavLinksForEdition() {
		hubID := sidebarNavItemID(item.Href)
		active := hubID == activeHubID
		href := html.EscapeString(s.shellAppURL(item.Href))
		tint := item.Tint
		if tint == "" {
			tint = "blue"
		}
		liClass := ""
		if active {
			liClass = ` class="is-active"`
		}
		fmt.Fprintf(&b, `<li%s><a href="%s"><span class="fed-shell-nav-icon fed-shell-nav-icon--%s" aria-hidden="true"><i class="material-icons">%s</i></span><span class="fed-shell-nav-label">%s</span></a></li>`,
			liClass, href, html.EscapeString(tint), html.EscapeString(item.Icon), html.EscapeString(item.Label))
	}
	b.WriteString(`</ul></div>`)
	logout := html.EscapeString(s.shellAppURL("/logout"))
	b.WriteString(`<div class="fed-shell-nav-footer"><ul class="fed-shell-nav-list browser-default">`)
	fmt.Fprintf(&b, `<li><a href="%s"><span class="fed-shell-nav-icon fed-shell-nav-icon--grey" aria-hidden="true"><i class="material-icons">logout</i></span><span class="fed-shell-nav-label">Log Out</span></a></li>`, logout)
	b.WriteString(`</ul></div>`)
	return template.HTML(b.String())
}

// sidebarNavItemID maps an app-nav href to the active-item id used by SSR chrome.
func sidebarNavItemID(href string) string {
	href = strings.TrimSpace(href)
	if strings.Contains(href, "/admin/audit-export") {
		return "audit-export"
	}
	if strings.Contains(href, "/admin/audit") {
		return "audit"
	}
	if strings.Contains(href, "/admin/usage") {
		return "usage"
	}
	if strings.Contains(href, "/admin/governance") {
		return "governance"
	}
	if strings.HasPrefix(href, "/hubs/") {
		return strings.TrimSuffix(strings.TrimPrefix(href, "/hubs/"), ".php")
	}
	href = strings.Trim(href, "/")
	if href == "" {
		return ""
	}
	if i := strings.IndexByte(href, '/'); i >= 0 {
		href = href[:i]
	}
	return strings.TrimSuffix(href, ".php")
}

func (s *Server) renderHubBodyHTML(hubID, title string) template.HTML {
	meta, ok := s.hubContentForEdition(hubID)
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="fed-shell-page" data-fed-ssr="1">`)
	fmt.Fprintf(&b, `<header class="fed-shell-page-header"><h1 class="fed-shell-page-title">%s</h1><p class="fed-shell-page-subtitle">%s</p></header>`,
		html.EscapeString(title), html.EscapeString(meta.Description))
	fmt.Fprintf(&b, `<div class="fed-app-nav fed-workspace-nav" role="navigation" aria-label="%s">`, html.EscapeString(title))
	fmt.Fprintf(&b, `<div class="fed-app-nav-section"><h2 class="fed-app-nav-section-title">%s</h2><div class="fed-app-nav-card"><ul class="fed-app-nav-list browser-default">`,
		html.EscapeString(meta.Section))
	for _, item := range meta.Items {
		href := html.EscapeString(s.shellAppURL(item.Href))
		fmt.Fprintf(&b, `<li><a href="%s"><i class="material-icons" aria-hidden="true">%s</i><span class="fed-app-nav-text"><span class="fed-app-nav-label">%s</span><span class="fed-app-nav-desc">%s</span></span><i class="material-icons fed-app-nav-chevron" aria-hidden="true">chevron_right</i></a></li>`,
			href, html.EscapeString(item.Icon), html.EscapeString(item.Label), html.EscapeString(item.Description))
	}
	b.WriteString(`</ul></div></div></div></div>`)
	return template.HTML(b.String())
}
