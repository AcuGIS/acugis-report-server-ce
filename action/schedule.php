<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

header('Content-Type: application/json; charset=utf-8');

$result = ['success' => false, 'message' => 'Error while processing your request.'];

try {
    $cconn = jrs_auth_require_central_db();
    jrs_require_schedule_admin($cconn);

    $conn = jrs_app_db();
    if (!$conn) {
        jrs_json_response(['success' => false, 'message' => 'Application database unavailable.'], 503);
    }

    $optionalKeys = ['email', 'email_subject', 'email_subj', 'email_body', 'url_opt_params', 'email_template', 'email_tmpl'];

    if (isset($_POST['delete'])) {
        $id = (int) ($_POST['id'] ?? 0);
        $sched = jrs_schedule_get($conn, $id);
        if ($sched === null) {
            jrs_json_response(['success' => false, 'message' => 'No such schedule.']);
        }

        jrs_schedule_remove_systemd($id);
        jrs_schedule_remove_env_file($id);

        if (jrs_schedule_delete($conn, $id)) {
            jrs_json_response(['success' => true, 'message' => 'Schedule ' . $id . ' deleted.']);
        }
        jrs_json_response(['success' => false, 'message' => 'Schedule delete failed.']);
    }

    if (isset($_POST['run_now'])) {
        $id = (int) ($_POST['id'] ?? 0);
        if (jrs_schedule_get($conn, $id) === null) {
            jrs_json_response(['success' => false, 'message' => 'No such schedule.']);
        }
        // Run in-process so Run now works even when PHP-FPM disables exec().
        $run = jrs_schedule_execute($id, 'manual', $conn);
        if (empty($run['ok'])) {
            jrs_json_response([
                'success' => false,
                'message' => (string) ($run['message'] ?? 'Schedule run failed.'),
                'execution_id' => (int) ($run['execution_id'] ?? 0),
            ]);
        }
        jrs_json_response([
            'success' => true,
            'message' => 'Report generated: ' . (string) ($run['output_file'] ?? ''),
            'execution_id' => (int) ($run['execution_id'] ?? 0),
            'output_file' => (string) ($run['output_file'] ?? ''),
            'email_status' => (string) ($run['email_status'] ?? ''),
        ]);
    }

    if (isset($_POST['update'])) {
        $id = (int) ($_POST['id'] ?? 0);
        if ($id <= 0) {
            jrs_json_response(['success' => false, 'message' => 'Invalid schedule id.']);
        }

        $noemail = !empty($_POST['noemail']) && (string) $_POST['noemail'] !== 'f';
        if (!$noemail && trim((string) ($_POST['email'] ?? '')) === '') {
            jrs_json_response(['success' => false, 'message' => 'Email recipients are required.']);
        }

        $normalized = jrs_schedule_normalize_post($_POST, $conn);
        if (!$normalized['ok']) {
            jrs_json_response(['success' => false, 'message' => (string) ($normalized['error'] ?? 'Invalid input.')]);
        }
        $data = $normalized['data'];
        if ($noemail) {
            $data['noemail'] = true;
            $data['email'] = '';
            $data['email_subject'] = '';
            $data['email_body'] = '';
            $data['email_template'] = '';
        }

        if (jrs_schedule_get($conn, $id) === null) {
            jrs_json_response(['success' => false, 'message' => 'No such schedule.']);
        }

        if (!jrs_schedule_update($conn, $id, $data)) {
            jrs_json_response(['success' => false, 'message' => 'Schedule record update failed.']);
        }

        $saved = jrs_schedule_get($conn, $id);
        if ($saved === null) {
            jrs_json_response(['success' => false, 'message' => 'Schedule saved but reload failed.']);
        }

        $sync = jrs_schedule_sync_systemd_after_save($id, $saved, $conn);
        if (!$sync['ok']) {
            $detail = trim((string) ($sync['error'] ?? ''));
            $message = !empty($sync['timer_required'])
                ? 'Schedule ' . $id . ' saved but systemd timer install failed.'
                    . ' The previous timer remains active when one existed.'
                : 'Schedule ' . $id . ' saved but environment file write failed.';
            jrs_json_response([
                'success' => false,
                'message' => $message . ($detail !== '' ? ' ' . $detail : ' Check schedules directory permissions and PHP error log.'),
            ]);
        }

        $cron = (string) ($saved['cron_period'] ?? 'never');
        if ($cron === 'now') {
            jrs_schedule_run_async($id);
            jrs_json_response(['success' => true, 'message' => 'Schedule ' . $id . ' updated and started.']);
        }

        jrs_json_response(['success' => true, 'message' => 'Schedule ' . $id . ' updated.']);
    }

    if (isset($_POST['create'])) {
        $noemail = !empty($_POST['noemail']) && (string) $_POST['noemail'] !== 'f';
        if (!$noemail && trim((string) ($_POST['email'] ?? '')) === '') {
            jrs_json_response(['success' => false, 'message' => 'Email recipients are required.']);
        }

        $normalized = jrs_schedule_normalize_post($_POST, $conn);
        if (!$normalized['ok']) {
            jrs_json_response(['success' => false, 'message' => (string) ($normalized['error'] ?? 'Invalid input.')]);
        }
        $data = $normalized['data'];
        if ($noemail) {
            $data['noemail'] = true;
            $data['email'] = '';
            $data['email_subject'] = '';
            $data['email_body'] = '';
            $data['email_template'] = '';
        }

        $dbErr = null;
        $id = jrs_schedule_create_ex($conn, $data, $dbErr);
        if ($id <= 0) {
            jrs_json_response(['success' => false, 'message' => $dbErr ?: 'Schedule record create failed.']);
        }

        $saved = jrs_schedule_get($conn, $id);
        if ($saved === null) {
            jrs_json_response(['success' => false, 'message' => 'Schedule created but reload failed.', 'id' => $id]);
        }

        $sync = jrs_schedule_sync_systemd_after_save($id, $saved, $conn);
        if (!$sync['ok']) {
            $detail = trim((string) ($sync['error'] ?? ''));
            $message = !empty($sync['timer_required'])
                ? 'Schedule ' . $id . ' created but systemd timer install failed.'
                : 'Schedule ' . $id . ' created but environment file write failed.';
            jrs_json_response([
                'success' => false,
                'message' => $message . ($detail !== '' ? ' ' . $detail : ' Check schedules directory permissions and PHP error log.'),
                'id' => $id,
            ]);
        }

        $cron = (string) ($saved['cron_period'] ?? 'never');
        if ($cron === 'now') {
            $started = jrs_schedule_run_async($id);
            jrs_json_response([
                'success' => true,
                'message' => $started
                    ? 'Schedule ' . $id . ' created and started.'
                    : 'Schedule ' . $id . ' created but background run could not start (exec disabled or missing run script).',
                'id' => $id,
            ]);
        }

        jrs_json_response(['success' => true, 'message' => 'Schedule ' . $id . ' created.', 'id' => $id]);
    }

    jrs_json_response($result);
} catch (Throwable $e) {
    error_log('[jrs-schedule] ' . $e->getMessage() . ' in ' . $e->getFile() . ':' . $e->getLine());
    jrs_json_response([
        'success' => false,
        'message' => 'Server error: ' . $e->getMessage(),
    ], 500);
}
