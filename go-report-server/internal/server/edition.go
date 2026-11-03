package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/modules"
)

// errFeatureUnavailable is the HTTP-facing edition entitlement denial.
var errFeatureUnavailable = &httpError{
	Status:  http.StatusForbidden,
	Message: "This feature is not available in the Community Edition",
}

// Edition returns the process edition/entitlement service.
func (s *Server) Edition() *edition.Service {
	if s == nil || s.edition == nil {
		return edition.New(edition.EditionCommunity)
	}
	return s.edition
}

// requireEditionFeature enforces a named entitlement before ACL checks complete
// the request. Edition gates are independent of user/group/folder permissions.
func (s *Server) requireEditionFeature(f edition.Feature) error {
	if !featureCompiled(f) {
		return errFeatureUnavailable
	}
	if err := s.Edition().RequireFeature(f); err != nil {
		var ue edition.ErrFeatureUnavailable
		if errors.As(err, &ue) {
			return errFeatureUnavailable
		}
		return err
	}
	return nil
}

func (s *Server) requireDesignerMapProvider(provider string) error {
	if s.qcartaProfile() {
		return nil
	}
	if !designerProviderCompiled(provider) {
		return errFeatureUnavailable
	}
	if err := s.Edition().RequireDesignerMapProvider(provider); err != nil {
		var ue edition.ErrFeatureUnavailable
		if errors.As(err, &ue) {
			return errFeatureUnavailable
		}
		return err
	}
	return nil
}

func (s *Server) requireEmbeddedMapProvider(provider string) error {
	if !embeddedProviderCompiled(provider) {
		return errFeatureUnavailable
	}
	if err := s.Edition().RequireEmbeddedMapProvider(provider); err != nil {
		var ue edition.ErrFeatureUnavailable
		if errors.As(err, &ue) {
			return errFeatureUnavailable
		}
		return err
	}
	return nil
}

// editionCapabilitiesJSON returns the capability document for SSR / API clients.
func (s *Server) editionCapabilities() edition.Capabilities {
	caps := s.Edition().Capabilities()
	if !modules.Has(modules.IDAdHoc) {
		caps.Features.DataExplorer = false
	}
	if !modules.Has(modules.IDDashboards) {
		caps.Features.Dashboards = false
	}
	if !modules.Has(modules.IDMapReports) {
		caps.Features.MapReports = false
	}
	if !modules.Has(modules.IDDataIntel) {
		caps.Features.DataIntelligence = false
	}
	if !modules.Has(modules.IDAudit) {
		caps.Features.Audit = false
	}
	if !modules.Has(modules.IDUsage) {
		caps.Features.UsageAnalytics = false
	}
	if !modules.Has(modules.IDGovernance) {
		caps.Features.Governance = false
	}
	if !modules.Has(modules.IDAuditExport) {
		caps.Features.AuditExport = false
	}
	if !modules.Has(modules.IDSSO) {
		caps.Features.SSOOIDC = false
		caps.Features.SSOSAML = false
	}
	if !modules.Has(modules.IDQGIS) {
		caps.Maps.Designer.QGIS = false
		caps.Maps.Embedded.QGIS = false
	}
	return caps
}

func (s *Server) allowMapReports() bool {
	if s.qcartaProfile() {
		return false
	}
	return modules.Has(modules.IDMapReports) && s.Edition().HasFeature(edition.FeatureMapReports)
}

func (s *Server) allowDashboards() bool {
	if s.qcartaProfile() {
		return false
	}
	return modules.Has(modules.IDDashboards) && s.Edition().HasFeature(edition.FeatureDashboards)
}

func featureCompiled(f edition.Feature) bool {
	switch f {
	case edition.FeatureDataExplorer:
		return modules.Has(modules.IDAdHoc)
	case edition.FeatureDashboards:
		return modules.Has(modules.IDDashboards)
	case edition.FeatureMapReports:
		return modules.Has(modules.IDMapReports)
	case edition.FeatureDataIntelligence:
		return modules.Has(modules.IDDataIntel)
	case edition.FeatureAudit:
		return modules.Has(modules.IDAudit)
	case edition.FeatureUsageAnalytics:
		return modules.Has(modules.IDUsage)
	case edition.FeatureGovernance:
		return modules.Has(modules.IDGovernance)
	case edition.FeatureAuditExport:
		return modules.Has(modules.IDAuditExport)
	case edition.FeatureSSOOIDC, edition.FeatureSSOSAML:
		return modules.Has(modules.IDSSO)
	case edition.FeatureMapEmbeddedQGIS:
		return modules.Has(modules.IDQGIS)
	default:
		return true
	}
}

func designerProviderCompiled(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "qgis":
		return modules.Has(modules.IDQGIS)
	case "arcgis":
		return modules.Has(modules.IDArcGIS)
	default:
		return true
	}
}

func embeddedProviderCompiled(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "qgis":
		return modules.Has(modules.IDQGIS)
	default:
		return true
	}
}

func (s *Server) handleEditionCapabilities(w http.ResponseWriter, _ *http.Request) {
	caps := s.editionCapabilities()
	s.writeJSON(w, http.StatusOK, map[string]any{
		"product":         config.ProductName,
		"version":         s.cfg.AppVersion,
		"edition":         caps.Edition,
		"edition_display": s.Edition().DisplayName(),
		// Installed Designer is discoverable before it is licensed.
		"report_designer": s.designerConfigured(),
		"capabilities":    caps,
	})
}
