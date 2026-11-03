<?php
declare(strict_types=1);

/**
 * Admin gate for scheduling UI and JSON actions (Central platform admin).
 */
function jrs_require_schedule_admin($conn): void
{
    jrs_require_platform_admin($conn);
}

function jrs_schedule_output_formats(): array
{
    return ['pdf', 'xlsx', 'csv', 'html', 'xls', 'docx'];
}

function jrs_jasper_home(): string
{
    $home = trim((string) jrs_config('jasper_home', ''));
    if ($home !== '') {
        return rtrim($home, '/\\');
    }
    return jrs_base_dir();
}

function jrs_schedule_dir(): string
{
    $dir = trim((string) jrs_config('jasper_schedule_dir', ''));
    if ($dir !== '') {
        return rtrim($dir, '/\\');
    }
    return jrs_jasper_home() . '/schedules';
}

function jrs_schedule_output_dir(): string
{
    return jrs_schedule_dir() . '/output';
}

function jrs_schedule_log_dir(): string
{
    return jrs_schedule_dir() . '/logs';
}

/** Unix user/group for systemd timers and scheduled report output (see service unit User=). */
function jrs_schedule_service_user(): string
{
    return 'www-data';
}

/**
 * Create a directory with group-writable mode; ownership follows the running process (www-data).
 */
function jrs_schedule_mkdir(string $dir): bool
{
    if ($dir === '') {
        return false;
    }
    if (is_dir($dir)) {
        return true;
    }
    return @mkdir($dir, 0775, true);
}

function jrs_schedule_dir_not_writable_message(string $dir, string $label = 'Output directory'): string
{
    $user = jrs_schedule_service_user();
    return $label . " is not writable:\n"
        . $dir . "\n\n"
        . "Current owner/group mismatch.\n"
        . 'Expected writable by ' . $user . ".\n"
        . "Run:\n\n"
        . 'chown -R ' . $user . ':' . $user . ' ' . $dir . "\n"
        . 'chmod 775 ' . $dir;
}

/**
 * Create dir (0775) and fail with remediation steps when not writable by the service user.
 */
function jrs_schedule_require_writable_dir(string $dir, string $label = 'Output directory'): void
{
    if (!jrs_schedule_mkdir($dir)) {
        throw new RuntimeException('Could not create directory: ' . $dir);
    }
    if (!is_writable($dir)) {
        throw new RuntimeException(jrs_schedule_dir_not_writable_message($dir, $label));
    }
}

/**
 * Prepare schedules/output/{id} for a run (mkdir only; no chown/chgrp from PHP).
 */
function jrs_schedule_prepare_output_dir(int $scheduleId): string
{
    $outputDir = jrs_schedule_output_dir() . '/' . $scheduleId;
    jrs_schedule_require_writable_dir(jrs_schedule_dir(), 'Schedule directory');
    jrs_schedule_require_writable_dir(jrs_schedule_output_dir(), 'Schedule output directory');
    jrs_schedule_require_writable_dir($outputDir, 'Output directory');
    return $outputDir;
}

function jrs_schedule_log_path(int $scheduleId): string
{
    jrs_schedule_mkdir(jrs_schedule_log_dir());
    return jrs_schedule_log_dir() . '/schedule-' . $scheduleId . '.log';
}

function jrs_systemctl_bin(): string
{
    $bin = trim((string) jrs_config('jasper_systemctl', '/bin/systemctl'));
    return $bin !== '' ? $bin : '/bin/systemctl';
}

function jrs_systemd_use_sudo(): bool
{
    $raw = '';
    if (function_exists('jrs_env_string')) {
        $raw = jrs_env_string('JRS_JASPER_SYSTEMD_USE_SUDO');
    }
    if ($raw === '') {
        $fromEnv = getenv('JRS_JASPER_SYSTEMD_USE_SUDO');
        if (is_string($fromEnv)) {
            $raw = trim($fromEnv);
        }
    }
    if ($raw !== '') {
        return !in_array(strtolower($raw), ['0', 'false', 'no', 'off'], true);
    }
    return (bool) jrs_config('jasper_systemd_use_sudo', true);
}

function jrs_systemd_unit_dir(): string
{
    $dir = trim((string) jrs_config('jasper_systemd_unit_dir', '/etc/systemd/system'));
    return $dir !== '' ? rtrim($dir, '/\\') : '/etc/systemd/system';
}

/** Where systemd loads unit files from (symlinks point here from jasper_systemd_unit_dir). */
function jrs_systemd_system_dir(): string
{
    $dir = trim((string) jrs_config('jasper_systemd_system_dir', '/etc/systemd/system'));
    return $dir !== '' ? rtrim($dir, '/\\') : '/etc/systemd/system';
}

function jrs_schedule_unit_prefix(): string
{
    return 'jrs-schedule-';
}

function jrs_schedule_service_name(int $scheduleId): string
{
    return jrs_schedule_unit_prefix() . $scheduleId . '.service';
}

function jrs_schedule_timer_name(int $scheduleId): string
{
    return jrs_schedule_unit_prefix() . $scheduleId . '.timer';
}

function jrs_json_response(array $payload, int $code = 200): void
{
    http_response_code($code);
    header('Content-Type: application/json; charset=utf-8');
    echo json_encode($payload, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE);
    exit;
}
