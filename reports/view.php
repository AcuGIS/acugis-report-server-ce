<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';
if (is_file(__DIR__ . '/../lib/map_report.php')) {
    require_once __DIR__ . '/../lib/map_report.php';
}

$cconn = jrs_auth_require_central_db();
jrs_auth_try_refresh_current_user($cconn);

$report = isset($_GET['report']) ? jrs_report_key((string) $_GET['report']) : '';
$datasource = isset($_GET['datasource']) ? trim((string) $_GET['datasource']) : '';
$definition = $report !== '' ? jrs_read_report($report, false) : null;
if ($datasource === '' && is_array($definition)) {
    $datasource = trim((string) ($definition['datasource'] ?? $definition['datasourceName'] ?? ''));
}
if (!jrs_is_valid_report_key($report) || $definition === null) {
    http_response_code(404);
    header('Content-Type: text/plain; charset=utf-8');
    echo 'Report not found.';
    exit;
}

jrs_require_report_runtime_permission($cconn, $report, 'view');

$reportEndpoint = (string) jrs_config('report_endpoint', 'execute_report.php');
$isMapReport = function_exists('jrs_is_map_report') && jrs_is_map_report($definition);
$reportTitle = (string) ($definition['name'] ?? $report);
?>
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title><?php echo jrs_html($reportTitle); ?> - AcuGIS Report Server</title>
  <?php if ($isMapReport): ?>
  <meta http-equiv="Content-Security-Policy" content="default-src 'self'; script-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'; style-src 'self' https://cdn.jsdelivr.net https://fonts.googleapis.com 'unsafe-inline'; img-src 'self' data: blob: https://tiles.openfreemap.org https://server.arcgisonline.com; font-src 'self' data: https://fonts.gstatic.com; connect-src 'self' https://tiles.openfreemap.org; worker-src 'self' blob:; frame-src 'self'; object-src 'none'; base-uri 'self'">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/ol@v9.2.4/ol.css">
  <?php endif; ?>
  <style>
body {
  margin: 0;
  font-family: "Roboto", Arial, sans-serif;
  background: #f4f6f9;
}

.container {
  display: flex;
  height: 100vh;
}

.sidebar {
  width: 320px;
  background: #ffffff;
  border-right: 1px solid #e0e0e0;
  padding: 20px;
  box-shadow: 2px 0 6px rgba(0,0,0,0.05);
  overflow-y: auto;
}

.sidebar.hidden {
  display: none;
}

.sidebar h3 {
  margin-top: 0;
  font-weight: 500;
  color: #2c3e50;
}

.field {
  margin-bottom: 18px;
}

.field label {
  display: block;
  font-size: 13px;
  color: #777;
  margin-bottom: 6px;
}

.field input,
.field select {
  width: 100%;
  padding: 10px;
  border-radius: 6px;
  border: 1px solid #ddd;
  background: #fafafa;
  font-size: 14px;
  transition: all 0.2s ease;
  box-sizing: border-box;
}

.field input:focus,
.field select:focus {
  border-color: #26a69a;
  outline: none;
  background: #fff;
}

.actions {
  display: flex;
  gap: 10px;
  margin-top: 10px;
}

button {
  padding: 10px 16px;
  border-radius: 6px;
  border: none;
  cursor: pointer;
  font-size: 14px;
  transition: all 0.2s ease;
}

#applyBtn {
  background: #26a69a;
  color: white;
}

#applyBtn:hover {
  background: #219388;
}

#resetBtn {
  background: #e0e0e0;
}

#resetBtn:hover {
  background: #d5d5d5;
}

.content {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.report-header {
  background: #ffffff;
  padding: 12px 20px;
  border-bottom: 1px solid #e0e0e0;
  font-weight: 500;
  font-size: 15px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.report-header-title {
  color: #2c3e50;
}

.toolbar-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.btn-export,
.btn-map {
  padding: 6px 12px;
  border-radius: 5px;
  border: none;
  background: #34495e;
  color: white;
  cursor: pointer;
  font-size: 12px;
}

.btn-export:hover,
.btn-map:hover:not(:disabled) {
  background: #2c3e50;
}

.btn-map {
  background: #26a69a;
}

.btn-map:hover:not(:disabled) {
  background: #219388;
}

.btn-map:disabled {
  background: #b0bec5;
  cursor: not-allowed;
}

iframe {
  flex: 1;
  border: none;
  background: white;
  width: 100%;
  min-height: 0;
}

.error {
  color: #c0392b;
  font-size: 13px;
  margin-top: 10px;
}

.split-body {
  flex: 1;
  display: flex;
  min-height: 0;
  position: relative;
}

.map-pane {
  width: 40%;
  min-width: 180px;
  position: relative;
  background: #e8eef3;
}

#map {
  position: absolute;
  inset: 0;
}

.split-divider {
  width: 6px;
  cursor: col-resize;
  background: #d0d7de;
  flex-shrink: 0;
  position: relative;
  z-index: 2;
}

.split-divider:hover,
.split-divider.active {
  background: #26a69a;
}

.report-pane {
  flex: 1;
  min-width: 240px;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.map-status {
  position: absolute;
  left: 10px;
  bottom: 10px;
  z-index: 1;
  background: rgba(255,255,255,0.92);
  border-radius: 4px;
  padding: 6px 10px;
  font-size: 12px;
  color: #546e7a;
  box-shadow: 0 1px 3px rgba(0,0,0,0.12);
  max-width: calc(100% - 20px);
}

.map-status.error {
  color: #c0392b;
}

.ol-popup {
  position: absolute;
  background: #fff;
  box-shadow: 0 2px 8px rgba(0,0,0,0.2);
  padding: 10px 12px;
  border-radius: 6px;
  border: 1px solid #e0e0e0;
  bottom: 12px;
  left: -50px;
  min-width: 160px;
  display: none;
  z-index: 10;
  pointer-events: auto;
}

.ol-popup:after, .ol-popup:before {
  top: 100%;
  border: solid transparent;
  content: " ";
  height: 0;
  width: 0;
  position: absolute;
  pointer-events: none;
}

.ol-popup:after {
  border-top-color: #fff;
  border-width: 8px;
  left: 48px;
  margin-left: -8px;
}

.ol-popup-closer {
  float: right;
  text-decoration: none;
  color: #90a4ae;
  font-size: 16px;
  line-height: 1;
  margin: -4px -2px 0 8px;
}

#mapLegendHost {
  position: absolute;
  top: 10px;
  right: 10px;
  z-index: 2;
  max-width: min(240px, calc(100% - 20px));
  max-height: calc(100% - 56px);
  overflow: auto;
  pointer-events: auto;
}

.jrs-map-legend {
  background: rgba(255, 255, 255, 0.95);
  border: 1px solid #e0e0e0;
  border-radius: 6px;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.12);
  padding: 8px 10px;
  font: 12px Roboto, Arial, sans-serif;
  color: #37474f;
}

.jrs-legend-title {
  font-weight: 600;
  font-size: 12px;
  margin-bottom: 6px;
  color: #263238;
  word-break: break-word;
}

.jrs-legend-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 3px 0;
}

.jrs-legend-row.jrs-legend-prop {
  align-items: flex-end;
  min-height: 28px;
}

.jrs-legend-swatch {
  width: 16px;
  height: 12px;
  border-radius: 2px;
  border: 1px solid rgba(0, 0, 0, 0.2);
  flex-shrink: 0;
}

.jrs-legend-circle {
  display: inline-block;
  border-radius: 50%;
  border: 1.5px solid;
  flex-shrink: 0;
  opacity: 0.85;
}

.jrs-legend-label {
  line-height: 1.25;
  word-break: break-word;
}

.jrs-legend-empty {
  color: #90a4ae;
  font-style: italic;
}

.jrs-legend-ramp-bar {
  height: 12px;
  border-radius: 3px;
  border: 1px solid rgba(0, 0, 0, 0.15);
  background: linear-gradient(to right, #0000ff, #00ffff, #00ff00, #ffff00, #ff0000);
  margin-bottom: 4px;
}

.jrs-legend-ramp-labels {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: #546e7a;
}

#mapTooltip {
  position: absolute;
  z-index: 3;
  display: none;
  max-width: 260px;
  padding: 6px 8px;
  background: rgba(38, 50, 56, 0.92);
  color: #fff;
  font: 12px Roboto, Arial, sans-serif;
  border-radius: 4px;
  white-space: pre-line;
  pointer-events: none;
  box-shadow: 0 1px 4px rgba(0,0,0,0.25);
}
  </style>
</head>
<body>
<div class="container">
  <div class="sidebar" id="paramSidebar">
    <h3>Options</h3>
    <form id="paramForm"></form>
    <div class="actions" id="paramActions">
      <button type="button" id="applyBtn">Apply</button>
      <button type="button" id="resetBtn">Reset</button>
    </div>
    <p id="errorBox" class="error"></p>
  </div>
  <div class="content">
    <div class="report-header">
      <span class="report-header-title"><?php echo jrs_html($isMapReport ? 'Map Report Viewer' : 'Report Viewer'); ?></span>
      <div class="toolbar-actions">
        <?php if ($isMapReport): ?>
        <button type="button" id="refreshFromMapBtn" class="btn-map" disabled title="Requires p_geometry and p_srid in the JRXML">Refresh Report from Map</button>
        <?php endif; ?>
        <button type="button" onclick="exportReport('pdf')" class="btn-export">Export PDF</button>
        <button type="button" onclick="exportReport('xlsx')" class="btn-export">Export Excel</button>
      </div>
    </div>
    <?php if ($isMapReport): ?>
    <div class="split-body" id="splitBody">
      <div class="map-pane" id="mapPane">
        <div id="map"></div>
        <div id="popup" class="ol-popup">
          <a href="#" id="popup-closer" class="ol-popup-closer" aria-label="Close">x</a>
          <div id="popup-content"></div>
        </div>
        <div id="mapLegendHost"></div>
        <div id="mapTooltip"></div>
        <div class="map-status" id="mapStatus">Loading map&hellip;</div>
      </div>
      <div class="split-divider" id="splitDivider" title="Drag to resize"></div>
      <div class="report-pane">
        <iframe id="reportFrame" title="Report output"></iframe>
      </div>
    </div>
    <?php else: ?>
    <iframe id="reportFrame" title="Report output"></iframe>
    <?php endif; ?>
  </div>
</div>

<?php if ($isMapReport): ?>
<script src="https://cdn.jsdelivr.net/npm/ol@v9.2.4/dist/ol.js"></script>
<script src="https://cdn.jsdelivr.net/npm/ol-mapbox-style@12.6.1/dist/olms.js"></script>
<script src="../assets/js/map-style.js"></script>
<?php endif; ?>
<script src="../assets/js/report-param-submit.js"></script>
<script>
const reportName = <?php echo json_encode($report, JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT); ?>;
const datasource = <?php echo json_encode($datasource, JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT); ?>;
const reportEndpoint = <?php echo json_encode($reportEndpoint, JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT); ?>;
const isMapReport = <?php echo $isMapReport ? 'true' : 'false'; ?>;

let currentParams = [];
let mapMeta = null;
let mapDefinition = null;
/** Single shared filter context consumed by both Jasper and the OpenLayers map. */
let filterContext = {};
let olMap = null;
let vectorSource = null;
let highlightSource = null;
let popupOverlay = null;
let mapInitialized = false;

const MAP_DRIVEN_PARAMS = new Set(['p_geometry', 'p_srid']);

function setError(msg) {
  document.getElementById('errorBox').textContent = msg || '';
}

function setMapStatus(msg, isError) {
  const el = document.getElementById('mapStatus');
  if (!el) return;
  el.textContent = msg || '';
  el.classList.toggle('error', !!isError);
  el.style.display = msg ? 'block' : 'none';
}

function setMapLegend(mapDef, styleMeta) {
  const host = document.getElementById('mapLegendHost');
  if (!host || !window.JrsMapStyle) return;
  host.innerHTML = JrsMapStyle.buildLegendHtml(mapDef || {}, styleMeta || {}) || '';
}

function pickDataFeature(evt) {
  if (!olMap || !vectorSource) {
    return null;
  }
  let found = null;
  olMap.forEachFeatureAtPixel(
    evt.pixel,
    (feature, layer) => {
      if (!feature) {
        return undefined;
      }
      const role = layer && typeof layer.get === 'function' ? layer.get('role') : null;
      if (role === 'highlight' || role === 'basemap') {
        return undefined;
      }
      // Prefer features from the report data source (multi-table and single-table).
      if (typeof vectorSource.hasFeature === 'function' && vectorSource.hasFeature(feature)) {
        found = feature;
        return feature;
      }
      if (role === 'data' || role === 'labels') {
        found = feature;
        return feature;
      }
      return undefined;
    },
    {
      hitTolerance: 8,
      layerFilter: (layer) => {
        if (!layer || typeof layer.get !== 'function') {
          return false;
        }
        const role = layer.get('role');
        if (role === 'highlight' || role === 'basemap') {
          return false;
        }
        const source = typeof layer.getSource === 'function' ? layer.getSource() : null;
        return source === vectorSource || role === 'data' || role === 'labels';
      }
    }
  );
  return found;
}

function showFeaturePopup(feature, coordinate) {
  const popupEl = document.getElementById('popup');
  const popupContent = document.getElementById('popup-content');
  if (!popupEl || !popupContent || !popupOverlay || !window.JrsMapStyle) {
    return;
  }
  const activeDef = mapDefinition || {};
  const html = JrsMapStyle.buildPopupHtml(feature, activeDef);
  if (!html) {
    return;
  }
  popupContent.innerHTML = html;
  popupEl.style.display = 'block';
  popupOverlay.setPosition(coordinate);
}

function hideFeaturePopup() {
  const popupEl = document.getElementById('popup');
  if (popupOverlay) {
    popupOverlay.setPosition(undefined);
  }
  if (popupEl) {
    popupEl.style.display = 'none';
  }
}

function normalizeSelectOptions(options) {
  const out = [];
  (options || []).forEach((opt) => {
    if (opt && typeof opt === 'object') {
      const label = opt.label ?? opt.value ?? '';
      const value = opt.value ?? opt.label ?? '';
      out.push({ label: String(label), value: String(value) });
    } else {
      out.push({ label: String(opt ?? ''), value: String(opt ?? '') });
    }
  });
  return out;
}

function buildForm(params) {
  const form = document.getElementById('paramForm');
  form.innerHTML = '';
  let visibleCount = 0;

  (params || []).forEach((p, i) => {
    if (!p || p.hidden === true || p.type === 'hidden') {
      return;
    }
    const name = String(p.name || ('param_' + i));
    if (MAP_DRIVEN_PARAMS.has(name)) {
      return;
    }
    visibleCount++;
    const type = String(p.type || 'string').toLowerCase();
    const required = !!p.required;

    const wrap = document.createElement('div');
    wrap.className = 'field';
    const label = document.createElement('label');
    label.textContent = p.label || name;
    wrap.appendChild(label);

    if (type === 'select') {
      const select = document.createElement('select');
      select.name = name;
      if (p.multiple) {
        select.multiple = true;
        select.size = 6;
      }
      if (required) {
        select.required = true;
      }
      const options = normalizeSelectOptions(p.options);
      options.forEach((o) => {
        const option = document.createElement('option');
        option.value = o.value;
        option.textContent = o.label;
        select.appendChild(option);
      });
      wrap.appendChild(select);
    } else if (type === 'boolean') {
      const select = document.createElement('select');
      select.name = name;
      if (required) {
        select.required = true;
      }
      [
        { value: '', label: '' },
        { value: 'true', label: 'True' },
        { value: 'false', label: 'False' }
      ].forEach((o) => {
        const option = document.createElement('option');
        option.value = o.value;
        option.textContent = o.label || '\u2014';
        select.appendChild(option);
      });
      if (p.default !== null && p.default !== undefined) {
        select.value = String(p.default);
      }
      wrap.appendChild(select);
    } else {
      const input = document.createElement('input');
      input.name = name;
      if (type === 'integer') {
        input.type = 'number';
        input.step = '1';
      } else if (type === 'double' || type === 'number' || type === 'float') {
        input.type = 'number';
        input.step = 'any';
      } else if (type === 'date') {
        input.type = 'date';
      } else {
        input.type = 'text';
      }
      if (required) {
        input.required = true;
      }
      if (p.default !== null && p.default !== undefined) {
        input.value = String(p.default);
      }
      input.placeholder = p.label || name;
      wrap.appendChild(input);
    }

    form.appendChild(wrap);
  });

  const sidebar = document.getElementById('paramSidebar');
  const actions = document.getElementById('paramActions');
  if (visibleCount === 0) {
    actions.style.display = 'none';
    sidebar.classList.add('hidden');
  } else {
    actions.style.display = 'flex';
    sidebar.classList.remove('hidden');
  }
}

/**
 * Rebuild the shared filter context from the parameter form and any active
 * spatial keys already on the context (p_geometry / p_srid).
 */
function syncFilterContextFromForm(options) {
  const keepSpatial = !options || options.keepSpatial !== false;
  const spatialGeometry = keepSpatial ? filterContext.p_geometry : null;
  const spatialSrid = keepSpatial ? filterContext.p_srid : null;

  const next = {};
  const form = document.getElementById('paramForm');
  if (form) {
    const names = [];
    const seen = new Set();
    Array.from(form.elements).forEach((el) => {
      const name = el && el.name ? String(el.name) : '';
      if (!name || MAP_DRIVEN_PARAMS.has(name) || seen.has(name)) {
        return;
      }
      seen.add(name);
      names.push(name);
    });

    names.forEach((name) => {
      const el = form.elements.namedItem(name);
      if (!el) {
        return;
      }
      const meta = (currentParams || []).find((p) => p && String(p.name) === name);
      const forceCollection = !!(meta && meta.multiple);
      if (el.tagName === 'SELECT' && (el.multiple || forceCollection)) {
        const values = Array.from(el.selectedOptions)
          .map((o) => String(o.value).trim())
          .filter((v) => v !== '');
        if (values.length > 0) {
          // Always keep multi-select values as arrays so Jasper $X{IN} and the
          // map binder share the same Collection-shaped filter context.
          next[name] = values;
        }
        return;
      }
      const value = String(el.value == null ? '' : el.value).trim();
      const submitted = window.JrsReportParams.scalarQueryValue(meta || { type: 'string' }, value, {
        preserveBlank: el.tagName !== 'SELECT'
      });
      if (submitted === null) {
        return;
      }
      next[name] = submitted;
    });
  }

  if (spatialGeometry) {
    next.p_geometry = spatialGeometry;
    if (spatialSrid != null && spatialSrid !== '') {
      next.p_srid = spatialSrid;
    }
  }

  filterContext = next;
  return filterContext;
}

function clearSpatialFromFilterContext() {
  delete filterContext.p_geometry;
  delete filterContext.p_srid;
}

function appendFilterContextParams(url, ctx) {
  const source = ctx || filterContext || {};
  Object.keys(source).forEach((key) => {
    const value = source[key];
    if (value == null) {
      return;
    }
    if (Array.isArray(value)) {
      value.forEach((v) => {
        if (v == null || String(v).trim() === '') {
          return;
        }
        url += `&${encodeURIComponent(key)}=${encodeURIComponent(String(v))}`;
      });
      return;
    }
    url = window.JrsReportParams.appendScalarToUrl(url, key, value);
  });
  return url;
}

function buildUrl(format) {
  let url = `${reportEndpoint}?_repName=${encodeURIComponent(reportName)}&_dataSource=${encodeURIComponent(datasource)}&_repFormat=${encodeURIComponent(format || 'html')}`;
  return appendFilterContextParams(url, filterContext);
}

function runReport() {
  setError('');
  if (!reportName) {
    setError('Missing report parameter.');
    return;
  }
  document.getElementById('reportFrame').src = buildUrl('html');
}

function exportReport(format) {
  window.open(buildUrl(format), '_blank');
}

function syncAllowsFilterReportFromMap() {
  return !mapDefinition || !mapDefinition.sync || mapDefinition.sync.filterReportFromMap !== false;
}

function syncAllowsFilterMapFromReport() {
  return !mapDefinition || !mapDefinition.sync || mapDefinition.sync.filterMapFromReport !== false;
}

function syncAllowsZoomToReportResults() {
  return !mapDefinition || !mapDefinition.sync || mapDefinition.sync.zoomToReportResults !== false;
}

/**
 * Apply the shared filter context to both the Jasper report and the map.
 */
function applyFilterContext(options) {
  const opts = options || {};
  if (opts.clearSpatial) {
    clearSpatialFromFilterContext();
  }
  syncFilterContextFromForm({ keepSpatial: !opts.clearSpatial });
  runReport();
  if (isMapReport && syncAllowsFilterMapFromReport()) {
    reloadMapFeatures({ fit: opts.fit !== false && syncAllowsZoomToReportResults() });
  }
}

function updateRefreshButton() {
  const btn = document.getElementById('refreshFromMapBtn');
  if (!btn) return;
  const enabled = !!(mapMeta && mapMeta.spatial_capable && syncAllowsFilterReportFromMap());
  btn.disabled = !enabled;
  btn.title = enabled
    ? 'Filter the report to the current map extent'
    : (!syncAllowsFilterReportFromMap()
      ? 'Filter report from map is disabled in the Map definition'
      : 'JRXML must define both p_geometry and p_srid parameters');
}

function initSplitResize() {
  const divider = document.getElementById('splitDivider');
  const mapPane = document.getElementById('mapPane');
  const splitBody = document.getElementById('splitBody');
  if (!divider || !mapPane || !splitBody) return;

  let dragging = false;

  divider.addEventListener('mousedown', (e) => {
    dragging = true;
    divider.classList.add('active');
    e.preventDefault();
  });

  window.addEventListener('mousemove', (e) => {
    if (!dragging) return;
    const rect = splitBody.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const pct = Math.min(70, Math.max(20, (x / rect.width) * 100));
    mapPane.style.width = pct + '%';
    if (olMap) {
      olMap.updateSize();
    }
  });

  window.addEventListener('mouseup', () => {
    if (!dragging) return;
    dragging = false;
    divider.classList.remove('active');
    if (olMap) {
      olMap.updateSize();
    }
  });
}

function applyMapFeatures(body, options) {
  const opts = options || {};
  const def = mapDefinition || {};
  const previousBasemapKey = window.JrsMapStyle.normalizeMapReportBasemap(def.basemap);
  if (body.mapDefinition) {
    mapDefinition = body.mapDefinition;
  }
  const features = new ol.format.GeoJSON().readFeatures(body, {
    dataProjection: 'EPSG:4326',
    featureProjection: 'EPSG:3857'
  });
  vectorSource.clear();
  if (highlightSource) {
    highlightSource.clear();
  }
  hideFeaturePopup();
  vectorSource.addFeatures(features);

  const built = JrsMapStyle.createDataLayers(mapDefinition || def, vectorSource, features);
  const existing = olMap.getLayers().getArray().slice();
  existing.forEach((layer) => {
    const role = layer.get('role');
    if (role === 'data' || role === 'labels') {
      olMap.removeLayer(layer);
    }
  });
  const highlight = olMap.getLayers().getArray().find((l) => l.get('role') === 'highlight');
  built.layers.forEach((layer) => {
    if (highlight) {
      olMap.getLayers().insertAt(olMap.getLayers().getArray().indexOf(highlight), layer);
    } else {
      olMap.addLayer(layer);
    }
  });

  setMapLegend(mapDefinition || def, built.styleMeta);

  const nextBasemapKey = window.JrsMapStyle.normalizeMapReportBasemap((mapDefinition || def).basemap);
  if (previousBasemapKey !== nextBasemapKey) {
    window.JrsMapStyle.replaceBasemapLayer(olMap, mapDefinition || def);
  }

  const zoomMode = (mapDefinition && mapDefinition.initialZoom) || 'fit';
  const shouldFit = opts.fit !== false && features.length > 0 && zoomMode === 'fit';
  if (shouldFit) {
    olMap.getView().fit(vectorSource.getExtent(), {
      padding: [40, 40, 40, 40],
      maxZoom: 16,
      duration: 250
    });
  }

  if (features.length === 0) {
    setMapStatus('No features match the current filters.', true);
  } else {
    setMapStatus(features.length + ' feature' + (features.length === 1 ? '' : 's'));
  }

  if (body.meta) {
    mapMeta = Object.assign({}, mapMeta || {}, body.meta, {
      spatial_capable: !!(mapMeta && mapMeta.spatial_capable) || !!body.meta.spatial_capable
    });
    updateRefreshButton();
  }
}

function reloadMapFeatures(options) {
  if (!isMapReport || !vectorSource || !olMap || !window.JrsMapStyle) {
    return;
  }
  if (!syncAllowsFilterMapFromReport() && options && options.force !== true) {
    return;
  }

  setMapStatus('Loading map features\u2026');
  let url = 'map_features.php?report=' + encodeURIComponent(reportName);
  url = appendFilterContextParams(url, filterContext);

  fetch(url, { credentials: 'same-origin' })
    .then((res) => res.json().then((body) => ({ ok: res.ok, body })))
    .then(({ ok, body }) => {
      if (!ok || body.error) {
        setMapStatus(body.error || 'Failed to load map features.', true);
        if (body.detail) {
          console.warn('map_features:', body.detail);
        }
        setMapLegend(null, null);
        return;
      }
      applyMapFeatures(body, options);
    })
    .catch(() => {
      setMapStatus('Failed to load map features.', true);
    });
}

function initMap() {
  if (!window.JrsMapStyle) {
    setMapStatus('Map style engine failed to load.', true);
    return;
  }

  const def = mapDefinition || {};
  vectorSource = new ol.source.Vector();
  highlightSource = new ol.source.Vector();

  const basemap = JrsMapStyle.createBasemapLayer(def);
  // Temporary empty data layer; replaced after features load with styles from mapDefinition.
  const placeholder = new ol.layer.Vector({
    source: vectorSource,
    properties: { role: 'data' },
    zIndex: 10
  });
  const highlightLayer = new ol.layer.Vector({
    source: highlightSource,
    style: JrsMapStyle.highlightStyle(ol),
    properties: { role: 'highlight' },
    zIndex: 20
  });

  olMap = new ol.Map({
    target: 'map',
    layers: [basemap, placeholder, highlightLayer],
    view: new ol.View({
      center: ol.proj.fromLonLat([-74.0, 40.7]),
      zoom: 10
    })
  });
  window.addEventListener('pagehide', function () {
    if (olMap) {
      olMap.setTarget(null);
    }
  });

  const popupEl = document.getElementById('popup');
  const popupCloser = document.getElementById('popup-closer');
  if (popupEl) {
    popupOverlay = new ol.Overlay({
      element: popupEl,
      autoPan: {
        animation: { duration: 200 },
        margin: 20
      },
      positioning: 'bottom-center',
      offset: [0, -12],
      stopEvent: true
    });
    olMap.addOverlay(popupOverlay);
    if (popupCloser) {
      popupCloser.addEventListener('click', (e) => {
        e.preventDefault();
        hideFeaturePopup();
      });
    }
    olMap.on('singleclick', (evt) => {
      const activeDef = mapDefinition || def;
      if (!activeDef.popups || !activeDef.popups.enabled) {
        hideFeaturePopup();
        return;
      }
      const feature = pickDataFeature(evt);
      if (!feature) {
        hideFeaturePopup();
        return;
      }
      showFeaturePopup(feature, evt.coordinate);
    });

    const tipEl = document.getElementById('mapTooltip');
    if (tipEl) {
      olMap.on('pointermove', (evt) => {
        const activeDef = mapDefinition || def;
        if (!activeDef.tooltips || !activeDef.tooltips.enabled) {
          tipEl.style.display = 'none';
          olMap.getTargetElement().style.cursor = '';
          return;
        }
        if (evt.dragging) {
          tipEl.style.display = 'none';
          return;
        }
        const feature = pickDataFeature(evt);
        if (!feature) {
          tipEl.style.display = 'none';
          olMap.getTargetElement().style.cursor = '';
          return;
        }
        const text = JrsMapStyle.buildTooltipText(feature, activeDef);
        if (!text) {
          tipEl.style.display = 'none';
          return;
        }
        tipEl.textContent = text;
        tipEl.style.display = 'block';
        const mapEl = olMap.getTargetElement();
        const rect = mapEl.getBoundingClientRect();
        tipEl.style.left = Math.min(evt.originalEvent.clientX - rect.left + 12, rect.width - 20) + 'px';
        tipEl.style.top = Math.min(evt.originalEvent.clientY - rect.top + 12, rect.height - 20) + 'px';
        mapEl.style.cursor = 'pointer';
      });
    }
  }

  mapInitialized = true;
  reloadMapFeatures({ fit: true, force: true });
}

function refreshReportFromMap() {
  if (!olMap || !mapMeta || !mapMeta.spatial_capable || !syncAllowsFilterReportFromMap()) {
    setError('Spatial refresh is not available for this report.');
    return;
  }

  const extent3857 = olMap.getView().calculateExtent(olMap.getSize());
  const extent4326 = ol.proj.transformExtent(extent3857, 'EPSG:3857', 'EPSG:4326');
  const [xmin, ymin, xmax, ymax] = extent4326;
  const srid = mapMeta.srid || 4326;

  setMapStatus('Refreshing report from map extent\u2026');
  const url = 'map_extent.php?report=' + encodeURIComponent(reportName)
    + '&xmin=' + encodeURIComponent(xmin)
    + '&ymin=' + encodeURIComponent(ymin)
    + '&xmax=' + encodeURIComponent(xmax)
    + '&ymax=' + encodeURIComponent(ymax)
    + '&srid=' + encodeURIComponent(srid);

  fetch(url, { credentials: 'same-origin' })
    .then((res) => res.json().then((body) => ({ ok: res.ok, body })))
    .then(({ ok, body }) => {
      if (!ok || body.error || !body.p_geometry) {
        setError(body.error || 'Failed to transform map extent.');
        setMapStatus('Extent transform failed.', true);
        return;
      }
      syncFilterContextFromForm({ keepSpatial: false });
      filterContext.p_geometry = body.p_geometry;
      filterContext.p_srid = body.p_srid;
      setError('');
      setMapStatus('Filtered to map extent');
      // Only refresh the Jasper report. The map already shows this extent; re-querying
      // map_features with p_geometry/p_srid is redundant and often fails (JRXML spatial
      // predicates are for the report fill, not the interactive layer).
      runReport();
    })
    .catch(() => {
      setError('Failed to transform map extent.');
      setMapStatus('Extent transform failed.', true);
    });
}

document.getElementById('applyBtn').addEventListener('click', () => {
  // Apply uses form parameters; clear prior map-extent spatial keys so form filters win.
  applyFilterContext({ clearSpatial: true, fit: true });
});
document.getElementById('resetBtn').addEventListener('click', () => {
  document.getElementById('paramForm').reset();
  filterContext = {};
  applyFilterContext({ clearSpatial: true, fit: true });
});

const refreshBtn = document.getElementById('refreshFromMapBtn');
if (refreshBtn) {
  refreshBtn.addEventListener('click', refreshReportFromMap);
}

if (isMapReport) {
  initSplitResize();
}

fetch('get_report_config.php?report=' + encodeURIComponent(reportName), { credentials: 'same-origin' })
  .then((res) => res.json())
  .then((config) => {
    currentParams = Array.isArray(config.parameters) ? config.parameters : [];
    mapMeta = config.map || null;
    mapDefinition = config.mapDefinition || null;
    buildForm(currentParams);
    updateRefreshButton();
    filterContext = {};
    applyFilterContext({ clearSpatial: true, fit: true });
    if (isMapReport && !mapInitialized) {
      initMap();
    }
  })
  .catch(() => {
    setError('Failed to load report configuration.');
  });
</script>
</body>
</html>
