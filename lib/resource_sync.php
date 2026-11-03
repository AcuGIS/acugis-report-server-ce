<?php
declare(strict_types=1);

/**
 * Central resources catalog backfill for Jasper reports (idempotent upsert via jrs_register_resource).
 */

function jrs_resource_sync_log(string $message): void
{
    error_log('[resource-sync] ' . $message);
}

function jrs_resource_sync_verbose(): bool
{
    $v = getenv('JRS_RESOURCE_SYNC_VERBOSE');
    if ($v === false || $v === '') {
        return false;
    }
    $v = strtolower(trim((string) $v));

    return in_array($v, ['1', 'true', 'yes', 'on'], true);
}

/**
 * Upsert every on-disk report definition into Central `resources`, granting default ACLs only for newly inserted rows.
 *
 * @return array{ok:bool, registered:int, already_exists:int, errors:int, skipped:bool, reason?:string}
 */
function jrs_sync_all_jasper_report_resources($conn): array
{
    if (!$conn) {
        jrs_resource_sync_log('jasper-report-service: central DB unavailable, skipping report catalog sync');
        return ['ok' => true, 'registered' => 0, 'already_exists' => 0, 'errors' => 0, 'skipped' => true, 'reason' => 'no central connection'];
    }

    $registered = 0;
    $already = 0;
    $errors = 0;
    $reports = jrs_all_reports(false);
    jrs_resource_sync_log('jasper-report-service: syncing ' . count($reports) . ' report definition(s)');

    foreach ($reports as $row) {
        if (!is_array($row)) {
            continue;
        }
        $key = trim((string) ($row['report'] ?? ''));
        if ($key === '' || !jrs_is_valid_report_key($key)) {
            continue;
        }
        $title = trim((string) ($row['name'] ?? ''));
        if ($title === '') {
            $title = trim((string) ($row['title'] ?? ''));
        }
        if ($title === '') {
            $title = $key;
        }
        $owner = 0;

        $existed = jrs_jasper_resource_exists($conn, $key);
        $reg = jrs_register_resource($conn, $key, $title, $owner);
        if ($reg === null || (int) ($reg['id'] ?? 0) <= 0) {
            jrs_resource_sync_log('error registering jasper_report: ' . $key);
            ++$errors;
            continue;
        }
        if ($existed) {
            if (jrs_resource_sync_verbose()) {
                jrs_resource_sync_log('resource already exists: jasper_report:' . $key);
            }
            ++$already;
        } else {
            jrs_resource_sync_log('registered jasper_report: ' . $key);
            jrs_grant_default_report_permissions($conn, (int) $reg['id']);
            ++$registered;
        }

        // Keep Access Groups (JSON) in sync with resource_permissions / service entry.
        $accessRaw = trim((string) ($row['access_groups'] ?? ''));
        $accessNames = $accessRaw === '' ? ['Public'] : array_values(array_filter(array_map('trim', explode(',', $accessRaw))));
        jrs_sync_report_access_groups($conn, $key, $title, $accessNames, $owner);
    }

    jrs_resource_sync_log(sprintf(
        'jasper-report-service: catalog sync complete — %d registered, %d already in catalog, %d errors',
        $registered,
        $already,
        $errors
    ));

    jrs_ensure_platform_service_resources($conn);

    return [
        'ok' => $errors === 0,
        'registered' => $registered,
        'already_exists' => $already,
        'errors' => $errors,
        'skipped' => false,
    ];
}
