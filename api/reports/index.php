<?php
declare(strict_types=1);

require_once __DIR__ . '/../../lib/bootstrap.php';

/**
 * GIS integration registry: published Jasper reports for dropdowns and validation.
 * GET /api/reports → [{ "name": "<_repName>", "title": "<label>", "datasource": "<_dataSource>" }, ...]
 */
header('Content-Type: application/json; charset=utf-8');

$items = [];
foreach (jrs_all_reports(false) as $row) {
    $name = trim((string) ($row['report'] ?? ''));
    if ($name === '') {
        continue;
    }
    $title = trim((string) ($row['name'] ?? ''));
    if ($title === '') {
        $title = $name;
    }
    $items[] = [
        'name' => $name,
        'title' => $title,
        'datasource' => trim((string) ($row['datasource'] ?? '')),
        'type' => jrs_normalize_report_type($row['type'] ?? $row['reportType'] ?? ''),
        'typeLabel' => jrs_report_type_label($row['type'] ?? $row['reportType'] ?? ''),
    ];
}

echo json_encode($items, JSON_UNESCAPED_SLASHES);
