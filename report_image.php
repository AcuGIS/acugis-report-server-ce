<?php
declare(strict_types=1);

require_once __DIR__ . '/lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_auth_try_refresh_current_user($cconn);

$qs = (string) ($_SERVER['QUERY_STRING'] ?? '');
parse_str($qs, $q);
$repName = isset($q['_repName']) ? jrs_report_key((string) $q['_repName']) : '';
if ($repName === '' && isset($q['report'])) {
    $repName = jrs_report_key((string) $q['report']);
}
if (!jrs_is_valid_report_key($repName)) {
    http_response_code(403);
    header('Content-Type: text/plain; charset=utf-8');
    exit;
}

jrs_require_report_runtime_permission($cconn, $repName, 'view');

$internal = rtrim((string) jrs_config('jasper_internal_image_url', ''), '/');
if ($internal === '') {
    http_response_code(503);
    exit;
}

$url = $internal . ($qs !== '' ? '?' . $qs : '');

$ch = curl_init($url);
curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
curl_setopt($ch, CURLOPT_FOLLOWLOCATION, false);
curl_setopt($ch, CURLOPT_TIMEOUT, 120);
$response = curl_exec($ch);
if ($response === false) {
    curl_close($ch);
    http_response_code(502);
    exit;
}
$contentType = curl_getinfo($ch, CURLINFO_CONTENT_TYPE);
$contentLength = curl_getinfo($ch, CURLINFO_CONTENT_LENGTH_DOWNLOAD);
curl_close($ch);

if (is_string($contentType) && $contentType !== '') {
    header('Content-Type: ' . $contentType);
}
if ($contentLength > 0) {
    header('Content-Length: ' . (int) $contentLength);
}

echo $response;
