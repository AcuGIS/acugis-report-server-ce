package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/modules"
)

func communityTestServer(t *testing.T, deps Dependencies) *Server {
	t.Helper()
	cfg := testConfig()
	cfg.Edition = "community"
	cfg.AuthDisabled = true
	return New(cfg, deps, slog.Default())
}

func professionalTestServer(t *testing.T, deps Dependencies) *Server {
	t.Helper()
	cfg := testConfig()
	cfg.Edition = "professional"
	cfg.AuthDisabled = true
	return New(cfg, deps, slog.Default())
}

func TestEditionCapabilitiesEndpoint(t *testing.T) {
	t.Parallel()
	srv := communityTestServer(t, Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/edition", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["edition"] != "community" {
		t.Fatalf("edition=%v", body["edition"])
	}
	caps, _ := body["capabilities"].(map[string]any)
	features, _ := caps["features"].(map[string]any)
	if features["dashboards"] != false || features["mapReports"] != false || features["dataIntelligence"] != false || features["audit"] != false {
		t.Fatalf("community features: %+v", features)
	}
	wantExplorer := modules.Has(modules.IDAdHoc)
	if features["dataExplorer"] != wantExplorer {
		t.Fatalf("dataExplorer=%v want %v (compiled adhoc=%v): %+v", features["dataExplorer"], wantExplorer, wantExplorer, features)
	}
	if body["report_designer"] != false {
		t.Fatalf("unconfigured report_designer=%v", body["report_designer"])
	}
}

func TestCommunityAllowsJasperDatasourcesConceptually(t *testing.T) {
	t.Parallel()
	svc := edition.New(edition.EditionCommunity)
	for _, engine := range []string{
		"postgresql", "mysql", "sqlserver", "oracle", "snowflake",
		"bigquery", "duckdb", "cassandra", "db2", "sqlite",
	} {
		if svc.HasFeature(edition.Feature("datasource." + engine)) {
			t.Fatalf("community must not define datasource entitlement for %s", engine)
		}
	}
	if !svc.HasDesignerMapProvider("postgis") || !svc.HasEmbeddedMapProvider("mysql") {
		t.Fatal("expected PostGIS/MySQL map entitlements")
	}
	if svc.HasDesignerMapProvider("arcgis") || svc.HasEmbeddedMapProvider("oracle") {
		t.Fatal("expected other map providers denied")
	}
}
