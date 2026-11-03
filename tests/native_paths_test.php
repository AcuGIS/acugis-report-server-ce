<?php
declare(strict_types=1);

/**
 * Native JRXML path resolution (shared with Go resolveJRXMLDir).
 *
 * Run: php tests/native_paths_test.php
 */

require dirname(__DIR__) . '/lib/native_paths.php';

$failures = 0;

function jrs_test_assert(string $name, bool $ok, string $detail = ''): void
{
    global $failures;
    if ($ok) {
        fwrite(STDOUT, "ok  {$name}\n");
        return;
    }
    $failures++;
    fwrite(STDOUT, "FAIL {$name}" . ($detail !== '' ? ": {$detail}" : '') . "\n");
}

function jrs_test_slash(string $path): string
{
    return rtrim(str_replace('\\', '/', $path), '/');
}

$saved = [];
foreach ([
    'JRS_JRXML_DIR',
    'JRS_JASPER_REPORTS_CONTAINER_DIR',
    'CATALINA_BASE',
    'CATALINA_HOME',
    'JRS_SYSTEMD_ENV_FILE',
    'JRS_SMTP_HOST',
    'JRS_SMTP_FROM',
] as $key) {
    $saved[$key] = getenv($key);
}

function jrs_test_clear_env(string $key): void
{
    putenv($key);
    unset($_ENV[$key], $_SERVER[$key]);
}

function jrs_test_set_env(string $key, string $value): void
{
    putenv($key . '=' . $value);
    $_ENV[$key] = $value;
    $_SERVER[$key] = $value;
}

foreach (['JRS_JRXML_DIR', 'JRS_JASPER_REPORTS_CONTAINER_DIR', 'CATALINA_BASE', 'CATALINA_HOME'] as $key) {
    jrs_test_clear_env($key);
}

$tmp = sys_get_temp_dir() . DIRECTORY_SEPARATOR . 'jrs_native_paths_' . bin2hex(random_bytes(4));
mkdir($tmp, 0775, true);
$appRoot = $tmp . DIRECTORY_SEPARATOR . 'app';
mkdir($appRoot);

jrs_test_set_env('JRS_JRXML_DIR', $tmp . DIRECTORY_SEPARATOR . 'custom-reports');
$got = jrs_resolve_jrxml_dir($appRoot);
jrs_test_assert(
    'JRS_JRXML_DIR wins',
    jrs_test_slash($got) === jrs_test_slash($tmp . '/custom-reports'),
    $got
);

jrs_test_clear_env('JRS_JRXML_DIR');
$catalina = $tmp . DIRECTORY_SEPARATOR . 'tomcat';
$reports = $catalina . DIRECTORY_SEPARATOR . 'acugis' . DIRECTORY_SEPARATOR . 'reports';
mkdir($reports, 0775, true);
jrs_test_set_env('CATALINA_BASE', $catalina);
$got = jrs_resolve_jrxml_dir($appRoot);
jrs_test_assert(
    'CATALINA_BASE/acugis/reports when present',
    jrs_test_slash($got) === jrs_test_slash($reports),
    $got
);

jrs_test_clear_env('CATALINA_BASE');
$got = jrs_resolve_jrxml_dir($appRoot);
jrs_test_assert(
    'docker fallback when no native dir',
    jrs_test_slash($got) === jrs_test_slash($appRoot . '/docker/report-server/reports'),
    $got
);

jrs_test_clear_env('JRS_JASPER_REPORTS_CONTAINER_DIR');
$jrxml = $reports;
$container = jrs_resolve_reports_container_dir($jrxml);
$abs = realpath($jrxml) ?: $jrxml;
jrs_test_assert(
    'container dir defaults to absolute JRXML dir',
    jrs_test_slash($container) === jrs_test_slash($abs),
    $container
);

jrs_test_set_env('JRS_JASPER_REPORTS_CONTAINER_DIR', '/usr/local/tomcat/jasper_reports/reports');
$container = jrs_resolve_reports_container_dir($jrxml);
jrs_test_assert(
    'container dir honors env override',
    $container === '/usr/local/tomcat/jasper_reports/reports',
    $container
);
jrs_test_clear_env('JRS_JASPER_REPORTS_CONTAINER_DIR');

$hostPng = $reports . DIRECTORY_SEPARATOR . '_map_cache' . DIRECTORY_SEPARATOR . 'aptest_abc';
mkdir(dirname($hostPng), 0775, true);
mkdir($hostPng, 0775, true);
$standalone = jrs_translate_host_path_for_tomcat($hostPng, $reports, $reports);
jrs_test_assert(
    'standalone P_MAP_IMAGE is the host directory',
    jrs_test_slash($standalone) === jrs_test_slash($hostPng)
    && !str_ends_with($standalone, '.png')
    && !str_contains($standalone, 'docker/report-server/reports'),
    $standalone
);

$docker = jrs_translate_host_path_for_tomcat(
    $hostPng,
    $reports,
    '/usr/local/tomcat/jasper_reports/reports'
);
jrs_test_assert(
    'docker P_MAP_IMAGE rewrites onto the container mount',
    $docker === '/usr/local/tomcat/jasper_reports/reports/_map_cache/aptest_abc',
    $docker
);

$envFile = $tmp . DIRECTORY_SEPARATOR . 'native.env';
file_put_contents($envFile, "JRS_SMTP_HOST=from-file\nJRS_SMTP_FROM=a@b.c\n");
jrs_test_set_env('JRS_SMTP_HOST', 'already-set');
jrs_test_clear_env('JRS_SMTP_FROM');
jrs_test_set_env('JRS_SYSTEMD_ENV_FILE', $envFile);
jrs_load_dotenv_files($appRoot);
jrs_test_assert('dotenv does not override existing env', getenv('JRS_SMTP_HOST') === 'already-set');
jrs_test_assert('dotenv fills unset keys', getenv('JRS_SMTP_FROM') === 'a@b.c');

foreach ($saved as $key => $value) {
    if ($value === false || $value === null) {
        jrs_test_clear_env($key);
    } else {
        jrs_test_set_env($key, (string) $value);
    }
}

$it = new RecursiveIteratorIterator(
    new RecursiveDirectoryIterator($tmp, FilesystemIterator::SKIP_DOTS),
    RecursiveIteratorIterator::CHILD_FIRST
);
foreach ($it as $file) {
    $file->isDir() ? @rmdir($file->getPathname()) : @unlink($file->getPathname());
}
@rmdir($tmp);

if ($failures > 0) {
    fwrite(STDOUT, "{$failures} test(s) failed\n");
    exit(1);
}
fwrite(STDOUT, "all tests passed\n");
exit(0);
