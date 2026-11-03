package acl

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

// Service evaluates report and service ACL using PHP precedence rules.
type Service interface {
	CanViewReport(ctx context.Context, user *auth.User, reportID string) (bool, error)
	CanEditReport(ctx context.Context, user *auth.User, reportID string) (bool, error)
	CanAdminReport(ctx context.Context, user *auth.User, reportID string) (bool, error)
	CanViewExplorerView(ctx context.Context, user *auth.User, viewRef string) (bool, error)
	CanEditExplorerView(ctx context.Context, user *auth.User, viewRef string) (bool, error)
	CanAccessReportImage(ctx context.Context, user *auth.User, reportID string) (bool, error)
	CanAccessReportService(ctx context.Context, user *auth.User) (bool, error)
	CheckPermission(ctx context.Context, user *auth.User, resourceType, resourceKey, permission string) (Decision, error)
	Configured() bool
	AuthDisabled() bool
}

// DBService is the concrete ACL service backed by PostgreSQL.
type DBService struct {
	repo         PermissionChecker
	authDisabled bool
	logger       *slog.Logger
	inheritance  InheritanceLookup
	serviceKey   string
}

// Options configures the ACL service.
type Options struct {
	AuthDisabled bool
	Logger       *slog.Logger
	// ServiceKey overrides the platform service resource key (default jasper_reports).
	ServiceKey string
}

// NewService constructs a database-backed ACL service.
func NewService(repo PermissionChecker, opts Options) *DBService {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	key := strings.TrimSpace(opts.ServiceKey)
	if key == "" {
		key = ServiceKeyJasperReports
	}
	return &DBService{
		repo:         repo,
		authDisabled: opts.AuthDisabled,
		logger:       opts.Logger,
		serviceKey:   key,
	}
}

func (s *DBService) reportServiceKey() string {
	if s != nil && s.serviceKey != "" {
		return s.serviceKey
	}
	return ServiceKeyJasperReports
}

// Configured reports whether ACL lookups can hit the database.
func (s *DBService) Configured() bool {
	return s != nil && s.repo != nil && s.repo.Configured()
}

// AuthDisabled reports JRS_AUTH_DISABLED mode.
func (s *DBService) AuthDisabled() bool {
	return s != nil && s.authDisabled
}

func (s *DBService) CanViewReport(ctx context.Context, user *auth.User, reportID string) (bool, error) {
	d, err := s.CheckPermission(ctx, user, ResourceTypeJasperReport, reportID, PermissionView)
	return d.Allowed, err
}

func (s *DBService) CanEditReport(ctx context.Context, user *auth.User, reportID string) (bool, error) {
	d, err := s.CheckPermission(ctx, user, ResourceTypeJasperReport, reportID, PermissionEdit)
	return d.Allowed, err
}

func (s *DBService) CanAdminReport(ctx context.Context, user *auth.User, reportID string) (bool, error) {
	d, err := s.CheckPermission(ctx, user, ResourceTypeJasperReport, reportID, PermissionAdmin)
	return d.Allowed, err
}

// CanViewExplorerView checks group ACL on an explorer_view resource (owner/key).
// Ownership bypass is applied by the server layer, not here.
func (s *DBService) CanViewExplorerView(ctx context.Context, user *auth.User, viewRef string) (bool, error) {
	d, err := s.CheckPermission(ctx, user, ResourceTypeExplorerView, viewRef, PermissionView)
	return d.Allowed, err
}

// CanEditExplorerView checks edit ACL on an explorer_view resource (owner/key).
func (s *DBService) CanEditExplorerView(ctx context.Context, user *auth.User, viewRef string) (bool, error) {
	d, err := s.CheckPermission(ctx, user, ResourceTypeExplorerView, viewRef, PermissionEdit)
	return d.Allowed, err
}

// CanAccessReportImage matches PHP report_image.php: same view ACL as report runtime.
func (s *DBService) CanAccessReportImage(ctx context.Context, user *auth.User, reportID string) (bool, error) {
	return s.CanViewReport(ctx, user, reportID)
}

// CanAccessReportService mirrors PHP jrs_require_service_access for jasper_reports,
// including the fallback that grants service view when the user can view any report
// or any explorer view/dashboard.
func (s *DBService) CanAccessReportService(ctx context.Context, user *auth.User) (bool, error) {
	d, err := s.CheckPermission(ctx, user, ResourceTypeService, s.reportServiceKey(), PermissionView)
	if err != nil {
		return false, err
	}
	if d.Allowed {
		return true, nil
	}
	if s.authDisabled {
		return true, nil
	}
	if !s.Configured() {
		return false, fmt.Errorf("%w: repository not configured", ErrDatabase)
	}

	groups, err := s.resolveGroups(ctx, user)
	if err != nil {
		return false, err
	}
	if auth.GroupsIncludeAdmin(groups) {
		return true, nil
	}
	effective := EffectiveGroupNames(groups)
	ok, err := s.repo.HasAnyJasperReportPermission(ctx, PermissionView, effective)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	return s.repo.HasAnyResourceTypePermission(ctx, ResourceTypeExplorerView, PermissionView, effective)
}

// CheckPermission centralizes PHP jrs_has_permission precedence:
//
//  1. JRS_AUTH_DISABLED → allow
//  2. dashboard_users.is_admin → allow (caller-level; also checked here)
//  3. Admin-named groups → allow
//  4. Group grants on the resource (including implicit Public)
//  5. Folder inheritance for folders / content resources (not jasper_report / explorer_view)
//  6. Default deny
//
// There are no per-user ACL rows in the PHP schema; only group grants exist.
// Permission hierarchy inside the SQL: exact match, or admin, or edit satisfying view.
// jasper_report and explorer_view do not inherit folder ACL (Access Groups are authoritative).
func (s *DBService) CheckPermission(ctx context.Context, user *auth.User, resourceType, resourceKey, permission string) (Decision, error) {
	start := time.Now()
	permission = strings.ToLower(strings.TrimSpace(permission))
	resourceType = strings.TrimSpace(resourceType)
	resourceKey = strings.TrimSpace(resourceKey)

	d := Decision{
		Permission:   permission,
		ResourceType: resourceType,
		ResourceKey:  resourceKey,
		Source:       SourceDefaultDeny,
	}

	if s == nil {
		return d, fmt.Errorf("%w: service nil", ErrDatabase)
	}

	if s.authDisabled {
		d.Allowed = true
		d.Source = SourceAuthDisabled
		s.logDecision(ctx, user, d, start)
		return d, nil
	}

	if user != nil && user.IsAdmin {
		d.Allowed = true
		d.Source = SourceAdminFlag
		s.logDecision(ctx, user, d, start)
		return d, nil
	}

	if !validPermission(permission) {
		d.Source = SourceDefaultDeny
		s.logDecision(ctx, user, d, start)
		return d, nil
	}

	if !s.Configured() {
		return d, fmt.Errorf("%w: repository not configured", ErrDatabase)
	}

	groups, err := s.resolveGroups(ctx, user)
	if err != nil {
		return d, err
	}
	if auth.GroupsIncludeAdmin(groups) {
		d.Allowed = true
		d.Source = SourceAdminGroup
		s.logDecision(ctx, user, d, start)
		return d, nil
	}

	effective := EffectiveGroupNames(groups)
	ok, err := s.repo.HasGroupPermission(ctx, resourceType, resourceKey, permission, effective)
	if err != nil {
		return d, err
	}
	if ok {
		d.Allowed = true
		if onlyPublicGrant(groups) {
			d.Source = SourcePublicGrant
		} else {
			d.Source = SourceGroupGrant
		}
		s.logDecision(ctx, user, d, start)
		return d, nil
	}

	// Folder inheritance: parent folder grants apply to child folders and
	// contained content unless the item/folder opted out (acl_inherit=false).
	if s.inheritance != nil {
		inherited, ierr := s.checkInherited(ctx, resourceType, resourceKey, permission, effective)
		if ierr != nil {
			return d, ierr
		}
		if inherited {
			d.Allowed = true
			d.Source = SourceInherited
			s.logDecision(ctx, user, d, start)
			return d, nil
		}
	}

	d.Allowed = false
	d.Source = SourceDefaultDeny
	s.logDecision(ctx, user, d, start)
	return d, nil
}

func (s *DBService) checkInherited(ctx context.Context, resourceType, resourceKey, permission string, groups []string) (bool, error) {
	switch resourceType {
	case ResourceTypeContentFolder:
		var folderID int64
		if _, err := fmt.Sscanf(resourceKey, "%d", &folderID); err != nil || folderID <= 0 {
			return false, nil
		}
		ancestors, err := s.inheritance.FolderAncestorIDs(ctx, folderID)
		if err != nil {
			return false, err
		}
		for _, id := range ancestors {
			ok, err := s.repo.HasGroupPermission(ctx, ResourceTypeContentFolder, fmt.Sprintf("%d", id), permission, groups)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	case ResourceTypeService, ResourceTypeJasperReport, ResourceTypeExplorerView:
		// Reports and explorer views/dashboards use resource_permissions only.
		// Folder Public grants must not bypass the Access Groups UI.
		return false, nil
	default:
		// content_resource and other folder-placed types inherit folder ACL.
		folderIDs, inherit, err := s.inheritance.ContentFolderChain(ctx, resourceType, resourceKey)
		if err != nil || !inherit || len(folderIDs) == 0 {
			return false, err
		}
		for _, id := range folderIDs {
			ok, err := s.repo.HasGroupPermission(ctx, ResourceTypeContentFolder, fmt.Sprintf("%d", id), permission, groups)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
}

func (s *DBService) resolveGroups(ctx context.Context, user *auth.User) ([]string, error) {
	if user == nil {
		return nil, nil
	}
	// Union session groups with live auth_group_members. Relying only on the DB
	// query drops legacy dashboard_users.groups entries whenever any membership
	// row exists; relying only on the session can miss freshly assigned groups.
	seen := map[string]struct{}{}
	var out []string
	add := func(groups []string) {
		for _, g := range groups {
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
	}
	add(user.Groups)
	if user.ID > 0 {
		groups, err := s.repo.UserGroupNames(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		add(groups)
	}
	return out, nil
}

func onlyPublicGrant(groups []string) bool {
	for _, g := range groups {
		if strings.TrimSpace(g) == "" {
			continue
		}
		if !strings.EqualFold(g, GroupPublic) {
			return false
		}
	}
	return true
}

func (s *DBService) logDecision(ctx context.Context, user *auth.User, d Decision, start time.Time) {
	if s.logger == nil {
		return
	}
	uid := int64(0)
	username := "anonymous"
	if user != nil {
		uid = user.ID
		if user.Username != "" {
			username = user.Username
		}
	}
	s.logger.Info("acl decision",
		"user_id", uid,
		"username", username,
		"resource_type", d.ResourceType,
		"resource_key", d.ResourceKey,
		"permission", d.Permission,
		"allowed", d.Allowed,
		"decision_source", string(d.Source),
		"duration", time.Since(start),
	)
}

// VerifiedPrecedence documents the order confirmed from lib/auth.php.
var VerifiedPrecedence = []string{
	"1. JRS_AUTH_DISABLED → allow all",
	"2. dashboard_users.is_admin → allow",
	"3. Admin-named groups (administrator|administrators|admin|admins|dashboard-admin) → allow",
	"4. Group grant on resources/resource_permissions (edit satisfies view; admin satisfies any), including implicit Public membership",
	"5. Folder inheritance for folders and content resources only (jasper_report and explorer_view do not inherit)",
	"6. Default deny",
}
