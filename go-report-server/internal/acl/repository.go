package acl

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PermissionChecker abstracts repository SQL for tests.
type PermissionChecker interface {
	Configured() bool
	UserGroupNames(ctx context.Context, userID int64) ([]string, error)
	HasGroupPermission(ctx context.Context, resourceType, resourceKey, permission string, groups []string) (bool, error)
	HasAnyJasperReportPermission(ctx context.Context, permission string, groups []string) (bool, error)
	HasAnyResourceTypePermission(ctx context.Context, resourceType, permission string, groups []string) (bool, error)
	FindResource(ctx context.Context, resourceType, resourceKey string) (*Resource, error)
}

// Repository performs read-only ACL lookups against PHP tables.
type Repository struct {
	pool                   *pgxpool.Pool
	platformServiceKey     string
	platformServiceDisplay string
	platformServiceName    string
}

// NewRepository constructs a read-only ACL repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// SetPlatformService overrides the platform service catalog row used when
// publish/edit syncs Access Groups. Empty values keep the standalone Jasper defaults.
func (r *Repository) SetPlatformService(key, displayName, serviceName string) {
	if r == nil {
		return
	}
	r.platformServiceKey = strings.TrimSpace(key)
	r.platformServiceDisplay = strings.TrimSpace(displayName)
	r.platformServiceName = strings.TrimSpace(serviceName)
}

// Configured reports whether a pool is available.
func (r *Repository) Configured() bool {
	return r != nil && r.pool != nil
}

// UserGroupNames mirrors PHP jrs_auth_user_group_names.
func (r *Repository) UserGroupNames(ctx context.Context, userID int64) ([]string, error) {
	if !r.Configured() {
		return nil, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	if userID <= 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
SELECT g.name
FROM auth_group_members gm
JOIN auth_groups g ON g.id = gm.group_id
WHERE gm.user_id = $1
ORDER BY g.name ASC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
		}
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return out, nil
}

// HasGroupPermission mirrors the EXISTS query in PHP jrs_has_permission.
func (r *Repository) HasGroupPermission(ctx context.Context, resourceType, resourceKey, permission string, groups []string) (bool, error) {
	if !r.Configured() {
		return false, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	resourceType = strings.TrimSpace(resourceType)
	resourceKey = strings.TrimSpace(resourceKey)
	permission = strings.ToLower(strings.TrimSpace(permission))
	if resourceType == "" || resourceKey == "" || !validPermission(permission) || len(groups) == 0 {
		return false, nil
	}

	// Dynamic IN list matching PHP placeholder construction.
	// Compare group names case-insensitively so "Client1" / "client1" both match.
	args := make([]any, 0, 3+len(groups))
	args = append(args, resourceType, resourceKey, permission)
	placeholders := make([]string, 0, len(groups))
	for i, g := range groups {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+4))
		args = append(args, strings.ToLower(strings.TrimSpace(g)))
	}
	sql := fmt.Sprintf(`
SELECT EXISTS (
    SELECT 1
    FROM resources r
    JOIN resource_permissions p ON p.resource_id = r.id
    JOIN auth_groups g ON g.id = p.group_id
    WHERE r.resource_type = $1
        AND r.resource_key = $2
        AND LOWER(g.name) IN (%s)
        AND (
            p.permission = $3
            OR p.permission = 'admin'
            OR ($3 = 'view' AND p.permission = 'edit')
        )
)
`, strings.Join(placeholders, ", "))

	var exists bool
	if err := r.pool.QueryRow(ctx, sql, args...).Scan(&exists); err != nil {
		return false, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return exists, nil
}

// HasAnyJasperReportPermission mirrors PHP jrs_user_has_any_jasper_report_permission.
func (r *Repository) HasAnyJasperReportPermission(ctx context.Context, permission string, groups []string) (bool, error) {
	return r.HasAnyResourceTypePermission(ctx, ResourceTypeJasperReport, permission, groups)
}

// HasAnyResourceTypePermission reports whether any resource of the given type
// grants the requested permission to one of the groups.
func (r *Repository) HasAnyResourceTypePermission(ctx context.Context, resourceType, permission string, groups []string) (bool, error) {
	if !r.Configured() {
		return false, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	resourceType = strings.TrimSpace(resourceType)
	permission = strings.ToLower(strings.TrimSpace(permission))
	if resourceType == "" || !validPermission(permission) || len(groups) == 0 {
		return false, nil
	}
	args := make([]any, 0, 2+len(groups))
	args = append(args, resourceType, permission)
	placeholders := make([]string, 0, len(groups))
	for i, g := range groups {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+3))
		args = append(args, strings.ToLower(strings.TrimSpace(g)))
	}
	sql := fmt.Sprintf(`
SELECT EXISTS (
    SELECT 1
    FROM resources r
    JOIN resource_permissions p ON p.resource_id = r.id
    JOIN auth_groups g ON g.id = p.group_id
    WHERE r.resource_type = $1
        AND LOWER(g.name) IN (%s)
        AND (
            p.permission = $2
            OR p.permission = 'admin'
            OR ($2 = 'view' AND p.permission = 'edit')
        )
)
`, strings.Join(placeholders, ", "))

	var exists bool
	if err := r.pool.QueryRow(ctx, sql, args...).Scan(&exists); err != nil {
		return false, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return exists, nil
}

// FindResource looks up a resources row by type and key.
func (r *Repository) FindResource(ctx context.Context, resourceType, resourceKey string) (*Resource, error) {
	if !r.Configured() {
		return nil, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	var res Resource
	err := r.pool.QueryRow(ctx, `
SELECT id, resource_type, resource_key, COALESCE(display_name, ''), COALESCE(service, '')
FROM resources
WHERE resource_type = $1 AND resource_key = $2
LIMIT 1
`, strings.TrimSpace(resourceType), strings.TrimSpace(resourceKey)).Scan(
		&res.ID, &res.ResourceType, &res.ResourceKey, &res.DisplayName, &res.Service,
	)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// EffectiveGroupNames mirrors PHP jrs_auth_effective_group_names_for_permission:
// always include implicit Public membership.
func EffectiveGroupNames(groups []string) []string {
	out := make([]string, 0, len(groups)+1)
	hasPublic := false
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		out = append(out, g)
		if strings.EqualFold(g, GroupPublic) {
			hasPublic = true
		}
	}
	if !hasPublic {
		out = append(out, GroupPublic)
	}
	return out
}

func validPermission(p string) bool {
	switch p {
	case PermissionView, PermissionEdit, PermissionAdmin:
		return true
	default:
		return false
	}
}
