<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_auth_try_refresh_current_user($cconn);

$report = isset($_GET['report']) ? jrs_report_key((string) $_GET['report']) : '';
if (!jrs_is_valid_report_key($report)) {
    http_response_code(400);
    header('Content-Type: text/plain; charset=utf-8');
    echo 'Invalid report.';
    exit;
}

$path = jrs_report_json_path($report);
if (!is_file($path)) {
    http_response_code(404);
    exit;
}

jrs_require_report_runtime_permission($cconn, $report, 'view');

header('Content-Type: application/json; charset=utf-8');
header('X-Content-Type-Options: nosniff');
readfile($path);
