package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/modules"
)

func navLabels(links []shellLink) []string {
	out := make([]string, 0, len(links))
	for _, l := range links {
		out = append(out, l.Label)
	}
	return out
}

func containsLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

func TestReportsHubHidesDesignerUntilConfigured(t *testing.T) {
	t.Parallel()
	srv := communityTestServer(t, Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/hubs/reports", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Publish") || !strings.Contains(body, "reports/publish_report") {
		t.Fatal("reports hub missing publish")
	}
	if strings.Contains(body, "Create Designer Report") || strings.Contains(body, "Import Designer Project") || strings.Contains(body, "reports/designer/") {
		t.Fatal("unconfigured designer items are in the reports menu")
	}
}

func TestCommunityAppNavIsFourCoreItems(t *testing.T) {
	t.Parallel()
	srv := communityTestServer(t, Dependencies{})
	got := navLabels(srv.appNavLinksForEdition())
	want := []string{"Reports", "Data Sources", "Scheduling", "Administration"}
	if len(got) != len(want) {
		t.Fatalf("community nav=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("community nav=%v want %v", got, want)
		}
	}
	for _, banned := range []string{"Ad Hoc", "Dashboards", "Data Quality", "Auditing"} {
		if containsLabel(got, banned) {
			t.Fatalf("community nav unexpectedly includes %q: %v", banned, got)
		}
	}
}

func TestProfessionalAppNavIncludesCompiledModules(t *testing.T) {
	t.Parallel()
	srv := professionalTestServer(t, Dependencies{})
	got := navLabels(srv.appNavLinksForEdition())
	for _, core := range []string{"Reports", "Data Sources", "Scheduling", "Administration"} {
		if !containsLabel(got, core) {
			t.Fatalf("professional nav missing %q: %v", core, got)
		}
	}
	if modules.Has(modules.IDAdHoc) != containsLabel(got, "Ad Hoc") {
		t.Fatalf("Ad Hoc presence=%v compiled=%v nav=%v", containsLabel(got, "Ad Hoc"), modules.Has(modules.IDAdHoc), got)
	}
	if modules.Has(modules.IDDashboards) != containsLabel(got, "Dashboards") {
		t.Fatalf("Dashboards presence=%v compiled=%v nav=%v", containsLabel(got, "Dashboards"), modules.Has(modules.IDDashboards), got)
	}
	if modules.Has(modules.IDDataIntel) != containsLabel(got, "Data Quality") {
		t.Fatalf("Data Quality presence=%v compiled=%v nav=%v", containsLabel(got, "Data Quality"), modules.Has(modules.IDDataIntel), got)
	}
	if modules.Has(modules.IDAudit) != containsLabel(got, "Auditing") {
		t.Fatalf("Auditing presence=%v compiled=%v nav=%v", containsLabel(got, "Auditing"), modules.Has(modules.IDAudit), got)
	}
}
