<?php
declare(strict_types=1);

/**
 * CLI entry for systemd timers and background runs.
 * Usage: php scripts/run_schedule.php <schedule_id>
 *
 * All execution logic lives in jrs_schedule_execute() (shared with Run Now).
 */

if (PHP_SAPI !== 'cli') {
    fwrite(STDERR, "CLI only\n");
    exit(1);
}

require_once dirname(__DIR__) . '/lib/bootstrap.php';

$scheduleId = isset($argv[1]) ? (int) $argv[1] : 0;
if ($scheduleId <= 0) {
    fwrite(STDERR, "Usage: php run_schedule.php <schedule_id>\n");
    exit(1);
}

try {
    $result = jrs_schedule_execute($scheduleId, 'scheduled');
} catch (Throwable $e) {
    jrs_schedule_run_log($scheduleId, 'ERROR: ' . $e->getMessage());
    jrs_schedule_run_log($scheduleId, $e->getTraceAsString());
    fwrite(STDERR, $e->getMessage() . "\n");
    exit(1);
}

if (empty($result['ok'])) {
    $message = (string) ($result['message'] ?? 'run failed');
    fwrite(STDERR, $message . "\n");
    exit(1);
}

fwrite(
    STDOUT,
    'OK execution_id=' . (int) ($result['execution_id'] ?? 0)
    . ' file=' . (string) ($result['output_file'] ?? '')
    . ' email=' . (string) ($result['email_status'] ?? '')
    . "\n"
);
exit(0);
