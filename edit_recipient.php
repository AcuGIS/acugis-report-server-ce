<?php
declare(strict_types=1);

require_once __DIR__ . '/lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'view');
jrs_require_schedule_admin($cconn);

$id = isset($_GET['id']) ? (int) $_GET['id'] : 0;
$conn = jrs_app_db();
$row = ['name' => '', 'email' => '', 'notes' => ''];
if ($conn && $id > 0) {
    $loaded = jrs_recipient_get($conn, $id);
    if ($loaded !== null) {
        $row = $loaded;
    }
    pg_close($conn);
}

$title = $id > 0 ? 'Edit recipient' : 'Add recipient';
jrs_material_page_header($title);
?>
<header class="page-header">
    <div><h1 class="page-title"><?php echo jrs_html($title); ?></h1></div>
    <div class="header-actions"><a href="recipients.php" class="btn btn-flat">Back</a></div>
</header>
<div class="card"><div class="card-content">
<form id="rcpt_form">
<?php if ($id > 0): ?><input type="hidden" name="id" value="<?php echo $id; ?>"><?php endif; ?>
<div class="input-field"><input name="name" value="<?php echo jrs_html((string) $row['name']); ?>"><label class="active">Name</label></div>
<div class="input-field"><input name="email" type="email" value="<?php echo jrs_html((string) $row['email']); ?>" required><label class="active">Email</label></div>
<div class="input-field"><textarea name="notes" class="materialize-textarea"><?php echo jrs_html((string) $row['notes']); ?></textarea><label class="active">Notes</label></div>
<button type="button" class="btn teal" id="save_btn">Save</button>
</form>
</div></div>
<script>
document.getElementById('save_btn').addEventListener('click', function () {
    fetch('action/recipient.php', { method: 'POST', body: new FormData(document.getElementById('rcpt_form')), credentials: 'same-origin' })
        .then(function (r) { return r.json(); })
        .then(function (res) { alert(res.message || ''); if (res.success) location.href = 'recipients.php'; });
});
</script>
<?php jrs_material_page_footer(); ?>
