<?php
declare(strict_types=1);

/**
 * ReportCompiler — JRXML → .jasper after embedded-map JRXML sync.
 *
 * Preferred: Tomcat POST /internal/compile when jasper_internal_compile_url is set
 * and the endpoint responds. Fallback: jasperstarter (same as publish_report.php).
 */

const JRS_COMPILE_USER_OK = 'Report compiled successfully.';
const JRS_COMPILE_USER_FAIL = 'Unable to compile the report. Please contact your administrator.';
const JRS_COMPILE_USER_SKIPPED = 'Report is up to date.';

/**
 * @return array{
 *   ok:bool,
 *   compiled:bool,
 *   skipped:bool,
 *   message:string,
 *   userMessage:string,
 *   compiler?:string,
 *   durationMs?:int,
 *   jrxmlMtime?:int|null,
 *   jasperMtime?:int|null,
 *   error?:string,
 *   httpStatus?:int,
 *   response?:array<string,mixed>|null
 * }
 */
function jrs_report_compile(string $reportKey, bool $force = false): array
{
    $reportKey = jrs_report_key($reportKey);
    if (!jrs_is_valid_report_key($reportKey)) {
        return jrs_report_compile_result(false, false, false, 'Invalid report key.', JRS_COMPILE_USER_FAIL, [
            'error' => 'Invalid report key.',
        ]);
    }

    $jrxmlPath = jrs_report_jrxml_path($reportKey);
    $jasperPath = jrs_report_jasper_path($reportKey);

    if (!is_file($jrxmlPath)) {
        return jrs_report_compile_result(false, false, false, 'JRXML not found.', JRS_COMPILE_USER_FAIL, [
            'error' => 'JRXML not found: ' . $jrxmlPath,
            'jrxmlMtime' => null,
            'jasperMtime' => is_file($jasperPath) ? (int) filemtime($jasperPath) : null,
        ]);
    }

    $jrxmlMtime = (int) filemtime($jrxmlPath);
    $jasperMtime = is_file($jasperPath) ? (int) filemtime($jasperPath) : null;

    if (!$force && $jasperMtime !== null && $jasperMtime >= $jrxmlMtime) {
        return jrs_report_compile_result(true, false, true, 'Compiled report is current.', JRS_COMPILE_USER_SKIPPED, [
            'compiler' => 'none',
            'jrxmlMtime' => $jrxmlMtime,
            'jasperMtime' => $jasperMtime,
        ]);
    }

    $started = microtime(true);

    $compileUrl = trim((string) jrs_config('jasper_internal_compile_url', ''));
    if ($compileUrl !== '') {
        $call = jrs_report_compile_via_tomcat($reportKey);
        if (!empty($call['ok'])) {
            $durationMs = (int) round((microtime(true) - $started) * 1000);
            error_log(sprintf(
                'jrs-compile ok report=%s compiler=tomcat durationMs=%d jrxml=%s jasper=%s',
                $reportKey,
                $durationMs,
                $jrxmlPath,
                $jasperPath
            ));
            return jrs_report_compile_result(true, true, false, 'Compiled via Tomcat.', JRS_COMPILE_USER_OK, [
                'compiler' => 'tomcat',
                'durationMs' => $durationMs,
                'jrxmlMtime' => is_file($jrxmlPath) ? (int) filemtime($jrxmlPath) : $jrxmlMtime,
                'jasperMtime' => is_file($jasperPath) ? (int) filemtime($jasperPath) : null,
                'httpStatus' => $call['httpStatus'] ?? null,
                'response' => $call['response'] ?? null,
            ]);
        }
        error_log(sprintf(
            'jrs-compile tomcat unavailable report=%s error=%s — trying jasperstarter',
            $reportKey,
            (string) ($call['error'] ?? 'unknown')
        ));
    }

    $starter = jrs_report_compile_via_jasperstarter($jrxmlPath, $jasperPath);
    $durationMs = (int) round((microtime(true) - $started) * 1000);
    $jrxmlMtimeAfter = is_file($jrxmlPath) ? (int) filemtime($jrxmlPath) : $jrxmlMtime;
    $jasperMtimeAfter = is_file($jasperPath) ? (int) filemtime($jasperPath) : null;

    if (!empty($starter['ok'])) {
        error_log(sprintf(
            'jrs-compile ok report=%s compiler=jasperstarter durationMs=%d jrxml=%s jasper=%s',
            $reportKey,
            $durationMs,
            $jrxmlPath,
            $jasperPath
        ));
        return jrs_report_compile_result(true, true, false, 'Compiled via jasperstarter.', JRS_COMPILE_USER_OK, [
            'compiler' => 'jasperstarter',
            'durationMs' => $durationMs,
            'jrxmlMtime' => $jrxmlMtimeAfter,
            'jasperMtime' => $jasperMtimeAfter,
        ]);
    }

    $detail = (string) ($starter['error'] ?? 'jasperstarter compile failed.');
    error_log(sprintf(
        'jrs-compile fail report=%s compiler=jasperstarter durationMs=%d error=%s jrxml=%s',
        $reportKey,
        $durationMs,
        $detail,
        $jrxmlPath
    ));

    return jrs_report_compile_result(false, false, false, $detail, JRS_COMPILE_USER_FAIL, [
        'compiler' => 'jasperstarter',
        'durationMs' => $durationMs,
        'jrxmlMtime' => $jrxmlMtimeAfter,
        'jasperMtime' => $jasperMtimeAfter,
        'error' => $detail,
    ]);
}

/**
 * Compile only when JRXML is newer than .jasper (or .jasper missing).
 *
 * @return array<string,mixed>
 */
function jrs_report_ensure_compiled(string $reportKey): array
{
    return jrs_report_compile($reportKey, false);
}

/**
 * Force recompile regardless of timestamps.
 *
 * @return array<string,mixed>
 */
function jrs_report_compile_force(string $reportKey): array
{
    return jrs_report_compile($reportKey, true);
}

/**
 * True when .jasper is missing or older than JRXML.
 */
function jrs_report_is_stale(string $reportKey): bool
{
    $jrxmlPath = jrs_report_jrxml_path($reportKey);
    $jasperPath = jrs_report_jasper_path($reportKey);
    if (!is_file($jrxmlPath)) {
        return false;
    }
    if (!is_file($jasperPath)) {
        return true;
    }
    return (int) filemtime($jrxmlPath) > (int) filemtime($jasperPath);
}

/**
 * Administrator diagnostics for the debug page.
 *
 * @return array<string,mixed>
 */
function jrs_report_compile_status(string $reportKey): array
{
    $reportKey = jrs_report_key($reportKey);
    $jrxmlPath = jrs_report_jrxml_path($reportKey);
    $jasperPath = jrs_report_jasper_path($reportKey);
    $jrxmlMtime = is_file($jrxmlPath) ? (int) filemtime($jrxmlPath) : null;
    $jasperMtime = is_file($jasperPath) ? (int) filemtime($jasperPath) : null;
    $stale = $jrxmlMtime !== null && ($jasperMtime === null || $jrxmlMtime > $jasperMtime);
    $compileUrl = trim((string) jrs_config('jasper_internal_compile_url', ''));

    return [
        'report' => $reportKey,
        'jrxmlPath' => $jrxmlPath,
        'jasperPath' => $jasperPath,
        'jrxmlExists' => is_file($jrxmlPath),
        'jasperExists' => is_file($jasperPath),
        'jrxmlMtime' => $jrxmlMtime,
        'jasperMtime' => $jasperMtime,
        'jrxmlMtimeIso' => $jrxmlMtime !== null ? date('c', $jrxmlMtime) : null,
        'jasperMtimeIso' => $jasperMtime !== null ? date('c', $jasperMtime) : null,
        'stale' => $stale,
        'compileEndpoint' => $compileUrl !== '' ? jrs_report_compile_endpoint_url() : '',
        'compiler' => $compileUrl !== '' ? 'tomcat-or-jasperstarter' : 'jasperstarter',
    ];
}

/**
 * @param array<string,mixed> $extra
 * @return array<string,mixed>
 */
function jrs_report_compile_result(
    bool $ok,
    bool $compiled,
    bool $skipped,
    string $message,
    string $userMessage,
    array $extra = []
): array {
    return array_merge([
        'ok' => $ok,
        'compiled' => $compiled,
        'skipped' => $skipped,
        'message' => $message,
        'userMessage' => $userMessage,
    ], $extra);
}

function jrs_report_compile_endpoint_url(): string
{
    $configured = trim((string) jrs_config('jasper_internal_compile_url', ''));
    if ($configured !== '') {
        return rtrim($configured, '/');
    }
    $reportUrl = rtrim((string) jrs_config('jasper_internal_report_url', 'http://127.0.0.1:9099/report'), '/');
    if (str_ends_with($reportUrl, '/report')) {
        return substr($reportUrl, 0, -strlen('/report')) . '/internal/compile';
    }
    $parts = parse_url($reportUrl);
    if (!is_array($parts) || empty($parts['host'])) {
        return 'http://127.0.0.1:9099/internal/compile';
    }
    $scheme = $parts['scheme'] ?? 'http';
    $host = $parts['host'];
    $port = isset($parts['port']) ? ':' . $parts['port'] : '';
    return $scheme . '://' . $host . $port . '/internal/compile';
}

/**
 * @return array{ok:bool,error?:string,httpStatus?:int,response?:array<string,mixed>|null}
 */
function jrs_report_compile_via_tomcat(string $reportKey): array
{
    $url = jrs_report_compile_endpoint_url();
    if ($url === '') {
        return ['ok' => false, 'error' => 'Compile endpoint is not configured.'];
    }

    $target = $url . '?_repName=' . rawurlencode($reportKey);
    $headers = [
        'Accept: application/json',
        'X-JRS-Compile-Client: jasper-report-service',
    ];
    $token = trim((string) jrs_config('jasper_compile_token', ''));
    if ($token !== '') {
        $headers[] = 'Authorization: Bearer ' . $token;
        $headers[] = 'X-JRS-Compile-Token: ' . $token;
    }

    $ch = curl_init($target);
    if ($ch === false) {
        return ['ok' => false, 'error' => 'Failed to initialize compile request.'];
    }
    curl_setopt($ch, CURLOPT_POST, true);
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
    curl_setopt($ch, CURLOPT_FOLLOWLOCATION, false);
    curl_setopt($ch, CURLOPT_TIMEOUT, 120);
    curl_setopt($ch, CURLOPT_HTTPHEADER, $headers);
    curl_setopt($ch, CURLOPT_POSTFIELDS, '');

    $body = curl_exec($ch);
    $err = curl_error($ch);
    $status = (int) curl_getinfo($ch, CURLINFO_HTTP_CODE);
    curl_close($ch);

    if ($body === false || $body === '') {
        $hint = $err !== '' ? ('Compile request failed: ' . $err) : 'Empty response from compile endpoint.';
        if ($status === 404 || $status === 0) {
            $hint .= ' Is Tomcat running with /internal/compile?';
        }
        return [
            'ok' => false,
            'error' => $hint,
            'httpStatus' => $status,
            'response' => null,
        ];
    }

    $json = json_decode($body, true);
    if (!is_array($json)) {
        $snippet = substr(preg_replace('/\s+/', ' ', $body) ?? $body, 0, 200);
        return [
            'ok' => false,
            'error' => 'Compile endpoint returned non-JSON (HTTP ' . $status . '): ' . $snippet,
            'httpStatus' => $status,
            'response' => null,
        ];
    }

    if (!empty($json['success'])) {
        return [
            'ok' => true,
            'httpStatus' => $status,
            'response' => $json,
        ];
    }

    $error = trim((string) ($json['error'] ?? ''));
    if ($error === '') {
        $error = 'Compile failed (HTTP ' . $status . ').';
    }

    return [
        'ok' => false,
        'error' => $error,
        'httpStatus' => $status,
        'response' => $json,
    ];
}

/**
 * Match publish_report.php: jasperstarter compile <jrxml> -o <jasper>
 *
 * @return array{ok:bool,error?:string}
 */
function jrs_report_compile_via_jasperstarter(string $jrxmlPath, string $jasperPath): array
{
    $compileCommand = 'jasperstarter compile '
        . escapeshellarg($jrxmlPath)
        . ' -o '
        . escapeshellarg($jasperPath)
        . ' 2>&1';
    $compileOutput = [];
    $compileExitCode = 0;
    @exec($compileCommand, $compileOutput, $compileExitCode);

    if ($compileExitCode === 0 && is_file($jasperPath)) {
        return ['ok' => true];
    }

    $detail = trim(implode("\n", $compileOutput));
    if ($detail === '') {
        $detail = 'jasperstarter compile failed (exit ' . $compileExitCode
            . '). Upload a compiled .jasper file or install jasperstarter on the server.';
    }
    return ['ok' => false, 'error' => $detail];
}
