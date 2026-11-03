package mapreport_test

import (
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/mapreport"
)

func TestPublishParametersFromJRXML_NamesAndTypesOnly(t *testing.T) {
	body := []byte(`<?xml version="1.0"?>
<jasperReport name="Demo">
  <parameter name="p_region" class="java.util.Collection"/>
  <parameter name="p_state" class="java.lang.String"/>
  <parameter name="p_geometry" class="java.lang.String" forPrompting="false"/>
  <parameter name="REPORT_DIR" class="java.lang.String"/>
  <property name="acugis.parameter.p_state.lookup.sql" value="SELECT 1"/>
</jasperReport>`)

	params := mapreport.PublishParametersFromJRXML(body)
	if len(params) != 3 {
		t.Fatalf("expected 3 params (skip REPORT_DIR), got %#v", params)
	}
	byName := map[string]map[string]any{}
	for _, p := range params {
		byName[p["name"].(string)] = p
		if _, hasLookup := p["lookup"]; hasLookup {
			t.Fatalf("must not import lookup: %#v", p)
		}
		if _, hasSource := p["source"]; hasSource {
			t.Fatalf("must not import source/sql: %#v", p)
		}
	}
	if byName["p_region"]["type"] != "select" || byName["p_region"]["multiple"] != true {
		t.Fatalf("p_region=%#v", byName["p_region"])
	}
	if byName["p_state"]["type"] != "string" {
		t.Fatalf("p_state=%#v", byName["p_state"])
	}
	if byName["p_geometry"]["hidden"] != true {
		t.Fatalf("p_geometry=%#v", byName["p_geometry"])
	}
	if mapreport.IsMapDrivenParameter("p_customer") || mapreport.IsPromptExcludedParameter("p_customer") {
		t.Fatal("p_customer must remain a user parameter")
	}
	if !mapreport.IsMapDrivenParameter("selected_ids") || !mapreport.IsPromptExcludedParameter("P_MAP_IMAGE") || !mapreport.IsPromptExcludedParameter("REPORT_CONNECTION") {
		t.Fatal("known internal parameters were not excluded")
	}
}
