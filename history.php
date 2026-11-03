<?php
declare(strict_types=1);

require_once __DIR__ . '/lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'view');
jrs_require_schedule_admin($cconn);

$scheduleFilter = isset($_GET['schedule_id']) ? (int) $_GET['schedule_id'] : null;
$conn = jrs_app_db();
$rows = $conn ? jrs_execution_list($conn, $scheduleFilter) : [];
$schedules = $conn ? jrs_schedule_list($conn) : [];
if ($conn) {
    pg_close($conn);
}

jrs_material_page_header('History');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">History</h1>
        <p class="page-subtitle">Recent scheduled and on-demand report executions.</p>
    </div>
</header>

<div class="card" style="margin-bottom: 20px;">
    <div class="card-content">
        <form method="get" class="row">
            <div class="input-field col s12 m8">
                <select name="schedule_id" onchange="this.form.submit()">
                    <option value="">All schedules</option>
                    <?php foreach ($schedules as $s): ?>
                        <option value="<?php echo (int) $s['id']; ?>" <?php echo $scheduleFilter === (int) $s['id'] ? 'selected' : ''; ?>>
                            #<?php echo (int) $s['id']; ?> — <?php echo jrs_html((string) $s['report_name']); ?>
                        </option>
                    <?php endforeach; ?>
                </select>
                <label>Filter by schedule</label>
            </div>
        </form>
    </div>
</div>

<div class="card">
    <div class="card-content connections-table-wrap">
        <?php if ($rows === []): ?>
            <div class="empty-state"><p>No execution history yet.</p></div>
        <?php else: ?>
            <table class="striped highlight connections-table">
                <thead>
                    <tr>
                        <th>ID</th>
                        <th>Schedule</th>
                        <th>Report</th>
                        <th>Status</th>
                        <th>Started</th>
                        <th>Finished</th>
                        <th>Email</th>
                        <th>Output</th>
                        <th>Error</th>
                    </tr>
                </thead>
                <tbody>
                <?php foreach ($rows as $row): ?>
                    <tr>
                        <td><?php echo (int) ($row['id'] ?? 0); ?></td>
                        <td><?php echo $row['schedule_id'] !== null ? (int) $row['schedule_id'] : '—'; ?></td>
                        <td><?php echo jrs_html((string) ($row['report_name'] ?? '')); ?></td>
                        <td><?php echo jrs_html((string) ($row['status'] ?? '')); ?></td>
                        <td><?php echo jrs_html((string) ($row['started_at'] ?? '')); ?></td>
                        <td><?php echo jrs_html((string) ($row['finished_at'] ?? '')); ?></td>
                        <td><?php echo jrs_html((string) ($row['email_status'] ?? '')); ?></td>
                        <td><code><?php echo jrs_html((string) ($row['output_file'] ?? '')); ?></code></td>
                        <td><?php echo jrs_html((string) ($row['error_message'] ?? '')); ?></td>
                    </tr>
                <?php endforeach; ?>
                </tbody>
            </table>
        <?php endif; ?>
    </div>
</div>
<?php jrs_material_page_footer(); ?>
