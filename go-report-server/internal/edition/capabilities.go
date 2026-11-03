package edition

// Capabilities is the resolved edition payload exposed to clients.
// Backend enforcement remains authoritative; this is for UI discovery only.
type Capabilities struct {
	Edition  string           `json:"edition"`
	Features FeatureFlags     `json:"features"`
	Maps     MapCapabilities  `json:"maps"`
}

// FeatureFlags are camelCase flags for frontend consumption.
type FeatureFlags struct {
	DataExplorer     bool `json:"dataExplorer"`
	Dashboards       bool `json:"dashboards"`
	MapReports       bool `json:"mapReports"`
	DataIntelligence bool `json:"dataIntelligence"`
	Audit            bool `json:"audit"`
	UsageAnalytics   bool `json:"usageAnalytics"`
	Governance       bool `json:"governance"`
	AuditExport      bool `json:"auditExport"`
	SSOOIDC          bool `json:"ssoOidc"`
	SSOSAML          bool `json:"ssoSaml"`
}

// MapCapabilities nests designer / embedded provider entitlements.
type MapCapabilities struct {
	Designer MapProviderFlags `json:"designer"`
	Embedded MapProviderFlags `json:"embedded"`
}

// MapProviderFlags lists Community-relevant providers; Professional enables all.
type MapProviderFlags struct {
	PostGIS   bool `json:"postgis"`
	MySQL     bool `json:"mysql"`
	GeoServer bool `json:"geoserver"`
	QGIS      bool `json:"qgis"`
}

// Capabilities returns the resolved capability document for the active edition.
func (s *Service) Capabilities() Capabilities {
	return Capabilities{
		Edition: string(s.CurrentEdition()),
		Features: FeatureFlags{
			DataExplorer:     s.HasFeature(FeatureDataExplorer),
			Dashboards:       s.HasFeature(FeatureDashboards),
			MapReports:       s.HasFeature(FeatureMapReports),
			DataIntelligence: s.HasFeature(FeatureDataIntelligence),
			Audit:            s.HasFeature(FeatureAudit),
			UsageAnalytics:   s.HasFeature(FeatureUsageAnalytics),
			Governance:       s.HasFeature(FeatureGovernance),
			AuditExport:      s.HasFeature(FeatureAuditExport),
			SSOOIDC:          s.HasFeature(FeatureSSOOIDC),
			SSOSAML:          s.HasFeature(FeatureSSOSAML),
		},
		Maps: MapCapabilities{
			Designer: MapProviderFlags{
				PostGIS:   s.HasDesignerMapProvider("postgis"),
				MySQL:     s.HasDesignerMapProvider("mysql"),
				GeoServer: s.HasDesignerMapProvider("geoserver"),
				QGIS:      s.HasDesignerMapProvider("qgis"),
			},
			Embedded: MapProviderFlags{
				PostGIS:   s.HasEmbeddedMapProvider("postgis"),
				MySQL:     s.HasEmbeddedMapProvider("mysql"),
				GeoServer: s.HasEmbeddedMapProvider("geoserver"),
				QGIS:      s.HasEmbeddedMapProvider("qgis"),
			},
		},
	}
}
