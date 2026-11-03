package acl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/auth"
)

type mockACLRepo struct {
	configured bool
	groups     map[int64][]string
	// key: type|key|perm → allowed group set that must intersect
	grants map[string]map[string]string // resourceKey → group → permission
	err    error
}

func (m *mockACLRepo) Configured() bool { return m.configured }

func (m *mockACLRepo) UserGroupNames(_ context.Context, userID int64) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.groups == nil {
		return nil, nil
	}
	return append([]string(nil), m.groups[userID]...), nil
}

func (m *mockACLRepo) HasGroupPermission(_ context.Context, resourceType, resourceKey, permission string, groups []string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	resGrants, ok := m.grants[resourceType+"|"+resourceKey]
	if !ok {
		return false, nil
	}
	for _, g := range groups {
		perm, ok := resGrants[g]
		if !ok {
			// case-insensitive Public etc.
			for gn, p := range resGrants {
				if equalFold(gn, g) {
					perm = p
					ok = true
					break
				}
			}
		}
		if !ok {
			continue
		}
		if satisfies(perm, permission) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockACLRepo) HasAnyJasperReportPermission(ctx context.Context, permission string, groups []string) (bool, error) {
	return m.HasAnyResourceTypePermission(ctx, acl.ResourceTypeJasperReport, permission, groups)
}

func (m *mockACLRepo) HasAnyResourceTypePermission(_ context.Context, resourceType, permission string, groups []string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	prefix := resourceType + "|"
	for key, resGrants := range m.grants {
		if len(key) < len(prefix) || key[:len(prefix)] != prefix {
			continue
		}
		for _, g := range groups {
			for gn, p := range resGrants {
				if equalFold(gn, g) && satisfies(p, permission) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (m *mockACLRepo) FindResource(context.Context, string, string) (*acl.Resource, error) {
	return nil, errors.New("not used")
}

func equalFold(a, b string) bool {
	return len(a) == len(b) && (a == b || stringEqualFold(a, b))
}

func stringEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func satisfies(granted, requested string) bool {
	if granted == requested {
		return true
	}
	if granted == acl.PermissionAdmin {
		return true
	}
	if requested == acl.PermissionView && granted == acl.PermissionEdit {
		return true
	}
	return false
}

func TestEffectiveGroupNamesAddsPublic(t *testing.T) {
	got := acl.EffectiveGroupNames([]string{"Editors"})
	if len(got) != 2 || got[1] != acl.GroupPublic {
		t.Fatalf("got=%v", got)
	}
	got = acl.EffectiveGroupNames([]string{"Public"})
	if len(got) != 1 {
		t.Fatalf("duplicate Public: %v", got)
	}
}

func TestAuthDisabledAllows(t *testing.T) {
	svc := acl.NewService(&mockACLRepo{configured: true}, acl.Options{AuthDisabled: true})
	ok, err := svc.CanViewReport(context.Background(), nil, "demo")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestAdminFlagBypass(t *testing.T) {
	svc := acl.NewService(&mockACLRepo{configured: true}, acl.Options{})
	u := &auth.User{ID: 1, Username: "admin", IsAdmin: true, Enabled: true}
	ok, err := svc.CanViewReport(context.Background(), u, "missing")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestAdminGroupBypass(t *testing.T) {
	repo := &mockACLRepo{
		configured: true,
		groups:     map[int64][]string{2: {"Administrators"}},
	}
	svc := acl.NewService(repo, acl.Options{})
	u := &auth.User{ID: 2, Username: "boss", Enabled: true}
	ok, err := svc.CanEditReport(context.Background(), u, "demo")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestExplicitGroupViewGrant(t *testing.T) {
	repo := &mockACLRepo{
		configured: true,
		groups:     map[int64][]string{3: {"Editors"}},
		grants: map[string]map[string]string{
			"jasper_report|demo": {"Editors": "view"},
		},
	}
	svc := acl.NewService(repo, acl.Options{})
	u := &auth.User{ID: 3, Username: "ed", Enabled: true}
	ok, err := svc.CanViewReport(context.Background(), u, "demo")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestEditSatisfiesView(t *testing.T) {
	repo := &mockACLRepo{
		configured: true,
		groups:     map[int64][]string{3: {"Editors"}},
		grants: map[string]map[string]string{
			"jasper_report|demo": {"Editors": "edit"},
		},
	}
	svc := acl.NewService(repo, acl.Options{})
	u := &auth.User{ID: 3, Username: "ed", Enabled: true}
	ok, err := svc.CanViewReport(context.Background(), u, "demo")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	ok, err = svc.CanEditReport(context.Background(), u, "demo")
	if err != nil || !ok {
		t.Fatalf("edit ok=%v err=%v", ok, err)
	}
}

func TestPublicGrantAnonymous(t *testing.T) {
	repo := &mockACLRepo{
		configured: true,
		grants: map[string]map[string]string{
			"jasper_report|demo": {"Public": "view"},
		},
	}
	svc := acl.NewService(repo, acl.Options{})
	ok, err := svc.CanViewReport(context.Background(), nil, "demo")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	d, err := svc.CheckPermission(context.Background(), nil, acl.ResourceTypeJasperReport, "demo", acl.PermissionView)
	if err != nil || !d.Allowed || d.Source != acl.SourcePublicGrant {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}

func TestNoMatchingGrantDenies(t *testing.T) {
	repo := &mockACLRepo{
		configured: true,
		groups:     map[int64][]string{5: {"Custom"}},
		grants: map[string]map[string]string{
			"jasper_report|demo": {"Editors": "view"},
		},
	}
	svc := acl.NewService(repo, acl.Options{})
	u := &auth.User{ID: 5, Username: "x", Enabled: true}
	ok, err := svc.CanViewReport(context.Background(), u, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected deny")
	}
}

func TestServicePermissionAndFallback(t *testing.T) {
	repo := &mockACLRepo{
		configured: true,
		groups:     map[int64][]string{9: {"Custom"}},
		grants: map[string]map[string]string{
			"jasper_report|demo": {"Custom": "view"},
		},
	}
	svc := acl.NewService(repo, acl.Options{})
	u := &auth.User{ID: 9, Username: "c", Enabled: true}
	ok, err := svc.CanAccessReportService(context.Background(), u)
	if err != nil || !ok {
		t.Fatalf("fallback ok=%v err=%v", ok, err)
	}
}

func TestDatabaseFailure(t *testing.T) {
	repo := &mockACLRepo{configured: true, err: errors.Join(acl.ErrDatabase, errors.New("down"))}
	svc := acl.NewService(repo, acl.Options{})
	u := &auth.User{ID: 1, Username: "u", Enabled: true}
	_, err := svc.CanViewReport(context.Background(), u, "demo")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestImageUsesView(t *testing.T) {
	repo := &mockACLRepo{
		configured: true,
		grants: map[string]map[string]string{
			"jasper_report|demo": {"Public": "view"},
		},
	}
	svc := acl.NewService(repo, acl.Options{})
	ok, err := svc.CanAccessReportImage(context.Background(), nil, "demo")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestUnconfiguredRepository(t *testing.T) {
	svc := acl.NewService(&mockACLRepo{configured: false}, acl.Options{})
	_, err := svc.CanViewReport(context.Background(), &auth.User{ID: 1}, "demo")
	if err == nil {
		t.Fatal("expected database error")
	}
}
