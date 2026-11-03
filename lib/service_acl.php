<?php
declare(strict_types=1);

/**
 * Service-level ACL (resources.resource_type = service) for AcuGIS Report Server.
 *
 * Loaded from bootstrap.php immediately after lib/auth.php so session helpers
 * and jrs_has_permission() are available. Keeping this in a dedicated file
 * avoids "undefined function" when deployments update entry scripts but omit
 * merging the full auth.php diff.
 */

if (!defined('JRS_RESOURCE_TYPE_SERVICE')) {
    define('JRS_RESOURCE_TYPE_SERVICE', 'service');
}
if (!defined('JRS_PLATFORM_SERVICE_NAME')) {
    define('JRS_PLATFORM_SERVICE_NAME', 'platform');
}

// Skip if a monolithic auth.php already defined these (avoids redeclare fatals on mixed deploys).
if (function_exists('jrs_require_service_access') && function_exists('jrs_ensure_platform_service_resources')) {
    return;
}

function jrs_is_valid_service_key(string $value): bool
{
    return preg_match('/^[a-z0-9_]+$/', $value) === 1;
}

/**
 * @return array{id:int}|null
 */
function jrs_register_service_resource($conn, string $serviceKey, string $displayName, int $createdBy): ?array
{
    if (!$conn || !jrs_is_valid_service_key($serviceKey)) {
        return null;
    }
    $sql = <<<'SQL'
INSERT INTO resources (resource_type, resource_key, display_name, service, created_by)
VALUES ($1, $2, NULLIF($3, ''), $4, $5)
ON CONFLICT (resource_type, resource_key) DO UPDATE
SET display_name = COALESCE(EXCLUDED.display_name, resources.display_name),
    service = COALESCE(EXCLUDED.service, resources.service)
RETURNING id
SQL;
    $res = @pg_query_params($conn, $sql, [
        JRS_RESOURCE_TYPE_SERVICE,
        $serviceKey,
        trim($displayName) !== '' ? trim($displayName) : $serviceKey,
        JRS_PLATFORM_SERVICE_NAME,
        $createdBy > 0 ? $createdBy : null,
    ]);
    if ($res === false) {
        return null;
    }
    $row = pg_fetch_assoc($res);
    pg_free_result($res);
    if ($row === false) {
        return null;
    }

    return ['id' => (int) ($row['id'] ?? 0)];
}

function jrs_grant_administrators_admin_on_resource($conn, int $resourceId): void
{
    if (!$conn || $resourceId <= 0) {
        return;
    }
    $gidRes = @pg_query_params($conn, 'SELECT id FROM auth_groups WHERE LOWER(name) = LOWER($1) LIMIT 1', ['Administrators']);
    if ($gidRes === false) {
        return;
    }
    $gRow = pg_fetch_assoc($gidRes);
    pg_free_result($gidRes);
    if ($gRow === false) {
        return;
    }
    $gid = (int) ($gRow['id'] ?? 0);
    if ($gid <= 0) {
        return;
    }
    @pg_query_params(
        $conn,
        <<<'SQL'
INSERT INTO resource_permissions (resource_id, group_id, permission)
VALUES ($1, $2, 'admin')
ON CONFLICT (resource_id, group_id, permission) DO UPDATE SET permission = EXCLUDED.permission
SQL
        ,
        [$resourceId, $gid]
    );
}

function jrs_service_resource_exists($conn, string $serviceKey): bool
{
    if (!$conn || !jrs_is_valid_service_key($serviceKey)) {
        return false;
    }
    $res = @pg_query_params(
        $conn,
        'SELECT 1 FROM resources WHERE resource_type = $1 AND resource_key = $2 LIMIT 1',
        [JRS_RESOURCE_TYPE_SERVICE, $serviceKey]
    );
    if ($res === false) {
        return false;
    }
    $exists = pg_num_rows($res) > 0;
    pg_free_result($res);

    return $exists;
}

function jrs_ensure_platform_service_resources($conn): void
{
    if (!$conn) {
        return;
    }
    $defs = [
        ['jasper_reports', 'AcuGIS Report Server'],
    ];
    foreach ($defs as $pair) {
        $key = (string) $pair[0];
        $title = (string) $pair[1];
        $existed = jrs_service_resource_exists($conn, $key);
        $reg = jrs_register_service_resource($conn, $key, $title, 0);
        if ($reg === null || (int) ($reg['id'] ?? 0) <= 0) {
            continue;
        }
        $rid = (int) $reg['id'];
        // Always ensure built-in service ACL so report consumers can open the catalog.
        if (function_exists('jrs_grant_group_resource_permission')) {
            jrs_grant_group_resource_permission($conn, $rid, 'Administrators', 'admin');
            jrs_grant_group_resource_permission($conn, $rid, 'Editors', 'edit');
            jrs_grant_group_resource_permission($conn, $rid, 'Public', 'view');
        } elseif (!$existed) {
            jrs_grant_administrators_admin_on_resource($conn, $rid);
        }
    }
}

/**
 * Require ACL on resource_type=service (call after jrs_require_login with the same $conn).
 */
function jrs_require_service_access($conn, string $serviceKey, string $permission = 'view'): void
{
    if (!jrs_auth_is_enabled()) {
        return;
    }
    if (!$conn) {
        jrs_emit_permission_denied();
        exit;
    }
    jrs_ensure_platform_service_resources($conn);
    $user = jrs_get_current_user();
    $uid = (int) ($user['id'] ?? 0);
    if ($uid <= 0) {
        jrs_require_login($conn);
        $user = jrs_get_current_user();
        $uid = (int) ($user['id'] ?? 0);
    }
    if (($user['is_admin'] ?? false) === true || jrs_auth_groups_include_admin((array) ($user['groups'] ?? []))) {
        return;
    }
    if (jrs_has_permission($conn, $uid, JRS_RESOURCE_TYPE_SERVICE, $serviceKey, $permission)) {
        return;
    }
    // Report consumers may have report ACL without an explicit service grant yet.
    if (
        $permission === 'view'
        && $serviceKey === 'jasper_reports'
        && function_exists('jrs_user_has_any_jasper_report_permission')
        && jrs_user_has_any_jasper_report_permission($conn, $uid, 'view')
    ) {
        return;
    }
    jrs_emit_permission_denied();
    exit;
}
