package acl_test

import (
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/acl"
)

func TestVerifiedPrecedenceOrder(t *testing.T) {
	if len(acl.VerifiedPrecedence) < 6 {
		t.Fatalf("precedence steps=%d", len(acl.VerifiedPrecedence))
	}
	want := []string{
		"JRS_AUTH_DISABLED",
		"is_admin",
		"Admin-named groups",
		"Group grant",
		"Folder inheritance",
		"Default deny",
	}
	for i, needle := range want {
		if !contains(acl.VerifiedPrecedence[i], needle) {
			t.Fatalf("step %d missing %q: %s", i+1, needle, acl.VerifiedPrecedence[i])
		}
	}
}

func TestContractConstants(t *testing.T) {
	if acl.ResourceTypeJasperReport != "jasper_report" {
		t.Fatal(acl.ResourceTypeJasperReport)
	}
	if acl.ResourceTypeExplorerView != "explorer_view" {
		t.Fatal(acl.ResourceTypeExplorerView)
	}
	if acl.ResourceTypeService != "service" {
		t.Fatal(acl.ResourceTypeService)
	}
	if acl.ServiceKeyJasperReports != "jasper_reports" {
		t.Fatal(acl.ServiceKeyJasperReports)
	}
}

func contains(s, needle string) bool {
	return len(s) >= len(needle) && (s == needle || stringContains(s, needle))
}

func stringContains(s, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
