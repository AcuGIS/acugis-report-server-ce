<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_platform_admin($cconn);

$message = '';
$messageClass = '';
$stats = null;
$mount = rtrim((string) jrs_config('app_mount', '/acugis-report-server'), '/');

if ($_SERVER['REQUEST_METHOD'] === 'POST' && isset($_POST['sync_reports'])) {
    $syncConn = jrs_app_db();
    if (!$syncConn) {
        $message = 'Application database is not configured or unreachable. Check JRS_DB_* / config.php.';
        $messageClass = 'amber lighten-4 amber-text text-darken-4';
    } else {
        $stats = jrs_sync_all_jasper_report_resources($syncConn);
        if (($stats['skipped'] ?? false) === true) {
            $message = 'Database is not connected; sync was skipped.';
            $messageClass = 'amber lighten-4 amber-text text-darken-4';
        } elseif (($stats['ok'] ?? false) === true) {
            $message = sprintf(
                'Sync finished: %d newly registered, %d already in catalog, %d errors.',
                (int) ($stats['registered'] ?? 0),
                (int) ($stats['already_exists'] ?? 0),
                (int) ($stats['errors'] ?? 0)
            );
            $messageClass = 'green lighten-4 green-text text-darken-4';
        } else {
            $message = sprintf(
                'Sync finished with errors: %d registered, %d already existed, %d errors.',
                (int) ($stats['registered'] ?? 0),
                (int) ($stats['already_exists'] ?? 0),
                (int) ($stats['errors'] ?? 0)
            );
            $messageClass = 'red lighten-4 red-text text-darken-4';
        }
    }
}

jrs_material_page_header('Resource sync');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">Resource catalog sync</h1>
        <p class="page-subtitle">Backfill Jasper reports into local <code>resources</code> (idempotent). Requires administrator.</p>
    </div>
    <div class="header-actions">
        <a href="index.php" class="btn waves-effect waves-light grey">Back to reports</a>
    </div>
</header>

<div class="card">
    <div class="card-content">
        <span class="card-title">Jasper reports</span>
        <p class="muted-small">Upsert all <code>*.report.json</code> definitions and grant default permissions only for new rows.</p>
        <?php if ($message !== ''): ?>
            <div class="card-panel <?php echo jrs_html($messageClass); ?>" role="status"><?php echo jrs_html($message); ?></div>
        <?php endif; ?>
        <form method="post" class="stack-sm" style="margin-top:16px;">
            <input type="hidden" name="sync_reports" value="1">
            <button type="submit" class="btn waves-effect waves-light teal">
                <i class="material-icons left">sync</i>Sync reports to catalog
            </button>
        </form>
        <?php if (is_array($stats) && ($stats['skipped'] ?? false) !== true): ?>
            <pre class="jrs-url-output" style="margin-top:16px;"><?php echo jrs_html(json_encode($stats, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES)); ?></pre>
        <?php endif; ?>
    </div>
</div>

<div class="card">
    <div class="card-content">
        <span class="card-title">API</span>
        <p>Programmatic trigger (session cookie required):</p>
        <pre class="jrs-url-output">POST <?php echo jrs_html($mount); ?>/api/resource-sync/index.php</pre>
    </div>
</div>
<?php jrs_material_page_footer(); ?>
