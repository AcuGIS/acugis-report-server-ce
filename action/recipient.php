<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_schedule_admin($cconn);

$conn = jrs_app_db();
if (!$conn) {
    jrs_json_response(['success' => false, 'message' => 'Database unavailable.'], 503);
}

if (isset($_POST['delete'])) {
    $id = (int) ($_POST['id'] ?? 0);
    jrs_json_response([
        'success' => jrs_recipient_delete($conn, $id),
        'message' => 'Recipient deleted.',
    ]);
}

$name = trim((string) ($_POST['name'] ?? ''));
$email = trim((string) ($_POST['email'] ?? ''));
$notes = trim((string) ($_POST['notes'] ?? ''));
$id = isset($_POST['id']) ? (int) $_POST['id'] : null;

if ($email === '' || !filter_var($email, FILTER_VALIDATE_EMAIL)) {
    jrs_json_response(['success' => false, 'message' => 'Valid email is required.']);
}

$ok = jrs_recipient_save($conn, $id > 0 ? $id : null, $name, $email, $notes);
pg_close($conn);
jrs_json_response(['success' => $ok, 'message' => $ok ? 'Recipient saved.' : 'Save failed.']);
