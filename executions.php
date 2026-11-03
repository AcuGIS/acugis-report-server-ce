<?php
declare(strict_types=1);

require_once __DIR__ . '/lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'view');
jrs_require_schedule_admin($cconn);

$conn = jrs_app_db();
$schedules = $conn ? jrs_schedule_list($conn) : [];
$latest = [];
if ($conn) {
    foreach ($schedules as $sched) {
        $sid = (int) ($sched['id'] ?? 0);
        $latest[$sid] = jrs_execution_latest_for_schedule($conn, $sid);
    }
    pg_close($conn);
}

jrs_material_page_header('Executions');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">Executions</h1>
        <p class="page-subtitle">Schedule status and most recent run per job.</p>
    </div>
    <div class="header-actions">
        <a href="history.php" class="btn btn-flat">Full history</a>
    </div>
</header>

<div class="card">
    <div class="card-content connections-table-wrap">
        <?php if ($schedules === []): ?>
            <div class="empty-state"><p>No schedules configured.</p></div>
        <?php else: ?>
            <table class="striped highlight connections-table">
                <thead>
                    <tr>
                        <th>Schedule</th>
                        <th>Report</th>
                        <th>Timing</th>
                        <th>Enabled</th>
                        <th>Last status</th>
                        <th>Last run</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                <?php foreach ($schedules as $sched): ?>
                    <?php
                    $sid = (int) ($sched['id'] ?? 0);
                    $last = $latest[$sid] ?? null;
                    ?>
                    <tr>
                        <td>#<?php echo $sid; ?></td>
                        <td><?php echo jrs_html((string) ($sched['report_name'] ?? '')); ?></td>
                        <td><?php echo jrs_html(jrs_schedule_summary($sched)); ?></td>
                        <td><?php echo jrs_schedule_is_enabled($sched) ? 'Yes' : 'No'; ?></td>
                        <td><?php echo jrs_html((string) ($last['status'] ?? '—')); ?></td>
                        <td><?php echo jrs_html((string) ($last['started_at'] ?? '—')); ?></td>
                        <td>
                            <a class="btn-small" href="history.php?schedule_id=<?php echo $sid; ?>">History</a>
                            <button type="button" class="btn-small blue-grey sch-run" data-id="<?php echo $sid; ?>">Run now</button>
                        </td>
                    </tr>
                <?php endforeach; ?>
                </tbody>
            </table>
        <?php endif; ?>
    </div>
</div>
<script>
document.querySelectorAll('.sch-run').forEach(function (btn) {
    btn.addEventListener('click', function () {
        fetch('action/schedule.php', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: 'run_now=1&id=' + encodeURIComponent(btn.getAttribute('data-id')),
            credentials: 'same-origin'
        }).then(function (r) { return r.json(); }).then(function (res) {
            alert(res.message || '');
            if (res.success) location.reload();
        });
    });
});
</script>
<?php jrs_material_page_footer(); ?>
