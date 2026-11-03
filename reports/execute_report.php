<?php
declare(strict_types=1);

/**
 * Jasper execution and export proxy.
 * Validates jasper_report resource ACL (not jasper_reports service) before forwarding to the engine.
 *
 * Future: scheduled runs and signed export links should call the same checks (or shell out here).
 */

require_once __DIR__ . '/../lib/bootstrap.php';
require_once __DIR__ . '/../lib/embedded_map.php';

$allowedFormats = [
    'html', 'pdf', 'xlsx', 'xls', 'csv', 'docx', 'rtf', 'odt', 'ods', 'pptx', 'xml',
];

$cconn = jrs_auth_require_central_db();
jrs_auth_try_refresh_current_user($cconn);

$repName = isset($_REQUEST['_repName']) ? jrs_report_key((string) $_REQUEST['_repName']) : '';
$format = strtolower(trim((string) ($_REQUEST['_repFormat'] ?? 'html')));

if (!jrs_is_valid_report_key($repName)) {
    http_response_code(400);
    header('Content-Type: text/plain; charset=utf-8');
    echo 'Invalid report.';
    exit;
}

if (!in_array($format, $allowedFormats, true)) {
    http_response_code(400);
    header('Content-Type: text/plain; charset=utf-8');
    echo 'Unsupported export format.';
    exit;
}

if (jrs_read_report($repName, false) === null) {
    http_response_code(404);
    header('Content-Type: text/plain; charset=utf-8');
    echo 'Report not found.';
    exit;
}

jrs_require_report_runtime_permission($cconn, $repName, 'view');

$definition = jrs_read_report($repName, false);
if (!is_array($definition)) {
    $definition = [];
}
if (trim((string) ($definition['report'] ?? '')) === '') {
    $definition['report'] = $repName;
}

$internalBase = rtrim((string) jrs_config('jasper_internal_report_url', ''), '/');
if ($internalBase === '') {
    http_response_code(503);
    header('Content-Type: text/plain; charset=utf-8');
    echo 'Jasper engine URL is not configured.';
    exit;
}

$query = $_SERVER['QUERY_STRING'] ?? '';
$queryParams = jrs_parse_query_string_multi($query);
$mapQuery = jrs_append_embedded_map_to_query($repName, $definition, $query, $queryParams);
if (isset($mapQuery['error']) && jrs_report_embedded_map($definition) !== null) {
    // Soft-fail: still run the report without the map image (JRXML onErrorType=Blank).
    error_log('jrs embedded map: ' . $mapQuery['error']);
    header('X-JRS-Embedded-Map: error');
    header('X-JRS-Embedded-Map-Error: ' . substr(preg_replace('/[\r\n]+/', ' ', $mapQuery['error']) ?? $mapQuery['error'], 0, 200));
} elseif (jrs_report_embedded_map($definition) !== null) {
    header('X-JRS-Embedded-Map: ok');
}
$query = $mapQuery['query'] ?? $query;
$target = $internalBase . ($query !== '' ? '?' . $query : '');

$ch = curl_init($target);
curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
curl_setopt($ch, CURLOPT_FOLLOWLOCATION, false);
curl_setopt($ch, CURLOPT_HEADER, true);
curl_setopt($ch, CURLOPT_TIMEOUT, 600);
curl_setopt($ch, CURLOPT_HTTPHEADER, [
    'X-JRS-Proxied-By: jasper-report-service',
]);

$raw = curl_exec($ch);
$err = curl_error($ch);
$status = (int) curl_getinfo($ch, CURLINFO_HTTP_CODE);
$headerSize = (int) curl_getinfo($ch, CURLINFO_HEADER_SIZE);
curl_close($ch);

if ($raw === false || $raw === '') {
    http_response_code(502);
    header('Content-Type: text/plain; charset=utf-8');
    echo $err !== '' ? 'Jasper proxy error: ' . $err : 'Jasper proxy error.';
    exit;
}

$headerBlock = substr($raw, 0, $headerSize);
$body = substr($raw, $headerSize);

$lines = preg_split('/\r\n|\r|\n/', $headerBlock) ?: [];
$first = array_shift($lines);
if (is_string($first) && preg_match('#HTTP/\d\.\d\s+(\d{3})#', $first, $m)) {
    http_response_code((int) $m[1]);
} elseif ($status > 0) {
    http_response_code($status);
}

$forward = ['content-type', 'content-disposition', 'cache-control', 'pragma', 'expires'];
$isHtml = false;
foreach ($lines as $line) {
    if ($line === '' || strpos($line, ':') === false) {
        continue;
    }
    [$k, $v] = explode(':', $line, 2);
    $lk = strtolower(trim($k));
    if ($lk === 'content-type' && stripos($v, 'text/html') !== false) {
        $isHtml = true;
    }
    if (in_array($lk, $forward, true)) {
        header($k . ':' . $v);
    }
}

/*
 * Chart / QR / barcode images in HTML are baked by Jasper as servlet paths:
 *   /report_image.php?image=…          (legacy)
 *   /acugis_report_image?image=…       (Tomcat 10 WAR)
 *   /{context}/acugis_report_image?…   (named WAR context)
 * Those hit Tomcat only from the server side. Rewrite to this app's
 * report_image.php proxy (ACL + _repName) so the browser can load them.
 */
if ($isHtml && $format === 'html') {
    $proxy = '../report_image.php?_repName=' . rawurlencode($repName) . '&image=';
    $body = str_replace('/report_image.php?image=', $proxy, $body);
    // Context-prefixed Tomcat URLs first, then root servlet path.
    $body = preg_replace(
        '#/[A-Za-z0-9._~-]+/acugis_report_image\?image=#',
        $proxy,
        $body
    ) ?? $body;
    $body = str_replace('/acugis_report_image?image=', $proxy, $body);
}

echo $body;
