#!/usr/bin/env php
<?php
declare(strict_types=1);

/**
 * CLI: backfill Jasper report rows into local resources (same as web catalog sync).
 *
 * Usage:
 *   php scripts/sync_jasper_resources.php
 */

if (php_sapi_name() !== 'cli') {
    fwrite(STDERR, "This script is for CLI use only.\n");
    exit(1);
}

require_once dirname(__DIR__) . '/lib/bootstrap.php';

$conn = jrs_app_db();
if (!$conn) {
    fwrite(STDERR, "[resource-sync] Could not connect to application database (check JRS_DB_*).\n");
    exit(1);
}

$stats = jrs_sync_all_jasper_report_resources($conn);
pg_close($conn);

echo json_encode($stats, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES) . "\n";
exit(($stats['ok'] ?? false) === true ? 0 : 1);
