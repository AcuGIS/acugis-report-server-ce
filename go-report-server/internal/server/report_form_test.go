package server

import "testing"

func TestReconcileEditParametersKeepsManualMetadata(t *testing.T) {
	existing := []map[string]any{
		{
			"name":    "parcel_id",
			"type":    "select",
			"label":   "Parcel",
			"source":  "sql:SELECT id, name FROM parcels",
			"default": "100",
			"hidden":  false,
		},
		{
			"name":    "legacy_note",
			"type":    "string",
			"label":   "Legacy Note",
			"default": "keep-me",
		},
	}
	form := []map[string]any{
		{"name": "parcel_id", "type": "select", "label": "Parcel ID"},
	}
	jrxml := []byte(`<?xml version="1.0"?>
<jasperReport name="parcel_card">
  <parameter name="parcel_id" class="java.lang.String"/>
  <parameter name="owner" class="java.lang.String"/>
</jasperReport>`)

	got := reconcileEditParameters(existing, form, jrxml)
	byName := paramMapByName(got)
	if len(got) != 2 {
		t.Fatalf("got %d params want 2 (form + new JRXML): %#v", len(got), got)
	}
	parcel := byName["parcel_id"]
	if parcel["label"] != "Parcel ID" {
		t.Fatalf("label=%v want form value", parcel["label"])
	}
	if parcel["source"] != "sql:SELECT id, name FROM parcels" {
		t.Fatalf("source=%v should keep SQL lookup", parcel["source"])
	}
	if parcel["default"] != "100" {
		t.Fatalf("default=%v should keep existing default", parcel["default"])
	}
	if byName["legacy_note"] != nil {
		t.Fatal("a parameter omitted from the submitted form is an intentional removal")
	}
	if byName["owner"]["type"] != "string" {
		t.Fatalf("owner=%#v should be imported from JRXML", byName["owner"])
	}
}

func TestReconcileEditParametersEmptyFormKeepsExisting(t *testing.T) {
	existing := []map[string]any{
		{"name": "p_region", "type": "select", "label": "Region", "options": []string{"North"}},
	}
	got := reconcileEditParameters(existing, nil, nil)
	if len(got) != 1 || got[0]["label"] != "Region" {
		t.Fatalf("got %#v", got)
	}
}
