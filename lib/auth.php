<?php
declare(strict_types=1);

/**
 * Local stand-alone authentication and authorization for AcuGIS Report Server.
 *
 * Cookie `jrs_session` holds the RAW session string. Table `dashboard_sessions.token_hash`
 * stores ONLY SHA-256(raw), lowercase hex. Validators MUST hash the cookie before any SQL
 * on token_hash — never compare the raw cookie to the DB column.
 *
 * Auth tables live in the application database (see jrs_app_db()). Set JRS_AUTH_DISABLED=1
 * to bypass auth for local smoke tests (synthetic local-dev admin user).
 */

const JRS_AUTH_SESSION_COOKIE = 'jrs_session';
const JRS_AUTH_SESSION_TTL_SECONDS = 604800; // 7 days
const JRS_RESOURCE_TYPE_JASPER_REPORT = 'jasper_report';
const JRS_SERVICE_NAME = 'jasper-report-service';

function jrs_auth_debug_enabled(): bool
{
    return jrs_truthy_env('JRS_AUTH_DEBUG') || jrs_truthy_env('JRS_AUTH_LOGIN_TRACE');
}

function jrs_auth_debug_log(string $message): void
{
    if (!jrs_auth_debug_enabled()) {
        return;
    }
    $uri = (string) ($_SERVER['REQUEST_URI'] ?? '');
    error_log('[jrs-auth] ' . $message . ' uri=' . $uri);
}

/** Non-empty value from getenv or $_SERVER (Apache SetEnv is often only in $_SERVER). */
function jrs_env_string(string $key): string
{
    $v = getenv($key);
    if (is_string($v) && trim($v) !== '') {
        return trim($v);
    }
    if (isset($_SERVER[$key]) && is_string($_SERVER[$key]) && trim($_SERVER[$key]) !== '') {
        return trim($_SERVER[$key]);
    }
    return '';
}

function jrs_truthy_env(string $key): bool
{
    $v = strtolower(jrs_env_string($key));

    return in_array($v, ['1', 'true', 'yes', 'on'], true);
}

function jrs_auth_is_enabled(): bool
{
    return !jrs_truthy_env('JRS_AUTH_DISABLED');
}

/**
 * Session token hash for dashboard_sessions.token_hash (SHA-256, lowercase hex).
 */
function jrs_auth_session_token_hash(string $rawCookieValue): string
{
    return hash('sha256', $rawCookieValue, false);
}

/** @deprecated Use jrs_auth_session_token_hash(); kept for existing call sites. */
function jrs_auth_central_session_token_hash(string $rawCookieValue): string
{
    return jrs_auth_session_token_hash($rawCookieValue);
}

function jrs_auth_raw_session_token(): string
{
    $c = $_COOKIE[JRS_AUTH_SESSION_COOKIE] ?? null;
    return is_string($c) ? trim($c) : '';
}

/**
 * @return resource|false|null null when auth is disabled; false on connection failure
 */
function jrs_auth_open_central_db()
{
    if (!jrs_auth_is_enabled()) {
        return null;
    }
    $c = jrs_app_db();
    if (!$c) {
        return false;
    }
    return $c;
}

function jrs_auth_require_central_db()
{
    $c = jrs_app_db();
    if (!$c) {
        if (jrs_auth_is_api_request()) {
            http_response_code(503);
            header('Content-Type: application/json; charset=utf-8');
            echo json_encode(['error' => 'application database unavailable'], JSON_UNESCAPED_SLASHES);
        } else {
            http_response_code(503);
            header('Content-Type: text/plain; charset=utf-8');
            echo 'Application database unavailable.';
        }
        exit;
    }
    return $c;
}

/** @return resource|false Compatibility alias — auth lives in the app DB. */
function jrs_auth_central_db()
{
    return jrs_app_db();
}

function jrs_auth_random_session_token(): string
{
    return rtrim(strtr(base64_encode(random_bytes(32)), '+/', '-_'), '=');
}

function jrs_auth_set_session_cookie(string $token, int $expiresAt): void
{
    $secure = (!empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off')
        || (isset($_SERVER['HTTP_X_FORWARDED_PROTO']) && strtolower((string) $_SERVER['HTTP_X_FORWARDED_PROTO']) === 'https');
    setcookie(JRS_AUTH_SESSION_COOKIE, $token, [
        'expires' => $expiresAt,
        'path' => '/',
        'httponly' => true,
        'samesite' => 'Lax',
        'secure' => $secure,
    ]);
}

function jrs_auth_clear_session_cookie(): void
{
    $secure = (!empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off')
        || (isset($_SERVER['HTTP_X_FORWARDED_PROTO']) && strtolower((string) $_SERVER['HTTP_X_FORWARDED_PROTO']) === 'https');
    setcookie(JRS_AUTH_SESSION_COOKIE, '', [
        'expires' => time() - 3600,
        'path' => '/',
        'httponly' => true,
        'samesite' => 'Lax',
        'secure' => $secure,
    ]);
}

function jrs_auth_create_session(string $username): void
{
    $conn = jrs_app_db();
    if (!$conn) {
        return;
    }
    $username = trim($username);
    if ($username === '') {
        return;
    }
    $token = jrs_auth_random_session_token();
    $tokenHash = jrs_auth_session_token_hash($token);
    $expiresAt = time() + JRS_AUTH_SESSION_TTL_SECONDS;
    @pg_query_params(
        $conn,
        'INSERT INTO dashboard_sessions (token_hash, username, expires_at) VALUES ($1, $2, to_timestamp($3))',
        [$tokenHash, $username, (string) $expiresAt]
    );
    jrs_auth_set_session_cookie($token, $expiresAt);
}

function jrs_auth_login(string $username, string $password): bool
{
    if (!jrs_auth_is_enabled()) {
        return false;
    }
    $conn = jrs_app_db();
    if (!$conn) {
        return false;
    }
    $username = trim($username);
    if ($username === '' || $password === '') {
        return false;
    }
    $res = @pg_query_params(
        $conn,
        'SELECT password_hash, enabled FROM dashboard_users WHERE username = $1 LIMIT 1',
        [$username]
    );
    if ($res === false) {
        return false;
    }
    $row = pg_fetch_assoc($res);
    pg_free_result($res);
    if ($row === false) {
        return false;
    }
    if (!jrs_pg_is_true($row['enabled'] ?? false)) {
        return false;
    }
    $hash = (string) ($row['password_hash'] ?? '');
    if ($hash === '' || !password_verify($password, $hash)) {
        return false;
    }
    jrs_auth_create_session($username);
    return true;
}

function jrs_auth_logout(): void
{
    $raw = jrs_auth_raw_session_token();
    if ($raw !== '') {
        $conn = jrs_app_db();
        if ($conn) {
            $tokenHash = jrs_auth_session_token_hash($raw);
            @pg_query_params($conn, 'DELETE FROM dashboard_sessions WHERE token_hash = $1', [$tokenHash]);
        }
    }
    jrs_auth_clear_session_cookie();
}

/**
 * @param string $sessionTokenSha256Hex 64-char lowercase hex; never pass the raw cookie
 *
 * @return array{id:int,username:string,email:string,is_admin:bool,groups:list<string>}|null
 */
function jrs_auth_lookup_user_by_session($conn, string $sessionTokenSha256Hex): ?array
{
    $sql = <<<'SQL'
SELECT u.id,
    u.username,
    COALESCE(u.email, '') AS email,
    COALESCE(u.is_admin, FALSE) AS is_admin,
    COALESCE(jsonb_agg(g.name ORDER BY g.name) FILTER (WHERE g.name IS NOT NULL), u.groups, '[]'::jsonb) AS groups_json
FROM dashboard_sessions s
JOIN dashboard_users u ON u.username = s.username
LEFT JOIN auth_group_members gm ON gm.user_id = u.id
LEFT JOIN auth_groups g ON g.id = gm.group_id
WHERE s.token_hash = $1 AND s.expires_at > NOW() AND COALESCE(u.enabled, TRUE) = TRUE
GROUP BY u.id, u.username, u.email, u.groups, u.is_admin, u.enabled, u.created_at
SQL;
    $res = @pg_query_params($conn, $sql, [$sessionTokenSha256Hex]);
    if ($res === false) {
        if (jrs_auth_debug_enabled()) {
            jrs_auth_debug_log('session SQL error: ' . (string) @pg_last_error($conn));
        }
        return null;
    }
    $row = pg_fetch_assoc($res);
    pg_free_result($res);
    if ($row === false) {
        jrs_auth_debug_log('session lookup: no matching dashboard_sessions row (cookie present)');
        return null;
    }
    $groupsRaw = $row['groups_json'] ?? '[]';
    $groups = [];
    if (is_string($groupsRaw)) {
        $decoded = json_decode($groupsRaw, true);
        if (is_array($decoded)) {
            foreach ($decoded as $g) {
                if (is_string($g) && $g !== '') {
                    $groups[] = $g;
                }
            }
        }
    }
    return [
        'id' => (int) ($row['id'] ?? 0),
        'username' => (string) ($row['username'] ?? ''),
        'email' => (string) ($row['email'] ?? ''),
        'is_admin' => jrs_pg_is_true($row['is_admin'] ?? false),
        'groups' => $groups,
    ];
}

function jrs_auth_groups_include_admin(array $groups): bool
{
    foreach ($groups as $group) {
        $n = strtolower(trim((string) $group));
        if (in_array($n, ['administrator', 'administrators', 'admin', 'admins', 'dashboard-admin'], true)) {
            return true;
        }
    }
    return false;
}

/**
 * @return list<string>
 */
function jrs_auth_user_group_names($conn, int $userId): array
{
    $res = @pg_query_params(
        $conn,
        'SELECT g.name FROM auth_group_members gm JOIN auth_groups g ON g.id = gm.group_id WHERE gm.user_id = $1 ORDER BY g.name ASC',
        [$userId]
    );
    if ($res === false) {
        return [];
    }
    $out = [];
    while ($row = pg_fetch_assoc($res)) {
        $n = trim((string) ($row['name'] ?? ''));
        if ($n !== '') {
            $out[] = $n;
        }
    }
    pg_free_result($res);
    return $out;
}

/**
 * All platform group names from auth_groups (same catalog as Users & Groups admin).
 *
 * @return list<string>
 */
function jrs_auth_all_group_names($conn): array
{
    if (!$conn) {
        return [];
    }
    $res = @pg_query($conn, 'SELECT name FROM auth_groups ORDER BY name ASC');
    if ($res === false) {
        return [];
    }
    $out = [];
    while ($row = pg_fetch_assoc($res)) {
        $n = trim((string) ($row['name'] ?? ''));
        if ($n !== '') {
            $out[] = $n;
        }
    }
    pg_free_result($res);
    return $out;
}

/**
 * Effective group names for permission checks.
 *
 * @param list<string> $groups
 * @return list<string>
 */
function jrs_auth_effective_group_names_for_permission(array $groups): array
{
    $out = $groups;
    $hasPublic = false;
    foreach ($out as $g) {
        if (strcasecmp(trim($g), 'Public') === 0) {
            $hasPublic = true;
            break;
        }
    }
    if (!$hasPublic) {
        $out[] = 'Public';
    }
    return $out;
}

function jrs_auth_valid_permission(string $permission): bool
{
    $p = strtolower(trim($permission));
    return in_array($p, ['view', 'edit', 'admin'], true);
}

function jrs_has_permission($conn, int $userId, string $resourceType, string $resourceKey, string $permission): bool
{
    if (!jrs_auth_is_enabled()) {
        return true;
    }
    if (!$conn) {
        return false;
    }
    $groups = jrs_auth_user_group_names($conn, $userId);
    if (jrs_auth_groups_include_admin($groups)) {
        return true;
    }
    $permission = strtolower(trim($permission));
    if (!jrs_auth_valid_permission($permission)) {
        return false;
    }
    $groups = jrs_auth_effective_group_names_for_permission($groups);
    if ($groups === []) {
        return false;
    }
    $placeholders = [];
    $params = [trim($resourceType), trim($resourceKey), $permission];
    $i = 4;
    foreach ($groups as $g) {
        $placeholders[] = '$' . $i;
        $params[] = $g;
        $i++;
    }
    $inList = implode(', ', $placeholders);
    $sql = 'SELECT EXISTS (
    SELECT 1
    FROM resources r
    JOIN resource_permissions p ON p.resource_id = r.id
    JOIN auth_groups g ON g.id = p.group_id
    WHERE r.resource_type = $1
        AND r.resource_key = $2
        AND g.name IN (' . $inList . ')
        AND (
            p.permission = $3
            OR p.permission = \'admin\'
            OR ($3 = \'view\' AND p.permission = \'edit\')
        )
)';
    $res = @pg_query_params($conn, $sql, $params);
    if ($res === false) {
        return false;
    }
    $row = pg_fetch_row($res);
    pg_free_result($res);
    return isset($row[0]) && $row[0] === 't';
}

function jrs_emit_permission_denied(): void
{
    if (jrs_auth_is_api_request()) {
        http_response_code(403);
        header('Content-Type: application/json; charset=utf-8');
        echo json_encode(['error' => 'Permission denied'], JSON_UNESCAPED_SLASHES);
    } else {
        http_response_code(403);
        header('Content-Type: text/html; charset=utf-8');
        echo '<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Permission denied</title></head><body><p>Permission denied</p></body></html>';
    }
}

function jrs_jasper_resource_exists($conn, string $resourceKey): bool
{
    if (!$conn || !jrs_is_valid_report_key($resourceKey)) {
        return false;
    }
    $res = @pg_query_params(
        $conn,
        'SELECT 1 FROM resources WHERE resource_type = $1 AND resource_key = $2 LIMIT 1',
        [JRS_RESOURCE_TYPE_JASPER_REPORT, $resourceKey]
    );
    if ($res === false) {
        return false;
    }
    $exists = pg_num_rows($res) > 0;
    pg_free_result($res);
    return $exists;
}

/**
 * Register or update a resource row (upsert).
 *
 * @return array{id:int}|null
 */
function jrs_register_resource($conn, string $resourceKey, string $displayName, int $createdBy): ?array
{
    if (!$conn || !jrs_is_valid_report_key($resourceKey)) {
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
        JRS_RESOURCE_TYPE_JASPER_REPORT,
        $resourceKey,
        trim($displayName) !== '' ? trim($displayName) : $resourceKey,
        JRS_SERVICE_NAME,
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

/**
 * Default resource_permissions for a new report.
 */
function jrs_grant_default_report_permissions($conn, int $resourceId): void
{
    if (!$conn || $resourceId <= 0) {
        return;
    }
    $defaults = [
        'Administrators' => 'admin',
        'Editors' => 'edit',
        'Public' => 'view',
    ];
    foreach ($defaults as $groupName => $perm) {
        jrs_grant_group_resource_permission($conn, $resourceId, $groupName, $perm);
    }
}

/**
 * Upsert one group permission on a resource (by group name).
 */
function jrs_grant_group_resource_permission($conn, int $resourceId, string $groupName, string $permission): bool
{
    if (!$conn || $resourceId <= 0) {
        return false;
    }
    $groupName = trim($groupName);
    $permission = strtolower(trim($permission));
    if ($groupName === '' || !jrs_auth_valid_permission($permission)) {
        return false;
    }
    $gidRes = @pg_query_params($conn, 'SELECT id FROM auth_groups WHERE LOWER(name) = LOWER($1) LIMIT 1', [$groupName]);
    if ($gidRes === false) {
        return false;
    }
    $gRow = pg_fetch_assoc($gidRes);
    pg_free_result($gidRes);
    if ($gRow === false) {
        return false;
    }
    $gid = (int) ($gRow['id'] ?? 0);
    if ($gid <= 0) {
        return false;
    }
    $ok = @pg_query_params(
        $conn,
        <<<'SQL'
INSERT INTO resource_permissions (resource_id, group_id, permission)
VALUES ($1, $2, $3)
ON CONFLICT (resource_id, group_id, permission) DO UPDATE SET permission = EXCLUDED.permission
SQL
        ,
        [$resourceId, $gid, $permission]
    );

    return $ok !== false;
}

/**
 * Ensure a group can enter the jasper_reports service (catalog / schedules UI).
 */
function jrs_ensure_group_service_permission($conn, string $groupName, string $permission = 'view'): void
{
    if (!$conn) {
        return;
    }
    jrs_ensure_platform_service_resources($conn);
    $res = @pg_query_params(
        $conn,
        'SELECT id FROM resources WHERE resource_type = $1 AND resource_key = $2 LIMIT 1',
        [JRS_RESOURCE_TYPE_SERVICE, 'jasper_reports']
    );
    if ($res === false) {
        return;
    }
    $row = pg_fetch_assoc($res);
    pg_free_result($res);
    if ($row === false) {
        return;
    }
    jrs_grant_group_resource_permission($conn, (int) ($row['id'] ?? 0), $groupName, $permission);
}

/**
 * Apply Access Groups from report JSON to resource_permissions and open service access for those groups.
 *
 * @param list<string> $groupNames
 */
function jrs_sync_report_access_groups($conn, string $reportKey, string $displayName, array $groupNames, int $createdBy = 0): void
{
    if (!$conn || !jrs_is_valid_report_key($reportKey)) {
        return;
    }
    $names = [];
    foreach ($groupNames as $g) {
        $g = trim((string) $g);
        if ($g !== '') {
            $names[] = $g;
        }
    }
    $names = array_values(array_unique($names));
    if ($names === []) {
        $names = ['Public'];
    }

    $reg = jrs_register_resource($conn, $reportKey, $displayName !== '' ? $displayName : $reportKey, $createdBy);
    if ($reg === null || (int) ($reg['id'] ?? 0) <= 0) {
        return;
    }
    $resourceId = (int) $reg['id'];

    // Replace (do not append) so revoked groups lose access immediately.
    // Matches Go SyncReportAccessGroups / folder ReplaceFolderPermissions.
    @pg_query_params($conn, 'DELETE FROM resource_permissions WHERE resource_id = $1', [$resourceId]);

    jrs_grant_group_resource_permission($conn, $resourceId, 'Administrators', 'admin');

    foreach ($names as $name) {
        if (strcasecmp($name, 'Administrators') === 0) {
            continue;
        }
        jrs_grant_group_resource_permission($conn, $resourceId, $name, 'view');
        jrs_ensure_group_service_permission($conn, $name, 'view');
    }
}

/**
 * True when the user has view (or higher) on any jasper_report resource.
 */
function jrs_user_has_any_jasper_report_permission($conn, int $userId, string $permission = 'view'): bool
{
    if (!$conn || $userId < 0) {
        return false;
    }
    $groups = jrs_auth_user_group_names($conn, $userId);
    if (jrs_auth_groups_include_admin($groups)) {
        return true;
    }
    $permission = strtolower(trim($permission));
    if (!jrs_auth_valid_permission($permission)) {
        return false;
    }
    $groups = jrs_auth_effective_group_names_for_permission($groups);
    if ($groups === []) {
        return false;
    }
    $placeholders = [];
    $params = [JRS_RESOURCE_TYPE_JASPER_REPORT, $permission];
    $i = 3;
    foreach ($groups as $g) {
        $placeholders[] = '$' . $i;
        $params[] = $g;
        $i++;
    }
    $inList = implode(', ', $placeholders);
    $sql = 'SELECT EXISTS (
    SELECT 1
    FROM resources r
    JOIN resource_permissions p ON p.resource_id = r.id
    JOIN auth_groups g ON g.id = p.group_id
    WHERE r.resource_type = $1
        AND g.name IN (' . $inList . ')
        AND (
            p.permission = $2
            OR p.permission = \'admin\'
            OR ($2 = \'view\' AND p.permission = \'edit\')
        )
)';
    $res = @pg_query_params($conn, $sql, $params);
    if ($res === false) {
        return false;
    }
    $row = pg_fetch_row($res);
    pg_free_result($res);

    return isset($row[0]) && $row[0] === 't';
}

function jrs_auth_is_api_request(): bool
{
    $path = (string) (parse_url((string) ($_SERVER['REQUEST_URI'] ?? ''), PHP_URL_PATH) ?: '');
    if (str_contains($path, '/api/')) {
        return true;
    }
    $script = basename((string) ($_SERVER['SCRIPT_NAME'] ?? ''));
    if (in_array($script, ['get_report_config.php', 'serve_report_definition.php'], true)) {
        return true;
    }
    $accept = strtolower((string) ($_SERVER['HTTP_ACCEPT'] ?? ''));
    return str_contains($accept, 'application/json');
}

function jrs_auth_login_url_with_next(): string
{
    $base = (string) jrs_config('login_url', '/acugis-report-server/login.php');
    $next = (string) ($_SERVER['REQUEST_URI'] ?? '/');
    if ($next === '') {
        $next = '/';
    }
    $sep = str_contains($base, '?') ? '&' : '?';
    return $base . $sep . 'next=' . rawurlencode($next);
}

/** @var ?array<string, mixed> */
$GLOBALS['JRS_AUTH_USER'] = null;

/**
 * @return array<string, mixed>
 */
function jrs_get_current_user(): array
{
    if (isset($GLOBALS['JRS_AUTH_USER']) && is_array($GLOBALS['JRS_AUTH_USER'])) {
        return $GLOBALS['JRS_AUTH_USER'];
    }
    return [];
}

/**
 * Platform / catalog admin. Used for manual resource sync API and debug pages.
 */
function jrs_require_platform_admin($conn): void
{
    if (!jrs_auth_is_enabled()) {
        return;
    }
    jrs_require_login($conn);
    $user = jrs_get_current_user();
    if (($user['is_admin'] ?? false) === true || jrs_auth_groups_include_admin((array) ($user['groups'] ?? []))) {
        return;
    }
    if (jrs_auth_is_api_request()) {
        http_response_code(403);
        header('Content-Type: application/json; charset=utf-8');
        echo json_encode(['error' => 'platform administrator required'], JSON_UNESCAPED_SLASHES);
    } else {
        http_response_code(403);
        header('Content-Type: text/plain; charset=utf-8');
        echo 'Platform administrator required.';
    }
    exit;
}

/**
 * Read-only session probe for /debug/session.php (enable with JRS_AUTH_DEBUG=1).
 *
 * @return array<string, mixed>
 */
function jrs_auth_session_debug_payload(): array
{
    $out = [
        'service' => JRS_SERVICE_NAME,
        'auth_enabled' => jrs_auth_is_enabled(),
        'cookie_name' => JRS_AUTH_SESSION_COOKIE,
        'cookie_present' => jrs_auth_raw_session_token() !== '',
        'token_len' => strlen(jrs_auth_raw_session_token()),
        'app_db' => 'skipped',
        'session_lookup_ok' => false,
        'authenticated' => false,
        'user' => null,
        'groups' => [],
    ];
    if (!jrs_auth_is_enabled()) {
        $out['app_db'] = 'auth_disabled';

        return $out;
    }
    $conn = jrs_app_db();
    if (!$conn) {
        $out['app_db'] = 'unreachable';

        return $out;
    }
    $out['app_db'] = 'ok';
    $raw = jrs_auth_raw_session_token();
    if ($raw === '') {
        return $out;
    }
    $tokenHashHex = jrs_auth_session_token_hash($raw);
    $u = jrs_auth_lookup_user_by_session($conn, $tokenHashHex);
    if ($u !== null && (int) ($u['id'] ?? 0) > 0) {
        $out['session_lookup_ok'] = true;
        $out['authenticated'] = true;
        $out['user'] = [
            'id' => (int) $u['id'],
            'username' => (string) ($u['username'] ?? ''),
            'email' => (string) ($u['email'] ?? ''),
            'is_admin' => (bool) ($u['is_admin'] ?? false),
        ];
        $out['groups'] = array_values((array) ($u['groups'] ?? []));
    }

    return $out;
}

function jrs_require_login($conn = null)
{
    if (!jrs_auth_is_enabled()) {
        $GLOBALS['JRS_AUTH_USER'] = [
            'id' => 0,
            'username' => 'local-dev',
            'email' => '',
            'is_admin' => true,
            'groups' => [],
        ];
        return;
    }
    $already = jrs_get_current_user();
    if ((int) ($already['id'] ?? 0) > 0) {
        jrs_auth_debug_log('require_login: already authenticated user_id=' . (string) (int) $already['id'] . ' username=' . ($already['username'] ?? ''));

        return;
    }
    if ($conn === null) {
        $conn = jrs_auth_require_central_db();
    }
    $raw = jrs_auth_raw_session_token();
    jrs_auth_debug_log('require_login: cookie_present=' . ($raw !== '' ? '1' : '0') . ' token_len=' . (string) strlen($raw) . ' cookie_name=' . JRS_AUTH_SESSION_COOKIE);
    if ($raw === '') {
        jrs_auth_debug_log('require_login: decision=redirect reason=no_cookie');
        if (jrs_auth_is_api_request()) {
            http_response_code(401);
            header('Content-Type: application/json; charset=utf-8');
            echo json_encode(['error' => 'authentication required'], JSON_UNESCAPED_SLASHES);
        } else {
            header('Location: ' . jrs_auth_login_url_with_next(), true, 303);
        }
        exit;
    }
    $tokenHashHex = jrs_auth_session_token_hash($raw);
    jrs_auth_debug_log('require_login: token_hash_prefix=' . substr($tokenHashHex, 0, 12) . '... (sha256 hex, full len=' . (string) strlen($tokenHashHex) . ')');
    $user = jrs_auth_lookup_user_by_session($conn, $tokenHashHex);
    $sessionOk = $user !== null && (int) ($user['id'] ?? 0) > 0;
    jrs_auth_debug_log('require_login: session_row_found=' . ($sessionOk ? '1' : '0') . ' user_found=' . ($sessionOk ? '1' : '0'));
    if (!$sessionOk) {
        jrs_auth_debug_log('require_login: decision=redirect reason=session_lookup_failed');
        if (jrs_auth_is_api_request()) {
            http_response_code(401);
            header('Content-Type: application/json; charset=utf-8');
            echo json_encode(['error' => 'authentication required'], JSON_UNESCAPED_SLASHES);
        } else {
            header('Location: ' . jrs_auth_login_url_with_next(), true, 303);
        }
        exit;
    }
    jrs_auth_debug_log('require_login: decision=ok username=' . ($user['username'] ?? '') . ' user_id=' . (string) (int) ($user['id'] ?? 0));
    $GLOBALS['JRS_AUTH_USER'] = $user;
}

/**
 * Load session user into $GLOBALS['JRS_AUTH_USER'] when a valid cookie exists; never redirects.
 */
function jrs_auth_try_refresh_current_user($conn): void
{
    if (!jrs_auth_is_enabled() || !$conn) {
        return;
    }
    $already = jrs_get_current_user();
    if ((int) ($already['id'] ?? 0) > 0) {
        return;
    }
    $raw = jrs_auth_raw_session_token();
    if ($raw === '') {
        return;
    }
    $tokenHashHex = jrs_auth_session_token_hash($raw);
    $user = jrs_auth_lookup_user_by_session($conn, $tokenHashHex);
    if ($user !== null && (int) ($user['id'] ?? 0) > 0) {
        $GLOBALS['JRS_AUTH_USER'] = $user;
    }
}

/**
 * Enforce resource_type=jasper_report only (not jasper_reports service ACL).
 * Anonymous requests use user id 0; jrs_has_permission includes implicit Public group membership.
 */
function jrs_require_report_runtime_permission($conn, string $reportKey, string $permission): void
{
    if (!jrs_auth_is_enabled()) {
        return;
    }
    if (!$conn) {
        if (jrs_auth_is_api_request()) {
            http_response_code(503);
            header('Content-Type: application/json; charset=utf-8');
            echo json_encode(['error' => 'application database unavailable'], JSON_UNESCAPED_SLASHES);
        } else {
            http_response_code(503);
            header('Content-Type: text/plain; charset=utf-8');
            echo 'Application database unavailable.';
        }
        exit;
    }
    jrs_auth_try_refresh_current_user($conn);
    $user = jrs_get_current_user();
    $uid = (int) ($user['id'] ?? 0);
    if (($user['is_admin'] ?? false) === true || jrs_auth_groups_include_admin((array) ($user['groups'] ?? []))) {
        return;
    }
    if (!jrs_has_permission($conn, $uid, JRS_RESOURCE_TYPE_JASPER_REPORT, $reportKey, $permission)) {
        jrs_emit_permission_denied();
        exit;
    }
}

function jrs_require_permission($conn, string $reportKey, string $permission): void
{
    if (!jrs_auth_is_enabled()) {
        return;
    }
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
    if (!jrs_has_permission($conn, $uid, JRS_RESOURCE_TYPE_JASPER_REPORT, $reportKey, $permission)) {
        jrs_emit_permission_denied();
        exit;
    }
}

/**
 * Reports the user may see in catalog / discovery (view or higher).
 *
 * @param list<array<string, mixed>> $reports
 * @return list<array<string, mixed>>
 */
function jrs_filter_reports_for_user($conn, array $reports): array
{
    if (!jrs_auth_is_enabled()) {
        return $reports;
    }
    if (!$conn) {
        return [];
    }
    jrs_auth_try_refresh_current_user($conn);
    $user = jrs_get_current_user();
    $uid = (int) ($user['id'] ?? 0);
    if (($user['is_admin'] ?? false) === true || jrs_auth_groups_include_admin((array) ($user['groups'] ?? []))) {
        return $reports;
    }
    $out = [];
    foreach ($reports as $json) {
        if (!is_array($json)) {
            continue;
        }
        $key = trim((string) ($json['report'] ?? ''));
        if ($key === '' || !jrs_is_valid_report_key($key)) {
            continue;
        }
        if (jrs_has_permission($conn, $uid, JRS_RESOURCE_TYPE_JASPER_REPORT, $key, 'view')) {
            $out[] = $json;
        }
    }
    return $out;
}
