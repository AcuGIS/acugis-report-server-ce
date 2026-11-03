package acl

// Resource type and permission constants matching PHP lib/auth.php and
// lib/service_acl.php.
const (
	ResourceTypeJasperReport  = "jasper_report"
	ResourceTypeService       = "service"
	ResourceTypeContentFolder = "content_folder"
	// ResourceTypeExplorerView covers saved Data Explorer Views and Dashboards
	// (same SavedView object; Dashboards are is_dashboard=true). Key is "owner/viewKey".
	ResourceTypeExplorerView = "explorer_view"

	ServiceKeyJasperReports  = "jasper_reports"
	ServiceKeyReportRuntime  = "report_runtime"

	PermissionView  = "view"
	PermissionEdit  = "edit"
	PermissionAdmin = "admin"

	GroupPublic         = "Public"
	GroupAdministrators = "Administrators"
	GroupEditors        = "Editors"
)

// Resource is a row from the resources table (read-only projection).
type Resource struct {
	ID           int64
	ResourceType string
	ResourceKey  string
	DisplayName  string
	Service      string
}
