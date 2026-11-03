<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$report = isset($_GET['report']) ? jrs_report_key((string) $_GET['report']) : '';
if (!jrs_is_valid_report_key($report)) {
    http_response_code(404);
    exit;
}

$cconn = jrs_auth_require_central_db();
jrs_auth_try_refresh_current_user($cconn);

$path = jrs_report_asset_path($report);
if (!is_file($path)) {
    http_response_code(404);
    exit;
}

jrs_require_report_runtime_permission($cconn, $report, 'view');

$mime = 'image/png';
header('Content-Type: ' . $mime);
header('Cache-Control: private, max-age=3600');
readfile($path);
