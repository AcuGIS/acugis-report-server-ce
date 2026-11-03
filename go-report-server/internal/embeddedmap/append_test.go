package embeddedmap

import (
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestAppendToQueryWhenDisabled(t *testing.T) {
	svc := &Service{}
	def := &reports.Definition{
		Key: "demo",
		Raw: map[string]any{
			"embeddedMap": map[string]any{"enabled": false},
		},
	}
	res := svc.AppendToQuery(t.Context(), def, "a=1", map[string]any{"a": "1"})
	if res.Query != "a=1" || res.Error != "" {
		t.Fatalf("unexpected: %+v", res)
	}
}

func TestReportEmbeddedMap(t *testing.T) {
	def := &reports.Definition{Raw: map[string]any{"embeddedMap": map[string]any{"enabled": true, "mapType": "locator"}}}
	if ReportEmbeddedMap(def) == nil {
		t.Fatal("expected enabled embed")
	}
	def.Raw["embeddedMap"] = map[string]any{"enabled": false}
	if ReportEmbeddedMap(def) != nil {
		t.Fatal("expected nil when disabled")
	}
}
