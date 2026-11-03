package openapi_test

import (
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/openapi"
)

func TestBundleValidates(t *testing.T) {
	doc, err := openapi.Bundle("0.1.0-foundation")
	if err != nil {
		t.Fatalf("Bundle: %v", err)
	}
	if err := openapi.ValidateBasic(doc); err != nil {
		t.Fatalf("ValidateBasic: %v", err)
	}
	paths, _ := doc["paths"].(map[string]any)
	minPaths := 40
	if openapi.HasProfessionalPaths() {
		minPaths = 50
	}
	if len(paths) < minPaths {
		t.Fatalf("expected many paths, got %d", len(paths))
	}
	required := []string{
		"/api/v1/auth/login",
		"/api/v1/reports",
		"/reports/execute",
		"/api/v1/folders/tree",
		"/api/v1/resources",
		"/api/v1/datasources",
		"/health",
	}
	if openapi.HasProfessionalPaths() {
		required = append(required,
			"/api/v1/map/{key}/features",
			"/api/v1/explorer/query",
			"/api/v1/explorer/dashboard-widgets",
			"/api/v1/explorer/dashboards",
		)
	}
	for _, p := range required {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %s", p)
		}
	}
	info, _ := doc["info"].(map[string]any)
	if info["version"] != "0.1.0-foundation" {
		t.Fatalf("version=%v", info["version"])
	}
}

func TestMarshalYAMLAndJSON(t *testing.T) {
	y, err := openapi.MarshalYAML("9.9.9-test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(y), "9.9.9-test") {
		t.Fatal("yaml missing version")
	}
	if !strings.Contains(string(y), "operationId: login") {
		t.Fatal("yaml missing login operationId")
	}
	j, err := openapi.MarshalJSON("9.9.9-test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), `"operationId": "listReports"`) {
		t.Fatal("json missing listReports")
	}
}

func TestDeprecatedAliasesPresent(t *testing.T) {
	doc, err := openapi.Bundle("test")
	if err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	aliases := []string{
		"/reports/execute_report.php",
		"/reports/parameter_lookup.php",
		"/reports/get_report_config.php",
	}
	for _, p := range aliases {
		item, ok := paths[p].(map[string]any)
		if !ok {
			t.Errorf("missing alias path %s", p)
			continue
		}
		found := false
		for _, m := range []string{"get", "post"} {
			op, _ := item[m].(map[string]any)
			if op == nil {
				continue
			}
			if dep, _ := op["deprecated"].(bool); dep {
				found = true
			}
		}
		if !found {
			t.Errorf("%s not marked deprecated", p)
		}
	}
}
