<?php
declare(strict_types=1);

require_once __DIR__ . '/../../lib/bootstrap.php';

/**
 * GIS integration registry: enabled Jasper datasources.
 * GET /api/datasources → [{ "name": "<_dataSource>" }, ...]
 */
header('Content-Type: application/json; charset=utf-8');

$conn = jrs_app_db();
$names = jrs_datasource_names($conn);
if ($conn) {
    pg_close($conn);
}

$items = [];
foreach ($names as $name) {
    $name = trim((string) $name);
    if ($name === '') {
        continue;
    }
    $items[] = ['name' => $name];
}

echo json_encode($items, JSON_UNESCAPED_SLASHES);
