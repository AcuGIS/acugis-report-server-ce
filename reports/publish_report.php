<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'edit');

$uploadMessage = '';
$uploadMessageType = '';
$publishNotes = [];
$runReportLink = '';
$editJsonLink = '';
$formErrors = [];

jrs_ensure_dir(jrs_json_dir());
jrs_ensure_dir(jrs_report_files_dir());
jrs_ensure_dir(jrs_report_assets_dir());

$dataSourceOptions = jrs_datasource_names();
$accessGroups = jrs_access_groups($cconn);

if ($_SERVER['REQUEST_METHOD'] === 'POST') {
    if (!isset($_FILES['jrxml_file']) || !is_array($_FILES['jrxml_file'])) {
        $formErrors[] = 'JRXML file upload is missing.';
    } elseif ((int) $_FILES['jrxml_file']['error'] !== UPLOAD_ERR_OK) {
        $formErrors[] = 'JRXML upload failed. Please try again.';
    }

    $originalName = isset($_FILES['jrxml_file']['name']) ? (string) $_FILES['jrxml_file']['name'] : '';
    $reportKey = jrs_report_key(pathinfo($originalName, PATHINFO_FILENAME));
    if ($reportKey === '') {
        $formErrors[] = 'Report file name must contain at least one letter or number.';
    }

    $reportName = trim((string) ($_POST['report_name'] ?? ''));
    if ($reportName === '') {
        $reportName = $reportKey;
    }
    $reportDescription = trim((string) ($_POST['report_description'] ?? ''));

    $dataSource = trim((string) ($_POST['data_source'] ?? ''));
    if ($dataSource === '') {
        $formErrors[] = 'Select a datasource.';
    }

    $reportType = jrs_normalize_report_type($_POST['report_type'] ?? JRS_REPORT_TYPE_JASPER);
    if ($reportType === JRS_REPORT_TYPE_MAP && $dataSource !== '') {
        $dsRow = jrs_find_datasource($dataSource);
        if ($dsRow !== null && jrs_normalize_db_type($dsRow['db_type'] ?? 'postgresql') !== 'postgresql') {
            $formErrors[] = 'Map Report requires a PostgreSQL/PostGIS datasource.';
        }
    }

    $groups = $_POST['access_groups'] ?? [];
    $groups = is_array($groups) ? $groups : [];
    $groupsStr = implode(',', array_values(array_filter(array_map('trim', $groups))));

    $inputParams = $_POST['params'] ?? [];
    $inputParams = is_array($inputParams) ? $inputParams : [];
    $parameters = [];

    foreach ($inputParams as $p) {
        if (!is_array($p)) {
            continue;
        }

        $name = trim((string) ($p['name'] ?? ''));
        if ($name === '') {
            $formErrors[] = 'Parameter name is required.';
            continue;
        }

        $type = trim((string) ($p['type'] ?? 'string'));
        if (!in_array($type, ['string', 'number', 'date', 'select'], true)) {
            $type = 'string';
        }

        $param = [
            'name' => $name,
            'type' => $type,
            'label' => trim((string) ($p['label'] ?? '')) ?: $name,
            'required' => !empty($p['required']),
            'hidden' => false,
        ];

        if ($type === 'select') {
            $sourceType = trim((string) ($p['source_type'] ?? 'static'));
            if ($sourceType === 'sql') {
                $sql = trim((string) ($p['sql'] ?? ''));
                if ($sql === '') {
                    $formErrors[] = 'SQL query is required for select parameter "' . $name . '".';
                    continue;
                }
                $param['source'] = 'sql:' . $sql;
            } else {
                $options = array_filter(array_map('trim', explode("\n", (string) ($p['options'] ?? ''))));
                if ($options === []) {
                    $formErrors[] = 'Select parameter "' . $name . '" requires at least one option.';
                    continue;
                }
                $param['options'] = array_values($options);
            }

            if (!empty($p['multiple'])) {
                $param['multiple'] = true;
            }
        }

        $parameters[] = $param;
    }

    if ($formErrors === []) {
        $targetPath = jrs_report_jrxml_path($reportKey);
        $targetJasperPath = jrs_report_jasper_path($reportKey);
        $tmpFilePath = (string) $_FILES['jrxml_file']['tmp_name'];

        if (!is_writable(jrs_report_files_dir())) {
            $formErrors[] = 'Report directory is not writable: ' . jrs_report_files_dir();
        } elseif (!move_uploaded_file($tmpFilePath, $targetPath)) {
            $formErrors[] = 'Failed to save JRXML file to destination.';
        } else {
            $reportJson = [
                'name' => $reportName,
                'description' => $reportDescription,
                'report' => $reportKey,
                'type' => $reportType,
                'datasource' => $dataSource,
                'format' => 'html',
                'parameters' => $parameters,
                'access_groups' => $groupsStr,
            ];
            if ($reportType === JRS_REPORT_TYPE_MAP) {
                $reportJson['mapDefinition'] = jrs_default_map_definition();
            }

            $jsonContent = json_encode($reportJson, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES);
            if ($jsonContent === false || file_put_contents(jrs_report_json_path($reportKey), $jsonContent) === false) {
                $formErrors[] = 'JRXML uploaded but failed to create JSON definition file.';
            }

            $hasJasperUpload = isset($_FILES['jasper_file'])
                && is_array($_FILES['jasper_file'])
                && (int) $_FILES['jasper_file']['error'] !== UPLOAD_ERR_NO_FILE;

            if ($hasJasperUpload) {
                if ((int) $_FILES['jasper_file']['error'] !== UPLOAD_ERR_OK) {
                    $publishNotes[] = 'JRXML uploaded, but .jasper upload failed.';
                } elseif (!move_uploaded_file((string) $_FILES['jasper_file']['tmp_name'], $targetJasperPath)) {
                    $publishNotes[] = 'JRXML uploaded, but failed to save uploaded .jasper file.';
                } else {
                    $publishNotes[] = 'Uploaded .jasper saved successfully.';
                }
            } else {
                $compileCommand = 'jasperstarter compile '
                    . escapeshellarg($targetPath)
                    . ' -o '
                    . escapeshellarg($targetJasperPath)
                    . ' 2>&1';
                $compileOutput = [];
                $compileExitCode = 0;
                @exec($compileCommand, $compileOutput, $compileExitCode);
                $publishNotes[] = $compileExitCode === 0
                    ? 'JRXML compiled to .jasper successfully.'
                    : 'JRXML uploaded, but compile to .jasper failed. Upload a compiled .jasper file or install jasperstarter on the server.';
            }

            $hasThumbnailUpload = isset($_FILES['thumbnail_file'])
                && is_array($_FILES['thumbnail_file'])
                && (int) $_FILES['thumbnail_file']['error'] !== UPLOAD_ERR_NO_FILE;

            if ($hasThumbnailUpload) {
                if ((int) $_FILES['thumbnail_file']['error'] !== UPLOAD_ERR_OK) {
                    $publishNotes[] = 'Thumbnail upload failed.';
                } elseif (!move_uploaded_file((string) $_FILES['thumbnail_file']['tmp_name'], jrs_report_asset_path($reportKey))) {
                    $publishNotes[] = 'Thumbnail upload received but could not be saved.';
                } else {
                    $publishNotes[] = 'Thumbnail uploaded successfully.';
                }
            }

            if ($formErrors === []) {
                $uploadMessage = 'Report published successfully.';
                $uploadMessageType = 'success';
                $runReportLink = 'view.php?report=' . rawurlencode($reportKey) . '&datasource=' . rawurlencode($dataSource);
                $editJsonLink = 'serve_report_definition.php?report=' . rawurlencode($reportKey);
                $central = jrs_auth_central_db();
                if ($central) {
                    $groupList = is_array($groups) ? $groups : [];
                    jrs_sync_report_access_groups(
                        $central,
                        $reportKey,
                        $reportName,
                        array_values(array_filter(array_map('trim', $groupList))),
                        (int) (jrs_get_current_user()['id'] ?? 0)
                    );
                } else {
                    $publishNotes[] = 'Application database unavailable; report was not registered for ACL.';
                }
            }
        }
    }

    if ($formErrors !== []) {
        $uploadMessage = implode(' ', $formErrors);
        $uploadMessageType = 'danger';
    }
}

jrs_material_page_header('Publish Report');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">Publish Report</h1>
        <p class="page-subtitle">Upload a JRXML file and bind it to a datasource.</p>
    </div>
</header>

<?php if ($dataSourceOptions === []): ?>
    <div class="card-panel amber lighten-4 amber-text text-darken-4">
        No datasources were found. Load <code>database/schema.sql</code> and add at least one row to
        <code>public.jasper_datasource</code> before publishing reports.
    </div>
<?php endif; ?>

<?php if ($uploadMessage !== ''): ?>
    <div class="card-panel <?php echo $uploadMessageType === 'success' ? 'green lighten-4 green-text text-darken-4' : 'red lighten-4 red-text text-darken-4'; ?>" role="alert">
        <div><?php echo jrs_html($uploadMessage); ?></div>
        <?php if ($uploadMessageType === 'success' && $runReportLink !== '' && $editJsonLink !== ''): ?>
            <div class="card-actions-row" style="margin-top:12px;">
                <a href="<?php echo jrs_html($runReportLink); ?>" class="btn-small waves-effect waves-light teal">Run report</a>
                <a href="<?php echo jrs_html($editJsonLink); ?>" class="btn-small waves-effect waves-light blue-grey" target="_blank" rel="noopener noreferrer">View JSON</a>
            </div>
        <?php endif; ?>
        <?php if ($publishNotes !== []): ?>
            <ul style="margin-top:12px;">
                <?php foreach ($publishNotes as $note): ?>
                    <li><?php echo jrs_html($note); ?></li>
                <?php endforeach; ?>
            </ul>
        <?php endif; ?>
    </div>
<?php endif; ?>

<div class="card">
    <div class="card-content">
        <span class="card-title">Report Package</span>
        <form action="" method="post" enctype="multipart/form-data" id="publishReportForm">
            <div class="row">
                <div class="input-field col s12 m6">
                    <input type="text" id="report_name" name="report_name" required placeholder="Enter report name">
                    <label for="report_name" class="active">Report Name</label>
                </div>
                <div class="input-field col s12 m6">
                    <input type="text" id="report_description" name="report_description" placeholder="Shown on report cards">
                    <label for="report_description" class="active">Description</label>
                </div>
                <div class="input-field col s12 m6">
                    <select id="report_type" name="report_type">
                        <option value="jasper" selected>Jasper Report</option>
                        <option value="map">Map Report</option>
                    </select>
                    <label>Report Type</label>
                    <span class="helper-text">Map Report pairs a Jasper report with an interactive map. Configure the Map tab after publishing.</span>
                </div>
                <div class="file-field input-field col s12 m6">
                    <div class="btn blue-grey">
                        <span>JRXML</span>
                        <input type="file" id="jrxml_file" name="jrxml_file" accept=".jrxml,application/xml,text/xml" required>
                    </div>
                    <div class="file-path-wrapper">
                        <input class="file-path validate" type="text" placeholder="Upload JRXML file">
                    </div>
                </div>
                <div class="file-field input-field col s12 m6">
                    <div class="btn blue-grey lighten-1">
                        <span>JASPER</span>
                        <input type="file" id="jasper_file" name="jasper_file" accept=".jasper,application/octet-stream">
                    </div>
                    <div class="file-path-wrapper">
                        <input class="file-path validate" type="text" placeholder="Optional compiled .jasper file">
                    </div>
                    <span class="helper-text">If provided, this file is saved as the compiled report.</span>
                </div>
                <div class="file-field input-field col s12 m6">
                    <div class="btn blue-grey lighten-1">
                        <span>Thumbnail</span>
                        <input type="file" id="thumbnail_file" name="thumbnail_file" accept=".png,.jpg,.jpeg,.webp">
                    </div>
                    <div class="file-path-wrapper">
                        <input class="file-path validate" type="text" placeholder="Optional report thumbnail">
                    </div>
                </div>
                <div class="input-field col s12 m6">
                    <select id="data_source" name="data_source" required>
                        <option value="" disabled selected>Select data source</option>
                        <?php foreach ($dataSourceOptions as $svcName): ?>
                            <option value="<?php echo jrs_html($svcName); ?>"><?php echo jrs_html($svcName); ?></option>
                        <?php endforeach; ?>
                    </select>
                    <label>Datasource</label>
                </div>
                <?php if ($accessGroups !== []): ?>
                    <div class="input-field col s12 m6">
                        <select name="access_groups[]" multiple>
                            <option value="" disabled>Choose access groups</option>
                            <?php foreach ($accessGroups as $name): ?>
                                <option value="<?php echo jrs_html($name); ?>"><?php echo jrs_html($name); ?></option>
                            <?php endforeach; ?>
                        </select>
                        <label>Access Groups</label>
                        <span class="helper-text">Optional metadata for external access control.</span>
                    </div>
                <?php endif; ?>
            </div>

            <hr class="section-divider">

            <div class="card-actions-row" style="justify-content:space-between; margin-bottom:16px;">
                <span class="card-title" style="margin:0;">Parameters</span>
                <button type="button" class="btn-small waves-effect waves-light blue-grey" id="addParameterBtn">
                    <i class="material-icons left">add</i>Add Parameter
                </button>
            </div>

            <div id="parameterRows"></div>

            <div class="form-actions">
                <button type="submit" class="btn waves-effect waves-light teal">
                    Publish Report
                    <i class="material-icons right">publish</i>
                </button>
            </div>
        </form>
    </div>
</div>

<template id="parameterRowTemplate">
    <div class="parameter-row">
        <div class="row">
            <div class="input-field col s12 m3">
                <input type="text" class="param-name" name="params[__INDEX__][name]" placeholder="e.g. parcel_id">
                <label class="active">Name</label>
            </div>
            <div class="input-field col s12 m2">
                <select class="param-type" name="params[__INDEX__][type]">
                    <option value="string">Text</option>
                    <option value="number">Number</option>
                    <option value="date">Date</option>
                    <option value="select">Select (Dropdown)</option>
                </select>
                <label>Type</label>
            </div>
            <div class="input-field col s12 m3">
                <input type="text" class="param-label" name="params[__INDEX__][label]" placeholder="Display label">
                <label class="active">Label</label>
            </div>
            <div class="col s12 m4">
                <div class="select-source-settings is-hidden">
                    <div style="margin-bottom:10px;">
                        <div class="muted-small" style="margin-bottom:6px;">Source Type</div>
                        <label style="margin-right:16px;">
                            <input type="radio" class="param-source-type" name="params[__INDEX__][source_type]" value="static" checked>
                            <span>Static Options</span>
                        </label>
                        <label>
                            <input type="radio" class="param-source-type" name="params[__INDEX__][source_type]" value="sql">
                            <span>SQL Query</span>
                        </label>
                    </div>
                    <div class="input-field static-options">
                        <textarea class="materialize-textarea param-options options-input" name="params[__INDEX__][options]"></textarea>
                        <label>Options (one per line)</label>
                    </div>
                    <div class="sql-options" style="display:none;">
                        <div class="input-field">
                            <textarea class="materialize-textarea param-sql sql-input" name="params[__INDEX__][sql]" placeholder="SELECT label, value FROM table"></textarea>
                            <label class="active">SQL Query</label>
                        </div>
                    </div>
                </div>
            </div>
        </div>
        <div class="row">
            <div class="col s12 m3">
                <label>
                    <input class="param-required" type="checkbox" name="params[__INDEX__][required]" value="1">
                    <span>Required</span>
                </label>
            </div>
            <div class="col s12 m4">
                <label>
                    <input class="param-multiple" type="checkbox" name="params[__INDEX__][multiple]" value="1">
                    <span>Allow multiple values</span>
                </label>
            </div>
            <div class="col s12 m5 right-align">
                <button type="button" class="btn-small waves-effect waves-light red remove-parameter-btn">
                    <i class="material-icons left">remove_circle</i>Remove
                </button>
            </div>
        </div>
    </div>
</template>

<script>
(() => {
    const parameterRows = document.getElementById('parameterRows');
    const addParameterBtn = document.getElementById('addParameterBtn');
    const template = document.getElementById('parameterRowTemplate');
    let paramIndex = 0;

    const syncRow = (row) => {
        const typeSelect = row.querySelector('.param-type');
        const isSelectType = typeSelect && typeSelect.value === 'select';
        const selectSourceSettings = row.querySelector('.select-source-settings');
        const multipleEl = row.querySelector('.param-multiple');
        const staticBox = row.querySelector('.static-options');
        const sqlBox = row.querySelector('.sql-options');
        const sourceType = row.querySelector('.param-source-type:checked');

        selectSourceSettings.classList.toggle('is-hidden', !isSelectType);
        if (!isSelectType) {
            multipleEl.checked = false;
            staticBox.style.display = 'block';
            sqlBox.style.display = 'none';
            return;
        }

        const useSql = sourceType && sourceType.value === 'sql';
        staticBox.style.display = useSql ? 'none' : 'block';
        sqlBox.style.display = useSql ? 'block' : 'none';
    };

    const addParameterRow = () => {
        const html = template.innerHTML.replaceAll('__INDEX__', String(paramIndex++));
        const wrapper = document.createElement('div');
        wrapper.innerHTML = html;
        const row = wrapper.firstElementChild;
        parameterRows.appendChild(row);
        syncRow(row);
        if (window.jrsInitMaterialize) {
            window.jrsInitMaterialize(row);
        }
    };

    addParameterBtn.addEventListener('click', addParameterRow);
    parameterRows.addEventListener('click', (event) => {
        const removeButton = event.target.closest('.remove-parameter-btn');
        if (!removeButton) {
            return;
        }
        removeButton.closest('.parameter-row')?.remove();
    });
    parameterRows.addEventListener('change', (event) => {
        const row = event.target.closest('.parameter-row');
        if (row) {
            syncRow(row);
        }
    });
})();
</script>
<?php jrs_material_page_footer(); ?>
