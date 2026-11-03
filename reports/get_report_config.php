<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_auth_try_refresh_current_user($cconn);

header('Content-Type: application/json; charset=utf-8');

$report = isset($_GET['report']) ? jrs_report_key((string) $_GET['report']) : '';
if (!jrs_is_valid_report_key($report)) {
    http_response_code(400);
    echo json_encode(['error' => 'invalid report']);
    exit;
}

$data = jrs_read_report($report, true);
if ($data === null) {
    http_response_code(404);
    echo json_encode(['error' => 'report not found']);
    exit;
}

if (trim((string) ($data['report'] ?? '')) === '') {
    $data['report'] = $report;
}

jrs_require_report_runtime_permission($cconn, $report, 'view');

$data['type'] = jrs_normalize_report_type($data['type'] ?? $data['reportType'] ?? '');
$data['reportTypeLabel'] = jrs_report_type_label($data['type']);
$data['map'] = jrs_resolve_map_layer($data);
$data['mapDefinition'] = jrs_report_map_definition($data);

echo json_encode($data);
