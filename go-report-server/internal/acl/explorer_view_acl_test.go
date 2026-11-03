package acl_test

import (
	"context"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
)

func (m *syncingACLRepo) SyncExplorerViewAccessGroups(_ context.Context, viewRef, _ string, groupNames []string, _ int64) error {
	viewRef = strings.TrimSpace(viewRef)
	names := uniqueNonEmptyTest(groupNames)
	if len(names) == 0 {
		names = []string{acl.GroupPublic}
	}
	key := acl.ResourceTypeExplorerView + "|" + viewRef
	next := map[string]string{
		acl.GroupAdministrators: acl.PermissionAdmin,
	}
	for _, name := range names {
		if strings.EqualFold(name, acl.GroupAdministrators) {
			continue
		}
		next[name] = acl.PermissionView
	}
	m.grants[key] = next
	return nil
}

func mustViewExplorer(t *testing.T, svc *acl.DBService, user *auth.User, ref string, want bool) {
	t.Helper()
	ok, err := svc.CanViewExplorerView(context.Background(), user, ref)
	if err != nil {
		t.Fatalf("CanViewExplorerView(%s): %v", ref, err)
	}
	if ok != want {
		t.Fatalf("CanViewExplorerView(%s)=%v, want %v", ref, ok, want)
	}
}

func filterVisibleViews(t *testing.T, svc *acl.DBService, user *auth.User, refs []string) []string {
	t.Helper()
	var out []string
	for _, ref := range refs {
		ok, err := svc.CanViewExplorerView(context.Background(), user, ref)
		if err != nil {
			t.Fatalf("filter %s: %v", ref, err)
		}
		if ok {
			out = append(out, ref)
		}
	}
	return out
}

func TestExplorerViewGroupGrantAndRevoke(t *testing.T) {
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 10, Username: "alice", Enabled: true}
	repo.setMembership(10, "Sales")

	// A. Grant View to Group -> member sees View
	if err := repo.SyncExplorerViewAccessGroups(context.Background(), "bob/customer-analysis", "Customer Analysis", []string{"Sales"}, 0); err != nil {
		t.Fatal(err)
	}
	mustViewExplorer(t, svc, alice, "bob/customer-analysis", true)

	// B. Revoke Group -> member no longer sees View
	if err := repo.SyncExplorerViewAccessGroups(context.Background(), "bob/customer-analysis", "Customer Analysis", []string{"Engineering"}, 0); err != nil {
		t.Fatal(err)
	}
	mustViewExplorer(t, svc, alice, "bob/customer-analysis", false)
}

func TestExplorerViewDirectGroupGrant(t *testing.T) {
	// C. Direct group grant (Report Server has no per-user ACL rows; users are granted via groups).
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 11, Username: "alice", Enabled: true}
	repo.setMembership(11, "AliceOnly")

	if err := repo.SyncExplorerViewAccessGroups(context.Background(), "bob/infra", "Infra", []string{"AliceOnly"}, 0); err != nil {
		t.Fatal(err)
	}
	mustViewExplorer(t, svc, alice, "bob/infra", true)

	bob := &auth.User{ID: 12, Username: "carol", Enabled: true}
	repo.setMembership(12, "Other")
	mustViewExplorer(t, svc, bob, "bob/infra", false)
}

func TestExplorerViewUnauthorizedDenied(t *testing.T) {
	// D/E. Unauthorized user cannot open/load view by ref (ACL deny).
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	_ = repo.SyncExplorerViewAccessGroups(context.Background(), "owner/secret-view", "Secret", []string{"Engineering"}, 0)

	outsider := &auth.User{ID: 20, Username: "outsider", Enabled: true}
	repo.setMembership(20, "Sales")
	mustViewExplorer(t, svc, outsider, "owner/secret-view", false)
}

func TestExplorerViewAdminSeesAll(t *testing.T) {
	// F. Admin sees View
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	_ = repo.SyncExplorerViewAccessGroups(context.Background(), "owner/hidden", "Hidden", []string{"Engineering"}, 0)

	admin := &auth.User{ID: 1, Username: "admin", IsAdmin: true, Enabled: true}
	mustViewExplorer(t, svc, admin, "owner/hidden", true)
}

func TestDashboardACLSameResourceAsView(t *testing.T) {
	// G–L. Dashboards use the same explorer_view ACL key (is_dashboard is display-only).
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 30, Username: "alice", Enabled: true}
	repo.setMembership(30, "Sales")

	ref := "owner/sales-overview"
	_ = repo.SyncExplorerViewAccessGroups(context.Background(), ref, "Sales Overview", []string{"Sales"}, 0)
	mustViewExplorer(t, svc, alice, ref, true)

	_ = repo.SyncExplorerViewAccessGroups(context.Background(), ref, "Sales Overview", []string{"Operations"}, 0)
	mustViewExplorer(t, svc, alice, ref, false)

	_ = repo.SyncExplorerViewAccessGroups(context.Background(), ref, "Sales Overview", []string{"Sales"}, 0)
	mustViewExplorer(t, svc, alice, ref, true)

	ops := &auth.User{ID: 31, Username: "ops", Enabled: true}
	repo.setMembership(31, "Operations")
	mustViewExplorer(t, svc, ops, ref, false)

	admin := &auth.User{ID: 1, Username: "admin", IsAdmin: true, Enabled: true}
	mustViewExplorer(t, svc, admin, ref, true)
}

func TestExplorerViewMultipleGroupGrants(t *testing.T) {
	// M–P. User in Group A + Group B; remove A keeps B; remove B loses access.
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 40, Username: "alice", Enabled: true}
	repo.setMembership(40, "GroupA", "GroupB")

	ref := "owner/shared-view"
	_ = repo.SyncExplorerViewAccessGroups(context.Background(), ref, "Shared", []string{"GroupA", "GroupB"}, 0)
	mustViewExplorer(t, svc, alice, ref, true)

	_ = repo.SyncExplorerViewAccessGroups(context.Background(), ref, "Shared", []string{"GroupB"}, 0)
	mustViewExplorer(t, svc, alice, ref, true)

	_ = repo.SyncExplorerViewAccessGroups(context.Background(), ref, "Shared", []string{"Other"}, 0)
	mustViewExplorer(t, svc, alice, ref, false)
}

func TestExplorerViewNoFolderInheritance(t *testing.T) {
	// Q–S. Folder Public does not grant explorer_view access (same as reports).
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	svc.SetInheritance(mockInheritance{folderIDs: []int64{1}, inherit: true})
	repo.grants[acl.ResourceTypeContentFolder+"|1"] = map[string]string{
		acl.GroupPublic: acl.PermissionView,
	}

	alice := &auth.User{ID: 50, Username: "alice", Enabled: true}
	repo.setMembership(50, "Sales")

	// View with only Engineering grant — folder Public must not open it.
	_ = repo.SyncExplorerViewAccessGroups(context.Background(), "owner/eng-only", "Eng", []string{"Engineering"}, 0)
	mustViewExplorer(t, svc, alice, "owner/eng-only", false)

	// Authorized view still visible via direct grant.
	_ = repo.SyncExplorerViewAccessGroups(context.Background(), "owner/sales-view", "Sales", []string{"Sales"}, 0)
	mustViewExplorer(t, svc, alice, "owner/sales-view", true)

	visible := filterVisibleViews(t, svc, alice, []string{"owner/eng-only", "owner/sales-view"})
	if len(visible) != 1 || visible[0] != "owner/sales-view" {
		t.Fatalf("visible=%v", visible)
	}
}

func TestExplorerViewRevocationImmediate(t *testing.T) {
	// T–Y. Grant → see → revoke → deny immediately (no logout).
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 60, Username: "alice", Enabled: true}
	repo.setMembership(60, "Sales")

	ref := "owner/sales-overview"
	_ = repo.SyncExplorerViewAccessGroups(context.Background(), ref, "Sales Overview", []string{"Sales"}, 0)
	mustViewExplorer(t, svc, alice, ref, true)

	_ = repo.SyncExplorerViewAccessGroups(context.Background(), ref, "Sales Overview", []string{"Managers"}, 0)
	mustViewExplorer(t, svc, alice, ref, false)
	mustViewExplorer(t, svc, alice, ref, false) // still denied on refresh / same session
}

func TestExplorerViewListingFilter(t *testing.T) {
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	_ = repo.SyncExplorerViewAccessGroups(context.Background(), "a/customer-analysis", "Customer", []string{"Sales"}, 0)
	_ = repo.SyncExplorerViewAccessGroups(context.Background(), "a/infra-analysis", "Infra", []string{"Engineering"}, 0)

	alice := &auth.User{ID: 70, Username: "alice", Enabled: true}
	repo.setMembership(70, "Sales")

	visible := filterVisibleViews(t, svc, alice, []string{"a/customer-analysis", "a/infra-analysis"})
	if len(visible) != 1 || visible[0] != "a/customer-analysis" {
		t.Fatalf("visible=%v", visible)
	}
}

func TestExplorerViewClient1MemberSeesGrantedView(t *testing.T) {
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	clientUser := &auth.User{ID: 99, Username: "clientuser", Enabled: true, Groups: []string{"Client1"}}
	// Membership only on the session Groups slice (DB membership empty) — resolveGroups must union.
	repo.setMembership(99) // empty DB membership

	ref := "admin/environment-dashboard"
	if err := repo.SyncExplorerViewAccessGroups(context.Background(), ref, "Environment Dashboard", []string{"Client1"}, 0); err != nil {
		t.Fatal(err)
	}
	mustViewExplorer(t, svc, clientUser, ref, true)

	outsider := &auth.User{ID: 100, Username: "other", Enabled: true, Groups: []string{"Editors"}}
	repo.setMembership(100, "Editors")
	mustViewExplorer(t, svc, outsider, ref, false)
}

func TestExplorerViewDefaultEmptyIsPublic(t *testing.T) {
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	_ = repo.SyncExplorerViewAccessGroups(context.Background(), "owner/new-view", "New", nil, 0)

	anon := &auth.User{ID: 0, Username: "", Enabled: false}
	mustViewExplorer(t, svc, anon, "owner/new-view", true)
}
