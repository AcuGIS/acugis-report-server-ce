package server

import (
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/mapreport"
)

func TestEnrichConfigWithJRXMLLookups(t *testing.T) {
	payload := map[string]any{
		"datasource": "sales",
		"parameters": []any{
			map[string]any{
				"name":  "p_country",
				"type":  "string",
				"label": "Country",
			},
			map[string]any{
				"name":  "p_industry",
				"type":  "select",
				"label": "Industry",
				"source": "sql:select distinct industry from t",
				"options": []map[string]any{
					{"label": "Energy", "value": "Energy"},
				},
			},
		},
	}
	lookups := map[string]mapreport.ParameterLookupMeta{
		"p_city": {
			Enabled:      true,
			Datasource:   mapreport.LookupDatasourceCurrentReport,
			Depends:      []string{"p_country"},
			DisplayField: "city",
			ValueField:   "city",
			SQL:          "SELECT city FROM t WHERE country = $P{p_country}",
		},
	}
	jrxmlParams := map[string]mapreport.ParamMeta{
		"p_country": {Name: "p_country", Class: "java.lang.String", Kind: "string"},
		"p_city":    {Name: "p_city", Class: "java.util.Collection", Kind: "collection"},
	}

	out := enrichConfigWithJRXMLLookups(payload, lookups, jrxmlParams)
	params := out["parameters"].([]any)
	if len(params) != 3 {
		t.Fatalf("len=%d params=%v", len(params), params)
	}

	var city map[string]any
	for _, raw := range params {
		m := raw.(map[string]any)
		if m["name"] == "p_city" {
			city = m
		}
		if m["name"] == "p_industry" {
			if m["lookup"] != nil {
				t.Fatal("static select should not gain lookup")
			}
			opts := m["options"].([]map[string]any)
			if len(opts) != 1 {
				t.Fatalf("industry options cleared: %v", opts)
			}
		}
	}
	if city == nil {
		t.Fatal("p_city missing")
	}
	if city["type"] != "select" {
		t.Fatalf("type=%v", city["type"])
	}
	if city["multiple"] != true {
		t.Fatal("expected multiple for Collection")
	}
	lookup := city["lookup"].(map[string]any)
	if lookup["sql"] == nil || lookup["enabled"] != true {
		t.Fatalf("lookup=%v", lookup)
	}
	deps := lookup["depends"].([]string)
	if len(deps) != 1 || deps[0] != "p_country" {
		t.Fatalf("depends=%v", deps)
	}
}

func TestEnrichConfigWithJRXMLLookups_NoLookupsUnchanged(t *testing.T) {
	payload := map[string]any{
		"parameters": []any{
			map[string]any{"name": "p_a", "type": "string"},
		},
	}
	out := enrichConfigWithJRXMLLookups(payload, nil, nil)
	if len(out["parameters"].([]any)) != 1 {
		t.Fatal("changed without lookups")
	}
}

func TestEnrichConfigWithJRXMLDefaults_OnlySetsDefault(t *testing.T) {
	payload := map[string]any{
		"parameters": []any{
			map[string]any{
				"name":     "p_city",
				"type":     "select",
				"multiple": true,
				"options": []any{
					map[string]any{"label": "Austin", "value": "Austin"},
					map[string]any{"label": "Dallas", "value": "Dallas"},
				},
			},
		},
	}
	jrxmlParams := map[string]mapreport.ParamMeta{
		"p_city": {
			Name:    "p_city",
			Class:   "java.util.Collection",
			Kind:    "collection",
			Default: []string{"Austin"},
		},
	}
	out := enrichConfigWithJRXMLLookups(payload, nil, jrxmlParams)
	params := out["parameters"].([]any)
	m := params[0].(map[string]any)
	def, ok := m["default"].([]string)
	if !ok || len(def) != 1 || def[0] != "Austin" {
		t.Fatalf("default=%#v", m["default"])
	}
	opts := m["options"].([]any)
	if len(opts) != 2 {
		t.Fatalf("options filtered: %#v", opts)
	}
}
