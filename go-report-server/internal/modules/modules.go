// Package modules records which product modules were compiled into this binary.
// Professional modules register only in the untagged / Professional build
// (//go:build !oss). The OSS build (-tags oss) compiles core modules only.
package modules

// ID is a stable module identifier used for navigation and tests.
type ID string

const (
	IDCore            ID = "core"
	IDAdHoc           ID = "adhoc"
	IDDashboards      ID = "dashboards"
	IDMapReports      ID = "map_reports"
	IDQGIS            ID = "qgis"
	IDArcGIS          ID = "arcgis"
	IDDataIntel       ID = "data_intelligence"
	IDAudit           ID = "audit"
	IDUsage           ID = "usage_analytics"
	IDGovernance      ID = "governance"
	IDAuditExport     ID = "audit_export"
	IDSSO             ID = "sso"
)

// Module is a compiled product surface. Nav is derived from registered modules
// rather than edition string checks in templates.
type Module struct {
	ID      ID
	NavHubs []string // hub ids this module contributes (optional)
}

var registry []Module

// Register adds a compiled module. Called from init in Professional files.
func Register(m Module) {
	registry = append(registry, m)
}

// All returns modules compiled into this binary.
func All() []Module {
	out := make([]Module, len(registry))
	copy(out, registry)
	return out
}

// Has reports whether id was compiled in.
func Has(id ID) bool {
	for _, m := range registry {
		if m.ID == id {
			return true
		}
	}
	return false
}
