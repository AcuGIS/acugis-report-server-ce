#!/usr/bin/env php
<?php
declare(strict_types=1);

/**
 * Idempotent bootstrap of the initial admin user for AcuGIS Report Server.
 *
 * Usage:
 *   JRS_ADMIN_PASSWORD='secret' php scripts/seed_admin.php
 *   JRS_ADMIN_USER=admin JRS_ADMIN_PASSWORD='secret' php scripts/seed_admin.php
 *
 * Requires database/schema.sql applied (auth tables + Administrators group).
 */

require_once dirname(__DIR__) . '/lib/bootstrap.php';

function jrs_seed_env(string $key): string
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

$username = jrs_seed_env('JRS_ADMIN_USER');
if ($username === '') {
    $username = 'admin';
}

$password = jrs_seed_env('JRS_ADMIN_PASSWORD');
if ($password === '') {
    fwrite(STDERR, "JRS_ADMIN_PASSWORD is required.\n");
    fwrite(STDERR, "Example: JRS_ADMIN_PASSWORD='secret' php scripts/seed_admin.php\n");
    exit(1);
}

$conn = jrs_app_db();
if (!$conn) {
    fwrite(STDERR, "Could not connect to application database (check JRS_DB_* / config.php db settings).\n");
    exit(1);
}

$passwordHash = password_hash($password, PASSWORD_DEFAULT);
if ($passwordHash === false) {
    fwrite(STDERR, "Failed to hash password.\n");
    exit(1);
}

$res = @pg_query_params(
    $conn,
    <<<'SQL'
INSERT INTO dashboard_users (username, email, password_hash, groups, is_admin, enabled)
VALUES ($1, '', $2, '[]'::jsonb, TRUE, TRUE)
ON CONFLICT (username) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    is_admin = TRUE,
    enabled = TRUE
RETURNING id, username
SQL,
    [$username, $passwordHash]
);
if ($res === false) {
    fwrite(STDERR, 'Failed to upsert admin user: ' . (string) @pg_last_error($conn) . "\n");
    exit(1);
}
$userRow = pg_fetch_assoc($res);
pg_free_result($res);
if ($userRow === false) {
    fwrite(STDERR, "Failed to upsert admin user (no row returned).\n");
    exit(1);
}

$userId = (int) ($userRow['id'] ?? 0);
if ($userId <= 0) {
    fwrite(STDERR, "Invalid user id after upsert.\n");
    exit(1);
}

$gidRes = @pg_query_params(
    $conn,
    'SELECT id FROM auth_groups WHERE LOWER(name) = LOWER($1) LIMIT 1',
    ['Administrators']
);
if ($gidRes === false) {
    fwrite(STDERR, 'Failed to look up Administrators group: ' . (string) @pg_last_error($conn) . "\n");
    exit(1);
}
$groupRow = pg_fetch_assoc($gidRes);
pg_free_result($gidRes);
if ($groupRow === false) {
    fwrite(STDERR, "Administrators group not found — apply database/schema.sql first.\n");
    exit(1);
}

$groupId = (int) ($groupRow['id'] ?? 0);
if ($groupId <= 0) {
    fwrite(STDERR, "Invalid Administrators group id.\n");
    exit(1);
}

$memberRes = @pg_query_params(
    $conn,
    <<<'SQL'
INSERT INTO auth_group_members (group_id, user_id)
VALUES ($1, $2)
ON CONFLICT (group_id, user_id) DO NOTHING
RETURNING id
SQL,
    [$groupId, $userId]
);
if ($memberRes === false) {
    fwrite(STDERR, 'Failed to assign Administrators membership: ' . (string) @pg_last_error($conn) . "\n");
    exit(1);
}
pg_free_result($memberRes);

fwrite(STDOUT, "Admin user upserted: {$username} (id={$userId}), Administrators group membership ensured.\n");
exit(0);
