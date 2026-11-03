<?php
declare(strict_types=1);

/**
 * Standalone report definition browser: lists *.report.json, previews metadata,
 * renders parameter inputs, and builds a Jasper /report query URL.
 */

require_once __DIR__ . '/../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_auth_try_refresh_current_user($cconn);

$jsonDir = __DIR__ . DIRECTORY_SEPARATOR . 'json';
$reportFiles = [];
$reportEndpoint = (string) jrs_config('report_endpoint', 'execute_report.php');

if (is_dir($jsonDir)) {
    foreach (glob($jsonDir . DIRECTORY_SEPARATOR . '*.report.json') ?: [] as $path) {
        $key = jrs_report_key(basename($path, '.report.json'));
        if (!jrs_is_valid_report_key($key)) {
            continue;
        }
        $uid = (int) (jrs_get_current_user()['id'] ?? 0);
        $u = jrs_get_current_user();
        $isAdm = ($u['is_admin'] ?? false) === true || jrs_auth_groups_include_admin((array) ($u['groups'] ?? []));
        if (!$isAdm && !jrs_has_permission($cconn, $uid, JRS_RESOURCE_TYPE_JASPER_REPORT, $key, 'view')) {
            continue;
        }
        $reportFiles[] = basename($path);
    }
    sort($reportFiles, SORT_STRING);
}

jrs_material_page_header('Report URL Builder');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">Report URL Builder</h1>
        <p class="page-subtitle">Preview report metadata, fill parameters, and generate a Jasper report URL.</p>
    </div>
</header>

<div class="card">
    <div class="card-content">
        <span class="card-title">Report Definition</span>
        <div class="input-field">
            <select id="reportFile" autocomplete="off">
                <option value="" selected>Select a report</option>
                <?php foreach ($reportFiles as $file): ?>
                    <option value="<?= htmlspecialchars($file, ENT_QUOTES, 'UTF-8') ?>"><?= htmlspecialchars($file, ENT_QUOTES, 'UTF-8') ?></option>
                <?php endforeach; ?>
            </select>
            <label>Report definition</label>
            <?php if ($reportFiles === []): ?>
                <p class="red-text text-darken-2">No <code>*.report.json</code> files found in <code>reports/json/</code>.</p>
            <?php endif; ?>
        </div>
    </div>
</div>

<div id="reportPanel" class="is-hidden">
    <div class="card">
        <div class="card-content">
            <span class="card-title">Report Details</span>
            <div class="report-meta">
                <div class="meta-row"><span class="meta-label">Name</span><span id="metaName">-</span></div>
                <div class="meta-row"><span class="meta-label">Datasource</span><span class="meta-value" id="metaDatasource">-</span></div>
            </div>
        </div>
    </div>

    <div class="card">
        <div class="card-content">
            <span class="card-title">Parameters</span>
            <form id="paramForm" class="row"></form>
            <p id="noVisibleParams" class="muted-small is-hidden">No visible parameters (all hidden).</p>
        </div>
    </div>

    <div class="card-actions-row" style="margin-bottom:16px;">
        <button type="button" id="btnBuild" class="btn waves-effect waves-light teal">
            Build Report URL
            <i class="material-icons right">link</i>
        </button>
        <button type="button" id="btnRun" class="btn waves-effect waves-light blue-grey">
            Run Report
            <i class="material-icons right">open_in_new</i>
        </button>
    </div>
    <div id="buildError" class="card-panel amber lighten-4 amber-text text-darken-4 is-hidden" role="alert"></div>

    <div class="card">
        <div class="card-content">
            <span class="card-title">Generated URL</span>
            <pre id="urlOut" class="jrs-url-output">-</pre>
        </div>
    </div>

    <div id="htmlPreviewWrap" class="card is-hidden">
        <div class="card-content">
            <span class="card-title">HTML Preview</span>
            <button type="button" id="btnIframe" class="btn-small waves-effect waves-light blue-grey" style="margin-bottom:12px;">
                <i class="material-icons left">preview</i>Load in iframe
            </button>
            <iframe id="reportIframe" title="Report preview" class="report-frame"></iframe>
        </div>
    </div>
</div>

<div id="loadError" class="card-panel red lighten-4 red-text text-darken-4 is-hidden" role="alert"></div>

<script src="../assets/js/report-param-submit.js"></script>
<script>
(function () {
    'use strict';

    const selectEl = document.getElementById('reportFile');
    const reportPanel = document.getElementById('reportPanel');
    const metaName = document.getElementById('metaName');
    const metaDatasource = document.getElementById('metaDatasource');
    const paramForm = document.getElementById('paramForm');
    const noVisibleParams = document.getElementById('noVisibleParams');
    const btnBuild = document.getElementById('btnBuild');
    const btnRun = document.getElementById('btnRun');
    const btnIframe = document.getElementById('btnIframe');
    const htmlPreviewWrap = document.getElementById('htmlPreviewWrap');
    const reportIframe = document.getElementById('reportIframe');
    const urlOut = document.getElementById('urlOut');
    const loadError = document.getElementById('loadError');
    const buildError = document.getElementById('buildError');
    const reportEndpoint = <?php echo json_encode($reportEndpoint, JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT); ?>;

    /** @type {{ definition: object } | null} */
    let current = null;

    /** Placeholder until a caller provides a map bounding box. */
    const PLACEHOLDER_MAP_BBOX = '12.34,45.67,12.89,45.99';
    /** Placeholder until a caller provides the real feature id. */
    const PLACEHOLDER_FEATURE_ID = '123';

    function isParamHidden(p) {
        if (p && p.type === 'hidden') {
            return true;
        }
        return !!(p && p.hidden === true);
    }

    /**
     * Parameters bound to map/feature context: no form field, always a value for URL.
     * @returns {string|null} resolved auto value, or null if not an auto-sourced param
     */
    function getAutomaticParamValue(p) {
        if (!p || p.source == null) {
            return null;
        }
        const src = String(p.source);
        if (src === 'map.bbox') {
            return PLACEHOLDER_MAP_BBOX;
        }
        if (src === 'feature.id') {
            return PLACEHOLDER_FEATURE_ID;
        }
        return null;
    }

    function isAutoSourcedParam(p) {
        return getAutomaticParamValue(p) !== null;
    }

    function setLoadError(msg) {
        if (!msg) {
            loadError.classList.add('is-hidden');
            loadError.textContent = '';
            return;
        }
        loadError.textContent = msg;
        loadError.classList.remove('is-hidden');
    }

    function setBuildError(msg) {
        if (!msg) {
            buildError.classList.add('is-hidden');
            buildError.textContent = '';
            return;
        }
        buildError.textContent = msg;
        buildError.classList.remove('is-hidden');
    }

    function clearForm() {
        paramForm.innerHTML = '';
    }

    function getBuiltUrlFromOutput() {
        const u = urlOut.textContent.trim();
        if (!u || u === '-' || u.indexOf('?') === -1) {
            return '';
        }
        return u;
    }

    function syncHtmlPreviewVisibility() {
        if (!current || !current.definition) {
            htmlPreviewWrap.classList.add('is-hidden');
            reportIframe.src = 'about:blank';
            return;
        }
        const fmt = getOutputFormat(current.definition);
        if (fmt === 'html') {
            htmlPreviewWrap.classList.remove('is-hidden');
        } else {
            htmlPreviewWrap.classList.add('is-hidden');
            reportIframe.src = 'about:blank';
        }
    }

    function getDatasourceName(definition) {
        if (!definition) {
            return '';
        }
        if (definition.dataSource != null) {
            return String(definition.dataSource).trim();
        }
        if (definition.datasource != null) {
            return String(definition.datasource).trim();
        }
        if (definition.datasourceName != null) {
            return String(definition.datasourceName).trim();
        }
        if (definition.datasource_name != null) {
            return String(definition.datasource_name).trim();
        }
        if (definition.data_source != null) {
            return String(definition.data_source).trim();
        }
        return '';
    }

    function getParameters(definition) {
        if (!definition) {
            return [];
        }
        if (Array.isArray(definition.params)) {
            return definition.params;
        }
        if (Array.isArray(definition.parameters)) {
            return definition.parameters;
        }
        return [];
    }

    function getOutputFormat(definition) {
        if (!definition) {
            return '';
        }
        const fmt = definition.format != null ? definition.format : definition.outputFormat;
        return String(fmt || '').toLowerCase();
    }

    function normalizeDefinition(data) {
        const def = data && typeof data === 'object' ? data : {};
        return {
            name: def.name != null ? String(def.name) : '',
            report: def.report != null ? String(def.report) : '',
            datasource: getDatasourceName(def),
            format: getOutputFormat(def),
            params: getParameters(def)
        };
    }

    function renderParameters(parameters) {
        clearForm();
        let visible = 0;
        (parameters || []).forEach(function (p, idx) {
            if (isParamHidden(p) || isAutoSourcedParam(p)) {
                return;
            }
            visible++;
            const name = p.name || ('param_' + idx);
            const id = 'param_' + name.replace(/[^\w-]/g, '_');
            const col = document.createElement('div');
            col.className = 'input-field col s12 m6';

            let input;
            const t = (p.type || 'string').toLowerCase();
            if (t === 'integer') {
                input = document.createElement('input');
                input.type = 'number';
                input.step = '1';
                if (p.default !== null && p.default !== undefined && p.default !== '') {
                    input.value = String(p.default);
                }
            } else if (t === 'date') {
                input = document.createElement('input');
                input.type = 'date';
                if (p.default) {
                    input.value = String(p.default).slice(0, 10);
                }
            } else {
                input = document.createElement('input');
                input.type = 'text';
                if (p.default !== null && p.default !== undefined) {
                    input.value = String(p.default);
                }
            }
            input.id = id;
            input.name = name;
            input.dataset.paramName = name;
            input.dataset.required = p.required ? '1' : '0';

            col.appendChild(input);
            const label = document.createElement('label');
            label.setAttribute('for', id);
            label.textContent = p.label || name;
            if (input.value !== '') {
                label.className = 'active';
            }
            if (p.required) {
                const req = document.createElement('span');
                req.className = 'red-text';
                req.textContent = ' *';
                label.appendChild(req);
            }
            col.appendChild(label);
            paramForm.appendChild(col);
        });
        if (visible === 0) {
            noVisibleParams.classList.remove('is-hidden');
        } else {
            noVisibleParams.classList.add('is-hidden');
        }
        if (window.jrsInitMaterialize) {
            window.jrsInitMaterialize(paramForm);
        }
    }

    function resolveParamValue(p) {
        const auto = getAutomaticParamValue(p);
        if (auto !== null) {
            return auto;
        }
        if (isParamHidden(p)) {
            if (p.default !== null && p.default !== undefined) {
                return String(p.default);
            }
            return '';
        }
        const el = paramForm.querySelector('[data-param-name="' + CSS.escape(p.name) + '"]');
        if (!el) {
            return '';
        }
        return (el.value || '').trim();
    }

    selectEl.addEventListener('change', function () {
        setLoadError('');
        setBuildError('');
        urlOut.textContent = '-';
        current = null;
        clearForm();
        noVisibleParams.classList.add('is-hidden');
        htmlPreviewWrap.classList.add('is-hidden');
        reportIframe.src = 'about:blank';

        const file = selectEl.value;
        if (!file) {
            reportPanel.classList.add('is-hidden');
            return;
        }

        const reportKey = file.replace(/\.report\.json$/, '');
        const url = 'get_report_config.php?report=' + encodeURIComponent(reportKey);
        fetch(url, { credentials: 'same-origin' })
            .then(function (r) {
                if (!r.ok) {
                    throw new Error('HTTP ' + r.status + ' loading ' + file);
                }
                return r.json();
            })
            .then(function (data) {
                const normalized = normalizeDefinition(data);
                current = { definition: normalized };
                metaName.textContent = normalized.name || '-';
                metaDatasource.textContent = normalized.datasource || '-';
                renderParameters(normalized.params);
                syncHtmlPreviewVisibility();
                reportPanel.classList.remove('is-hidden');
            })
            .catch(function (e) {
                reportPanel.classList.add('is-hidden');
                setLoadError(e.message || String(e));
            });
    });

    btnBuild.addEventListener('click', function () {
        setBuildError('');
        if (!current) {
            setBuildError('Select a report first.');
            return;
        }

        const def = current.definition;
        const params = getParameters(def);
        const missing = [];
        params.forEach(function (p) {
            if (!p || !p.name) {
                return;
            }
            if (isParamHidden(p) || isAutoSourcedParam(p)) {
                return;
            }
            if (p.required && !resolveParamValue(p)) {
                missing.push(p.label || p.name);
            }
        });
        if (missing.length) {
            setBuildError('Required parameters missing: ' + missing.join(', '));
            return;
        }

        const q = [];
        const repName = def.report != null ? String(def.report) : '';
        q.push('_repName=' + encodeURIComponent(repName));
        const ds = getDatasourceName(def);
        q.push('_dataSource=' + encodeURIComponent(ds));
        const repFormat = getOutputFormat(def) || 'pdf';
        q.push('_repFormat=' + encodeURIComponent(repFormat));

        params.forEach(function (p) {
            if (!p || !p.name) {
                return;
            }
            const val = resolveParamValue(p);
            const submitted = window.JrsReportParams.scalarQueryValue(p, val, {
                hidden: isParamHidden(p),
                autoSourced: isAutoSourcedParam(p)
            });
            if (submitted === null) {
                return;
            }
            q.push(encodeURIComponent(p.name) + '=' + encodeURIComponent(submitted));
        });

        const built = reportEndpoint + '?' + q.join('&');
        urlOut.textContent = built;
    });

    btnRun.addEventListener('click', function () {
        setBuildError('');
        const u = getBuiltUrlFromOutput();
        if (!u) {
            setBuildError('Build the URL first.');
            return;
        }
        window.open(u, '_blank');
    });

    btnIframe.addEventListener('click', function () {
        setBuildError('');
        const u = getBuiltUrlFromOutput();
        if (!u) {
            setBuildError('Build the URL first.');
            return;
        }
        reportIframe.src = u;
    });
})();
</script>
<?php jrs_material_page_footer(); ?>
