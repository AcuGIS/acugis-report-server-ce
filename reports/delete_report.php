<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'edit');

$report = isset($_GET['report']) ? jrs_report_key((string) $_GET['report']) : '';

if (jrs_is_valid_report_key($report)) {
    jrs_require_permission($cconn, $report, 'admin');
    foreach ([jrs_report_json_path($report), jrs_report_jrxml_path($report), jrs_report_jasper_path($report), jrs_report_asset_path($report)] as $path) {
        if (is_file($path)) {
            @unlink($path);
        }
    }
}

header('Location: index.php');
exit;
