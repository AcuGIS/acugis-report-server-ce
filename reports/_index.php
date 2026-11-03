<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

// Local auth via app database (lib/auth.php).
$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'view');

if ($cconn) {
    jrs_sync_all_jasper_report_resources($cconn);
}

jrs_ensure_dir(jrs_json_dir());
jrs_ensure_dir(jrs_report_files_dir());
jrs_ensure_dir(jrs_report_assets_dir());

$reports = [];
foreach (jrs_filter_reports_for_user($cconn, jrs_all_reports(false)) as $json) {
    $reportKey = trim((string) ($json['report'] ?? ''));
    if ($reportKey === '') {
        continue;
    }

    $reports[] = [
        'name' => (string) ($json['name'] ?? 'Report'),
        'description' => trim((string) ($json['description'] ?? $json['summary'] ?? '')),
        'report' => $reportKey,
        'datasource' => (string) ($json['datasource'] ?? ''),
        'thumbnail_exists' => is_file(jrs_report_asset_path($reportKey)),
    ];
}

$user = jrs_get_current_user();
$uid = (int) ($user['id'] ?? 0);
$isAdminOrEditor = (bool) ($user['is_admin'] ?? false)
    || jrs_auth_groups_include_admin((array) ($user['groups'] ?? []))
    || jrs_has_permission($cconn, $uid, JRS_RESOURCE_TYPE_SERVICE, 'jasper_reports', 'edit');

jrs_material_page_header('Reports', [
    'no_sidebar' => true,
    'no_back' => !$isAdminOrEditor,
]);
?>
<header class="page-header">
    <div>
        <h1 class="page-title">Reports</h1>
        <p class="page-subtitle">Standalone Jasper report catalog backed by configured datasources.</p>
    </div>
    <?php if ($isAdminOrEditor): ?>
    <div class="header-actions">
        <a href="publish_report.php" class="btn waves-effect waves-light teal">
            <i class="material-icons left">add</i>Publish new report
        </a>
    </div>
    <?php endif; ?>
</header>

<?php if ($reports === []): ?>
    <div class="card">
        <div class="card-content empty-state">
            <i class="material-icons">assessment</i>
            <p>No reports published yet.</p>
            <?php if ($isAdminOrEditor): ?>
            <a href="publish_report.php" class="btn waves-effect waves-light teal">Publish your first report</a>
            <?php endif; ?>
        </div>
    </div>
<?php else: ?>
    <div class="report-grid">
        <?php foreach ($reports as $row): ?>
            <?php
            $report = $row['report'];
            $datasource = $row['datasource'];
            $uid = (int) (jrs_get_current_user()['id'] ?? 0);
            $isPrivAdmin = (bool) (jrs_get_current_user()['is_admin'] ?? false)
                || jrs_auth_groups_include_admin((array) (jrs_get_current_user()['groups'] ?? []));
            $canEdit = $isPrivAdmin || jrs_has_permission($cconn, $uid, JRS_RESOURCE_TYPE_JASPER_REPORT, $report, 'edit');
            $canAdmin = $isPrivAdmin || jrs_has_permission($cconn, $uid, JRS_RESOURCE_TYPE_JASPER_REPORT, $report, 'admin');
            $thumbUrl = $row['thumbnail_exists'] ? 'report_thumbnail.php?report=' . rawurlencode($report) : '';
            $viewerUrl = 'view.php?report=' . rawurlencode($report) . '&datasource=' . rawurlencode($datasource);
            $description = (string) ($row['description'] ?? '');
            ?>
            <div class="card report-card">
                <?php if ($thumbUrl !== ''): ?>
                    <img src="<?php echo jrs_html($thumbUrl); ?>" class="report-card-thumb" alt="">
                <?php else: ?>
                    <div class="report-card-placeholder">
                        <i class="material-icons">image_not_supported</i>
                    </div>
                <?php endif; ?>
                <div class="card-content">
                    <span class="card-title"><?php echo jrs_html($row['name']); ?></span>
                    <?php if ($description !== ''): ?>
                        <p class="report-card-description"><?php echo jrs_html($description); ?></p>
                    <?php endif; ?>
                </div>
                <div class="card-action card-actions-row">
                    <a class="btn-small waves-effect waves-light teal" href="<?php echo jrs_html($viewerUrl); ?>">
                        <i class="material-icons left">open_in_browser</i>Open
                    </a>
                    <?php if ($canEdit): ?>
                    <a class="btn-small waves-effect waves-light blue-grey" href="edit_report.php?report=<?php echo rawurlencode($report); ?>">
                        <i class="material-icons left">edit</i>Edit
                    </a>
                    <?php endif; ?>
                    <?php if ($canAdmin): ?>
                    <a class="btn-small waves-effect waves-light red" href="delete_report.php?report=<?php echo rawurlencode($report); ?>" onclick="return confirm('Delete this report?');">
                        <i class="material-icons left">delete</i>Delete
                    </a>
                    <?php endif; ?>
                </div>
            </div>
        <?php endforeach; ?>
    </div>
<?php endif; ?>
<?php jrs_material_page_footer(); ?>
