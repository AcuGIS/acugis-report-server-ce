package acl_test

import (
	"context"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
)

// syncingACLRepo is a mutable mock that applies SyncReportAccessGroups
// replace semantics (delete prior grants, then apply selected groups).
type syncingACLRepo struct {
	mockACLRepo
	serviceGrants map[string]string // group → permission on jasper_reports service
}

func newSyncingACLRepo() *syncingACLRepo {
	return &syncingACLRepo{
		mockACLRepo: mockACLRepo{
			configured: true,
			groups:     map[int64][]string{},
			grants:     map[string]map[string]string{},
		},
		serviceGrants: map[string]string{},
	}
}

func (m *syncingACLRepo) setMembership(userID int64, groups ...string) {
	m.groups[userID] = append([]string(nil), groups...)
}

// SyncReportAccessGroups mirrors the fixed Repository behavior: replace, don't append.
func (m *syncingACLRepo) SyncReportAccessGroups(_ context.Context, reportKey, _ string, groupNames []string, _ int64) error {
	reportKey = strings.TrimSpace(reportKey)
	names := uniqueNonEmptyTest(groupNames)
	if len(names) == 0 {
		names = []string{acl.GroupPublic}
	}
	key := acl.ResourceTypeJasperReport + "|" + reportKey
	next := map[string]string{
		acl.GroupAdministrators: acl.PermissionAdmin,
	}
	for _, name := range names {
		if strings.EqualFold(name, acl.GroupAdministrators) {
			continue
		}
		next[name] = acl.PermissionView
		m.serviceGrants[name] = acl.PermissionView
	}
	m.grants[key] = next
	return nil
}

func uniqueNonEmptyTest(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, g := range in {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		key := strings.ToLower(g)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, g)
	}
	return out
}

func mustView(t *testing.T, svc *acl.DBService, user *auth.User, report string, want bool) {
	t.Helper()
	ok, err := svc.CanViewReport(context.Background(), user, report)
	if err != nil {
		t.Fatalf("CanViewReport(%s): %v", report, err)
	}
	if ok != want {
		t.Fatalf("CanViewReport(%s)=%v, want %v", report, ok, want)
	}
}

func filterVisible(t *testing.T, svc *acl.DBService, user *auth.User, keys []string) []string {
	t.Helper()
	var out []string
	for _, key := range keys {
		ok, err := svc.CanViewReport(context.Background(), user, key)
		if err != nil {
			t.Fatalf("filter %s: %v", key, err)
		}
		if ok {
			out = append(out, key)
		}
	}
	return out
}

func TestGroupPermissionGrantAndRevoke(t *testing.T) {
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 10, Username: "alice", Enabled: true}
	repo.setMembership(10, "Sales Team")

	// A. Group has View → member sees Report
	if err := repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales Report", []string{"Sales Team"}, 0); err != nil {
		t.Fatal(err)
	}
	mustView(t, svc, alice, "sales-report", true)

	// B. Revoke Group View → member no longer sees Report
	if err := repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales Report", []string{"Managers"}, 0); err != nil {
		t.Fatal(err)
	}
	mustView(t, svc, alice, "sales-report", false)

	// K. Immediate effect (no logout): same session user is denied after revoke
	mustView(t, svc, alice, "sales-report", false)
}

func TestTwoAuthorizedGroupsPartialRevoke(t *testing.T) {
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 11, Username: "alice", Enabled: true}
	repo.setMembership(11, "Sales Team", "Managers")

	if err := repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales",
		[]string{"Sales Team", "Managers"}, 0); err != nil {
		t.Fatal(err)
	}
	mustView(t, svc, alice, "sales-report", true)

	// C. Remove one authorized group → still allowed via the other
	if err := repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales",
		[]string{"Managers"}, 0); err != nil {
		t.Fatal(err)
	}
	mustView(t, svc, alice, "sales-report", true)

	// D. Remove both → access removed
	if err := repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales",
		[]string{"Editors"}, 0); err != nil {
		t.Fatal(err)
	}
	mustView(t, svc, alice, "sales-report", false)
}

func TestRemainingGrantSurvivesGroupRevoke(t *testing.T) {
	// Schema has no per-user ACL rows; residual access is another group grant.
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 12, Username: "alice", Enabled: true}
	repo.setMembership(12, "Sales Team", "Managers")

	if err := repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales",
		[]string{"Sales Team", "Managers"}, 0); err != nil {
		t.Fatal(err)
	}
	mustView(t, svc, alice, "sales-report", true)

	// E. Revoking Sales Team does not remove Managers grant
	if err := repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales",
		[]string{"Managers"}, 0); err != nil {
		t.Fatal(err)
	}
	mustView(t, svc, alice, "sales-report", true)
	if _, ok := repo.grants["jasper_report|sales-report"]["Sales Team"]; ok {
		t.Fatal("revoked Sales Team grant must not remain in ACL map")
	}
}

func TestUnauthorizedReportHiddenFromListingAndFolder(t *testing.T) {
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 13, Username: "alice", Enabled: true}
	repo.setMembership(13, "Sales Team")

	_ = repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales", []string{"Sales Team"}, 0)
	_ = repo.SyncReportAccessGroups(context.Background(), "hr-report", "HR", []string{"HR Team"}, 0)
	_ = repo.SyncReportAccessGroups(context.Background(), "public-report", "Public", []string{acl.GroupPublic}, 0)

	catalog := []string{"sales-report", "hr-report", "public-report"}
	// F. Unauthorized report does not appear in listing
	visible := filterVisible(t, svc, alice, catalog)
	if len(visible) != 2 || visible[0] != "sales-report" || visible[1] != "public-report" {
		t.Fatalf("listing filter got %v", visible)
	}

	// G. Unauthorized report does not appear inside folder browse (same canView filter)
	folderContents := []string{"hr-report", "sales-report"}
	inFolder := filterVisible(t, svc, alice, folderContents)
	if len(inFolder) != 1 || inFolder[0] != "sales-report" {
		t.Fatalf("folder filter got %v", inFolder)
	}
}

func TestUnauthorizedDirectOpenAndExecuteDenied(t *testing.T) {
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 14, Username: "alice", Enabled: true}
	repo.setMembership(14, "Sales Team")

	_ = repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales", []string{"Sales Team"}, 0)
	mustView(t, svc, alice, "sales-report", true)

	_ = repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales", []string{"Managers"}, 0)

	// H. Direct Report ID / URL — CanViewReport is what loadAuthorizedReport uses
	mustView(t, svc, alice, "sales-report", false)

	// I. Execute / export / image use the same view check
	ok, err := svc.CanAccessReportImage(context.Background(), alice, "sales-report")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected execute/image path denied after revoke")
	}
}

func TestAdminStillAccessesAfterGroupRevoke(t *testing.T) {
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	admin := &auth.User{ID: 1, Username: "admin", IsAdmin: true, Enabled: true}
	alice := &auth.User{ID: 15, Username: "alice", Enabled: true}
	repo.setMembership(15, "Sales Team")

	_ = repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales", []string{"Sales Team"}, 0)
	_ = repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales", []string{"Managers"}, 0)

	// J. Admin bypass preserved
	mustView(t, svc, admin, "sales-report", true)
	mustView(t, svc, alice, "sales-report", false)
}

func TestSyncReplaceDoesNotLeaveStaleGroupGrant(t *testing.T) {
	repo := newSyncingACLRepo()
	_ = repo.SyncReportAccessGroups(context.Background(), "demo", "Demo", []string{"Sales Team", "Editors"}, 0)
	grants := repo.grants["jasper_report|demo"]
	if grants["Sales Team"] != acl.PermissionView || grants["Editors"] != acl.PermissionView {
		t.Fatalf("initial grants: %+v", grants)
	}

	_ = repo.SyncReportAccessGroups(context.Background(), "demo", "Demo", []string{"Editors"}, 0)
	grants = repo.grants["jasper_report|demo"]
	if _, ok := grants["Sales Team"]; ok {
		t.Fatalf("stale Sales Team grant still present: %+v", grants)
	}
	if grants["Editors"] != acl.PermissionView {
		t.Fatalf("Editors should remain: %+v", grants)
	}
	if grants[acl.GroupAdministrators] != acl.PermissionAdmin {
		t.Fatalf("Administrators admin missing: %+v", grants)
	}
}

type mockInheritance struct {
	folderIDs []int64
	inherit   bool
}

func (m mockInheritance) ContentFolderChain(context.Context, string, string) ([]int64, bool, error) {
	return append([]int64(nil), m.folderIDs...), m.inherit, nil
}

func (m mockInheritance) FolderAncestorIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func TestReportDoesNotInheritFolderPublicGrant(t *testing.T) {
	// Home folder commonly grants Public view. That must not let a Public-only
	// user run a report whose Access Groups exclude Public.
	repo := newSyncingACLRepo()
	repo.grants["content_folder|1"] = map[string]string{acl.GroupPublic: acl.PermissionView}
	svc := acl.NewService(repo, acl.Options{})
	svc.SetInheritance(mockInheritance{folderIDs: []int64{1}, inherit: true})

	todd := &auth.User{ID: 2, Username: "todd", Enabled: true}
	repo.setMembership(2, acl.GroupPublic)

	_ = repo.SyncReportAccessGroups(context.Background(), "prccascade", "PRC",
		[]string{"Administrators", "Client1", "Editors"}, 0)

	mustView(t, svc, todd, "prccascade", false)

	// Content resources still inherit folder ACL.
	d, err := svc.CheckPermission(context.Background(), todd, "content_resource", "img-1", acl.PermissionView)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed || d.Source != acl.SourceInherited {
		t.Fatalf("content_resource should inherit folder Public, got %+v", d)
	}
}

func TestGrantRevokeRefreshDirectURLFlow(t *testing.T) {
	repo := newSyncingACLRepo()
	svc := acl.NewService(repo, acl.Options{})
	alice := &auth.User{ID: 16, Username: "alice", Enabled: true}
	repo.setMembership(16, "Sales Team")

	_ = repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales", []string{"Sales Team"}, 0)
	mustView(t, svc, alice, "sales-report", true)

	_ = repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales", []string{acl.GroupPublic}, 0)

	// Refresh as alice: only Public remains; alice still has Public via EffectiveGroupNames
	// Use a non-Public remaining set to assert disappearance.
	_ = repo.SyncReportAccessGroups(context.Background(), "sales-report", "Sales", []string{"Managers"}, 0)
	visible := filterVisible(t, svc, alice, []string{"sales-report"})
	if len(visible) != 0 {
		t.Fatalf("report should disappear from listing, got %v", visible)
	}
	mustView(t, svc, alice, "sales-report", false)
}
