<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

/**
 * Portal-facing report list filtered by jasper_report ACL when Central auth is configured.
 * Uses the same session cookie as other Jasper endpoints; anonymous callers see Public reports only.
 *
 * @param resource|null $conn Legacy Jasper app DB connection (unused).
 * @return list<array<string, mixed>>
 */
function get_reports($conn = null, $user_id = null, $usr_grp_names = [], $usr_grps_ids = ''): array
{
    $all = jrs_all_reports(true);
    if (jrs_auth_central_dsn() === '') {
        return $all;
    }
    $cconn = jrs_auth_central_db();
    if (!$cconn) {
        return [];
    }
    jrs_auth_try_refresh_current_user($cconn);
    $filtered = jrs_filter_reports_for_user($cconn, $all);
    pg_close($cconn);
    return $filtered;
}
