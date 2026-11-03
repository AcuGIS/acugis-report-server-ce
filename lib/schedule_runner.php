<?php
declare(strict_types=1);

/**
 * Append a line to the per-schedule run log.
 * systemd StandardOutput already appends CLI stdout to this file, so stdout is
 * mirrored only when running on a terminal (not under a timer).
 */
function jrs_schedule_run_log(int $scheduleId, string $message): void
{
    jrs_schedule_mkdir(jrs_schedule_log_dir());
    $line = date('Y-m-d H:i:s') . ' ' . $message . "\n";
    @file_put_contents(jrs_schedule_log_path($scheduleId), $line, FILE_APPEND | LOCK_EX);
    // systemd units already append STDOUT to this same file (StandardOutput=append).
    // Mirror to the terminal only so interactive CLI runs stay visible without duplicating
    // timer-triggered logs line-for-line.
    if (PHP_SAPI === 'cli' && defined('STDOUT') && function_exists('stream_isatty') && @stream_isatty(STDOUT)) {
        fwrite(STDOUT, $line);
    }
}

/**
 * @param array<string, mixed> $schedule
 */
function jrs_schedule_run_log_context(int $scheduleId, array $schedule, string $mode): void
{
    jrs_schedule_run_log($scheduleId, 'Loaded env (mode=' . $mode . ')');
    $envPath = jrs_schedule_env_path($scheduleId);
    if (is_file($envPath)) {
        jrs_schedule_run_log($scheduleId, 'EnvironmentFile=' . $envPath);
    }
    jrs_schedule_run_log($scheduleId, 'Report name=' . (string) ($schedule['report_name'] ?? ''));
    jrs_schedule_run_log($scheduleId, 'Datasource=' . (string) ($schedule['datasource_name'] ?? ''));
    jrs_schedule_run_log($scheduleId, 'Format=' . (string) ($schedule['format'] ?? 'pdf'));
    if (jrs_schedule_wants_email($schedule)) {
        jrs_schedule_run_log($scheduleId, 'Email recipients=' . (string) ($schedule['email'] ?? ''));
        $tmpl = trim((string) ($schedule['email_template'] ?? ''));
        if ($tmpl !== '') {
            jrs_schedule_run_log($scheduleId, 'Email template=' . $tmpl);
        }
    } else {
        jrs_schedule_run_log($scheduleId, 'Email=disabled (noemail=' . (jrs_schedule_noemail_flag($schedule) ? '1' : '0') . ')');
    }
}

/**
 * Parse systemd EnvironmentFile (KEY=value lines).
 *
 * @return array<string, string>
 */
function jrs_schedule_read_env_file(int $scheduleId): array
{
    $path = jrs_schedule_env_path($scheduleId);
    if (!is_file($path) || !is_readable($path)) {
        return [];
    }
    $lines = file($path, FILE_IGNORE_NEW_LINES);
    if ($lines === false) {
        return [];
    }

    $vars = [];
    foreach ($lines as $line) {
        $line = trim($line);
        if ($line === '' || str_starts_with($line, '#')) {
            continue;
        }
        $eq = strpos($line, '=');
        if ($eq === false) {
            continue;
        }
        $key = trim(substr($line, 0, $eq));
        $value = jrs_schedule_parse_env_value(substr($line, $eq + 1));
        if ($key !== '') {
            $vars[$key] = $value;
        }
    }
    return $vars;
}

function jrs_schedule_parse_env_value(string $raw): string
{
    $raw = trim($raw);
    if ($raw === '') {
        return '';
    }
    if ($raw[0] === "'" && str_ends_with($raw, "'")) {
        return str_replace("'\\''", "'", substr($raw, 1, -1));
    }
    if ($raw[0] === '"' && str_ends_with($raw, '"')) {
        return stripcslashes(substr($raw, 1, -1));
    }
    return $raw;
}

function jrs_schedule_env_bool(string $value): bool
{
    $v = strtolower(trim($value));
    return in_array($v, ['1', 'true', 't', 'yes', 'on'], true);
}

/** True when an EnvironmentFile value should override the database row. */
function jrs_schedule_env_override_present(string $value): bool
{
    return trim($value) !== '';
}

/**
 * Process environment from systemd EnvironmentFile (file + optional non-empty getenv overlay).
 *
 * @return array<string, string>
 */
function jrs_schedule_runtime_env(int $scheduleId): array
{
    $vars = [];
    foreach (jrs_schedule_read_env_file($scheduleId) as $key => $value) {
        if (jrs_schedule_env_override_present($value)) {
            $vars[$key] = $value;
        }
    }

    foreach ([
        'JRS_SCHEDULE_ID',
        'JRS_REPORT_NAME',
        'JRS_DATASOURCE',
        'JRS_FORMAT',
        'JRS_FILENAME',
        'JRS_EMAIL',
        'JRS_EMAIL_SUBJECT',
        'JRS_EMAIL_BODY',
        'JRS_EMAIL_TEMPLATE',
        'JRS_NOEMAIL',
        'JRS_URL_OPT_PARAMS',
    ] as $key) {
        $v = getenv($key);
        if ($v !== false && jrs_schedule_env_override_present($v)) {
            $vars[$key] = $v;
        }
    }

    return $vars;
}

/**
 * Apply non-empty EnvironmentFile values on top of the database row (scheduled/systemd runs).
 * Database row is source of truth; env only overrides when a value is present.
 *
 * @param array<string, mixed> $schedule
 * @return array<string, mixed>
 */
function jrs_schedule_apply_runtime_env(array $schedule, array $env): array
{
    if ($env === []) {
        return $schedule;
    }

    $map = [
        'JRS_REPORT_NAME' => 'report_name',
        'JRS_DATASOURCE' => 'datasource_name',
        'JRS_FORMAT' => 'format',
        'JRS_FILENAME' => 'filename',
        'JRS_EMAIL' => 'email',
        'JRS_EMAIL_SUBJECT' => 'email_subject',
        'JRS_EMAIL_BODY' => 'email_body',
        'JRS_EMAIL_TEMPLATE' => 'email_template',
        'JRS_URL_OPT_PARAMS' => 'url_opt_params',
    ];
    foreach ($map as $envKey => $field) {
        if (!array_key_exists($envKey, $env)) {
            continue;
        }
        $value = (string) $env[$envKey];
        if (jrs_schedule_env_override_present($value)) {
            $schedule[$field] = $value;
        }
    }

    if (array_key_exists('JRS_NOEMAIL', $env) && jrs_schedule_env_override_present((string) $env['JRS_NOEMAIL'])) {
        $schedule['noemail'] = jrs_schedule_env_bool((string) $env['JRS_NOEMAIL']);
    }

    return $schedule;
}

/**
 * @param array<string, mixed> $schedule
 */
function jrs_schedule_noemail_flag(array $schedule): bool
{
    return !empty($schedule['noemail']);
}

/**
 * @param array<string, mixed> $schedule
 */
function jrs_schedule_wants_email(array $schedule): bool
{
    if (jrs_schedule_noemail_flag($schedule)) {
        return false;
    }
    return trim((string) ($schedule['email'] ?? '')) !== '';
}

/**
 * Load schedule row for execution (database + optional EnvironmentFile for scheduled mode).
 *
 * @return array<string, mixed>|null
 */
function jrs_schedule_load_for_run(int $scheduleId, $conn, string $mode): ?array
{
    $schedule = jrs_schedule_get($conn, $scheduleId);
    if ($schedule === null) {
        return null;
    }
    if ($mode !== 'scheduled') {
        return $schedule;
    }

    $env = jrs_schedule_runtime_env($scheduleId);
    if ($env === []) {
        return $schedule;
    }

    return jrs_schedule_apply_runtime_env($schedule, $env);
}

function jrs_schedule_validate_report_body(string $body, string $format): void
{
    if ($body === '') {
        throw new RuntimeException('Report response body is empty');
    }
    $format = strtolower($format);
    if ($format === 'pdf' && !str_starts_with($body, '%PDF')) {
        throw new RuntimeException('Report response is not a valid PDF (missing %PDF header)');
    }
    if ($format === 'xlsx' && !str_starts_with($body, "PK\x03\x04")) {
        throw new RuntimeException('Report response is not a valid XLSX archive');
    }
}

/**
 * Execute a schedule (report + optional email + history). Shared by Run Now and systemd.
 *
 * @param 'manual'|'scheduled' $mode
 * @return array{ok:bool, execution_id?:int, output_file?:string, email_status?:string, message?:string}
 */
function jrs_schedule_execute(int $scheduleId, string $mode = 'manual', $conn = null): array
{
    if ($mode !== 'manual' && $mode !== 'scheduled') {
        $mode = 'manual';
    }

    $ownConn = false;
    $executionId = 0;
    $connRef = $conn;
    $outputFile = '';
    $emailStatus = '';

    try {
        jrs_schedule_run_log($scheduleId, 'START schedule=' . $scheduleId . ' mode=' . $mode);

        if (!$connRef) {
            $connRef = jrs_app_db();
            $ownConn = (bool) $connRef;
        }
        if (!$connRef) {
            throw new RuntimeException('Database unavailable');
        }

        $schedule = jrs_schedule_load_for_run($scheduleId, $connRef, $mode);
        if ($schedule === null) {
            throw new RuntimeException('Schedule not found');
        }

        jrs_schedule_run_log_context($scheduleId, $schedule, $mode);

        $executionId = jrs_execution_create($connRef, $scheduleId);
        if ($executionId <= 0) {
            throw new RuntimeException('Failed to create execution history row');
        }
        jrs_schedule_run_log($scheduleId, 'Created execution_id=' . $executionId);

        $report = (string) ($schedule['report_name'] ?? '');
        $format = strtolower((string) ($schedule['format'] ?? 'pdf'));
        $datasource = (string) ($schedule['datasource_name'] ?? '');

        $outputDir = jrs_schedule_prepare_output_dir($scheduleId);

        $baseName = trim((string) ($schedule['filename'] ?? 'report'));
        $baseName = preg_replace('/[^A-Za-z0-9._-]+/', '_', $baseName) ?? 'report';
        $ext = $format === 'html' ? 'html' : $format;
        $outputFile = $outputDir . '/' . $baseName . '_' . date('Ymd_His') . '.' . $ext;

        $params = jrs_schedule_parse_url_params((string) ($schedule['url_opt_params'] ?? ''));
        $query = array_merge([
            '_repName' => $report,
            '_dataSource' => $datasource,
            '_repFormat' => $format,
        ], $params);

        $internalBase = rtrim((string) jrs_config('jasper_internal_report_url', ''), '/');
        if ($internalBase === '') {
            throw new RuntimeException('Jasper engine URL not configured (jasper_internal_report_url)');
        }

        require_once __DIR__ . '/report_compiler.php';
        jrs_schedule_run_log($scheduleId, 'JRXML dir=' . jrs_report_files_dir());
        if (jrs_report_is_stale($report) || !is_file(jrs_report_jasper_path($report))) {
            $compileResult = jrs_report_ensure_compiled($report);
            if (empty($compileResult['ok'])) {
                jrs_schedule_run_log(
                    $scheduleId,
                    'Compile warning before execution: ' . ($compileResult['error'] ?? $compileResult['message'] ?? 'failed')
                );
            } else {
                jrs_schedule_run_log($scheduleId, 'Compiled report before execution');
            }
        }

        require_once __DIR__ . '/embedded_map.php';
        $definition = jrs_read_report($report, false);
        if (is_array($definition)) {
            if (trim((string) ($definition['report'] ?? '')) === '') {
                $definition['report'] = $report;
            }
            $qs = http_build_query($query);
            $mapQuery = jrs_append_embedded_map_to_query($report, $definition, $qs, $params);
            if (isset($mapQuery['error']) && jrs_report_embedded_map($definition) !== null) {
                jrs_schedule_run_log($scheduleId, 'Embedded map warning: ' . $mapQuery['error']);
            }
            $qs = $mapQuery['query'] ?? $qs;
            $target = $internalBase . '?' . $qs;
        } else {
            $target = $internalBase . '?' . http_build_query($query);
        }
        jrs_schedule_run_log($scheduleId, 'Calling report execution');
        jrs_schedule_run_log($scheduleId, 'Request URL=' . $target);

        $ch = curl_init($target);
        curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
        curl_setopt($ch, CURLOPT_FOLLOWLOCATION, false);
        curl_setopt($ch, CURLOPT_TIMEOUT, 600);
        curl_setopt($ch, CURLOPT_HTTPHEADER, ['X-JRS-Scheduled-Run: 1']);
        $body = curl_exec($ch);
        $err = curl_error($ch);
        $status = (int) curl_getinfo($ch, CURLINFO_HTTP_CODE);
        $contentType = (string) curl_getinfo($ch, CURLINFO_CONTENT_TYPE);
        curl_close($ch);

        if ($body === false) {
            throw new RuntimeException($err !== '' ? $err : 'curl_exec failed');
        }

        $bodyStr = (string) $body;
        jrs_schedule_run_log(
            $scheduleId,
            'Report HTTP status=' . $status . ' bytes=' . strlen($bodyStr) . ' content-type=' . $contentType
        );

        if ($status < 200 || $status >= 300) {
            $msg = 'HTTP ' . $status;
            $snippet = trim(preg_replace('/\s+/', ' ', substr($bodyStr, 0, 300)) ?? '');
            if ($snippet !== '') {
                $msg .= ': ' . $snippet;
            }
            throw new RuntimeException($msg);
        }

        jrs_schedule_validate_report_body($bodyStr, $format);

        if (!is_writable($outputDir)) {
            throw new RuntimeException(jrs_schedule_dir_not_writable_message($outputDir));
        }

        if (file_put_contents($outputFile, $bodyStr) === false) {
            throw new RuntimeException('Could not write output file: ' . $outputFile);
        }
        jrs_schedule_run_log($scheduleId, 'Generated output file=' . $outputFile);

        if (jrs_schedule_wants_email($schedule)) {
            $recipients = (string) ($schedule['email'] ?? '');
            jrs_schedule_run_log($scheduleId, 'Sending email to=' . $recipients);
            $emailStatus = jrs_schedule_deliver_email($connRef, $schedule, $outputFile, $report, $format);
            if ($emailStatus === 'no_recipients' || str_starts_with($emailStatus, 'failed')) {
                throw new RuntimeException('Email delivery failed: ' . $emailStatus);
            }
            jrs_schedule_run_log($scheduleId, 'Email success status=' . $emailStatus);
        } else {
            $emailStatus = 'skipped';
            jrs_schedule_run_log($scheduleId, 'Email skipped (noemail or no recipients)');
        }

        jrs_execution_finish($connRef, $executionId, 'success', $outputFile, $emailStatus, '');
        jrs_schedule_run_log($scheduleId, 'DONE success execution_id=' . $executionId . ' email_status=' . $emailStatus);

        return [
            'ok' => true,
            'execution_id' => $executionId,
            'output_file' => $outputFile,
            'email_status' => $emailStatus,
        ];
    } catch (Throwable $e) {
        jrs_schedule_run_log($scheduleId, 'ERROR: ' . $e->getMessage());
        jrs_schedule_run_log($scheduleId, $e->getTraceAsString());
        if ($connRef && $executionId > 0) {
            jrs_execution_finish($connRef, $executionId, 'failed', $outputFile, $emailStatus, $e->getMessage());
        }
        return [
            'ok' => false,
            'message' => $e->getMessage(),
            'execution_id' => $executionId,
            'email_status' => $emailStatus,
        ];
    } finally {
        if ($ownConn && $connRef) {
            pg_close($connRef);
        }
    }
}

/**
 * @return array{ok:bool, execution_id?:int, output_file?:string, email_status?:string, message?:string}
 */
function jrs_schedule_run(int $scheduleId, $conn = null): array
{
    return jrs_schedule_execute($scheduleId, 'manual', $conn);
}

/**
 * @return array<string, string>
 */
function jrs_schedule_parse_url_params(string $raw): array
{
    $params = [];
    $raw = trim($raw);
    if ($raw === '') {
        return $params;
    }
    if (str_contains($raw, '=')) {
        foreach (preg_split('/\s*,\s*/', $raw) ?: [] as $pair) {
            if (!str_contains($pair, '=')) {
                continue;
            }
            [$k, $v] = explode('=', $pair, 2);
            $k = trim($k);
            if ($k !== '') {
                $params[$k] = trim($v);
            }
        }
        return $params;
    }
    foreach (preg_split('/\s*,\s*/', $raw) ?: [] as $token) {
        $token = trim($token);
        if ($token !== '') {
            $params[$token] = '';
        }
    }
    return $params;
}

/**
 * @param array<string, mixed> $schedule
 */
function jrs_schedule_deliver_email($conn, array $schedule, string $outputFile, string $reportName, string $format): string
{
    $recipients = jrs_schedule_parse_recipients((string) ($schedule['email'] ?? ''));
    if ($recipients === []) {
        return 'no_recipients';
    }

    $vars = [
        'report_name' => $reportName,
        'filename' => basename($outputFile),
        'run_date' => date('Y-m-d H:i:s T'),
        'format' => $format,
    ];

    $subject = (string) ($schedule['email_subject'] ?? '');
    $body = (string) ($schedule['email_body'] ?? '');
    $templateName = trim((string) ($schedule['email_template'] ?? ''));
    $bodyIsHtml = false;
    $scheduleId = (int) ($schedule['id'] ?? 0);

    if ($templateName !== '') {
        $tmplHtml = jrs_email_tmpl_load($templateName);
        if ($tmplHtml !== null) {
            $body = $tmplHtml;
            $bodyIsHtml = jrs_email_tmpl_is_html($templateName, $tmplHtml);
            if ($scheduleId > 0) {
                jrs_schedule_run_log($scheduleId, 'Using email template file (html=' . ($bodyIsHtml ? 'yes' : 'no') . ')');
            }
        } elseif ($scheduleId > 0) {
            jrs_schedule_run_log(
                $scheduleId,
                'Email template not found: ' . $templateName . ' (dir=' . jrs_email_tmpl_dir() . ')'
            );
        }
    }

    if ($subject === '') {
        $subject = 'Report: {{report_name}}';
    }
    if ($body === '') {
        $body = "Attached report {{report_name}} ({{format}}) generated {{run_date}}.\n";
    }

    $subject = jrs_email_apply_placeholders($subject, $vars);
    $body = jrs_email_apply_placeholders($body, $vars);

    $result = jrs_schedule_send_mail(
        $recipients,
        $subject,
        $body,
        $outputFile,
        (string) ($schedule['filename'] ?? basename($outputFile)),
        $bodyIsHtml
    );

    return !empty($result['ok']) ? 'sent' : ('failed: ' . (string) ($result['message'] ?? ''));
}
