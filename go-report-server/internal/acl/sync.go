package acl

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const serviceNameJasper = "jasper-report-service"

// ResourceWriter performs ACL writes used by publish/edit (PHP jrs_sync_report_access_groups)
// and explorer view/dashboard Access Groups.
type ResourceWriter interface {
	SyncReportAccessGroups(ctx context.Context, reportKey, displayName string, groupNames []string, createdBy int64) error
	SyncExplorerViewAccessGroups(ctx context.Context, viewRef, displayName string, groupNames []string, createdBy int64) error
	ListAccessGroups(ctx context.Context) ([]string, error)
	ListResourcePermissionGroups(ctx context.Context, resourceType, resourceKey string) ([]string, error)
	DeleteResource(ctx context.Context, resourceType, resourceKey string) error
}

// SyncReportAccessGroups mirrors PHP jrs_sync_report_access_groups.
//
// It replaces (not appends) report ACL grants so that unchecked groups lose
// access immediately, matching ReplaceFolderPermissions semantics.
func (r *Repository) SyncReportAccessGroups(ctx context.Context, reportKey, displayName string, groupNames []string, createdBy int64) error {
	return r.syncAccessGroups(ctx, ResourceTypeJasperReport, reportKey, displayName, groupNames, createdBy, true)
}

// SyncExplorerViewAccessGroups replaces Access Groups for a saved View/Dashboard.
// viewRef must be the stable "owner/viewKey" string. Empty groupNames defaults to Public,
// matching report publish/edit behavior. Administrators always retain admin.
func (r *Repository) SyncExplorerViewAccessGroups(ctx context.Context, viewRef, displayName string, groupNames []string, createdBy int64) error {
	// ensureService=true matches reports: granting a View/Dashboard Access Group
	// also ensures that group can enter the Report Server / Data Explorer service.
	return r.syncAccessGroups(ctx, ResourceTypeExplorerView, viewRef, displayName, groupNames, createdBy, true)
}

func (r *Repository) syncAccessGroups(ctx context.Context, resourceType, resourceKey, displayName string, groupNames []string, createdBy int64, ensureService bool) error {
	if !r.Configured() {
		return fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	resourceKey = strings.TrimSpace(resourceKey)
	if resourceKey == "" {
		return ErrBadRequest
	}

	names := uniqueNonEmpty(groupNames)
	if len(names) == 0 {
		names = []string{GroupPublic}
	}

	disp := strings.TrimSpace(displayName)
	if disp == "" {
		disp = resourceKey
	}

	resourceID, err := r.registerTypedResource(ctx, resourceType, resourceKey, disp, createdBy)
	if err != nil || resourceID <= 0 {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	defer tx.Rollback(ctx)

	// Clear prior grants so revoked groups no longer authorize view/execute.
	if _, err := tx.Exec(ctx, `
DELETE FROM resource_permissions WHERE resource_id = $1
`, resourceID); err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO resource_permissions (resource_id, group_id, permission)
SELECT $1, g.id, $2
FROM auth_groups g
WHERE LOWER(g.name) = LOWER($3)
ON CONFLICT (resource_id, group_id, permission) DO UPDATE SET permission = EXCLUDED.permission
`, resourceID, PermissionAdmin, GroupAdministrators); err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}

	for _, name := range names {
		if strings.EqualFold(name, GroupAdministrators) {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO resource_permissions (resource_id, group_id, permission)
SELECT $1, g.id, $2
FROM auth_groups g
WHERE LOWER(g.name) = LOWER($3)
ON CONFLICT (resource_id, group_id, permission) DO UPDATE SET permission = EXCLUDED.permission
`, resourceID, PermissionView, name); err != nil {
			return fmt.Errorf("%w: %v", ErrDatabase, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}

	if ensureService {
		for _, name := range names {
			if strings.EqualFold(name, GroupAdministrators) {
				continue
			}
			_ = r.ensureGroupServicePermission(ctx, name, PermissionView)
		}
	}
	return nil
}

// ListAccessGroups mirrors PHP jrs_access_groups (auth_groups, then legacy jasper_access_group).
func (r *Repository) ListAccessGroups(ctx context.Context) ([]string, error) {
	if !r.Configured() {
		return nil, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	rows, err := r.pool.Query(ctx, `SELECT name FROM auth_groups ORDER BY name ASC`)
	if err == nil {
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
		if len(out) > 0 {
			return out, nil
		}
	}

	legacy, err := r.pool.Query(ctx, `SELECT name FROM jasper_access_group ORDER BY name`)
	if err != nil {
		// Missing legacy table is fine — return empty list.
		return []string{}, nil
	}
	defer legacy.Close()
	var out []string
	for legacy.Next() {
		var name string
		if err := legacy.Scan(&name); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
		}
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out, legacy.Err()
}

func (r *Repository) registerResource(ctx context.Context, resourceKey, displayName string, createdBy int64) (int64, error) {
	return r.registerTypedResource(ctx, ResourceTypeJasperReport, resourceKey, displayName, createdBy)
}

func (r *Repository) registerTypedResource(ctx context.Context, resourceType, resourceKey, displayName string, createdBy int64) (int64, error) {
	var createdByArg any
	if createdBy > 0 {
		createdByArg = createdBy
	}
	var id int64
	service := ""
	if resourceType == ResourceTypeJasperReport {
		service = serviceNameJasper
	}
	err := r.pool.QueryRow(ctx, `
INSERT INTO resources (resource_type, resource_key, display_name, service, created_by)
VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5)
ON CONFLICT (resource_type, resource_key) DO UPDATE
SET display_name = COALESCE(EXCLUDED.display_name, resources.display_name),
    service = COALESCE(EXCLUDED.service, resources.service)
RETURNING id
`, resourceType, resourceKey, displayName, service, createdByArg).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return id, nil
}

// ListResourcePermissionGroups returns distinct group names with any permission on the resource.
// Administrators admin grants are included when present.
func (r *Repository) ListResourcePermissionGroups(ctx context.Context, resourceType, resourceKey string) ([]string, error) {
	if !r.Configured() {
		return nil, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	resourceType = strings.TrimSpace(resourceType)
	resourceKey = strings.TrimSpace(resourceKey)
	if resourceType == "" || resourceKey == "" {
		return nil, ErrBadRequest
	}
	rows, err := r.pool.Query(ctx, `
SELECT DISTINCT g.name
FROM resources r
JOIN resource_permissions p ON p.resource_id = r.id
JOIN auth_groups g ON g.id = p.group_id
WHERE r.resource_type = $1 AND r.resource_key = $2
ORDER BY g.name
`, resourceType, resourceKey)
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
	return out, rows.Err()
}

// DeleteResource removes a resources row and its permissions (cascade or explicit).
func (r *Repository) DeleteResource(ctx context.Context, resourceType, resourceKey string) error {
	if !r.Configured() {
		return fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	resourceType = strings.TrimSpace(resourceType)
	resourceKey = strings.TrimSpace(resourceKey)
	if resourceType == "" || resourceKey == "" {
		return ErrBadRequest
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
SELECT id FROM resources WHERE resource_type = $1 AND resource_key = $2 LIMIT 1
`, resourceType, resourceKey).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM resource_permissions WHERE resource_id = $1`, id); err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM resources WHERE id = $1`, id); err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return tx.Commit(ctx)
}

func (r *Repository) grantGroupResourcePermission(ctx context.Context, resourceID int64, groupName, permission string) bool {
	groupName = strings.TrimSpace(groupName)
	permission = strings.ToLower(strings.TrimSpace(permission))
	if resourceID <= 0 || groupName == "" || !validPermission(permission) {
		return false
	}
	var gid int64
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM auth_groups WHERE LOWER(name) = LOWER($1) LIMIT 1`,
		groupName,
	).Scan(&gid)
	if err != nil {
		return false
	}
	_, err = r.pool.Exec(ctx, `
INSERT INTO resource_permissions (resource_id, group_id, permission)
VALUES ($1, $2, $3)
ON CONFLICT (resource_id, group_id, permission) DO UPDATE SET permission = EXCLUDED.permission
`, resourceID, gid, permission)
	return err == nil
}

func (r *Repository) ensureGroupServicePermission(ctx context.Context, groupName, permission string) error {
	if err := r.ensurePlatformServiceResources(ctx); err != nil {
		return err
	}
	key, _, _ := r.platformService()
	var id int64
	err := r.pool.QueryRow(ctx, `
SELECT id FROM resources WHERE resource_type = $1 AND resource_key = $2 LIMIT 1
`, ResourceTypeService, key).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	_ = r.grantGroupResourcePermission(ctx, id, groupName, permission)
	return nil
}

func (r *Repository) platformService() (key, display, service string) {
	key = ServiceKeyJasperReports
	display = "AcuGIS Report Server"
	service = serviceNameJasper
	if r != nil {
		if strings.TrimSpace(r.platformServiceKey) != "" {
			key = r.platformServiceKey
		}
		if strings.TrimSpace(r.platformServiceDisplay) != "" {
			display = r.platformServiceDisplay
		}
		if strings.TrimSpace(r.platformServiceName) != "" {
			service = r.platformServiceName
		}
	}
	return key, display, service
}

func (r *Repository) ensurePlatformServiceResources(ctx context.Context) error {
	key, display, service := r.platformService()
	var id int64
	err := r.pool.QueryRow(ctx, `
INSERT INTO resources (resource_type, resource_key, display_name, service, created_by)
VALUES ($1, $2, $3, $4, NULL)
ON CONFLICT (resource_type, resource_key) DO UPDATE
SET display_name = COALESCE(EXCLUDED.display_name, resources.display_name)
RETURNING id
`, ResourceTypeService, key, display, service).Scan(&id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	_ = r.grantGroupResourcePermission(ctx, id, GroupAdministrators, PermissionAdmin)
	// QCarta seeds service ACL from Central. Do not auto-grant Public/Editors
	// on report_runtime the way standalone Report Server does for jasper_reports.
	if key == ServiceKeyJasperReports {
		_ = r.grantGroupResourcePermission(ctx, id, GroupEditors, PermissionEdit)
		_ = r.grantGroupResourcePermission(ctx, id, GroupPublic, PermissionView)
	}
	return nil
}

func uniqueNonEmpty(in []string) []string {
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
