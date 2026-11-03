<?php
declare(strict_types=1);

function jrs_schedule_ensure_schema($conn): void
{
    if (!$conn || !jrs_has_table($conn, 'jasper_schedule')) {
        return;
    }
}

function jrs_schedule_row_from_db(array $row): array
{
    $out = [
        'id' => (int) ($row['id'] ?? 0),
        'cron_period' => (string) ($row['cron_period'] ?? 'never'),
        'cron_custom' => (string) ($row['cron_custom'] ?? ''),
        'report_name' => (string) ($row['report_name'] ?? ''),
        'datasource_name' => (string) ($row['datasource_name'] ?? ''),
        'format' => (string) ($row['format'] ?? 'pdf'),
        'filename' => (string) ($row['filename'] ?? ''),
        'email' => (string) ($row['email'] ?? ''),
        'email_subject' => (string) ($row['email_subject'] ?? ''),
        'email_body' => (string) ($row['email_body'] ?? ''),
        'email_template' => (string) ($row['email_template'] ?? ''),
        'noemail' => jrs_schedule_bool_value($row['noemail'] ?? false, false),
        'url_opt_params' => (string) ($row['url_opt_params'] ?? ''),
        'enabled' => jrs_schedule_bool_value($row['enabled'] ?? true, true),
        'created_at' => (string) ($row['created_at'] ?? ''),
        'updated_at' => (string) ($row['updated_at'] ?? ''),
    ];

    if (array_key_exists('schedule_mode', $row)) {
        $out['schedule_mode'] = (string) ($row['schedule_mode'] ?? '');
        $out['repeat_minutes'] = isset($row['repeat_minutes']) ? (int) $row['repeat_minutes'] : null;
        $out['hourly_minute'] = isset($row['hourly_minute']) ? (int) $row['hourly_minute'] : null;
        $out['daily_time'] = (string) ($row['daily_time'] ?? '');
        $out['weekly_day'] = (string) ($row['weekly_day'] ?? '');
        $out['weekly_time'] = (string) ($row['weekly_time'] ?? '');
        $out['monthly_day'] = isset($row['monthly_day']) ? (int) $row['monthly_day'] : null;
        $out['monthly_time'] = (string) ($row['monthly_time'] ?? '');
        $out['once_datetime'] = (string) ($row['once_datetime'] ?? '');
    }

    return $out;
}

/**
 * @param array<string, mixed> $data
 * @return list<string|int|null>
 */
function jrs_schedule_repeat_insert_params(array $data): array
{
    return [
        (string) ($data['schedule_mode'] ?? ''),
        isset($data['repeat_minutes']) ? (int) $data['repeat_minutes'] : null,
        isset($data['hourly_minute']) ? (int) $data['hourly_minute'] : null,
        (string) ($data['daily_time'] ?? ''),
        (string) ($data['weekly_day'] ?? ''),
        (string) ($data['weekly_time'] ?? ''),
        isset($data['monthly_day']) ? (int) $data['monthly_day'] : null,
        (string) ($data['monthly_time'] ?? ''),
        (string) ($data['once_datetime'] ?? ''),
    ];
}

/**
 * @return list<array<string, mixed>>
 */
function jrs_schedule_list($conn): array
{
    if (!$conn || !jrs_has_table($conn, 'jasper_schedule')) {
        return [];
    }
    $sql = 'SELECT * FROM ' . jrs_table('jasper_schedule') . ' ORDER BY id DESC';
    $result = @pg_query($conn, $sql);
    if ($result === false) {
        return [];
    }
    $rows = [];
    while ($row = pg_fetch_assoc($result)) {
        $rows[] = jrs_schedule_row_from_db($row);
    }
    pg_free_result($result);
    return $rows;
}

function jrs_schedule_get($conn, int $id): ?array
{
    if (!$conn || $id <= 0 || !jrs_has_table($conn, 'jasper_schedule')) {
        return null;
    }
    $sql = 'SELECT * FROM ' . jrs_table('jasper_schedule') . ' WHERE id = $1';
    $result = @pg_query_params($conn, $sql, [$id]);
    if ($result === false) {
        return null;
    }
    $row = pg_fetch_assoc($result);
    pg_free_result($result);
    if ($row === false) {
        return null;
    }
    return jrs_schedule_row_from_db($row);
}

function jrs_schedule_delete($conn, int $id): bool
{
    if (!$conn || $id <= 0) {
        return false;
    }
    $result = @pg_query_params(
        $conn,
        'DELETE FROM ' . jrs_table('jasper_schedule') . ' WHERE id = $1',
        [$id]
    );
    if ($result === false) {
        return false;
    }
    $ok = pg_affected_rows($result) > 0;
    pg_free_result($result);
    return $ok;
}

/**
 * @param array<string, mixed> $data
 */
function jrs_schedule_create($conn, array $data): int
{
    $err = null;
    $id = jrs_schedule_create_ex($conn, $data, $err);
    return $id;
}

function jrs_schedule_create_ex($conn, array $data, ?string &$error = null): int
{
    $error = null;
    if (!$conn) {
        $error = 'Database connection unavailable.';
        return 0;
    }
    if (!jrs_has_table($conn, 'jasper_schedule')) {
        $error = 'Table jasper_schedule is missing. Run database/schema.sql on the Jasper app database.';
        return 0;
    }
    $baseCols = 'cron_period, cron_custom, report_name, datasource_name, format, filename,
        email, email_subject, email_body, email_template, noemail, url_opt_params, enabled';
    $basePlaceholders = '$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13';
    $params = [
        (string) ($data['cron_period'] ?? 'never'),
        (string) ($data['cron_custom'] ?? ''),
        (string) ($data['report_name'] ?? ''),
        (string) ($data['datasource_name'] ?? ''),
        (string) ($data['format'] ?? 'pdf'),
        (string) ($data['filename'] ?? ''),
        (string) ($data['email'] ?? ''),
        (string) ($data['email_subject'] ?? ''),
        (string) ($data['email_body'] ?? ''),
        (string) ($data['email_template'] ?? ''),
        jrs_pg_bool(!empty($data['noemail'])),
        (string) ($data['url_opt_params'] ?? ''),
        jrs_pg_bool(jrs_schedule_is_enabled($data)),
    ];

    if (jrs_schedule_schema_has_repeat_fields($conn)) {
        $baseCols .= ', schedule_mode, repeat_minutes, hourly_minute, daily_time, weekly_day, weekly_time, monthly_day, monthly_time, once_datetime';
        $basePlaceholders .= ',$14,$15,$16,$17,$18,$19,$20,$21,$22';
        $params = array_merge($params, jrs_schedule_repeat_insert_params($data));
    }

    $sql = 'INSERT INTO ' . jrs_table('jasper_schedule') . ' (' . $baseCols . ') VALUES (' . $basePlaceholders . ') RETURNING id';
    $result = @pg_query_params($conn, $sql, $params);
    if ($result === false) {
        $error = trim((string) @pg_last_error($conn));
        if ($error === '') {
            $error = 'INSERT into jasper_schedule failed.';
        }
        return 0;
    }
    $row = pg_fetch_assoc($result);
    pg_free_result($result);
    return (int) ($row['id'] ?? 0);
}

/**
 * @param array<string, mixed> $data
 */
function jrs_schedule_update($conn, int $id, array $data): bool
{
    if (!$conn || $id <= 0) {
        return false;
    }
    $setClause = 'cron_period = $2,
        cron_custom = $3,
        report_name = $4,
        datasource_name = $5,
        format = $6,
        filename = $7,
        email = $8,
        email_subject = $9,
        email_body = $10,
        email_template = $11,
        noemail = $12,
        url_opt_params = $13,
        enabled = $14';
    $params = [
        $id,
        (string) ($data['cron_period'] ?? 'never'),
        (string) ($data['cron_custom'] ?? ''),
        (string) ($data['report_name'] ?? ''),
        (string) ($data['datasource_name'] ?? ''),
        (string) ($data['format'] ?? 'pdf'),
        (string) ($data['filename'] ?? ''),
        (string) ($data['email'] ?? ''),
        (string) ($data['email_subject'] ?? ''),
        (string) ($data['email_body'] ?? ''),
        (string) ($data['email_template'] ?? ''),
        jrs_pg_bool(!empty($data['noemail'])),
        (string) ($data['url_opt_params'] ?? ''),
        jrs_pg_bool(jrs_schedule_is_enabled($data)),
    ];

    if (jrs_schedule_schema_has_repeat_fields($conn)) {
        $setClause .= ',
        schedule_mode = $15,
        repeat_minutes = $16,
        hourly_minute = $17,
        daily_time = $18,
        weekly_day = $19,
        weekly_time = $20,
        monthly_day = $21,
        monthly_time = $22,
        once_datetime = $23';
        $params = array_merge($params, jrs_schedule_repeat_insert_params($data));
    }

    $sql = 'UPDATE ' . jrs_table('jasper_schedule') . ' SET ' . $setClause . ', updated_at = now() WHERE id = $1';
    $result = @pg_query_params($conn, $sql, $params);
    if ($result === false) {
        return false;
    }
    pg_free_result($result);
    return true;
}

/**
 * Normalize POST into schedule row fields.
 *
 * @return array{ok:bool, data?:array<string,mixed>, error?:string}
 */
function jrs_schedule_normalize_post(array $post, $conn = null): array
{
    $legacyCron = strtolower(trim((string) ($post['cron_period'] ?? '')));
    $mode = strtolower(trim((string) ($post['schedule_mode'] ?? '')));
    if ($mode === '' && $legacyCron !== '') {
        if (in_array($legacyCron, jrs_schedule_modes(), true)) {
            $mode = $legacyCron;
        } elseif (in_array($legacyCron, ['never', 'now'], true)) {
            return ['ok' => false, 'error' => 'Select how often this schedule should run.'];
        }
    }
    if ($mode === '') {
        $mode = 'daily';
    }

    $modeCheck = jrs_schedule_validate_mode_fields($mode, $post);
    if (!$modeCheck['ok']) {
        return ['ok' => false, 'error' => (string) ($modeCheck['error'] ?? 'Invalid schedule.')];
    }

    $modeFields = $modeCheck['fields'] ?? [];
    $legacyCron = jrs_schedule_derive_legacy_cron($mode, $modeFields);
    $cronPeriod = $legacyCron['cron_period'];
    $cronCustom = $legacyCron['cron_custom'];

    $report = jrs_report_key((string) ($post['report_name'] ?? $post['name'] ?? ''));
    if (!jrs_is_valid_report_key($report)) {
        return ['ok' => false, 'error' => 'Select a valid Jasper report.'];
    }

    $definition = jrs_read_report($report, false);
    if ($definition === null) {
        return ['ok' => false, 'error' => 'Report definition not found.'];
    }

    $datasource = trim((string) ($post['datasource_name'] ?? $post['datasource'] ?? $definition['datasource'] ?? ''));
    if ($datasource === '') {
        return ['ok' => false, 'error' => 'Datasource is required.'];
    }
    if (jrs_find_datasource($datasource, $conn) === null) {
        return ['ok' => false, 'error' => 'Unknown datasource.'];
    }

    $format = strtolower(trim((string) ($post['format'] ?? 'pdf')));
    if (!in_array($format, jrs_schedule_output_formats(), true)) {
        return ['ok' => false, 'error' => 'Unsupported output format.'];
    }

    $filename = trim((string) ($post['filename'] ?? ''));
    if ($filename === '') {
        return ['ok' => false, 'error' => 'Filename is required.'];
    }

    $noemail = !empty($post['noemail']) && (string) $post['noemail'] !== 'f';
    $email = trim((string) ($post['email'] ?? ''));
    if (!$noemail && $email === '') {
        return ['ok' => false, 'error' => 'Email recipients are required unless email is disabled.'];
    }

    $row = [
        'cron_period' => $cronPeriod,
        'cron_custom' => $cronCustom,
        'schedule_mode' => $mode,
        'report_name' => $report,
        'datasource_name' => $datasource,
        'format' => $format,
        'filename' => $filename,
        'email' => $noemail ? '' : $email,
        'email_subject' => $noemail ? '' : trim((string) ($post['email_subject'] ?? $post['email_subj'] ?? '')),
        'email_body' => $noemail ? '' : trim((string) ($post['email_body'] ?? '')),
        'email_template' => $noemail ? '' : trim((string) ($post['email_template'] ?? $post['email_tmpl'] ?? '')),
        'noemail' => $noemail,
        'url_opt_params' => trim((string) ($post['url_opt_params'] ?? '')),
        'enabled' => jrs_schedule_enabled_from_post($post, $mode),
    ];

    if ($mode === 'repeat_minutes') {
        $row['repeat_minutes'] = (int) ($modeFields['repeat_minutes'] ?? 10);
    }
    if ($mode === 'hourly') {
        $row['hourly_minute'] = (int) ($modeFields['hourly_minute'] ?? 0);
    }
    if ($mode === 'daily') {
        $row['daily_time'] = (string) ($modeFields['daily_time'] ?? '08:00');
    }
    if ($mode === 'weekly') {
        $row['weekly_day'] = (string) ($modeFields['weekly_day'] ?? 'Monday');
        $row['weekly_time'] = (string) ($modeFields['weekly_time'] ?? '08:00');
    }
    if ($mode === 'monthly') {
        $row['monthly_day'] = (int) ($modeFields['monthly_day'] ?? 1);
        $row['monthly_time'] = (string) ($modeFields['monthly_time'] ?? '08:00');
    }
    if ($mode === 'once') {
        $row['once_datetime'] = (string) ($modeFields['once_datetime'] ?? '');
    }

    return ['ok' => true, 'data' => $row];
}
