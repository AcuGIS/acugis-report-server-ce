<?php
declare(strict_types=1);

require_once __DIR__ . '/../../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);

if ($cconn) {
    jrs_sync_all_jasper_report_resources($cconn);
}

function jrs_discovery_string(array $row, array $keys): string
{
    foreach ($keys as $key) {
        $value = trim((string) ($row[$key] ?? ''));
        if ($value !== '') {
            return $value;
        }
    }
    return '';
}

$items = [];
foreach (jrs_filter_reports_for_user($cconn, jrs_all_reports(false)) as $row) {
    $report = jrs_discovery_string($row, ['report', 'id', 'key']);
    if ($report === '') {
        continue;
    }

    $datasource = jrs_discovery_string($row, ['datasource', 'datasourceName']);
    $query = ['report' => $report];
    if ($datasource !== '') {
        $query['datasource'] = $datasource;
    }

    $mount = rtrim((string) jrs_config('app_mount', '/acugis-report-server'), '/');

    $item = [
        'id' => $report,
        'resource_type' => JRS_RESOURCE_TYPE_JASPER_REPORT,
        'resource_key' => $report,
        'title' => jrs_discovery_string($row, ['name', 'title']) ?: $report,
        'description' => jrs_discovery_string($row, ['description', 'summary']) ?: 'Jasper report',
        'url' => $mount . '/reports/view.php?' . http_build_query($query),
        'category' => 'Reports',
        'service' => JRS_SERVICE_NAME,
    ];

    if (is_file(jrs_report_asset_path($report))) {
        $item['thumbnail'] = $mount . '/reports/report_thumbnail.php?' . http_build_query(['report' => $report]);
    }

    $items[] = $item;
}

header('Content-Type: application/json; charset=utf-8');
echo json_encode([
    'service' => JRS_SERVICE_NAME,
    'resource_type' => JRS_RESOURCE_TYPE_JASPER_REPORT,
    'items' => $items,
], JSON_UNESCAPED_SLASHES);
