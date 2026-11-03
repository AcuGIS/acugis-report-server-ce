<?php
declare(strict_types=1);

require_once __DIR__ . '/lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'view');
jrs_require_schedule_admin($cconn);

$conn = jrs_app_db();
$schedules = $conn ? jrs_schedule_list($conn) : [];
$datasourceMap = [];
if ($conn) {
    foreach (jrs_datasources($conn) as $ds) {
        $datasourceMap[(string) $ds['name']] = (string) $ds['name'];
    }
    pg_close($conn);
}

jrs_material_page_header('Schedules');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">Schedules</h1>
        <p class="page-subtitle">Automate Jasper report generation and email delivery.</p>
    </div>
    <div class="header-actions">
        <a href="edit_schedule.php" class="btn waves-effect waves-light teal">
            <i class="material-icons left">add</i>Add schedule
        </a>
    </div>
</header>

<div class="card">
    <div class="card-content connections-table-wrap">
        <?php if ($schedules === []): ?>
            <div class="empty-state">
                <i class="material-icons">schedule</i>
                <p>No schedules configured yet.</p>
            </div>
        <?php else: ?>
            <table class="striped highlight connections-table responsive-table">
                <thead>
                    <tr>
                        <th>ID</th>
                        <th>Schedule</th>
                        <th>Report</th>
                        <th>Datasource</th>
                        <th>Format</th>
                        <th>Filename</th>
                        <th>Email</th>
                        <th>Enabled</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                <?php foreach ($schedules as $sched): ?>
                    <?php $sid = (int) ($sched['id'] ?? 0); ?>
                    <tr data-id="<?php echo $sid; ?>">
                        <td><?php echo $sid; ?></td>
                        <td><?php echo jrs_html(jrs_schedule_summary($sched)); ?></td>
                        <td><?php echo jrs_html((string) ($sched['report_name'] ?? '')); ?></td>
                        <td><?php echo jrs_html((string) ($sched['datasource_name'] ?? '')); ?></td>
                        <td><?php echo jrs_html((string) ($sched['format'] ?? '')); ?></td>
                        <td><?php echo jrs_html((string) ($sched['filename'] ?? '')); ?></td>
                        <td><?php echo !empty($sched['noemail']) ? '—' : jrs_html((string) ($sched['email'] ?? '')); ?></td>
                        <td><?php echo jrs_schedule_is_enabled($sched) ? 'Yes' : 'No'; ?></td>
                        <td class="card-actions-row">
                            <a href="edit_schedule.php?id=<?php echo $sid; ?>" class="btn-small waves-effect" title="Edit">
                                <i class="material-icons">edit</i>
                            </a>
                            <button type="button" class="btn-small waves-effect blue-grey sch-run" data-id="<?php echo $sid; ?>" title="Run now">
                                <i class="material-icons">play_arrow</i>
                            </button>
                            <button type="button" class="btn-small waves-effect red sch-delete" data-id="<?php echo $sid; ?>" title="Delete">
                                <i class="material-icons">delete</i>
                            </button>
                        </td>
                    </tr>
                <?php endforeach; ?>
                </tbody>
            </table>
        <?php endif; ?>
    </div>
</div>

<script>
document.addEventListener('DOMContentLoaded', function () {
    function postSchedule(body) {
        return fetch('action/schedule.php', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: new URLSearchParams(body).toString(),
            credentials: 'same-origin'
        }).then(function (r) {
            return r.text().then(function (text) {
                try {
                    return JSON.parse(text);
                } catch (e) {
                    throw new Error(text ? text.slice(0, 500) : ('HTTP ' + r.status));
                }
            });
        });
    }

    document.querySelectorAll('.sch-delete').forEach(function (btn) {
        btn.addEventListener('click', function () {
            var id = btn.getAttribute('data-id');
            if (!confirm('Delete schedule #' + id + '?')) return;
            postSchedule({ delete: '1', id: id }).then(function (res) {
                alert(res.message || (res.success ? 'Deleted' : 'Failed'));
                if (res.success) location.reload();
            }).catch(function (err) { alert(err.message || String(err)); });
        });
    });

    document.querySelectorAll('.sch-run').forEach(function (btn) {
        btn.addEventListener('click', function () {
            var id = btn.getAttribute('data-id');
            postSchedule({ run_now: '1', id: id }).then(function (res) {
                alert(res.message || (res.success ? 'Started' : 'Failed'));
            }).catch(function (err) { alert(err.message || String(err)); });
        });
    });
});
</script>
<?php jrs_material_page_footer(); ?>
