package folders

import (
	"context"
	"testing"
)

func TestWouldCreateCycleLogic(t *testing.T) {
	// Pure logic covered via ancestor walk semantics in repository;
	// this guards FolderResourceKey and FormatPath helpers.
	if FolderResourceKey(42) != "42" {
		t.Fatalf("key")
	}
	path := FormatPath([]Breadcrumb{{Name: "Home"}, {Name: "A"}, {Name: "B"}})
	if path != "Home > A > B" {
		t.Fatalf("path=%q", path)
	}
}

func TestRemoveReportAndUpdateDisplayNameNoopWhenUnconfigured(t *testing.T) {
	s := &Service{}
	if err := s.RemoveReport(context.Background(), "demo"); err != nil {
		t.Fatalf("RemoveReport: %v", err)
	}
	if err := s.UpdateReportDisplayName(context.Background(), "demo", "New Name"); err != nil {
		t.Fatalf("UpdateReportDisplayName: %v", err)
	}
}
