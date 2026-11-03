<?php
declare(strict_types=1);

require_once __DIR__ . '/lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'view');
jrs_require_schedule_admin($cconn);

$conn = jrs_app_db();
$recipients = $conn ? jrs_recipient_list($conn) : [];
if ($conn) {
    pg_close($conn);
}

jrs_material_page_header('Recipients');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">Recipients</h1>
        <p class="page-subtitle">Directory of email addresses used when building schedules.</p>
    </div>
    <div class="header-actions">
        <a href="edit_recipient.php" class="btn waves-effect waves-light teal"><i class="material-icons left">add</i>Add recipient</a>
    </div>
</header>
<div class="card"><div class="card-content connections-table-wrap">
<?php if ($recipients === []): ?>
    <div class="empty-state"><p>No recipients yet.</p></div>
<?php else: ?>
<table class="striped highlight connections-table">
<thead><tr><th>Name</th><th>Email</th><th>Notes</th><th></th></tr></thead>
<tbody>
<?php foreach ($recipients as $row): ?>
<tr>
    <td><?php echo jrs_html((string) $row['name']); ?></td>
    <td><?php echo jrs_html((string) $row['email']); ?></td>
    <td><?php echo jrs_html((string) $row['notes']); ?></td>
    <td>
        <a class="btn-small" href="edit_recipient.php?id=<?php echo (int) $row['id']; ?>"><i class="material-icons">edit</i></a>
        <button type="button" class="btn-small red rcpt-delete" data-id="<?php echo (int) $row['id']; ?>"><i class="material-icons">delete</i></button>
    </td>
</tr>
<?php endforeach; ?>
</tbody>
</table>
<?php endif; ?>
</div></div>
<script>
document.querySelectorAll('.rcpt-delete').forEach(function (btn) {
    btn.addEventListener('click', function () {
        if (!confirm('Delete recipient?')) return;
        fetch('action/recipient.php', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: 'delete=1&id=' + encodeURIComponent(btn.getAttribute('data-id')),
            credentials: 'same-origin'
        }).then(function (r) { return r.json(); }).then(function (res) {
            if (res.success) location.reload();
            else alert(res.message || 'Failed');
        });
    });
});
</script>
<?php jrs_material_page_footer(); ?>
