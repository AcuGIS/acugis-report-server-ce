// Package edition provides centralized Community / Professional edition
// resolution and feature entitlements for AcuGIS Report Server.
//
// Professional entitlement is a live ACL1 decision. ACUGIS_EDITION does not
// grant features. HasFeature and RequireFeature read the current edition on
// each call so activation, refresh, and expiry apply without a restart.
package edition

import (
	"fmt"
	"strings"
	"sync"
)

// Edition identifies the active product edition.
type Edition string

const (
	EditionCommunity    Edition = "community"
	EditionProfessional Edition = "professional"
)

// Feature is a named entitlement checked via HasFeature / RequireFeature.
// Prefer these constants over scattering edition string comparisons.
type Feature string

const (
	FeatureDataExplorer     Feature = "data_explorer"
	FeatureDashboards       Feature = "dashboards"
	FeatureMapReports       Feature = "map_reports"
	FeatureDataIntelligence Feature = "data_intelligence"
	// FeatureAudit gates the top-level Auditing nav item (Professional).
	FeatureAudit Feature = "audit"
	// FeatureUsageAnalytics gates Administration → Usage Analytics (Professional).
	FeatureUsageAnalytics Feature = "usage_analytics"
	// FeatureGovernance gates Governance / Certification (Professional).
	FeatureGovernance Feature = "governance"
	// FeatureAuditExport gates Administration → Audit Export / SIEM forwarding (Professional).
	FeatureAuditExport          Feature = "audit_export"
	FeatureMapDesignerPostGIS   Feature = "map.designer.postgis"
	FeatureMapDesignerMySQL     Feature = "map.designer.mysql"
	FeatureMapEmbeddedPostGIS   Feature = "map.embedded.postgis"
	FeatureMapEmbeddedMySQL     Feature = "map.embedded.mysql"
	FeatureMapEmbeddedGeoServer Feature = "map.embedded.geoserver"
	FeatureMapEmbeddedQGIS      Feature = "map.embedded.qgis"
	// FeatureSSOOIDC gates optional OpenID Connect SSO (Professional / enterprise).
	// Protocol code must not scatter edition checks; call HasFeature / RequireFeature at boundaries.
	FeatureSSOOIDC Feature = "enterprise.sso.oidc"
	// FeatureSSOSAML gates optional SAML 2.0 SSO (Professional / enterprise).
	FeatureSSOSAML Feature = "enterprise.sso.saml"
)

// communityFeatures is the explicit Community allow-list.
// Professional does not use an allow-list: HasFeature returns true for every feature.
var communityFeatures = map[Feature]bool{
	FeatureDataExplorer:         true,
	FeatureDashboards:           false,
	FeatureMapReports:           false,
	FeatureDataIntelligence:     false,
	FeatureUsageAnalytics:       false,
	FeatureGovernance:           false,
	FeatureAudit:                false,
	FeatureAuditExport:          false,
	FeatureMapDesignerPostGIS:   true,
	FeatureMapDesignerMySQL:     true,
	FeatureMapEmbeddedPostGIS:   true,
	FeatureMapEmbeddedMySQL:     true,
	FeatureMapEmbeddedGeoServer: true,
	FeatureMapEmbeddedQGIS:      true,
}

// Service resolves the active edition and answers entitlement questions.
// A bound live function, when set, is the entitlement source.
type Service struct {
	mu      sync.RWMutex
	edition Edition
	live    func() Edition
}

// New returns a Service for the given edition. Unknown values default to Community.
// This does not read ACUGIS_EDITION. Production binds a license resolver.
func New(ed Edition) *Service {
	return &Service{edition: Normalize(string(ed))}
}

// FromEnv returns Community. ACUGIS_EDITION and JRS_EDITION are not entitlement.
// The getenv argument is retained so existing callers compile; it is ignored.
func FromEnv(getenv func(string) string) *Service {
	_ = getenv
	return New(EditionCommunity)
}

// BindLive makes every entitlement check call fn. fn must be safe for
// concurrent use. Passing nil restores the static edition from New.
func (s *Service) BindLive(fn func() Edition) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.live = fn
	s.mu.Unlock()
}

func (s *Service) resolved() Edition {
	if s == nil {
		return EditionCommunity
	}
	s.mu.RLock()
	live := s.live
	ed := s.edition
	s.mu.RUnlock()
	if live != nil {
		return Normalize(string(live()))
	}
	if ed == "" {
		return EditionCommunity
	}
	return ed
}

// Normalize maps free-form edition strings to a known Edition.
// Unrecognized values become Community (safe default).
func Normalize(raw string) Edition {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "professional", "pro", "commercial":
		// "commercial" is accepted as an alias for older product naming.
		return EditionProfessional
	case "community", "ce", "oss", "":
		return EditionCommunity
	default:
		return EditionCommunity
	}
}

// CurrentEdition returns the active edition.
func (s *Service) CurrentEdition() Edition {
	return s.resolved()
}

// DisplayName returns a human-readable edition label for version output and UI.
// Entitlement identifiers remain community / professional.
func (s *Service) DisplayName() string {
	switch s.CurrentEdition() {
	case EditionProfessional:
		return "Professional"
	default:
		return "Open Source"
	}
}

// HasFeature reports whether the active edition entitles the given feature.
//
// Professional Edition allows ALL features (including future ones not listed
// in communityFeatures). Community Edition only allows features explicitly
// enabled in communityFeatures; unknown features are denied.
func (s *Service) HasFeature(f Feature) bool {
	if s == nil {
		return communityFeatures[f]
	}
	if s.resolved() == EditionProfessional {
		return true
	}
	return communityFeatures[f]
}

// RequireFeature returns nil when entitled, or ErrFeatureUnavailable otherwise.
func (s *Service) RequireFeature(f Feature) error {
	if s.HasFeature(f) {
		return nil
	}
	return ErrFeatureUnavailable{Feature: f, Edition: s.CurrentEdition()}
}

// ErrFeatureUnavailable is returned when Community Edition blocks a feature.
type ErrFeatureUnavailable struct {
	Feature Feature
	Edition Edition
}

func (e ErrFeatureUnavailable) Error() string {
	ed := e.Edition
	if ed == "" {
		ed = EditionCommunity
	}
	return fmt.Sprintf("feature %q is not available in the %s edition", e.Feature, ed)
}

// DesignerMapFeature maps a designer map provider id to its entitlement.
// Unknown providers map to empty Feature (denied in Community via HasDesignerMapProvider).
func DesignerMapFeature(provider string) Feature {
	switch normalizeProvider(provider) {
	case "postgis", "postgresql", "postgres", "pgsql":
		return FeatureMapDesignerPostGIS
	case "mysql", "mariadb":
		return FeatureMapDesignerMySQL
	default:
		return ""
	}
}

// EmbeddedMapFeature maps an embedded map provider/renderer id to its entitlement.
func EmbeddedMapFeature(provider string) Feature {
	switch normalizeProvider(provider) {
	case "postgis", "postgresql", "postgres", "pgsql":
		return FeatureMapEmbeddedPostGIS
	case "mysql", "mariadb":
		return FeatureMapEmbeddedMySQL
	case "geoserver", "gs":
		return FeatureMapEmbeddedGeoServer
	case "qgis", "qgisserver", "qgis_server", "qgis-server":
		return FeatureMapEmbeddedQGIS
	default:
		return ""
	}
}

// HasDesignerMapProvider reports whether the edition allows the given
// Report Designer map provider (e.g. postgis, mysql, arcgis).
func (s *Service) HasDesignerMapProvider(provider string) bool {
	if s != nil && s.resolved() == EditionProfessional {
		return true
	}
	f := DesignerMapFeature(provider)
	if f == "" {
		return false
	}
	return s.HasFeature(f)
}

// HasEmbeddedMapProvider reports whether the edition allows the given
// embedded map provider/renderer (e.g. postgis, mysql).
func (s *Service) HasEmbeddedMapProvider(provider string) bool {
	if s != nil && s.resolved() == EditionProfessional {
		return true
	}
	f := EmbeddedMapFeature(provider)
	if f == "" {
		return false
	}
	return s.HasFeature(f)
}

// RequireDesignerMapProvider returns ErrFeatureUnavailable when denied.
func (s *Service) RequireDesignerMapProvider(provider string) error {
	if s.HasDesignerMapProvider(provider) {
		return nil
	}
	f := DesignerMapFeature(provider)
	if f == "" {
		f = Feature("map.designer." + normalizeProvider(provider))
	}
	return ErrFeatureUnavailable{Feature: f, Edition: s.CurrentEdition()}
}

// RequireEmbeddedMapProvider returns ErrFeatureUnavailable when denied.
func (s *Service) RequireEmbeddedMapProvider(provider string) error {
	if s.HasEmbeddedMapProvider(provider) {
		return nil
	}
	f := EmbeddedMapFeature(provider)
	if f == "" {
		f = Feature("map.embedded." + normalizeProvider(provider))
	}
	return ErrFeatureUnavailable{Feature: f, Edition: s.CurrentEdition()}
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// KnownFeatures returns registered feature constants (for diagnostics / tests).
func KnownFeatures() []Feature {
	return []Feature{
		FeatureDataExplorer,
		FeatureDashboards,
		FeatureMapReports,
		FeatureDataIntelligence,
		FeatureAudit,
		FeatureUsageAnalytics,
		FeatureGovernance,
		FeatureAuditExport,
		FeatureMapDesignerPostGIS,
		FeatureMapDesignerMySQL,
		FeatureMapEmbeddedPostGIS,
		FeatureMapEmbeddedMySQL,
		FeatureMapEmbeddedGeoServer,
		FeatureMapEmbeddedQGIS,
		FeatureSSOOIDC,
		FeatureSSOSAML,
	}
}
