<?php
declare(strict_types=1);

require_once __DIR__ . '/bootstrap.php';
require_once __DIR__ . '/schedule_admin.php';

function jrs_schedule_systemd_last_error(): string
{
    return (string) ($GLOBALS['jrs_schedule_systemd_last_error'] ?? '');
}

function jrs_schedule_systemd_set_error(string $message, int $scheduleId = 0): void
{
    $GLOBALS['jrs_schedule_systemd_last_error'] = $message;
    jrs_schedule_systemd_log($message, $scheduleId);
}

function jrs_schedule_systemd_log(string $message, int $scheduleId = 0): void
{
    $prefix = '[jrs-schedule-systemd]';
    if ($scheduleId > 0) {
        $prefix .= ' schedule=' . $scheduleId;
    }
    error_log($prefix . ' ' . $message);
}

function jrs_schedule_systemd_exec(string $cmd, int $scheduleId = 0): array
{
    if (!jrs_can_shell_exec()) {
        return ['code' => 127, 'output' => 'exec() is disabled in PHP'];
    }
    $output = [];
    $code = 0;
    exec($cmd . ' 2>&1', $output, $code);
    $result = ['code' => $code, 'output' => implode("\n", $output)];
    if ($code !== 0) {
        jrs_schedule_systemd_log('command failed (' . $code . '): ' . $cmd . ' — ' . $result['output'], $scheduleId);
    }
    return $result;
}

/** Whether a path is already absent (missing paths are OK for cleanup). */
function jrs_schedule_systemd_path_missing(string $path): bool
{
    if (jrs_systemd_use_sudo()) {
        $result = jrs_schedule_systemd_exec('sudo -n test ! -e ' . escapeshellarg($path));
        return ($result['code'] ?? 1) === 0;
    }
    return !file_exists($path) && !is_link($path);
}

function jrs_schedule_systemd_paths_equal(string $expected, string $actual): bool
{
    if ($expected === $actual) {
        return true;
    }
    $resolvedExpected = realpath($expected);
    $resolvedActual = realpath($actual);
    if ($resolvedExpected !== false && $resolvedActual !== false) {
        return $resolvedExpected === $resolvedActual;
    }
    return false;
}

function jrs_schedule_systemd_link_matches(string $link, string $expectedSource): bool
{
    if (!is_link($link)) {
        return false;
    }
    $target = @readlink($link);
    if ($target === false) {
        return false;
    }
    if (jrs_schedule_systemd_paths_equal($expectedSource, $target)) {
        return true;
    }
    $resolvedLink = realpath($link);
    $resolvedExpected = realpath($expectedSource);
    return $resolvedLink !== false
        && $resolvedExpected !== false
        && $resolvedLink === $resolvedExpected;
}

function jrs_schedule_systemd_symlink(string $source, string $link, int $scheduleId = 0): bool
{
    if (!is_file($source)) {
        jrs_schedule_systemd_set_error('unit file does not exist: ' . $source, $scheduleId);
        return false;
    }

    if (jrs_schedule_systemd_link_matches($link, $source)) {
        return true;
    }

    if (!jrs_schedule_systemd_path_missing($link)) {
        if (!jrs_schedule_systemd_unlink($link, $scheduleId)) {
            return false;
        }
    }

    if (jrs_systemd_use_sudo()) {
        $result = jrs_schedule_systemd_exec(
            'sudo -n ln -sf ' . escapeshellarg($source) . ' ' . escapeshellarg($link),
            $scheduleId
        );
        if (($result['code'] ?? 1) !== 0) {
            jrs_schedule_systemd_set_error(
                'failed to create symlink ' . $link . ' -> ' . $source . ': ' . ($result['output'] ?? ''),
                $scheduleId
            );
            return false;
        }
        if (!jrs_schedule_systemd_link_matches($link, $source)) {
            jrs_schedule_systemd_set_error(
                'symlink verification failed after ln: ' . $link . ' -> ' . $source,
                $scheduleId
            );
            return false;
        }
        return true;
    }

    if (@symlink($source, $link)) {
        return true;
    }
    jrs_schedule_systemd_set_error(
        'failed to create symlink ' . $link . ' -> ' . $source . ': ' . (error_get_last()['message'] ?? 'unknown error'),
        $scheduleId
    );
    return false;
}

function jrs_schedule_systemd_unlink(string $path, int $scheduleId = 0): bool
{
    if (jrs_schedule_systemd_path_missing($path)) {
        return true;
    }

    if (jrs_systemd_use_sudo()) {
        $result = jrs_schedule_systemd_exec('sudo -n rm -f ' . escapeshellarg($path), $scheduleId);
        if (($result['code'] ?? 1) !== 0) {
            jrs_schedule_systemd_set_error(
                'failed to remove ' . $path . ': ' . ($result['output'] ?? ''),
                $scheduleId
            );
            return false;
        }
        return true;
    }

    if (@unlink($path)) {
        return true;
    }
    jrs_schedule_systemd_set_error(
        'failed to remove ' . $path . ': ' . (error_get_last()['message'] ?? 'unknown error'),
        $scheduleId
    );
    return false;
}

/**
 * @return list<string>
 */
function jrs_schedule_systemd_unit_names(int $scheduleId): array
{
    return [
        jrs_schedule_service_name($scheduleId),
        jrs_schedule_timer_name($scheduleId),
    ];
}

function jrs_schedule_link_systemd_units(int $scheduleId): bool
{
    $unitDir = jrs_systemd_unit_dir();
    $systemDir = jrs_systemd_system_dir();

    foreach (jrs_schedule_systemd_unit_names($scheduleId) as $name) {
        $source = $unitDir . '/' . $name;
        $link = $systemDir . '/' . $name;
        if (!jrs_schedule_systemd_symlink($source, $link, $scheduleId)) {
            return false;
        }
    }

    jrs_schedule_systemd_log('symlinks installed under ' . $systemDir, $scheduleId);
    return true;
}

/**
 * @return list<string> errors from best-effort unlink (empty = all OK or already absent)
 */
function jrs_schedule_unlink_systemd_units(int $scheduleId, bool $bestEffort = false): array
{
    $errors = [];
    $systemDir = jrs_systemd_system_dir();
    foreach (jrs_schedule_systemd_unit_names($scheduleId) as $name) {
        $link = $systemDir . '/' . $name;
        if (jrs_schedule_systemd_path_missing($link)) {
            continue;
        }
        if (!jrs_schedule_systemd_unlink($link, $scheduleId)) {
            $errors[] = jrs_schedule_systemd_last_error() ?: ('failed to remove ' . $link);
            if (!$bestEffort) {
                return $errors;
            }
        }
    }
    return $errors;
}

/**
 * Whether this schedule row should have a recurring systemd timer.
 *
 * @param array<string, mixed> $schedule
 */
function jrs_schedule_needs_systemd(array $schedule): bool
{
    if (!jrs_schedule_is_enabled($schedule)) {
        return false;
    }
    $cron = strtolower((string) ($schedule['cron_period'] ?? 'never'));
    if ($cron === 'never' || $cron === 'now') {
        return false;
    }
    if (in_array($cron, ['hourly', 'daily', 'weekly', 'monthly', 'custom', 'once', 'repeat_minutes'], true)) {
        return true;
    }
    $mode = jrs_schedule_effective_mode($schedule);
    return $mode !== 'never';
}

function jrs_schedule_env_path(int $scheduleId): string
{
    return jrs_schedule_dir() . '/' . $scheduleId . '_env.sh';
}

/** systemd EnvironmentFile line: KEY=value (not a shell script). */
function jrs_schedule_env_line(string $key, string $value): string
{
    if ($value === '' || preg_match('/^[A-Za-z0-9_@%+=:,.\/-]+$/', $value)) {
        return $key . '=' . $value;
    }
    return sprintf('%s=%s', $key, escapeshellarg($value));
}

/**
 * Load the persisted schedule row for systemd install/env generation (always from DB).
 *
 * @param array<string, mixed>|null $schedule
 * @return array<string, mixed>|null
 */
function jrs_schedule_resolve_for_systemd(int $scheduleId, ?array $schedule = null, $conn = null): ?array
{
    if (!$conn) {
        $conn = jrs_app_db();
    }
    if ($conn) {
        $fromDb = jrs_schedule_get($conn, $scheduleId);
        if ($fromDb !== null) {
            return jrs_schedule_prepare_for_systemd($fromDb);
        }
    }

    return $schedule !== null ? jrs_schedule_prepare_for_systemd($schedule) : null;
}

/**
 * Write {scheduleId}_env.sh from the database row (not form POST). Called on every save.
 */
function jrs_schedule_persist_env_from_db(int $scheduleId, $conn = null): bool
{
    if (!$conn) {
        $conn = jrs_app_db();
    }
    if (!$conn) {
        jrs_schedule_systemd_set_error('Database unavailable for environment file', $scheduleId);
        return false;
    }
    $schedule = jrs_schedule_get($conn, $scheduleId);
    if ($schedule === null) {
        jrs_schedule_systemd_set_error('Schedule not found for environment file', $scheduleId);
        return false;
    }
    return jrs_schedule_build_env_file($scheduleId, $schedule);
}

function jrs_schedule_env_snapshot_is_usable(?string $contents): bool
{
    if ($contents === null || trim($contents) === '') {
        return false;
    }
    return preg_match('/^JRS_REPORT_NAME=\S/m', $contents) === 1;
}

/**
 * @param array<string, mixed> $schedule
 */
function jrs_schedule_build_env_file(int $scheduleId, array $schedule): bool
{
    $path = jrs_schedule_env_path($scheduleId);
    $reportName = trim((string) ($schedule['report_name'] ?? ''));
    $datasource = trim((string) ($schedule['datasource_name'] ?? ''));
    if ($reportName === '') {
        jrs_schedule_systemd_set_error(
            'cannot build EnvironmentFile: report_name is empty at ' . $path
                . ' (keys: ' . implode(', ', array_keys($schedule)) . ')',
            $scheduleId
        );
        return false;
    }
    if ($datasource === '') {
        jrs_schedule_systemd_set_error(
            'cannot build EnvironmentFile: datasource_name is empty at ' . $path,
            $scheduleId
        );
        return false;
    }

    if (!jrs_ensure_dir(jrs_schedule_dir())) {
        jrs_schedule_systemd_set_error('could not create schedule directory: ' . jrs_schedule_dir(), $scheduleId);
        return false;
    }
    $lines = [
        'JRS_SCHEDULE_ID=' . (int) $scheduleId,
        jrs_schedule_env_line('JRS_REPORT_NAME', (string) ($schedule['report_name'] ?? '')),
        jrs_schedule_env_line('JRS_DATASOURCE', (string) ($schedule['datasource_name'] ?? '')),
        jrs_schedule_env_line('JRS_FORMAT', (string) ($schedule['format'] ?? 'pdf')),
        jrs_schedule_env_line('JRS_FILENAME', (string) ($schedule['filename'] ?? '')),
        jrs_schedule_env_line('JRS_EMAIL', (string) ($schedule['email'] ?? '')),
        jrs_schedule_env_line('JRS_EMAIL_SUBJECT', (string) ($schedule['email_subject'] ?? '')),
        jrs_schedule_env_line('JRS_EMAIL_BODY', (string) ($schedule['email_body'] ?? '')),
        jrs_schedule_env_line('JRS_EMAIL_TEMPLATE', (string) ($schedule['email_template'] ?? '')),
        'JRS_NOEMAIL=' . (!empty($schedule['noemail']) ? '1' : '0'),
        jrs_schedule_env_line('JRS_URL_OPT_PARAMS', (string) ($schedule['url_opt_params'] ?? '')),
        jrs_schedule_env_line('JRS_JASPER_HOME', jrs_jasper_home()),
    ];
    $written = file_put_contents($path, implode("\n", $lines) . "\n");
    if ($written === false) {
        jrs_schedule_systemd_set_error('failed to write env file: ' . $path, $scheduleId);
        return false;
    }
    @chmod($path, 0640);
    jrs_schedule_systemd_log('wrote environment file: ' . $path, $scheduleId);
    return true;
}

function jrs_schedule_remove_env_file(int $scheduleId): void
{
    $path = jrs_schedule_env_path($scheduleId);
    if (is_file($path)) {
        @unlink($path);
    }
}

function jrs_php_cli_binary(): string
{
    foreach (['/usr/bin/php', '/bin/php'] as $candidate) {
        if (is_executable($candidate)) {
            return $candidate;
        }
    }
    if (defined('PHP_BINARY') && is_string(PHP_BINARY) && PHP_BINARY !== '' && is_executable(PHP_BINARY)) {
        if (stripos(PHP_BINARY, 'fpm') === false) {
            return PHP_BINARY;
        }
    }
    return 'php';
}

function jrs_can_shell_exec(): bool
{
    if (!function_exists('exec')) {
        return false;
    }
    $disabled = array_map('trim', explode(',', (string) ini_get('disable_functions')));
    return !in_array('exec', $disabled, true);
}

function jrs_systemctl_cmd(string ...$args): array
{
    if (!jrs_can_shell_exec()) {
        return ['code' => 127, 'output' => 'exec() is disabled in PHP'];
    }
    $bin = jrs_systemctl_bin();
    if (jrs_systemd_use_sudo()) {
        array_unshift($args, $bin);
        $cmd = 'sudo -n ' . implode(' ', array_map('escapeshellarg', $args));
    } else {
        $cmd = escapeshellarg($bin) . ' ' . implode(' ', array_map('escapeshellarg', $args));
    }
    $output = [];
    $code = 0;
    exec($cmd . ' 2>&1', $output, $code);
    return ['code' => $code, 'output' => implode("\n", $output)];
}

function jrs_schedule_runner_script(): string
{
    return jrs_jasper_home() . '/scripts/run_schedule.php';
}

function jrs_schedule_write_unit_files(int $scheduleId, array $schedule): bool
{
    $unitDir = jrs_systemd_unit_dir();
    if (!is_dir($unitDir)) {
        if (!jrs_ensure_dir($unitDir)) {
            jrs_schedule_systemd_set_error('unit directory does not exist and could not be created: ' . $unitDir, $scheduleId);
            return false;
        }
    }
    if (!is_writable($unitDir)) {
        jrs_schedule_systemd_set_error('unit directory is not writable: ' . $unitDir, $scheduleId);
        return false;
    }

    $serviceName = jrs_schedule_service_name($scheduleId);
    $timerName = jrs_schedule_timer_name($scheduleId);
    $runner = jrs_schedule_runner_script();
    $envFile = jrs_schedule_env_path($scheduleId);
    $logFile = jrs_schedule_log_path($scheduleId);
    $phpBin = jrs_php_cli_binary();

    $service = <<<UNIT
[Unit]
Description=Jasper Report Services schedule #{$scheduleId}
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
EnvironmentFile=-{$envFile}
ExecStart={$phpBin} {$runner} {$scheduleId}
User=www-data
Group=www-data
StandardOutput=append:{$logFile}
StandardError=append:{$logFile}

UNIT;

    $spec = jrs_schedule_timer_spec($schedule);
    if (($spec['type'] ?? 'none') === 'none') {
        jrs_schedule_systemd_set_error(
            'invalid or empty schedule timer spec: ' . ($spec['error'] ?? jrs_schedule_effective_mode($schedule)),
            $scheduleId
        );
        return false;
    }

    if ($spec['type'] === 'interval') {
        $interval = (string) ($spec['interval'] ?? '');
        $timerBody = "OnUnitActiveSec={$interval}\nOnBootSec={$interval}\nPersistent=true\nUnit={$serviceName}";
    } else {
        $calendar = (string) ($spec['calendar'] ?? '');
        if ($calendar === '') {
            jrs_schedule_systemd_set_error('invalid or empty schedule calendar', $scheduleId);
            return false;
        }
        $timerBody = "OnCalendar={$calendar}\nPersistent=true\nUnit={$serviceName}";
    }

    $timer = <<<UNIT
[Unit]
Description=Timer for Jasper schedule #{$scheduleId}

[Timer]
{$timerBody}

[Install]
WantedBy=timers.target

UNIT;

    $servicePath = $unitDir . '/' . $serviceName;
    $timerPath = $unitDir . '/' . $timerName;

    if (file_put_contents($servicePath, $service) === false) {
        jrs_schedule_systemd_set_error('failed to write service unit: ' . $servicePath, $scheduleId);
        return false;
    }
    if (file_put_contents($timerPath, $timer) === false) {
        jrs_schedule_systemd_set_error('failed to write timer unit: ' . $timerPath, $scheduleId);
        return false;
    }

    jrs_schedule_systemd_log('wrote unit files to ' . $unitDir, $scheduleId);
    return true;
}

/**
 * Snapshot env, unit files, symlinks, and timer enabled state before a transactional install.
 *
 * @return array{env: ?string, units: array<string, ?string>, symlinks: array<string, ?string>, timer_enabled: bool}
 */
function jrs_schedule_systemd_capture_state(int $scheduleId): array
{
    $state = [
        'env' => null,
        'units' => [],
        'symlinks' => [],
        'timer_enabled' => false,
    ];

    $envPath = jrs_schedule_env_path($scheduleId);
    if (is_file($envPath) && is_readable($envPath)) {
        $contents = @file_get_contents($envPath);
        if ($contents !== false) {
            $state['env'] = $contents;
        }
    }

    $unitDir = jrs_systemd_unit_dir();
    foreach (jrs_schedule_systemd_unit_names($scheduleId) as $name) {
        $path = $unitDir . '/' . $name;
        $state['units'][$name] = (is_file($path) && is_readable($path))
            ? (string) file_get_contents($path)
            : null;
    }

    $systemDir = jrs_systemd_system_dir();
    foreach (jrs_schedule_systemd_unit_names($scheduleId) as $name) {
        $link = $systemDir . '/' . $name;
        if (is_link($link)) {
            $target = @readlink($link);
            $state['symlinks'][$name] = $target !== false ? $target : null;
        } else {
            $state['symlinks'][$name] = null;
        }
    }

    $timer = jrs_schedule_timer_name($scheduleId);
    $enabled = jrs_systemctl_cmd('is-enabled', $timer);
    $enabledOut = trim((string) ($enabled['output'] ?? ''));
    $state['timer_enabled'] = in_array($enabledOut, ['enabled', 'enabled-runtime'], true);

    return $state;
}

/**
 * @param array{env: ?string, units: array<string, ?string>, symlinks: array<string, ?string>, timer_enabled: bool} $state
 */
function jrs_schedule_systemd_restore_state(int $scheduleId, array $state): bool
{
    jrs_schedule_systemd_log('rolling back systemd state after failed install', $scheduleId);
    // Environment file is persisted separately on save; never delete or roll it back here.

    $unitDir = jrs_systemd_unit_dir();
    foreach ($state['units'] as $name => $content) {
        $path = $unitDir . '/' . $name;
        if ($content !== null) {
            if (file_put_contents($path, $content) === false) {
                jrs_schedule_systemd_log('rollback: failed to restore unit file: ' . $path, $scheduleId);
            }
        } elseif (is_file($path)) {
            @unlink($path);
        }
    }

    $systemDir = jrs_systemd_system_dir();
    foreach ($state['symlinks'] as $name => $target) {
        $link = $systemDir . '/' . $name;
        if ($target === null) {
            if (is_link($link) || is_file($link)) {
                jrs_schedule_systemd_unlink($link, $scheduleId);
            }
            continue;
        }
        if (!jrs_schedule_systemd_symlink($target, $link, $scheduleId)) {
            jrs_schedule_systemd_log('rollback: failed to restore symlink ' . $link, $scheduleId);
        }
    }

    $reload = jrs_systemctl_cmd('daemon-reload');
    if (($reload['code'] ?? 1) !== 0) {
        jrs_schedule_systemd_log('rollback: daemon-reload failed: ' . ($reload['output'] ?? ''), $scheduleId);
    }

    if (!empty($state['timer_enabled'])) {
        $timer = jrs_schedule_timer_name($scheduleId);
        $enable = jrs_systemctl_cmd('enable', '--now', $timer);
        if (($enable['code'] ?? 1) !== 0) {
            jrs_schedule_systemd_log(
                'rollback: re-enable failed for ' . $timer . ': ' . ($enable['output'] ?? ''),
                $scheduleId
            );
            return false;
        }
    }

    return true;
}

function jrs_schedule_verify_unit_files_on_disk(int $scheduleId): bool
{
    $unitDir = jrs_systemd_unit_dir();
    foreach (jrs_schedule_systemd_unit_names($scheduleId) as $name) {
        $path = $unitDir . '/' . $name;
        if (!is_file($path) || !is_readable($path) || @filesize($path) === 0) {
            jrs_schedule_systemd_set_error(
                'systemd unit file missing after install: ' . $path
                    . ' (expected under ' . $unitDir . ')',
                $scheduleId
            );
            return false;
        }
    }
    return true;
}

function jrs_schedule_validate_unit_files(int $scheduleId): bool
{
    if (!jrs_schedule_verify_unit_files_on_disk($scheduleId)) {
        return false;
    }

    $unitDir = jrs_systemd_unit_dir();
    $logFile = jrs_schedule_log_path($scheduleId);
    foreach (jrs_schedule_systemd_unit_names($scheduleId) as $name) {
        $path = $unitDir . '/' . $name;
        if (!is_file($path) || !is_readable($path)) {
            jrs_schedule_systemd_set_error('unit file missing after write: ' . $path, $scheduleId);
            return false;
        }
        if (@filesize($path) === 0) {
            jrs_schedule_systemd_set_error('unit file is empty: ' . $path, $scheduleId);
            return false;
        }
        if ($name === jrs_schedule_service_name($scheduleId)) {
            $contents = (string) file_get_contents($path);
            if (!str_contains($contents, $logFile)) {
                jrs_schedule_systemd_set_error(
                    'service unit log path mismatch (expected ' . $logFile . ' in ' . $path . ')',
                    $scheduleId
                );
                return false;
            }
        }
    }
    return true;
}

function jrs_schedule_verify_systemd_units(int $scheduleId): bool
{
    $timer = jrs_schedule_timer_name($scheduleId);
    $service = jrs_schedule_service_name($scheduleId);

    foreach ([$timer, $service] as $unit) {
        $cat = jrs_systemctl_cmd('cat', $unit);
        if (($cat['code'] ?? 1) !== 0) {
            jrs_schedule_systemd_set_error(
                'unit not loaded after install: ' . $unit . ' (' . trim((string) ($cat['output'] ?? '')) . ')',
                $scheduleId
            );
            return false;
        }
    }

    $enabled = jrs_systemctl_cmd('is-enabled', $timer);
    $enabledOut = trim((string) ($enabled['output'] ?? ''));
    if (($enabled['code'] ?? 1) !== 0 || !in_array($enabledOut, ['enabled', 'enabled-runtime'], true)) {
        jrs_schedule_systemd_set_error(
            'timer is not enabled after install: ' . $timer . ' (' . $enabledOut . ')',
            $scheduleId
        );
        return false;
    }

    $active = jrs_systemctl_cmd('is-active', $timer);
    $activeOut = trim((string) ($active['output'] ?? ''));
    if (!in_array($activeOut, ['active', 'waiting'], true)) {
        jrs_schedule_systemd_set_error(
            'timer is not active or waiting after install: ' . $timer . ' (' . $activeOut . ')',
            $scheduleId
        );
        return false;
    }

    $systemDir = jrs_systemd_system_dir();
    $unitDir = jrs_systemd_unit_dir();
    foreach (jrs_schedule_systemd_unit_names($scheduleId) as $name) {
        $link = $systemDir . '/' . $name;
        $source = $unitDir . '/' . $name;
        if (!jrs_schedule_systemd_link_matches($link, $source)) {
            jrs_schedule_systemd_set_error(
                'systemd symlink missing or incorrect for ' . $name . ' (expected ' . $source . ')',
                $scheduleId
            );
            return false;
        }
    }

    return true;
}

/**
 * Best-effort cleanup before install or when a timer is not required. Never fails the save flow.
 */
function jrs_schedule_remove_systemd_best_effort(int $scheduleId): void
{
    $timer = jrs_schedule_timer_name($scheduleId);
    jrs_systemctl_cmd('disable', '--now', $timer);
    jrs_systemctl_cmd('stop', $timer);
    jrs_systemctl_cmd('reset-failed', $timer);

    $unlinkErrors = jrs_schedule_unlink_systemd_units($scheduleId, true);
    foreach ($unlinkErrors as $message) {
        jrs_schedule_systemd_log('cleanup warning: ' . $message, $scheduleId);
    }

    $unitDir = jrs_systemd_unit_dir();
    foreach (jrs_schedule_systemd_unit_names($scheduleId) as $name) {
        $path = $unitDir . '/' . $name;
        if (is_file($path) && !@unlink($path)) {
            jrs_schedule_systemd_log('cleanup warning: failed to remove unit file ' . $path, $scheduleId);
        }
    }

    $reload = jrs_systemctl_cmd('daemon-reload');
    if (($reload['code'] ?? 1) !== 0) {
        jrs_schedule_systemd_log(
            'cleanup warning: daemon-reload failed: ' . ($reload['output'] ?? ''),
            $scheduleId
        );
    }
}

/**
 * Full uninstall with rollback on failure (explicit delete or strict disable).
 */
function jrs_schedule_remove_systemd(int $scheduleId): bool
{
    $backup = jrs_schedule_systemd_capture_state($scheduleId);
    $hadUnits = $backup['timer_enabled']
        || array_filter($backup['symlinks']) !== []
        || array_filter($backup['units']) !== [];

    $timer = jrs_schedule_timer_name($scheduleId);
    jrs_systemctl_cmd('disable', '--now', $timer);
    jrs_systemctl_cmd('stop', $timer);
    jrs_systemctl_cmd('reset-failed', $timer);

    $unlinkErrors = jrs_schedule_unlink_systemd_units($scheduleId, false);
    if ($unlinkErrors !== []) {
        $detail = implode('; ', $unlinkErrors);
        if ($hadUnits) {
            jrs_schedule_systemd_set_error('failed to remove systemd symlinks during uninstall: ' . $detail, $scheduleId);
            jrs_schedule_systemd_restore_state($scheduleId, $backup);
        }
        return false;
    }

    $unitDir = jrs_systemd_unit_dir();
    foreach (jrs_schedule_systemd_unit_names($scheduleId) as $name) {
        $path = $unitDir . '/' . $name;
        if (is_file($path) && !@unlink($path)) {
            if ($hadUnits) {
                jrs_schedule_systemd_set_error('failed to remove unit file: ' . $path, $scheduleId);
                jrs_schedule_systemd_restore_state($scheduleId, $backup);
                return false;
            }
        }
    }

    $reload = jrs_systemctl_cmd('daemon-reload');
    if (($reload['code'] ?? 1) !== 0 && $hadUnits) {
        jrs_schedule_systemd_set_error('daemon-reload failed during uninstall: ' . ($reload['output'] ?? ''), $scheduleId);
        jrs_schedule_systemd_restore_state($scheduleId, $backup);
        return false;
    }

    jrs_schedule_systemd_log('systemd units removed for schedule ' . $scheduleId, $scheduleId);
    return true;
}

/**
 * @param array<string, mixed> $schedule
 */
function jrs_schedule_install_systemd(int $scheduleId, array $schedule, $conn = null): bool
{
    $GLOBALS['jrs_schedule_systemd_last_error'] = '';

    $schedule = jrs_schedule_resolve_for_systemd($scheduleId, $schedule, $conn);
    if ($schedule === null) {
        jrs_schedule_systemd_set_error('schedule not found for systemd install', $scheduleId);
        return false;
    }

    if (!jrs_schedule_needs_systemd($schedule)) {
        jrs_schedule_remove_systemd_best_effort($scheduleId);
        return true;
    }

    $backup = jrs_schedule_systemd_capture_state($scheduleId);

    if (!jrs_schedule_build_env_file($scheduleId, $schedule)) {
        jrs_schedule_systemd_restore_state($scheduleId, $backup);
        return false;
    }
    if (!jrs_schedule_write_unit_files($scheduleId, $schedule)) {
        jrs_schedule_systemd_restore_state($scheduleId, $backup);
        return false;
    }
    if (!jrs_schedule_validate_unit_files($scheduleId)) {
        jrs_schedule_systemd_restore_state($scheduleId, $backup);
        return false;
    }
    if (!jrs_schedule_link_systemd_units($scheduleId)) {
        jrs_schedule_systemd_restore_state($scheduleId, $backup);
        return false;
    }

    $reload = jrs_systemctl_cmd('daemon-reload');
    if (($reload['code'] ?? 1) !== 0) {
        jrs_schedule_systemd_set_error('daemon-reload failed: ' . ($reload['output'] ?? ''), $scheduleId);
        jrs_schedule_systemd_restore_state($scheduleId, $backup);
        return false;
    }

    $timer = jrs_schedule_timer_name($scheduleId);
    $enable = jrs_systemctl_cmd('enable', '--now', $timer);
    if (($enable['code'] ?? 1) !== 0) {
        jrs_schedule_systemd_set_error('enable --now failed for ' . $timer . ': ' . ($enable['output'] ?? ''), $scheduleId);
        jrs_schedule_systemd_restore_state($scheduleId, $backup);
        return false;
    }

    if (!jrs_schedule_verify_unit_files_on_disk($scheduleId)) {
        jrs_schedule_systemd_restore_state($scheduleId, $backup);
        return false;
    }

    if (!jrs_schedule_verify_systemd_units($scheduleId)) {
        jrs_schedule_systemd_restore_state($scheduleId, $backup);
        return false;
    }

    jrs_schedule_systemd_log('timer enabled and verified: ' . $timer, $scheduleId);
    return true;
}

/**
 * Install or remove systemd units after a schedule save, with verification.
 *
 * @param array<string, mixed> $schedule
 * @return array{ok: bool, timer_required: bool, error: string}
 */
function jrs_schedule_sync_systemd_after_save(int $scheduleId, array $schedule, $conn = null): array
{
    $schedule = jrs_schedule_resolve_for_systemd($scheduleId, $schedule, $conn);
    if ($schedule === null) {
        return [
            'ok' => false,
            'timer_required' => false,
            'error' => 'Schedule not found after save',
        ];
    }

    if (!jrs_schedule_persist_env_from_db($scheduleId, $conn)) {
        return [
            'ok' => false,
            'timer_required' => jrs_schedule_needs_systemd($schedule),
            'error' => jrs_schedule_systemd_last_error() ?: 'Failed to write schedule environment file',
        ];
    }

    $timerRequired = jrs_schedule_needs_systemd($schedule);

    if (!$timerRequired) {
        jrs_schedule_remove_systemd_best_effort($scheduleId);
        return ['ok' => true, 'timer_required' => false, 'error' => ''];
    }

    if (!jrs_schedule_is_enabled($schedule)) {
        return [
            'ok' => false,
            'timer_required' => true,
            'error' => 'Schedule is disabled (enabled=false). Enable the schedule and save again.',
        ];
    }

    if (!jrs_schedule_install_systemd($scheduleId, $schedule, $conn)) {
        return [
            'ok' => false,
            'timer_required' => true,
            'error' => jrs_schedule_systemd_last_error() ?: 'systemd timer install failed',
        ];
    }

    if (!jrs_schedule_verify_unit_files_on_disk($scheduleId)) {
        return [
            'ok' => false,
            'timer_required' => true,
            'error' => jrs_schedule_systemd_last_error() ?: 'systemd unit files missing after install',
        ];
    }

    if (!jrs_schedule_verify_systemd_units($scheduleId)) {
        return [
            'ok' => false,
            'timer_required' => true,
            'error' => jrs_schedule_systemd_last_error() ?: 'Timer verification failed after save',
        ];
    }

    return ['ok' => true, 'timer_required' => true, 'error' => ''];
}

function jrs_schedule_run_async(int $scheduleId): bool
{
    if (!jrs_can_shell_exec()) {
        return false;
    }
    $runner = jrs_schedule_runner_script();
    if (!is_file($runner)) {
        return false;
    }
    $php = jrs_php_cli_binary();
    $log = jrs_schedule_log_path($scheduleId);
    $cmd = escapeshellarg($php) . ' ' . escapeshellarg($runner) . ' ' . (int) $scheduleId
        . ' >> ' . escapeshellarg($log) . ' 2>&1 &';
    exec($cmd);
    return true;
}
