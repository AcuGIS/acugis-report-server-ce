<?php
declare(strict_types=1);

require_once __DIR__ . '/../../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_platform_admin($cconn);

if ($_SERVER['REQUEST_METHOD'] === 'GET') {
    header('Content-Type: application/json; charset=utf-8');
    echo json_encode([
        'ok' => true,
        'service' => JRS_SERVICE_NAME,
        'hint' => 'POST to run a full Jasper report resource sync into the local resources table (idempotent).',
    ], JSON_UNESCAPED_SLASHES);
    exit;
}

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    http_response_code(405);
    header('Allow: GET, POST');
    header('Content-Type: application/json; charset=utf-8');
    echo json_encode(['error' => 'method not allowed'], JSON_UNESCAPED_SLASHES);
    exit;
}

$syncConn = jrs_app_db();
if (!$syncConn) {
    http_response_code(503);
    header('Content-Type: application/json; charset=utf-8');
    echo json_encode([
        'ok' => false,
        'service' => JRS_SERVICE_NAME,
        'error' => 'application database unavailable (check JRS_DB_* / config.php)',
    ], JSON_UNESCAPED_SLASHES);
    exit;
}

$stats = jrs_sync_all_jasper_report_resources($syncConn);
header('Content-Type: application/json; charset=utf-8');
echo json_encode(array_merge(['ok' => true, 'service' => JRS_SERVICE_NAME], $stats), JSON_UNESCAPED_SLASHES);
