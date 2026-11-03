<?php
declare(strict_types=1);

/**
 * Embedded maps in Jasper report output (PDF/HTML/DOCX).
 *
 * Pipeline: MapRequest → MapRenderer factory → PNG → Jasper image parameter.
 * Report generation never branches on PostGIS / QGIS / GeoServer.
 */

require_once __DIR__ . '/native_paths.php';
foreach (array('map_report.php', 'map_request.php', 'map_renderer.php', 'point_style.php', 'map_thematic.php') as $jrsMapLib) {
    $jrsMapLibPath = __DIR__ . '/' . $jrsMapLib;
    if (is_file($jrsMapLibPath)) {
        require_once $jrsMapLibPath;
    }
}
if (!defined('JRS_MAP_RENDERER_POSTGIS')) {
    define('JRS_MAP_RENDERER_POSTGIS', 'postgis');
}
if (!defined('JRS_MAP_RENDERER_GEOSERVER')) {
    define('JRS_MAP_RENDERER_GEOSERVER', 'geoserver');
}
if (!defined('JRS_MAP_RENDERER_QGIS')) {
    define('JRS_MAP_RENDERER_QGIS', 'qgis');
}
if (!defined('JRS_EMBEDDED_MAP_PARAM')) {
    define('JRS_EMBEDDED_MAP_PARAM', 'P_MAP_IMAGE');
}
if (!defined('JRS_EMBEDDED_MAP_BANDS')) {
    define('JRS_EMBEDDED_MAP_BANDS', array(
        'summary' => 'Summary',
        'detail' => 'Detail',
        'groupHeader' => 'Group Header',
    ));
}

/** Bottom padding (px) when fitting a band height to its content. */
const JRS_EMBEDDED_MAP_BAND_PADDING = 8;

/**
 * Default embedded-map config for report JSON.
 *
 * @return array<string,mixed>
 */
function jrs_default_embedded_map(): array
{
    return [
        'enabled' => false,
        'band' => 'summary',
        'groupName' => null,
        'width' => 535,
        'height' => 300,
        'x' => 0,
        'y' => 10,
        'padding' => 0.08,
        'background' => '#ffffff',
        'dpi' => 96,
        /** Map Source: postgis | qgis | geoserver */
        'renderer' => JRS_MAP_RENDERER_POSTGIS,
        'parameterName' => JRS_EMBEDDED_MAP_PARAM,
        // Thematic map type (PostGIS symbology). QGIS/GeoServer use native styles.
        'mapType' => 'locator',
        'field' => null,
        'classification' => 'quantile',
        'classes' => 5,
        'colorRamp' => 'Blue',
        'includeLegend' => true,
        'showTitle' => false,
        'title' => null,
        'highlightSelected' => true,
        'grayContext' => true,
        'zoomToFiltered' => true,
        'pointSymbol' => [
            'shape' => 'circle',
        ],
        // QGIS Project source
        'qgisProjectId' => null,
        'qgisLayer' => null,
        'qgisMapTheme' => null,
        'qgisStyle' => null,
        // GeoServer Layer source (catalog via GeoVault; base URL snapshotted for WMS)
        'geoserverConnectionId' => null,
        'geoserverBaseUrl' => null,
        'geoserverWorkspace' => null,
        'geoserverLayer' => null,
        'geoserverStyle' => null,
        /**
         * Optional report→renderer parameter renames for QGIS Server.
         * Empty = forward every report parameter under its original name.
         *
         * @var list<array{reportParameter:string,rendererParameter:string}>
         */
        'parameterMappings' => [],
        /**
         * Declarative QGIS FILTER mappings (report parameter → layer field).
         * Empty = no FILTER.
         *
         * @var list<array{reportParameter:string,layerField:string}>
         */
        'qgisFilterMappings' => [],
        /**
         * Declarative GeoServer CQL filter mappings.
         * Empty = no CQL_FILTER.
         *
         * @var list<array{reportParameter:string,geoserverField:string,operator:string}>
         */
        'geoserverFilterMappings' => [],
    ];
}

/** Supported GeoServer CQL filter operators (UI + runtime). */
const JRS_GEOSERVER_FILTER_OPERATORS = [
    '=' => '=',
    '<>' => '<>',
    '>' => '>',
    '>=' => '>=',
    '<' => '<',
    '<=' => '<=',
    'LIKE' => 'LIKE',
    'ILIKE' => 'ILIKE',
    'IN' => 'IN',
];

/**
 * @param mixed $raw
 * @return array<string,mixed>
 */
function jrs_normalize_embedded_map($raw): array
{
    $base = jrs_default_embedded_map();
    if (!is_array($raw)) {
        return $base;
    }

    $out = $base;
    $out['enabled'] = !empty($raw['enabled']);

    $out['band'] = jrs_normalize_embedded_map_band((string) ($raw['band'] ?? $base['band']));

    $groupName = trim((string) ($raw['groupName'] ?? $raw['group_name'] ?? ''));
    $out['groupName'] = $groupName !== '' ? $groupName : null;

    $out['width'] = max(40, min(2000, (int) ($raw['width'] ?? $base['width'])));
    $out['height'] = max(40, min(2000, (int) ($raw['height'] ?? $base['height'])));
    $out['x'] = max(0, min(2000, (int) ($raw['x'] ?? $base['x'])));
    $out['y'] = max(0, min(2000, (int) ($raw['y'] ?? $base['y'])));
    $out['padding'] = max(0.0, min(0.4, (float) ($raw['padding'] ?? $base['padding'])));

    $bg = trim((string) ($raw['background'] ?? $base['background']));
    if (strtolower($bg) === 'transparent') {
        $out['background'] = 'transparent';
    } elseif (preg_match('/^#?[0-9A-Fa-f]{6}$/', $bg)) {
        $out['background'] = '#' . ltrim($bg, '#');
    } else {
        $out['background'] = (string) $base['background'];
    }

    $dpi = (int) ($raw['dpi'] ?? $base['dpi']);
    $out['dpi'] = max(72, min(600, $dpi > 0 ? $dpi : 96));

    $renderer = strtolower(trim((string) ($raw['renderer'] ?? $raw['mapSource'] ?? $raw['map_source'] ?? $base['renderer'])));
    if ($renderer === '' || !in_array($renderer, jrs_map_renderer_ids(), true)) {
        $renderer = JRS_MAP_RENDERER_POSTGIS;
    }
    $out['renderer'] = $renderer;

    $param = trim((string) ($raw['parameterName'] ?? $raw['parameter_name'] ?? $base['parameterName']));
    if ($param === '' || !preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $param)) {
        $param = JRS_EMBEDDED_MAP_PARAM;
    }
    $out['parameterName'] = $param;

    $mapType = strtolower(str_replace([' ', '-'], '_', trim((string) ($raw['mapType'] ?? $raw['map_type'] ?? $base['mapType']))));
    if ($mapType === 'graduated') {
        $mapType = 'graduated_symbols';
    }
    if ($mapType === 'overview') {
        $mapType = 'overview_highlight';
    }
    if ($mapType === 'selected') {
        $mapType = 'selected_features';
    }
    $out['mapType'] = array_key_exists($mapType, JRS_EMBEDDED_MAP_TYPES) ? $mapType : 'locator';

    $field = trim((string) ($raw['field'] ?? ''));
    $out['field'] = ($field !== '' && jrs_is_safe_field_key($field)) ? $field : null;

    $classification = strtolower(str_replace([' ', '-'], '_', trim((string) ($raw['classification'] ?? $base['classification']))));
    if ($classification === 'equalinterval') {
        $classification = 'equal_interval';
    }
    if ($classification === 'naturalbreaks') {
        $classification = 'natural_breaks';
    }
    $out['classification'] = array_key_exists($classification, JRS_THEMATIC_CLASSIFICATIONS)
        ? $classification
        : 'quantile';

    $classes = (int) ($raw['classes'] ?? $base['classes']);
    $out['classes'] = max(3, min(7, $classes > 0 ? $classes : 5));

    $out['colorRamp'] = jrs_normalize_thematic_color_ramp((string) ($raw['colorRamp'] ?? $raw['color_ramp'] ?? $base['colorRamp']));

    $out['includeLegend'] = array_key_exists('includeLegend', $raw)
        ? !empty($raw['includeLegend'])
        : (array_key_exists('include_legend', $raw) ? !empty($raw['include_legend']) : true);
    $out['showTitle'] = array_key_exists('showTitle', $raw)
        ? !empty($raw['showTitle'])
        : (array_key_exists('show_title', $raw) ? !empty($raw['show_title']) : false);

    $title = trim((string) ($raw['title'] ?? ''));
    $out['title'] = $title !== '' ? $title : null;

    $out['highlightSelected'] = array_key_exists('highlightSelected', $raw)
        ? !empty($raw['highlightSelected'])
        : (array_key_exists('highlight_selected', $raw) ? !empty($raw['highlight_selected']) : true);
    $out['grayContext'] = array_key_exists('grayContext', $raw)
        ? !empty($raw['grayContext'])
        : (array_key_exists('gray_context', $raw) ? !empty($raw['gray_context']) : true);
    $out['zoomToFiltered'] = array_key_exists('zoomToFiltered', $raw)
        ? !empty($raw['zoomToFiltered'])
        : (array_key_exists('zoom_to_filtered', $raw) ? !empty($raw['zoom_to_filtered']) : true);

    $pointSymbol = is_array($raw['pointSymbol'] ?? $raw['point_symbol'] ?? null) ? ($raw['pointSymbol'] ?? $raw['point_symbol']) : [];
    $out['pointSymbol'] = [
        'shape' => jrs_normalize_point_shape((string) ($pointSymbol['shape'] ?? $base['pointSymbol']['shape'])),
    ];

    $out['qgisProjectId'] = jrs_embedded_map_optional_string(
        $raw['qgisProjectId'] ?? $raw['qgis_project_id'] ?? null
    );
    $out['qgisLayer'] = jrs_embedded_map_optional_string(
        $raw['qgisLayer'] ?? $raw['qgis_layer'] ?? null
    );
    $out['qgisMapTheme'] = jrs_embedded_map_optional_string(
        $raw['qgisMapTheme'] ?? $raw['qgis_map_theme'] ?? null
    );
    $out['qgisStyle'] = jrs_embedded_map_optional_string(
        $raw['qgisStyle'] ?? $raw['qgis_style'] ?? null
    );

    $out['geoserverConnectionId'] = null;
    $connId = (int) ($raw['geoserverConnectionId'] ?? $raw['geoserver_connection_id'] ?? 0);
    if ($connId > 0) {
        $out['geoserverConnectionId'] = $connId;
    }
    $out['geoserverBaseUrl'] = jrs_embedded_map_optional_string(
        $raw['geoserverBaseUrl'] ?? $raw['geoserver_base_url'] ?? null
    );
    $out['geoserverWorkspace'] = jrs_embedded_map_optional_string(
        $raw['geoserverWorkspace'] ?? $raw['geoserver_workspace'] ?? null
    );
    $out['geoserverLayer'] = jrs_embedded_map_optional_string(
        $raw['geoserverLayer'] ?? $raw['geoserver_layer'] ?? null
    );
    $out['geoserverStyle'] = jrs_embedded_map_optional_string(
        $raw['geoserverStyle'] ?? $raw['geoserver_style'] ?? null
    );

    $out['parameterMappings'] = jrs_normalize_parameter_mappings(
        $raw['parameterMappings'] ?? $raw['parameter_mappings'] ?? []
    );
    $out['qgisFilterMappings'] = jrs_normalize_qgis_filter_mappings(
        $raw['qgisFilterMappings'] ?? $raw['qgis_filter_mappings'] ?? []
    );
    $out['geoserverFilterMappings'] = jrs_normalize_geoserver_filter_mappings(
        $raw['geoserverFilterMappings'] ?? $raw['geoserver_filter_mappings'] ?? []
    );

    return $out;
}

/**
 * Normalize optional report→renderer parameter mappings.
 *
 * @param mixed $raw
 * @return list<array{reportParameter:string,rendererParameter:string}>
 */
function jrs_normalize_parameter_mappings($raw): array
{
    if (!is_array($raw)) {
        return [];
    }

    $out = [];
    foreach ($raw as $row) {
        if (!is_array($row)) {
            continue;
        }
        $reportParam = trim((string) ($row['reportParameter'] ?? $row['report_parameter'] ?? $row['from'] ?? ''));
        $rendererParam = trim((string) ($row['rendererParameter'] ?? $row['renderer_parameter'] ?? $row['to'] ?? ''));
        if ($reportParam === '' || $rendererParam === '') {
            continue;
        }
        if (!preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $reportParam)) {
            continue;
        }
        // Renderer names are unrestricted (e.g. region, VIEWPARAMS:state, MAP_THEME).
        $out[] = [
            'reportParameter' => $reportParam,
            'rendererParameter' => $rendererParam,
        ];
    }

    return $out;
}

/**
 * Validate parameter mappings against known report parameter names.
 *
 * @param list<array{reportParameter:string,rendererParameter:string}> $mappings
 * @param list<string> $reportParamNames
 * @return list<string> human-readable errors (empty = valid)
 */
function jrs_validate_parameter_mappings(array $mappings, array $reportParamNames): array
{
    $known = [];
    foreach ($reportParamNames as $name) {
        $n = trim((string) $name);
        if ($n !== '') {
            $known[$n] = true;
        }
    }

    $errors = [];
    $seenReport = [];
    $seenRenderer = [];
    foreach ($mappings as $i => $row) {
        if (!is_array($row)) {
            continue;
        }
        $reportParam = trim((string) ($row['reportParameter'] ?? ''));
        $rendererParam = trim((string) ($row['rendererParameter'] ?? ''));
        $label = 'mapping #' . ($i + 1);

        if ($reportParam === '') {
            $errors[] = 'Parameter Mapping: ' . $label . ' is missing a report parameter.';
            continue;
        }
        if ($rendererParam === '') {
            $errors[] = 'Parameter Mapping: ' . $label . ' is missing a renderer parameter.';
            continue;
        }
        if ($known !== [] && !isset($known[$reportParam])) {
            $errors[] = 'Parameter Mapping: "' . $reportParam . '" is not a report parameter.';
        }
        if (isset($seenReport[$reportParam])) {
            $errors[] = 'Parameter Mapping: report parameter "' . $reportParam . '" is mapped more than once.';
        }
        $seenReport[$reportParam] = true;
        if (isset($seenRenderer[$rendererParam])) {
            $errors[] = 'Parameter Mapping: renderer parameter "' . $rendererParam . '" is used more than once.';
        }
        $seenRenderer[$rendererParam] = true;
    }

    return $errors;
}

/**
 * Normalize declarative QGIS FILTER mappings (report parameter → layer field).
 *
 * @param mixed $raw
 * @return list<array{reportParameter:string,layerField:string}>
 */
function jrs_normalize_qgis_filter_mappings($raw): array
{
    if (!is_array($raw)) {
        return [];
    }

    $out = [];
    foreach ($raw as $row) {
        if (!is_array($row)) {
            continue;
        }
        $reportParam = trim((string) ($row['reportParameter'] ?? $row['report_parameter'] ?? ''));
        $field = trim((string) ($row['layerField'] ?? $row['layer_field'] ?? $row['field'] ?? ''));
        if ($reportParam === '' || $field === '') {
            continue;
        }
        if (!preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $reportParam)) {
            continue;
        }
        if (!preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $field)) {
            continue;
        }
        $out[] = [
            'reportParameter' => $reportParam,
            'layerField' => $field,
        ];
    }

    return $out;
}

/**
 * Validate QGIS filter mappings.
 *
 * @param list<array{reportParameter:string,layerField:string}> $mappings
 * @param list<string> $reportParamNames
 * @return list<string>
 */
function jrs_validate_qgis_filter_mappings(array $mappings, array $reportParamNames): array
{
    $known = [];
    foreach ($reportParamNames as $name) {
        $n = trim((string) $name);
        if ($n !== '') {
            $known[$n] = true;
        }
    }

    $errors = [];
    $seenReport = [];
    foreach ($mappings as $i => $row) {
        if (!is_array($row)) {
            continue;
        }
        $reportParam = trim((string) ($row['reportParameter'] ?? ''));
        $field = trim((string) ($row['layerField'] ?? ''));
        $label = 'filter mapping #' . ($i + 1);

        if ($reportParam === '') {
            $errors[] = 'QGIS Filter Mapping: ' . $label . ' is missing a report parameter.';
            continue;
        }
        if ($field === '') {
            $errors[] = 'QGIS Filter Mapping: ' . $label . ' is missing a layer field.';
            continue;
        }
        if (!preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $field)) {
            $errors[] = 'QGIS Filter Mapping: "' . $field . '" is not a valid field name.';
        }
        if ($known !== [] && !isset($known[$reportParam])) {
            $errors[] = 'QGIS Filter Mapping: "' . $reportParam . '" is not a report parameter.';
        }
        if (isset($seenReport[$reportParam])) {
            $errors[] = 'QGIS Filter Mapping: report parameter "' . $reportParam . '" is mapped more than once.';
        }
        $seenReport[$reportParam] = true;
    }

    return $errors;
}

/**
 * Normalize declarative GeoServer CQL filter mappings.
 *
 * @param mixed $raw
 * @return list<array{reportParameter:string,geoserverField:string,operator:string}>
 */
function jrs_normalize_geoserver_filter_mappings($raw): array
{
    if (!is_array($raw)) {
        return [];
    }

    $out = [];
    foreach ($raw as $row) {
        if (!is_array($row)) {
            continue;
        }
        $reportParam = trim((string) ($row['reportParameter'] ?? $row['report_parameter'] ?? ''));
        $field = trim((string) ($row['geoserverField'] ?? $row['geoserver_field'] ?? $row['field'] ?? ''));
        if ($reportParam === '' || $field === '') {
            continue;
        }
        if (!preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $reportParam)) {
            continue;
        }
        if (!preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $field)) {
            continue;
        }
        $opRaw = trim((string) ($row['operator'] ?? $row['op'] ?? '='));
        $opUpper = strtoupper($opRaw);
        if (in_array($opUpper, ['LIKE', 'ILIKE', 'IN'], true)) {
            $opKey = $opUpper;
        } elseif (isset(JRS_GEOSERVER_FILTER_OPERATORS[$opRaw])) {
            $opKey = $opRaw;
        } else {
            $opKey = '=';
        }
        $out[] = [
            'reportParameter' => $reportParam,
            'geoserverField' => $field,
            'operator' => $opKey,
        ];
    }

    return $out;
}

/**
 * Validate GeoServer filter mappings.
 *
 * @param list<array{reportParameter:string,geoserverField:string,operator:string}> $mappings
 * @param list<string> $reportParamNames
 * @return list<string>
 */
function jrs_validate_geoserver_filter_mappings(array $mappings, array $reportParamNames): array
{
    $known = [];
    foreach ($reportParamNames as $name) {
        $n = trim((string) $name);
        if ($n !== '') {
            $known[$n] = true;
        }
    }

    $errors = [];
    $seenReport = [];
    foreach ($mappings as $i => $row) {
        if (!is_array($row)) {
            continue;
        }
        $reportParam = trim((string) ($row['reportParameter'] ?? ''));
        $field = trim((string) ($row['geoserverField'] ?? ''));
        $operator = trim((string) ($row['operator'] ?? ''));
        $label = 'filter mapping #' . ($i + 1);

        if ($reportParam === '') {
            $errors[] = 'GeoServer Filter Mapping: ' . $label . ' is missing a report parameter.';
            continue;
        }
        if ($field === '') {
            $errors[] = 'GeoServer Filter Mapping: ' . $label . ' is missing a GeoServer field.';
            continue;
        }
        if (!preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $field)) {
            $errors[] = 'GeoServer Filter Mapping: "' . $field . '" is not a valid field name.';
        }
        if ($operator === '' || !isset(JRS_GEOSERVER_FILTER_OPERATORS[$operator])) {
            $errors[] = 'GeoServer Filter Mapping: ' . $label . ' has an unsupported operator.';
        }
        if ($known !== [] && !isset($known[$reportParam])) {
            $errors[] = 'GeoServer Filter Mapping: "' . $reportParam . '" is not a report parameter.';
        }
        if (isset($seenReport[$reportParam])) {
            $errors[] = 'GeoServer Filter Mapping: report parameter "' . $reportParam . '" is mapped more than once.';
        }
        $seenReport[$reportParam] = true;
    }

    return $errors;
}

/**
 * @param mixed $value
 */
function jrs_embedded_map_optional_string($value): ?string
{
    $s = trim((string) ($value ?? ''));
    return $s !== '' ? $s : null;
}

/**
 * Parse embeddedMap from edit/publish POST.
 *
 * @param array<string,mixed> $post
 * @return array<string,mixed>
 */
function jrs_embedded_map_from_post(array $post): array
{
    $embedPost = $post['embedded_map'] ?? $post['embeddedMap'] ?? [];
    if (!is_array($embedPost)) {
        $embedPost = [];
    }

    return jrs_normalize_embedded_map([
        'enabled' => !empty($embedPost['enabled']),
        'band' => $embedPost['band'] ?? 'summary',
        'groupName' => $embedPost['groupName'] ?? $embedPost['group_name'] ?? null,
        'width' => $embedPost['width'] ?? 535,
        'height' => $embedPost['height'] ?? 300,
        'x' => $embedPost['x'] ?? 0,
        'y' => $embedPost['y'] ?? 10,
        'padding' => $embedPost['padding'] ?? 0.08,
        'background' => $embedPost['background'] ?? '#ffffff',
        'dpi' => $embedPost['dpi'] ?? 96,
        'renderer' => $embedPost['renderer'] ?? $embedPost['mapSource'] ?? JRS_MAP_RENDERER_POSTGIS,
        'parameterName' => $embedPost['parameterName'] ?? JRS_EMBEDDED_MAP_PARAM,
        'mapType' => $embedPost['mapType'] ?? $embedPost['map_type'] ?? 'locator',
        'field' => $embedPost['field'] ?? null,
        'classification' => $embedPost['classification'] ?? 'quantile',
        'classes' => $embedPost['classes'] ?? 5,
        'colorRamp' => $embedPost['colorRamp'] ?? $embedPost['color_ramp'] ?? 'Blue',
        'includeLegend' => !empty($embedPost['includeLegend']) || !empty($embedPost['include_legend']),
        'showTitle' => !empty($embedPost['showTitle']) || !empty($embedPost['show_title']),
        'title' => $embedPost['title'] ?? null,
        'highlightSelected' => !empty($embedPost['highlightSelected']) || !empty($embedPost['highlight_selected']),
        'grayContext' => !empty($embedPost['grayContext']) || !empty($embedPost['gray_context']),
        'zoomToFiltered' => !empty($embedPost['zoomToFiltered']) || !empty($embedPost['zoom_to_filtered']),
        'pointSymbol' => $embedPost['pointSymbol'] ?? $embedPost['point_symbol'] ?? ['shape' => 'circle'],
        'qgisProjectId' => $embedPost['qgisProjectId'] ?? $embedPost['qgis_project_id'] ?? null,
        'qgisLayer' => $embedPost['qgisLayer'] ?? $embedPost['qgis_layer'] ?? null,
        'qgisMapTheme' => $embedPost['qgisMapTheme'] ?? $embedPost['qgis_map_theme'] ?? null,
        'qgisStyle' => $embedPost['qgisStyle'] ?? $embedPost['qgis_style'] ?? null,
        'geoserverConnectionId' => $embedPost['geoserverConnectionId'] ?? $embedPost['geoserver_connection_id'] ?? null,
        'geoserverBaseUrl' => $embedPost['geoserverBaseUrl'] ?? $embedPost['geoserver_base_url'] ?? null,
        'geoserverWorkspace' => $embedPost['geoserverWorkspace'] ?? $embedPost['geoserver_workspace'] ?? null,
        'geoserverLayer' => $embedPost['geoserverLayer'] ?? $embedPost['geoserver_layer'] ?? null,
        'geoserverStyle' => $embedPost['geoserverStyle'] ?? $embedPost['geoserver_style'] ?? null,
        'parameterMappings' => $embedPost['parameterMappings'] ?? $embedPost['parameter_mappings'] ?? [],
        'qgisFilterMappings' => $embedPost['qgisFilterMappings'] ?? $embedPost['qgis_filter_mappings'] ?? [],
        'geoserverFilterMappings' => $embedPost['geoserverFilterMappings'] ?? $embedPost['geoserver_filter_mappings'] ?? [],
    ]);
}

/**
 * Effective embedded-map config, or null when disabled.
 *
 * @param array<string,mixed> $definition
 * @return array<string,mixed>|null
 */
function jrs_report_embedded_map(array $definition): ?array
{
    $raw = $definition['embeddedMap'] ?? $definition['embedded_map'] ?? null;
    $normalized = jrs_normalize_embedded_map($raw);
    return !empty($normalized['enabled']) ? $normalized : null;
}

/**
 * Canonical band id (Go normalizeBand). Do not blindly strtolower — groupHeader
 * would become groupheader and fall back to summary.
 */
function jrs_normalize_embedded_map_band(string $band): string
{
    $band = trim($band);
    if ($band !== '' && isset(JRS_EMBEDDED_MAP_BANDS[$band])) {
        return $band;
    }
    $folded = strtolower(str_replace([' ', '-'], '', $band));
    return match ($folded) {
        'summary' => 'summary',
        'detail' => 'detail',
        'groupheader', 'group_header', 'group' => 'groupHeader',
        default => 'summary',
    };
}

/** PostGIS/MySQL map in a group header — pre-render one PNG per group value. */
function jrs_grouped_spatial_map(array $embedded): bool
{
    if (jrs_normalize_embedded_map_band((string) ($embedded['band'] ?? '')) !== 'groupHeader') {
        return false;
    }
    $renderer = strtolower(trim((string) ($embedded['renderer'] ?? '')));
    return in_array($renderer, ['', 'postgis', 'mysql', JRS_MAP_RENDERER_POSTGIS], true);
}

function jrs_jrxml_uses_postgis_map_renderer(string $jrxmlPath): bool
{
    if (!is_file($jrxmlPath)) {
        return false;
    }
    $data = @file_get_contents($jrxmlPath);
    return is_string($data) && str_contains($data, 'com.acugis.map.MapRenderer.render(');
}

function jrs_jrxml_group_value_field(string $jrxmlPath, ?string $groupName): string
{
    $groupName = trim((string) $groupName);
    if (!is_file($jrxmlPath)) {
        return $groupName;
    }
    $xml = @simplexml_load_file($jrxmlPath);
    if ($xml === false) {
        return $groupName;
    }
    $fallback = '';
    foreach ($xml->xpath('//*[local-name()="group"]') ?: [] as $group) {
        $name = trim((string) ($group->attributes()['name'] ?? ''));
        $expr = '';
        foreach (['expression', 'groupExpression'] as $tag) {
            $nodes = $group->xpath('./*[local-name()="' . $tag . '"]');
            if (!empty($nodes)) {
                $expr = trim((string) $nodes[0]);
                break;
            }
        }
        if (preg_match('/\$F\{([A-Za-z_][A-Za-z0-9_]*)\}/', $expr, $m) !== 1) {
            continue;
        }
        $field = $m[1];
        if ($groupName === '' || $name === $groupName) {
            return $field;
        }
        if ($fallback === '') {
            $fallback = $field;
        }
    }
    if ($groupName !== '' && jrs_is_safe_field_key($groupName)) {
        return $groupName;
    }
    return $fallback;
}

function jrs_feature_group_key(array $feature, string $field): string
{
    $props = is_array($feature['properties'] ?? null) ? $feature['properties'] : [];
    $value = jrs_lookup_feature_prop($props, $field);
    if ($value === null) {
        return 'null';
    }
    $s = trim((string) $value);
    return $s === '' ? 'null' : $s;
}

/**
 * @param array<string,mixed> $props
 */
function jrs_lookup_feature_prop(array $props, string $field): mixed
{
    $field = trim($field);
    if ($field === '') {
        return null;
    }
    if (array_key_exists($field, $props)) {
        return $props[$field];
    }
    foreach ($props as $k => $v) {
        if (strcasecmp((string) $k, $field) === 0) {
            return $v;
        }
        $dot = strrpos((string) $k, '.');
        if ($dot !== false && strcasecmp(substr((string) $k, $dot + 1), $field) === 0) {
            return $v;
        }
    }
    $dot = strrpos($field, '.');
    if ($dot !== false) {
        return jrs_lookup_feature_prop($props, substr($field, $dot + 1));
    }
    return null;
}

/**
 * @param list<array<string,mixed>> $features
 * @return array<string, list<array<string,mixed>>>
 */
function jrs_split_features_by_group(array $features, string $field): array
{
    $out = [];
    foreach ($features as $feature) {
        if (!is_array($feature)) {
            continue;
        }
        $key = jrs_feature_group_key($feature, $field);
        $out[$key][] = $feature;
    }
    return $out;
}

function jrs_fs_remove(string $path): bool
{
    if (is_link($path) || is_file($path)) {
        return @unlink($path);
    }
    if (!is_dir($path)) {
        return true;
    }
    $items = @scandir($path);
    if ($items === false) {
        return false;
    }
    foreach ($items as $item) {
        if ($item === '.' || $item === '..') {
            continue;
        }
        if (!jrs_fs_remove($path . '/' . $item)) {
            return false;
        }
    }
    return @rmdir($path);
}

/**
 * Group names declared in a JRXML (for Group Header placement).
 *
 * @return list<string>
 */
function jrs_jrxml_group_names(string $jrxmlPath): array
{
    if (!is_file($jrxmlPath)) {
        return [];
    }
    $xml = @simplexml_load_file($jrxmlPath);
    if ($xml === false) {
        return [];
    }
    $names = [];
    foreach ($xml->xpath('//*[local-name()="group"]') ?: [] as $group) {
        $attrs = $group->attributes();
        $name = trim((string) ($attrs['name'] ?? ''));
        if ($name !== '') {
            $names[] = $name;
        }
    }
    return array_values(array_unique($names));
}

/**
 * Inject or remove the embedded map image element and parameter in JRXML.
 *
 * Uses JasperReports Studio 7 element syntax already present in this codebase.
 * Band height is always recalculated from the current layout (grow or shrink).
 *
 * @param array<string,mixed> $embeddedMap normalized config (enabled may be false → remove)
 * @return array{ok:bool,message:string,recompiled?:bool,jrxmlChanged?:bool}
 */
function jrs_jrxml_sync_embedded_map(string $jrxmlPath, array $embeddedMap): array
{
    if (!is_file($jrxmlPath)) {
        return ['ok' => false, 'message' => 'JRXML file not found.'];
    }
    if (!is_writable($jrxmlPath) && !is_writable(dirname($jrxmlPath))) {
        return ['ok' => false, 'message' => 'JRXML is not writable.'];
    }

    $xml = @simplexml_load_file($jrxmlPath);
    if ($xml === false) {
        return ['ok' => false, 'message' => 'Failed to parse JRXML.'];
    }

    $dom = dom_import_simplexml($xml);
    if ($dom === false || $dom->ownerDocument === null) {
        return ['ok' => false, 'message' => 'Failed to convert JRXML DOM.'];
    }
    $doc = $dom->ownerDocument;
    $doc->formatOutput = true;
    $root = $doc->documentElement;
    if ($root === null) {
        return ['ok' => false, 'message' => 'Invalid JRXML root.'];
    }

    $paramName = (string) ($embeddedMap['parameterName'] ?? JRS_EMBEDDED_MAP_PARAM);
    $enabled = !empty($embeddedMap['enabled']);
    $bottomPad = JRS_EMBEDDED_MAP_BAND_PADDING;

    $hadMap = jrs_jrxml_dom_has_embedded_map($doc, $paramName);

    if (!$enabled && !$hadMap) {
        return ['ok' => true, 'message' => 'No embedded map configured.', 'recompiled' => false];
    }

    // Remove all map images/params (dedupe). Recalculate former parent bands afterward.
    $touchedBands = jrs_jrxml_dom_remove_embedded_map($doc, $paramName);
    foreach ($touchedBands as $bandEl) {
        jrs_jrxml_band_fit_content($bandEl, $bottomPad);
    }

    if (!$enabled) {
        // Heal leftover inflation in detail/summary from earlier map placements.
        jrs_jrxml_refit_layout_bands($doc, $root, $bottomPad);
        if ($doc->save($jrxmlPath) === false) {
            return ['ok' => false, 'message' => 'Failed to write JRXML after removing map.'];
        }
        return ['ok' => true, 'message' => 'Embedded map removed from JRXML.', 'recompiled' => false, 'jrxmlChanged' => true];
    }

    // Ensure parameter exists (after other parameters if possible).
    $paramEl = $doc->createElement('parameter');
    $paramEl->setAttribute('name', $paramName);
    $paramEl->setAttribute('class', 'java.lang.String');
    $paramEl->setAttribute('forPrompting', 'false');
    $defaultExpr = $doc->createElement('defaultValueExpression');
    $defaultExpr->appendChild($doc->createCDATASection('""'));
    $paramEl->appendChild($defaultExpr);

    $insertBefore = null;
    foreach ($root->childNodes as $child) {
        if ($child->nodeType !== XML_ELEMENT_NODE) {
            continue;
        }
        $local = $child->localName ?? $child->nodeName;
        if (in_array($local, ['query', 'queryString', 'field', 'variable', 'group', 'background', 'title', 'pageHeader', 'columnHeader', 'detail', 'columnFooter', 'pageFooter', 'summary', 'lastPageFooter'], true)) {
            $insertBefore = $child;
            break;
        }
    }
    if ($insertBefore !== null) {
        $root->insertBefore($paramEl, $insertBefore);
    } else {
        $root->appendChild($paramEl);
    }

    $band = (string) ($embeddedMap['band'] ?? 'summary');
    $width = (int) ($embeddedMap['width'] ?? 535);
    $height = (int) ($embeddedMap['height'] ?? 300);
    $x = (int) ($embeddedMap['x'] ?? 0);
    $y = (int) ($embeddedMap['y'] ?? 10);

    $imageEl = $doc->createElement('element');
    $imageEl->setAttribute('kind', 'image');
    $imageEl->setAttribute('uuid', JRS_EMBEDDED_MAP_UUID);
    $imageEl->setAttribute('x', (string) $x);
    $imageEl->setAttribute('y', (string) $y);
    $imageEl->setAttribute('width', (string) $width);
    $imageEl->setAttribute('height', (string) $height);
    $imageEl->setAttribute('onErrorType', 'Blank');
    $expr = $doc->createElement('expression');
    $expr->appendChild($doc->createCDATASection('$P{' . $paramName . '}'));
    $imageEl->appendChild($expr);

    $targetBand = jrs_jrxml_dom_ensure_band($doc, $root, $band, (string) ($embeddedMap['groupName'] ?? ''), 0);
    if ($targetBand === null) {
        return ['ok' => false, 'message' => 'Could not locate or create target band: ' . $band];
    }

    $targetBand->appendChild($imageEl);

    // Shrink any band left oversized from earlier map placements (e.g. Detail after move to Summary).
    jrs_jrxml_refit_layout_bands($doc, $root, $bottomPad);
    // Exact fit for the map's band (grow or shrink).
    jrs_jrxml_band_fit_content($targetBand, $bottomPad);

    if ($doc->save($jrxmlPath) === false) {
        return ['ok' => false, 'message' => 'Failed to write JRXML with embedded map.'];
    }

    return [
        'ok' => true,
        'message' => 'Embedded map image inserted into JRXML (' . $band . ').',
        'recompiled' => false,
        'jrxmlChanged' => true,
    ];
}

function jrs_jrxml_dom_has_embedded_map(DOMDocument $doc, string $paramName): bool
{
    foreach ($doc->getElementsByTagName('parameter') as $param) {
        if ($param->getAttribute('name') === $paramName) {
            return true;
        }
    }
    foreach ($doc->getElementsByTagName('element') as $el) {
        if ($el->getAttribute('uuid') === JRS_EMBEDDED_MAP_UUID) {
            return true;
        }
        if ($el->getAttribute('kind') === 'image' && str_contains($el->textContent ?? '', '$P{' . $paramName . '}')) {
            return true;
        }
    }
    foreach ($doc->getElementsByTagName('image') as $el) {
        if (str_contains($el->textContent ?? '', '$P{' . $paramName . '}')) {
            return true;
        }
    }
    return false;
}

/**
 * Bottom edge (y + height) of a JRXML visual element (Studio 7 or classic).
 */
function jrs_jrxml_element_bottom(DOMElement $el): int
{
    if ($el->hasAttribute('y') || $el->hasAttribute('height')) {
        return max(0, (int) $el->getAttribute('y')) + max(0, (int) $el->getAttribute('height'));
    }
    foreach ($el->childNodes as $child) {
        if (!($child instanceof DOMElement)) {
            continue;
        }
        $local = $child->localName ?? $child->nodeName;
        if ($local === 'reportElement') {
            return max(0, (int) $child->getAttribute('y')) + max(0, (int) $child->getAttribute('height'));
        }
    }
    return 0;
}

/**
 * Required band height = max(element bottoms) + padding. Empty band → 0.
 */
function jrs_jrxml_band_required_height(DOMElement $band, int $padding = JRS_EMBEDDED_MAP_BAND_PADDING): int
{
    $maxBottom = 0;
    foreach ($band->childNodes as $child) {
        if (!($child instanceof DOMElement)) {
            continue;
        }
        $local = $child->localName ?? $child->nodeName;
        // Non-visual / metadata children.
        if (in_array($local, ['printWhenExpression', 'property', 'propertyExpression', 'break'], true)) {
            continue;
        }
        $maxBottom = max($maxBottom, jrs_jrxml_element_bottom($child));
    }
    if ($maxBottom <= 0) {
        return 0;
    }
    return $maxBottom + max(0, $padding);
}

/**
 * Set band height exactly to content (grow or shrink).
 */
function jrs_jrxml_band_fit_content(DOMElement $band, int $padding = JRS_EMBEDDED_MAP_BAND_PADDING): void
{
    $band->setAttribute('height', (string) jrs_jrxml_band_required_height($band, $padding));
}

/**
 * Refit every content band/section that can retain leftover height after map moves
 * (detail, summary, title, headers, group bands, etc.).
 *
 * Shrinks oversized bands to content; does not grow bands that are already tight
 * (avoids adding padding to normal detail rows on every save).
 */
function jrs_jrxml_refit_layout_bands(DOMDocument $doc, DOMElement $root, int $padding = JRS_EMBEDDED_MAP_BAND_PADDING): void
{
    $seen = [];
    $fitShrink = static function (DOMElement $el) use (&$seen, $padding): void {
        $id = spl_object_id($el);
        if (isset($seen[$id])) {
            return;
        }
        $seen[$id] = true;
        $required = jrs_jrxml_band_required_height($el, $padding);
        $current = (int) $el->getAttribute('height');
        if ($current > $required) {
            $el->setAttribute('height', (string) $required);
        }
    };

    foreach ($doc->getElementsByTagName('band') as $bandEl) {
        if ($bandEl instanceof DOMElement) {
            $fitShrink($bandEl);
        }
    }

    foreach ($root->childNodes as $child) {
        if (!($child instanceof DOMElement) || !jrs_jrxml_is_band_container($child)) {
            continue;
        }
        $hasNestedBand = false;
        foreach ($child->childNodes as $inner) {
            if ($inner instanceof DOMElement && ($inner->localName ?? $inner->nodeName) === 'band') {
                $hasNestedBand = true;
                break;
            }
        }
        if (!$hasNestedBand) {
            $fitShrink($child);
        }
    }
}

/**
 * Remove previously injected map parameter and all matching image elements (dedupe).
 *
 * @return list<DOMElement> parent band/section elements that lost children (for height refit)
 */
function jrs_jrxml_dom_remove_embedded_map(DOMDocument $doc, string $paramName): array
{
    $root = $doc->documentElement;
    if ($root === null) {
        return [];
    }

    $toRemove = [];
    foreach ($root->getElementsByTagName('parameter') as $param) {
        if ($param->getAttribute('name') === $paramName) {
            $toRemove[] = $param;
        }
    }
    foreach ($root->getElementsByTagName('element') as $el) {
        if ($el->getAttribute('uuid') === JRS_EMBEDDED_MAP_UUID
            || ($el->getAttribute('kind') === 'image' && str_contains($el->textContent ?? '', '$P{' . $paramName . '}'))
        ) {
            $toRemove[] = $el;
        }
    }
    // Classic JRXML <image> with expression containing $P{P_MAP_IMAGE}.
    foreach ($root->getElementsByTagName('image') as $el) {
        if (str_contains($el->textContent ?? '', '$P{' . $paramName . '}')) {
            $toRemove[] = $el;
        }
    }

    $touched = [];
    $seen = [];
    foreach ($toRemove as $node) {
        $parent = $node->parentNode;
        if ($parent instanceof DOMElement && jrs_jrxml_is_band_container($parent)) {
            $id = spl_object_id($parent);
            if (!isset($seen[$id])) {
                $seen[$id] = true;
                $touched[] = $parent;
            }
        }
        if ($node->parentNode !== null) {
            $node->parentNode->removeChild($node);
        }
    }
    return $touched;
}

/**
 * True when the element is a band or Studio-7 section that carries a height attribute.
 */
function jrs_jrxml_is_band_container(DOMElement $el): bool
{
    $local = $el->localName ?? $el->nodeName;
    if ($local === 'band') {
        return true;
    }
    return in_array($local, [
        'summary', 'title', 'pageHeader', 'pageFooter',
        'columnHeader', 'columnFooter', 'lastPageFooter',
        'background', 'noData',
    ], true);
}

/**
 * Ensure the target band element exists and return it (the <band> for detail/groupHeader,
 * or the section element itself when it holds children directly like <summary height="...">).
 *
 * Does not permanently enlarge existing bands — callers must call jrs_jrxml_band_fit_content().
 */
function jrs_jrxml_dom_ensure_band(
    DOMDocument $doc,
    DOMElement $root,
    string $band,
    string $groupName,
    int $initialHeight = 0
): ?DOMElement {
    if ($band === 'groupHeader') {
        $groupEl = null;
        foreach ($root->getElementsByTagName('group') as $g) {
            if (!($g instanceof DOMElement)) {
                continue;
            }
            if ($groupName === '' || $g->getAttribute('name') === $groupName) {
                $groupEl = $g;
                break;
            }
        }
        if (!($groupEl instanceof DOMElement)) {
            return null;
        }
        $gh = null;
        foreach ($groupEl->childNodes as $child) {
            if ($child instanceof DOMElement && ($child->localName ?? $child->nodeName) === 'groupHeader') {
                $gh = $child;
                break;
            }
        }
        if ($gh === null) {
            $gh = $doc->createElement('groupHeader');
            $groupEl->appendChild($gh);
        }
        $bandEl = null;
        foreach ($gh->childNodes as $child) {
            if ($child instanceof DOMElement && ($child->localName ?? $child->nodeName) === 'band') {
                $bandEl = $child;
                break;
            }
        }
        if ($bandEl === null) {
            $bandEl = $doc->createElement('band');
            $bandEl->setAttribute('height', (string) max(0, $initialHeight));
            $gh->appendChild($bandEl);
        }
        return $bandEl;
    }

    if ($band === 'detail') {
        $detail = null;
        foreach ($root->childNodes as $child) {
            if ($child instanceof DOMElement && ($child->localName ?? $child->nodeName) === 'detail') {
                $detail = $child;
                break;
            }
        }
        if ($detail === null) {
            $detail = $doc->createElement('detail');
            $root->appendChild($detail);
        }
        $bandEl = null;
        foreach ($detail->childNodes as $child) {
            if ($child instanceof DOMElement && ($child->localName ?? $child->nodeName) === 'band') {
                $bandEl = $child;
                break;
            }
        }
        if ($bandEl === null) {
            $bandEl = $doc->createElement('band');
            $bandEl->setAttribute('height', (string) max(0, $initialHeight));
            $detail->appendChild($bandEl);
        }
        return $bandEl;
    }

    // summary (default) — Studio 7 often uses <summary height="N"> with direct children.
    $summary = null;
    foreach ($root->childNodes as $child) {
        if ($child instanceof DOMElement && ($child->localName ?? $child->nodeName) === 'summary') {
            $summary = $child;
            break;
        }
    }
    if ($summary === null) {
        $summary = $doc->createElement('summary');
        $summary->setAttribute('height', (string) max(0, $initialHeight));
        $root->appendChild($summary);
        return $summary;
    }

    // Prefer nested <band> if present; otherwise use summary itself (Studio 7 style).
    foreach ($summary->childNodes as $child) {
        if ($child instanceof DOMElement && ($child->localName ?? $child->nodeName) === 'band') {
            return $child;
        }
    }

    if (!$summary->hasAttribute('height')) {
        $summary->setAttribute('height', (string) max(0, $initialHeight));
    }

    return $summary;
}

/**
 * Request Tomcat to recompile this JRXML → .jasper.
 * Kept for callers that only have a filesystem path; Tomcat is the sole compiler.
 */
function jrs_jrxml_invalidate_compiled(string $jrxmlPath): bool
{
    require_once __DIR__ . '/report_compiler.php';

    $base = basename($jrxmlPath);
    $key = preg_replace('/\.jrxml$/i', '', $base) ?? '';
    $key = jrs_report_key((string) $key);
    if (!jrs_is_valid_report_key($key)) {
        error_log('jrs-compile skip: could not derive report key from ' . $jrxmlPath);
        return false;
    }

    $result = jrs_report_compile_force($key);
    return !empty($result['ok']);
}

/**
 * @param list<string> $names
 */
function jrs_find_executable(array $names): ?string
{
    foreach ($names as $name) {
        $out = [];
        $exit = 0;
        if (strncasecmp(PHP_OS, 'WIN', 3) === 0) {
            @exec('where ' . escapeshellarg($name) . ' 2>nul', $out, $exit);
        } else {
            @exec('command -v ' . escapeshellarg($name) . ' 2>/dev/null', $out, $exit);
        }
        if ($exit === 0 && isset($out[0]) && trim($out[0]) !== '') {
            return trim($out[0]);
        }
    }
    return null;
}

/**
 * @param list<string> $parts
 */
function jrs_java_classpath(array $parts): string
{
    $sep = strncasecmp(PHP_OS, 'WIN', 3) === 0 ? ';' : ':';
    return implode($sep, $parts);
}

/**
 * True when any report parameter in the filter context has a usable value.
 *
 * @param array<string,mixed> $filterContext
 */
function jrs_filter_context_is_active(array $filterContext): bool
{
    foreach ($filterContext as $value) {
        if ($value === null || $value === '') {
            continue;
        }
        if (is_array($value) && $value === []) {
            continue;
        }
        return true;
    }
    return false;
}

/**
 * Host path under reports/ for cached map PNGs (shared with Tomcat via volume).
 */
function jrs_embedded_map_cache_dir(): string
{
    $dir = jrs_report_files_dir() . '/_map_cache';
    jrs_ensure_dir($dir);
    return $dir;
}

/**
 * Convert a host path under the reports volume to the path Tomcat sees.
 */
function jrs_embedded_map_tomcat_path(string $hostPath): string
{
    $reportsDir = realpath(jrs_report_files_dir()) ?: jrs_report_files_dir();
    return jrs_translate_host_path_for_tomcat(
        $hostPath,
        $reportsDir,
        jrs_jasper_reports_container_dir()
    );
}

/**
 * Build a MapRequest from report definition + runtime parameters.
 *
 * Report generation only needs this + MapRenderer — never branches on source.
 *
 * @param array<string,mixed> $definition
 * @param array<string,mixed> $queryParams
 */
function jrs_build_map_request(string $reportKey, array $definition, array $queryParams): MapRequest
{
    $embedded = jrs_report_embedded_map($definition);
    if ($embedded === null) {
        throw new RuntimeException('Embedded map is not enabled for this report.');
    }

    if (trim((string) ($definition['report'] ?? '')) === '') {
        $definition['report'] = $reportKey;
    }

    $mapDef = jrs_normalize_map_definition($definition['mapDefinition'] ?? $definition['map_definition'] ?? null);
    $jrxmlPath = jrs_report_jrxml_path($reportKey);
    $jrxmlParams = jrs_jrxml_parameters($jrxmlPath);
    $reportParams = is_array($definition['parameters'] ?? null) ? $definition['parameters'] : [];
    $filterContext = jrs_collect_filter_context($queryParams, $jrxmlParams, $reportParams);
    unset($filterContext[$embedded['parameterName']], $filterContext[JRS_EMBEDDED_MAP_PARAM]);

    $request = new MapRequest();
    $request->source = (string) ($embedded['renderer'] ?? JRS_MAP_RENDERER_POSTGIS);
    $request->width = (int) ($embedded['width'] ?? 535);
    $request->height = (int) ($embedded['height'] ?? 300);
    $request->padding = (float) ($embedded['padding'] ?? 0.08);
    $request->background = (string) ($embedded['background'] ?? '#ffffff');
    $request->dpi = (int) ($embedded['dpi'] ?? 96);
    $request->includeLegend = !empty($embedded['includeLegend']);
    $request->showTitle = !empty($embedded['showTitle']);
    $request->title = isset($embedded['title']) ? (string) $embedded['title'] : null;
    $request->highlightSelected = !empty($embedded['highlightSelected']);
    $request->grayContext = !empty($embedded['grayContext']);
    $request->zoomToFiltered = !empty($embedded['zoomToFiltered']);
    $request->mapType = (string) ($embedded['mapType'] ?? 'locator');
    $request->field = isset($embedded['field']) ? (string) $embedded['field'] : null;
    $request->classification = (string) ($embedded['classification'] ?? 'quantile');
    $request->classes = (int) ($embedded['classes'] ?? 5);
    $request->colorRamp = (string) ($embedded['colorRamp'] ?? 'Blue');
    $request->parameters = $filterContext;
    $request->filterContext = $filterContext;
    $request->jrxmlParams = $jrxmlParams;
    $request->reportKey = $reportKey;
    $request->definition = $definition;
    $request->reportName = trim((string) ($definition['name'] ?? $reportKey));
    $request->singleSymbol = is_array($mapDef['singleSymbol'] ?? null) ? $mapDef['singleSymbol'] : [];
    $request->pointSymbol = is_array($embedded['pointSymbol'] ?? null) ? $embedded['pointSymbol'] : ['shape' => 'circle'];
    $request->datasource = trim((string) ($definition['datasource'] ?? $definition['dataSource'] ?? ''));
    $request->reportSql = jrs_jrxml_sql_query($jrxmlPath);

    $request->qgisProjectId = (string) ($embedded['qgisProjectId'] ?? '');
    $request->qgisLayer = (string) ($embedded['qgisLayer'] ?? '');
    $request->qgisMapTheme = (string) ($embedded['qgisMapTheme'] ?? '');
    $request->qgisStyle = (string) ($embedded['qgisStyle'] ?? '');

    $request->geoserverConnectionId = (int) ($embedded['geoserverConnectionId'] ?? 0);
    $request->geoserverBaseUrl = (string) ($embedded['geoserverBaseUrl'] ?? '');
    $request->geoserverWorkspace = (string) ($embedded['geoserverWorkspace'] ?? '');
    $request->geoserverLayer = (string) ($embedded['geoserverLayer'] ?? '');
    $request->geoserverStyle = (string) ($embedded['geoserverStyle'] ?? '');
    $request->parameterMappings = is_array($embedded['parameterMappings'] ?? null)
        ? $embedded['parameterMappings']
        : [];
    $request->qgisFilterMappings = is_array($embedded['qgisFilterMappings'] ?? null)
        ? $embedded['qgisFilterMappings']
        : [];
    $request->geoserverFilterMappings = is_array($embedded['geoserverFilterMappings'] ?? null)
        ? $embedded['geoserverFilterMappings']
        : [];

    // Layer resolution is best-effort — required for PostGIS, optional for WMS sources.
    $probe = $definition;
    $probe['mapDefinition'] = $mapDef;
    $probe['type'] = JRS_REPORT_TYPE_MAP;
    $layer = jrs_resolve_map_layer($probe);
    if (($layer['error'] ?? null) === null && !empty($layer['table']) && !empty($layer['geometry_column'])) {
        $request->layer = $layer;
    }

    return $request;
}

/**
 * Generate map PNG via MapRenderer factory, return Jasper parameter binding.
 *
 * @param array<string,mixed> $definition report JSON
 * @param array<string,mixed> $queryParams from jrs_parse_query_string_multi() or schedule params
 * @return array{param:string,value:string,hostPath:string}|array{error:string}
 */
function jrs_prepare_embedded_map_image(string $reportKey, array $definition, array $queryParams): array
{
    $embedded = jrs_report_embedded_map($definition);
    if ($embedded === null) {
        return ['error' => 'Embedded map is not enabled for this report.'];
    }

    try {
        $request = jrs_build_map_request($reportKey, $definition, $queryParams);
        $renderer = jrs_map_renderer($request->source);
        $cacheDir = jrs_embedded_map_cache_dir();
        $cacheKey = $reportKey . '|' . json_encode($request->cacheFingerprint());
        $hash = substr(hash('sha256', $cacheKey), 0, 16);

        $provider = strtolower(trim((string) ($embedded['renderer'] ?? $request->source)));
        $grouped = jrs_grouped_spatial_map($embedded)
            && $provider !== JRS_MAP_RENDERER_GEOSERVER
            && $provider !== JRS_MAP_RENDERER_QGIS;

        if ($grouped) {
            if (method_exists($renderer, 'loadGeojson')) {
                $renderer->loadGeojson($request);
            } else {
                $renderer->renderMap($request);
            }
            $features = is_array($request->geojson['features'] ?? null) ? $request->geojson['features'] : [];
            $jrxmlPath = jrs_report_jrxml_path($reportKey);
            $field = jrs_jrxml_group_value_field($jrxmlPath, $embedded['groupName'] ?? null);
            if ($field === '') {
                $field = trim((string) ($embedded['groupName'] ?? ''));
            }
            $groups = jrs_split_features_by_group($features, $field);
            $hostDir = $cacheDir . '/' . $reportKey . '_' . $hash;
            if (!jrs_fs_remove($hostDir)) {
                return ['error' => 'Failed to write embedded map PNG cache.'];
            }
            if (!jrs_ensure_dir($hostDir)) {
                return ['error' => 'Failed to write embedded map PNG cache.'];
            }
            foreach ($groups as $key => $feats) {
                $groupReq = clone $request;
                $groupReq->geojson = ['type' => 'FeatureCollection', 'features' => $feats];
                $groupReq->contextFeatures = [];
                $groupReq->grayContext = false;
                $groupReq->zoomToFiltered = true;
                $png = $renderer->renderMap($groupReq);
                $pngPath = $hostDir . '/' . $key . '.png';
                if (file_put_contents($pngPath, $png) === false) {
                    return ['error' => 'Failed to write embedded map PNG cache.'];
                }
            }
            $hostPath = $hostDir;
        } else {
            $png = $renderer->renderMap($request);
            $hostPath = $cacheDir . '/' . $reportKey . '_' . $hash . '.png';
            if (file_put_contents($hostPath, $png) === false) {
                return ['error' => 'Failed to write embedded map PNG cache.'];
            }
        }
    } catch (Throwable $e) {
        return ['error' => $e->getMessage()];
    }

    $tomcatPath = jrs_embedded_map_tomcat_path($hostPath);
    if ($tomcatPath === '' || !jrs_is_absolute_fs_path($tomcatPath)) {
        $abs = realpath($hostPath);
        $tomcatPath = str_replace('\\', '/', $abs !== false ? $abs : $hostPath);
    }
    return [
        'param' => (string) $embedded['parameterName'],
        'value' => $tomcatPath,
        'hostPath' => $hostPath,
    ];
}

function jrs_is_absolute_fs_path(string $path): bool
{
    $path = trim($path);
    if ($path === '') {
        return false;
    }
    if (str_starts_with($path, '/')) {
        return true;
    }
    return strlen($path) >= 3 && $path[1] === ':' && ($path[2] === '\\' || $path[2] === '/');
}

/**
 * Append embedded-map image parameter to a query string (or build from array).
 *
 * @param array<string,mixed> $definition
 * @param array<string,mixed> $queryParams
 * @return array{query:string,error?:string}
 */
function jrs_append_embedded_map_to_query(string $reportKey, array $definition, string $queryString, array $queryParams): array
{
    if (jrs_report_embedded_map($definition) === null) {
        return ['query' => $queryString];
    }

    $jrxmlPath = jrs_report_jrxml_path($reportKey);
    // Designer-published MapRenderer images render at fill time; do not overlay P_MAP_IMAGE.
    if (jrs_jrxml_uses_postgis_map_renderer($jrxmlPath)) {
        return ['query' => $queryString];
    }

    $prepared = jrs_prepare_embedded_map_image($reportKey, $definition, $queryParams);
    if (isset($prepared['error'])) {
        return ['query' => $queryString, 'error' => (string) $prepared['error']];
    }

    $param = rawurlencode((string) $prepared['param']);
    $value = rawurlencode((string) $prepared['value']);
    $pair = $param . '=' . $value;
    if ($queryString === '') {
        return ['query' => $pair];
    }
    // Replace existing param if present.
    $filtered = [];
    foreach (explode('&', $queryString) as $part) {
        if ($part === '') {
            continue;
        }
        $key = urldecode(explode('=', $part, 2)[0]);
        if ($key === (string) $prepared['param']) {
            continue;
        }
        $filtered[] = $part;
    }
    $filtered[] = $pair;
    return ['query' => implode('&', $filtered)];
}
