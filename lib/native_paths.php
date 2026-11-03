<?php
declare(strict_types=1);

/**
 * Native / Docker JRXML path resolution shared with Go (internal/config.resolveJRXMLDir).
 *
 * Order: JRS_JRXML_DIR → existing $CATALINA_BASE|HOME/acugis/reports →
 * {appRoot}/docker/report-server/reports.
 *
 * Dotenv loading mirrors Go LoadDotEnvFiles: fill unset keys only, never override
 * process environment (systemd EnvironmentFile / Apache SetEnv).
 */

/**
 * Load KEY=VALUE pairs from native install env files without overriding getenv().
 */
function jrs_load_dotenv_files(string $appRoot): void
{
    $candidates = [];
    $cwd = getcwd();
    if (is_string($cwd) && $cwd !== '') {
        $candidates[] = $cwd . DIRECTORY_SEPARATOR . '.env';
        $candidates[] = $cwd . DIRECTORY_SEPARATOR . 'go-report-server' . DIRECTORY_SEPARATOR . '.env';
        if (basename($cwd) === 'go-report-server') {
            $candidates[] = dirname($cwd) . DIRECTORY_SEPARATOR . '.env';
        }
    }
    if ($appRoot !== '') {
        $candidates[] = $appRoot . DIRECTORY_SEPARATOR . '.env';
        $candidates[] = $appRoot . DIRECTORY_SEPARATOR . 'go-report-server' . DIRECTORY_SEPARATOR . '.env';
    }
    $systemdEnv = jrs_native_env_get('JRS_SYSTEMD_ENV_FILE');
    if ($systemdEnv !== '') {
        $candidates[] = $systemdEnv;
    }
    $candidates[] = '/etc/acugis-report-server/env';

    $seen = [];
    foreach ($candidates as $path) {
        $path = str_replace('\\', '/', $path);
        if (isset($seen[$path])) {
            continue;
        }
        $seen[$path] = true;
        jrs_load_dotenv_file($path);
    }
}

function jrs_load_dotenv_file(string $path): void
{
    if ($path === '' || !is_file($path) || !is_readable($path)) {
        return;
    }
    $lines = file($path, FILE_IGNORE_NEW_LINES);
    if ($lines === false) {
        return;
    }
    foreach ($lines as $line) {
        $line = trim(str_replace("\r", '', $line));
        if ($line === '' || str_starts_with($line, '#')) {
            continue;
        }
        if (str_starts_with($line, 'export ')) {
            $line = trim(substr($line, 7));
        }
        $eq = strpos($line, '=');
        if ($eq === false || $eq === 0) {
            continue;
        }
        $key = trim(substr($line, 0, $eq));
        if ($key === '') {
            continue;
        }
        // Match Go LoadDotEnvFiles: never override a variable already present.
        if (getenv($key) !== false || jrs_native_env_get($key) !== '') {
            continue;
        }
        $val = jrs_unquote_env_value(trim(substr($line, $eq + 1)));
        putenv($key . '=' . $val);
        $_ENV[$key] = $val;
        $_SERVER[$key] = $val;
    }
}

function jrs_unquote_env_value(string $val): string
{
    $len = strlen($val);
    if ($len >= 2) {
        $q = $val[0];
        if (($q === '"' || $q === "'") && $val[$len - 1] === $q) {
            return substr($val, 1, -1);
        }
    }
    return $val;
}

function jrs_native_env_get(string $key): string
{
    $v = getenv($key);
    if (is_string($v) && trim($v) !== '') {
        return trim($v);
    }
    if (isset($_SERVER[$key]) && is_string($_SERVER[$key]) && trim($_SERVER[$key]) !== '') {
        return trim($_SERVER[$key]);
    }
    if (isset($_ENV[$key]) && is_string($_ENV[$key]) && trim($_ENV[$key]) !== '') {
        return trim($_ENV[$key]);
    }
    return '';
}

function jrs_slash_path(string $path): string
{
    return rtrim(str_replace('\\', '/', $path), '/');
}

/**
 * Canonical JRXML / .jasper directory (Go resolveJRXMLDir).
 */
function jrs_resolve_jrxml_dir(string $appRoot): string
{
    $fromEnv = jrs_native_env_get('JRS_JRXML_DIR');
    if ($fromEnv === '' && function_exists('jrs_config')) {
        $fromEnv = trim((string) jrs_config('jrxml_dir', ''));
    }
    if ($fromEnv !== '') {
        return jrs_slash_path($fromEnv);
    }

    foreach (['CATALINA_BASE', 'CATALINA_HOME'] as $key) {
        $base = jrs_native_env_get($key);
        if ($base === '') {
            continue;
        }
        $dir = jrs_slash_path($base) . '/acugis/reports';
        if (is_dir($dir)) {
            return $dir;
        }
    }

    return jrs_slash_path($appRoot) . '/docker/report-server/reports';
}

/**
 * Path Tomcat uses in P_MAP_IMAGE (Go resolveReportsContainerDir).
 * Default: absolute JRXML dir. Docker host≠container mounts set JRS_JASPER_REPORTS_CONTAINER_DIR.
 */
function jrs_resolve_reports_container_dir(string $jrxmlDir): string
{
    $fromEnv = jrs_native_env_get('JRS_JASPER_REPORTS_CONTAINER_DIR');
    if ($fromEnv === '' && function_exists('jrs_config')) {
        $fromEnv = trim((string) jrs_config('jasper_reports_container_dir', ''));
    }
    if ($fromEnv !== '') {
        return jrs_slash_path($fromEnv);
    }
    $abs = realpath($jrxmlDir);
    if ($abs === false) {
        $abs = $jrxmlDir;
    }
    return jrs_slash_path($abs);
}

function jrs_jasper_reports_container_dir(): string
{
    $appRoot = function_exists('jrs_base_dir') ? jrs_base_dir() : dirname(__DIR__);
    return jrs_resolve_reports_container_dir(jrs_resolve_jrxml_dir($appRoot));
}

/**
 * Rewrite a host path under the JRXML dir to the path Tomcat opens (Go Paths.TomcatPath).
 * Standalone: host path == Tomcat path. Docker: rewrite onto the container mount.
 */
function jrs_translate_host_path_for_tomcat(string $hostPath, string $reportsDir, string $containerBase): string
{
    $reportsDir = jrs_slash_path($reportsDir);
    $hostReal = realpath($hostPath);
    $hostReal = jrs_slash_path($hostReal !== false ? $hostReal : $hostPath);
    $containerBase = jrs_slash_path($containerBase);
    if ($containerBase === '') {
        $containerBase = $reportsDir;
    }

    $rel = '';
    if ($reportsDir !== '' && strncasecmp($hostReal, $reportsDir, strlen($reportsDir)) === 0) {
        $rel = ltrim(substr($hostReal, strlen($reportsDir)), '/');
    } else {
        $rel = '_map_cache/' . basename($hostReal);
    }

    if (strcasecmp($containerBase, $reportsDir) === 0) {
        return $hostReal;
    }
    return $containerBase . '/' . $rel;
}
