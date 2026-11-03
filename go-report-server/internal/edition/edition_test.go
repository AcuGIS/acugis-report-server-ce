package edition

import (
	"errors"
	"testing"
)

func TestNormalize(t *testing.T) {
	t.Parallel()
	cases := map[string]Edition{
		"":             EditionCommunity,
		"community":    EditionCommunity,
		"Community":    EditionCommunity,
		"ce":           EditionCommunity,
		"oss":          EditionCommunity,
		"professional": EditionProfessional,
		"PRO":          EditionProfessional,
		"commercial":   EditionProfessional,
		"bogus":        EditionCommunity,
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q)=%q want %q", in, got, want)
		}
	}
}

func TestFromEnvDoesNotGrantProfessional(t *testing.T) {
	t.Parallel()
	svc := FromEnv(func(k string) string {
		if k == "ACUGIS_EDITION" || k == "JRS_EDITION" {
			return "professional"
		}
		return ""
	})
	if svc.CurrentEdition() != EditionCommunity {
		t.Fatalf("env must not grant professional, got %s", svc.CurrentEdition())
	}
	if svc.HasFeature(FeatureDashboards) || svc.HasFeature(FeatureAudit) {
		t.Fatal("env professional must not enable Professional features")
	}
}

func TestLiveEntitlementUpdates(t *testing.T) {
	t.Parallel()
	svc := New(EditionCommunity)
	professional := false
	svc.BindLive(func() Edition {
		if professional {
			return EditionProfessional
		}
		return EditionCommunity
	})
	if svc.CurrentEdition() != EditionCommunity || svc.HasFeature(FeatureDashboards) {
		t.Fatal("expected community before activation")
	}
	professional = true
	if svc.CurrentEdition() != EditionProfessional || !svc.HasFeature(FeatureAudit) {
		t.Fatal("expected professional after live update")
	}
	professional = false
	if svc.HasFeature(FeatureMapReports) {
		t.Fatal("expected community after expiry")
	}
}

func TestDisplayName(t *testing.T) {
	t.Parallel()
	if got := New(EditionCommunity).DisplayName(); got != "Open Source" {
		t.Fatalf("community display=%q want Open Source", got)
	}
	if got := New(EditionProfessional).DisplayName(); got != "Professional" {
		t.Fatalf("professional display=%q want Professional", got)
	}
}

func TestCommunityEntitlements(t *testing.T) {
	t.Parallel()
	svc := New(EditionCommunity)

	allowed := []Feature{
		FeatureDataExplorer,
		FeatureMapDesignerPostGIS,
		FeatureMapDesignerMySQL,
		FeatureMapEmbeddedPostGIS,
		FeatureMapEmbeddedMySQL,
		FeatureMapEmbeddedGeoServer,
		FeatureMapEmbeddedQGIS,
	}
	denied := []Feature{
		FeatureDashboards,
		FeatureMapReports,
		FeatureDataIntelligence,
		FeatureUsageAnalytics,
		FeatureGovernance,
		FeatureAuditExport,
		FeatureSSOOIDC,
		FeatureSSOSAML,
	}
	for _, f := range allowed {
		if !svc.HasFeature(f) {
			t.Errorf("Community should allow %s", f)
		}
		if err := svc.RequireFeature(f); err != nil {
			t.Errorf("RequireFeature(%s): %v", f, err)
		}
	}
	for _, f := range denied {
		if svc.HasFeature(f) {
			t.Errorf("Community should deny %s", f)
		}
		if err := svc.RequireFeature(f); err == nil {
			t.Errorf("RequireFeature(%s) expected error", f)
		} else {
			var ue ErrFeatureUnavailable
			if !errors.As(err, &ue) {
				t.Errorf("expected ErrFeatureUnavailable, got %T", err)
			}
		}
	}
	// Unknown features are denied in Community (not on Professional allow-list model).
	if svc.HasFeature(Feature("future_widget")) {
		t.Fatal("Community must deny unknown features")
	}
}

func TestProfessionalAllFeatures(t *testing.T) {
	t.Parallel()
	svc := New(EditionProfessional)
	for _, f := range KnownFeatures() {
		if !svc.HasFeature(f) {
			t.Errorf("Professional should allow %s", f)
		}
	}
	// Future / unregistered features remain allowed.
	if !svc.HasFeature(Feature("future_widget")) {
		t.Fatal("Professional must allow unknown features")
	}
	if !svc.HasDesignerMapProvider("arcgis") {
		t.Fatal("Professional must allow other designer map providers")
	}
	if !svc.HasEmbeddedMapProvider("geoserver") {
		t.Fatal("Professional must allow other embedded map providers")
	}
}

func TestMapProvidersCommunity(t *testing.T) {
	t.Parallel()
	svc := New(EditionCommunity)

	for _, p := range []string{"postgis", "postgresql", "mysql", "mariadb"} {
		if !svc.HasDesignerMapProvider(p) {
			t.Errorf("designer %s should be allowed", p)
		}
		if !svc.HasEmbeddedMapProvider(p) {
			t.Errorf("embedded %s should be allowed", p)
		}
	}
	if !svc.HasEmbeddedMapProvider("geoserver") {
		t.Errorf("embedded geoserver should be allowed in Community")
	}
	if !svc.HasEmbeddedMapProvider("qgis") {
		t.Errorf("embedded qgis should be allowed in Community")
	}
	for _, p := range []string{"arcgis", "wms", "oracle"} {
		if svc.HasDesignerMapProvider(p) {
			t.Errorf("designer %s should be denied in Community", p)
		}
		if svc.HasEmbeddedMapProvider(p) {
			t.Errorf("embedded %s should be denied in Community", p)
		}
		if err := svc.RequireDesignerMapProvider(p); err == nil {
			t.Errorf("RequireDesignerMapProvider(%s) expected error", p)
		}
		if err := svc.RequireEmbeddedMapProvider(p); err == nil {
			t.Errorf("RequireEmbeddedMapProvider(%s) expected error", p)
		}
	}
	// GeoServer remains Designer-denied in Community (Designer catalog is separate).
	if svc.HasDesignerMapProvider("geoserver") {
		t.Errorf("designer geoserver should be denied in Community")
	}
	if svc.HasDesignerMapProvider("qgis") {
		t.Errorf("designer qgis should be denied in Community")
	}
}

func TestCapabilitiesPayload(t *testing.T) {
	t.Parallel()
	ce := New(EditionCommunity).Capabilities()
	if ce.Edition != "community" || ce.Features.Dashboards || ce.Features.MapReports || ce.Features.DataIntelligence || ce.Features.Audit || ce.Features.SSOOIDC {
		t.Fatalf("unexpected community caps: %+v", ce)
	}
	if !ce.Features.DataExplorer || !ce.Maps.Designer.PostGIS || !ce.Maps.Embedded.MySQL || !ce.Maps.Embedded.GeoServer || !ce.Maps.Embedded.QGIS {
		t.Fatalf("community missing expected allows: %+v", ce)
	}
	if ce.Maps.Designer.GeoServer || ce.Maps.Designer.QGIS {
		t.Fatalf("community designer geoserver/qgis should be false: %+v", ce)
	}

	pro := New(EditionProfessional).Capabilities()
	if pro.Edition != "professional" || !pro.Features.Dashboards || !pro.Features.MapReports || !pro.Features.Audit || !pro.Features.SSOOIDC {
		t.Fatalf("unexpected professional caps: %+v", pro)
	}
}

func TestJasperDatasourcesNotEditionRestricted(t *testing.T) {
	t.Parallel()
	// Documented regression guard: edition package must not expose datasource
	// entitlements that could be mistaken for Jasper report restrictions.
	svc := New(EditionCommunity)
	for _, name := range []string{
		"postgresql", "mysql", "sqlserver", "oracle", "snowflake",
		"bigquery", "duckdb", "cassandra", "db2", "sqlite",
	} {
		// No FeatureDatasource* helpers — HasFeature of a made-up datasource
		// key must remain false in Community without implying Jasper denial.
		_ = name
		if svc.HasFeature(Feature("datasource." + name)) {
			t.Fatalf("unexpected datasource entitlement for %s", name)
		}
	}
	// Professional still returns true for any feature key (permissive model).
	pro := New(EditionProfessional)
	if !pro.HasFeature(Feature("datasource.snowflake")) {
		t.Fatal("professional permissive model broken")
	}
}
