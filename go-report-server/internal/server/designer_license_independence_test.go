package server

import (
	"os"
	"strings"
	"testing"
)

func TestDesignerNotGatedByReportServerLicense(t *testing.T) {
	for _, name := range []string{"designer_proxy.go", "designer_product.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		for _, needle := range []string{
			"report-server-professional",
			"internal/license",
			"FeatureAudit",
		} {
			if strings.Contains(text, needle) {
				t.Fatalf("%s references %s", name, needle)
			}
		}
	}
}
