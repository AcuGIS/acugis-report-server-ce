package reports_test

import (
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestCatalogParametersFromJRXML(t *testing.T) {
	body := []byte(`<?xml version="1.0"?>
<jasperReport name="Demo">
  <parameter name="p_region" class="java.util.Collection"/>
  <parameter name="p_state" class="java.lang.String"/>
  <parameter name="p_geometry" class="java.lang.String"/>
  <parameter name="REPORT_DIR" class="java.lang.String"/>
  <property name="acugis.parameter.p_state.lookup.sql" value="SELECT 1"/>
</jasperReport>`)

	params := reports.CatalogParametersFromJRXML(body)
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %#v", params)
	}
	byName := map[string]map[string]any{}
	for _, p := range params {
		byName[p["name"].(string)] = p
		if _, has := p["lookup"]; has {
			t.Fatalf("lookup imported: %#v", p)
		}
	}
	if byName["p_region"]["type"] != "select" || byName["p_region"]["multiple"] != true {
		t.Fatalf("p_region=%#v", byName["p_region"])
	}
	if byName["p_state"]["type"] != "string" || byName["p_state"]["hidden"] != false {
		t.Fatalf("p_state=%#v", byName["p_state"])
	}
	if byName["p_geometry"]["hidden"] != true || byName["p_geometry"]["required"] != false {
		t.Fatalf("p_geometry=%#v", byName["p_geometry"])
	}
	if _, ok := byName["REPORT_DIR"]; ok {
		t.Fatal("built-in parameter was imported")
	}
}
