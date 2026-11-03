<?php
declare(strict_types=1);

/**
 * Repeat scheduler: friendly modes, validation, summaries, and systemd timer specs.
 */

function jrs_schedule_modes(): array
{
    return ['once', 'repeat_minutes', 'hourly', 'daily', 'weekly', 'monthly', 'custom'];
}

function jrs_schedule_weekly_days(): array
{
    return ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
}

function jrs_schedule_weekly_day_to_systemd(string $day): string
{
    $map = [
        'monday' => 'Mon',
        'tuesday' => 'Tue',
        'wednesday' => 'Wed',
        'thursday' => 'Thu',
        'friday' => 'Fri',
        'saturday' => 'Sat',
        'sunday' => 'Sun',
    ];
    $key = strtolower(trim($day));
    return $map[$key] ?? 'Mon';
}

function jrs_schedule_cron_periods(): array
{
    return ['never', 'now', 'once', 'repeat_minutes', 'hourly', 'daily', 'weekly', 'monthly', 'custom'];
}

/** Parse PostgreSQL / form boolean (t/f, 1/0, true/false). */
function jrs_schedule_bool_value($value, bool $default = true): bool
{
    if ($value === true || $value === 1) {
        return true;
    }
    if ($value === false || $value === 0) {
        return false;
    }
    if ($value === null) {
        return $default;
    }
    $v = strtolower(trim((string) $value));
    if ($v === '') {
        return $default;
    }
    if (in_array($v, ['t', 'true', '1', 'yes', 'on'], true)) {
        return true;
    }
    if (in_array($v, ['f', 'false', '0', 'no', 'off'], true)) {
        return false;
    }
    return $default;
}

/**
 * Whether schedule row is enabled (explicit off only when stored as false).
 *
 * @param array<string, mixed> $schedule
 */
function jrs_schedule_is_enabled(array $schedule): bool
{
    if (!array_key_exists('enabled', $schedule)) {
        return jrs_schedule_effective_mode($schedule) !== 'never'
            && strtolower((string) ($schedule['cron_period'] ?? 'never')) !== 'never';
    }
    return jrs_schedule_bool_value($schedule['enabled'], true);
}

/**
 * enabled flag from form POST: default true for all timing modes unless user unchecked.
 */
function jrs_schedule_enabled_from_post(array $post, string $mode): bool
{
    if (!array_key_exists('enabled', $post)) {
        return in_array($mode, jrs_schedule_modes(), true);
    }
    $raw = $post['enabled'];
    if (is_array($raw)) {
        $raw = (string) end($raw);
    }
    return jrs_schedule_bool_value($raw, true);
}

function jrs_schedule_schema_has_repeat_fields($conn): bool
{
    static $cache = null;
    if ($cache !== null) {
        return $cache;
    }
    $cache = false;
    if (!$conn || !jrs_has_table($conn, 'jasper_schedule')) {
        return false;
    }
    $schema = jrs_app_schema();
    $sql = 'SELECT 1 FROM information_schema.columns
        WHERE table_schema = $1 AND table_name = $2 AND column_name = $3 LIMIT 1';
    $result = @pg_query_params($conn, $sql, [$schema, 'jasper_schedule', 'schedule_mode']);
    if ($result !== false) {
        $row = pg_fetch_assoc($result);
        pg_free_result($result);
        $cache = $row !== false;
    }
    return $cache;
}

/**
 * Effective schedule mode for timer generation and UI.
 */
function jrs_schedule_effective_mode(array $schedule): string
{
    $mode = strtolower(trim((string) ($schedule['schedule_mode'] ?? '')));
    if ($mode !== '' && in_array($mode, jrs_schedule_modes(), true)) {
        return $mode;
    }

    $cron = strtolower(trim((string) ($schedule['cron_period'] ?? 'never')));
    if ($cron === 'repeat_minutes') {
        return 'repeat_minutes';
    }
    if ($cron === 'once') {
        return 'once';
    }
    if (in_array($cron, ['hourly', 'daily', 'weekly', 'monthly', 'custom'], true)) {
        return $cron;
    }
    if ($cron === 'now') {
        return 'once';
    }

    return 'never';
}

/**
 * @return array{hourly_minute:int,daily_time:string,weekly_day:string,weekly_time:string,monthly_day:int,monthly_time:string,repeat_minutes:int,once_datetime:string,cron_custom:string}
 */
function jrs_schedule_form_field_defaults(array $schedule): array
{
    $mode = jrs_schedule_effective_mode($schedule);
    $cron = strtolower(trim((string) ($schedule['cron_period'] ?? 'never')));

    $repeatMinutes = (int) ($schedule['repeat_minutes'] ?? 0);
    if ($repeatMinutes < 1) {
        $repeatMinutes = 10;
    }

    $hourlyMinute = array_key_exists('hourly_minute', $schedule)
        ? (int) $schedule['hourly_minute']
        : 0;

    $dailyTime = trim((string) ($schedule['daily_time'] ?? ''));
    if ($dailyTime === '') {
        $dailyTime = '08:00';
    }

    $weeklyDay = trim((string) ($schedule['weekly_day'] ?? ''));
    if ($weeklyDay === '') {
        $weeklyDay = 'Monday';
    }

    $weeklyTime = trim((string) ($schedule['weekly_time'] ?? ''));
    if ($weeklyTime === '') {
        $weeklyTime = '08:00';
    }

    $monthlyDay = (int) ($schedule['monthly_day'] ?? 0);
    if ($monthlyDay < 1 || $monthlyDay > 31) {
        $monthlyDay = 1;
    }

    $monthlyTime = trim((string) ($schedule['monthly_time'] ?? ''));
    if ($monthlyTime === '') {
        $monthlyTime = '08:00';
    }

    $onceDatetime = trim((string) ($schedule['once_datetime'] ?? ''));
    $cronCustom = trim((string) ($schedule['cron_custom'] ?? ''));
    if ($cronCustom === '') {
        $cronCustom = '*/10 * * * *';
    }

    if ($mode === 'custom' && $cronCustom !== '' && preg_match('#^\*/(\d+)\s+\*\s+\*\s+\*\s+\*$#', $cronCustom, $m)) {
        $mode = 'repeat_minutes';
        $repeatMinutes = max(1, (int) $m[1]);
    }

    if ($cron === 'never' && ($schedule['schedule_mode'] ?? '') === '') {
        $mode = 'daily';
    }

    return [
        'schedule_mode' => $mode === 'never' ? 'daily' : $mode,
        'repeat_minutes' => $repeatMinutes,
        'hourly_minute' => $hourlyMinute,
        'daily_time' => $dailyTime,
        'weekly_day' => $weeklyDay,
        'weekly_time' => $weeklyTime,
        'monthly_day' => $monthlyDay,
        'monthly_time' => $monthlyTime,
        'once_datetime' => $onceDatetime,
        'cron_custom' => $cronCustom,
    ];
}

function jrs_schedule_parse_time_hm(string $time): ?array
{
    $time = trim($time);
    if (preg_match('/^(\d{1,2}):(\d{2})$/', $time, $m) !== 1) {
        return null;
    }
    $hour = (int) $m[1];
    $minute = (int) $m[2];
    if ($hour < 0 || $hour > 23 || $minute < 0 || $minute > 59) {
        return null;
    }
    return ['hour' => $hour, 'minute' => $minute];
}

function jrs_schedule_parse_datetime_local(string $value): ?DateTimeImmutable
{
    $value = trim($value);
    if ($value === '') {
        return null;
    }
    $value = str_replace('T', ' ', $value);
    $formats = ['Y-m-d H:i', 'Y-m-d H:i:s'];
    foreach ($formats as $fmt) {
        $dt = DateTimeImmutable::createFromFormat($fmt, $value);
        if ($dt instanceof DateTimeImmutable) {
            return $dt;
        }
    }
    return null;
}

/**
 * @return array{ok:bool, error?:string, mode?:string, fields?:array<string,mixed>}
 */
function jrs_schedule_validate_mode_fields(string $mode, array $post): array
{
    $mode = strtolower(trim($mode));
    if (!in_array($mode, jrs_schedule_modes(), true)) {
        return ['ok' => false, 'error' => 'Invalid schedule mode.'];
    }

    $fields = [
        'schedule_mode' => $mode,
        'repeat_minutes' => null,
        'hourly_minute' => null,
        'daily_time' => null,
        'weekly_day' => null,
        'weekly_time' => null,
        'monthly_day' => null,
        'monthly_time' => null,
        'once_datetime' => null,
        'cron_custom' => null,
    ];

    switch ($mode) {
        case 'repeat_minutes':
            $n = (int) ($post['repeat_minutes'] ?? 0);
            if ($n < 1 || $n > 1440) {
                return ['ok' => false, 'error' => 'Repeat interval must be between 1 and 1440 minutes.'];
            }
            $fields['repeat_minutes'] = $n;
            break;

        case 'hourly':
            $min = (int) ($post['hourly_minute'] ?? -1);
            if ($min < 0 || $min > 59) {
                return ['ok' => false, 'error' => 'Hourly minute must be between 0 and 59.'];
            }
            $fields['hourly_minute'] = $min;
            break;

        case 'daily':
            $t = trim((string) ($post['daily_time'] ?? ''));
            if (jrs_schedule_parse_time_hm($t) === null) {
                return ['ok' => false, 'error' => 'Daily time must be a valid HH:MM value.'];
            }
            $fields['daily_time'] = $t;
            break;

        case 'weekly':
            $day = trim((string) ($post['weekly_day'] ?? ''));
            if (!in_array($day, jrs_schedule_weekly_days(), true)) {
                return ['ok' => false, 'error' => 'Select a valid day of the week.'];
            }
            $t = trim((string) ($post['weekly_time'] ?? ''));
            if (jrs_schedule_parse_time_hm($t) === null) {
                return ['ok' => false, 'error' => 'Weekly time must be a valid HH:MM value.'];
            }
            $fields['weekly_day'] = $day;
            $fields['weekly_time'] = $t;
            break;

        case 'monthly':
            $dom = (int) ($post['monthly_day'] ?? 0);
            if ($dom < 1 || $dom > 31) {
                return ['ok' => false, 'error' => 'Monthly day must be between 1 and 31.'];
            }
            $t = trim((string) ($post['monthly_time'] ?? ''));
            if (jrs_schedule_parse_time_hm($t) === null) {
                return ['ok' => false, 'error' => 'Monthly time must be a valid HH:MM value.'];
            }
            $fields['monthly_day'] = $dom;
            $fields['monthly_time'] = $t;
            break;

        case 'once':
            $raw = trim((string) ($post['once_datetime'] ?? ''));
            $dt = jrs_schedule_parse_datetime_local($raw);
            if ($dt === null) {
                return ['ok' => false, 'error' => 'Run date and time is required for a one-time schedule.'];
            }
            $fields['once_datetime'] = $dt->format('Y-m-d\TH:i');
            break;

        case 'custom':
            $cronCustom = trim((string) ($post['cron_custom'] ?? ''));
            if ($cronCustom === '') {
                return ['ok' => false, 'error' => 'Cron expression is required for custom schedules.'];
            }
            $parts = preg_split('/\s+/', $cronCustom) ?: [];
            if (count($parts) !== 5) {
                return ['ok' => false, 'error' => 'Cron expression must have five fields (minute hour day month weekday).'];
            }
            $fields['cron_custom'] = $cronCustom;
            break;
    }

    return ['ok' => true, 'mode' => $mode, 'fields' => $fields];
}

/**
 * Map Repeat Scheduler UI fields to legacy cron_period / cron_custom for DB and systemd.
 *
 * @param array<string, mixed> $fields
 * @return array{cron_period: string, cron_custom: string}
 */
function jrs_schedule_derive_legacy_cron(string $mode, array $fields): array
{
    switch ($mode) {
        case 'repeat_minutes':
            $n = max(1, min(1440, (int) ($fields['repeat_minutes'] ?? 10)));
            return ['cron_period' => 'custom', 'cron_custom' => '*/' . $n . ' * * * *'];

        case 'hourly':
            $minute = (int) ($fields['hourly_minute'] ?? 0);
            if ($minute > 0 && $minute <= 59) {
                return ['cron_period' => 'custom', 'cron_custom' => $minute . ' * * * *'];
            }
            return ['cron_period' => 'hourly', 'cron_custom' => ''];

        case 'daily':
            return ['cron_period' => 'daily', 'cron_custom' => ''];

        case 'weekly':
            return ['cron_period' => 'weekly', 'cron_custom' => ''];

        case 'monthly':
            return ['cron_period' => 'monthly', 'cron_custom' => ''];

        case 'custom':
            return [
                'cron_period' => 'custom',
                'cron_custom' => trim((string) ($fields['cron_custom'] ?? '')),
            ];

        case 'once':
            return ['cron_period' => 'once', 'cron_custom' => ''];

        default:
            return ['cron_period' => 'never', 'cron_custom' => ''];
    }
}

/**
 * Hydrate schedule_mode and interval fields from legacy cron + DB row before systemd install.
 *
 * @param array<string, mixed> $schedule
 * @return array<string, mixed>
 */
function jrs_schedule_prepare_for_systemd(array $schedule): array
{
    $cron = strtolower(trim((string) ($schedule['cron_period'] ?? 'never')));
    $cronCustom = trim((string) ($schedule['cron_custom'] ?? ''));

    if ($cron === 'repeat_minutes') {
        $n = max(1, min(1440, (int) ($schedule['repeat_minutes'] ?? 10)));
        $schedule['cron_period'] = 'custom';
        $schedule['cron_custom'] = '*/' . $n . ' * * * *';
        $schedule['schedule_mode'] = 'repeat_minutes';
        $schedule['repeat_minutes'] = $n;
        $cron = 'custom';
        $cronCustom = $schedule['cron_custom'];
    }

    if (preg_match('#^\*/(\d+)\s+\*\s+\*\s+\*\s+\*$#', $cronCustom, $m)) {
        $schedule['schedule_mode'] = 'repeat_minutes';
        $schedule['repeat_minutes'] = (int) $m[1];
        $schedule['cron_period'] = 'custom';
        $schedule['cron_custom'] = $cronCustom;
    } elseif ($cron === 'custom' && $cronCustom !== '') {
        $schedule['schedule_mode'] = 'custom';
    } elseif ($cron === 'once') {
        $schedule['schedule_mode'] = 'once';
    } elseif (in_array($cron, jrs_schedule_modes(), true)) {
        $schedule['schedule_mode'] = $cron;
    } elseif (in_array($cron, ['hourly', 'daily', 'weekly', 'monthly'], true)) {
        $schedule['schedule_mode'] = $cron;
    }

    if (preg_match('#^(\d+)\s+\*\s+\*\s+\*\s+\*$#', $cronCustom, $hm) && $cron === 'custom') {
        $schedule['schedule_mode'] = 'hourly';
        $schedule['hourly_minute'] = (int) $hm[1];
    }

    $schedule['enabled'] = jrs_schedule_is_enabled($schedule);

    return $schedule;
}

/**
 * @param array<string, mixed> $schedule
 */
function jrs_schedule_summary(array $schedule): string
{
    $cron = strtolower(trim((string) ($schedule['cron_period'] ?? 'never')));
    if ($cron === 'never') {
        return 'Not scheduled';
    }
    if ($cron === 'now') {
        return 'Runs immediately on save';
    }

    $mode = jrs_schedule_effective_mode($schedule);
    switch ($mode) {
        case 'repeat_minutes':
            $n = (int) ($schedule['repeat_minutes'] ?? 10);
            return 'Runs every ' . $n . ' minute' . ($n === 1 ? '' : 's');

        case 'hourly':
            $m = (int) ($schedule['hourly_minute'] ?? 0);
            return 'Runs hourly at minute ' . sprintf('%02d', $m);

        case 'daily':
            $t = trim((string) ($schedule['daily_time'] ?? '08:00'));
            if ($t === '' && $cron === 'daily' && empty($schedule['schedule_mode'])) {
                $t = '00:00';
            }
            return 'Runs daily at ' . $t;

        case 'weekly':
            $day = trim((string) ($schedule['weekly_day'] ?? 'Monday'));
            $t = trim((string) ($schedule['weekly_time'] ?? '08:00'));
            if ($day === '' && empty($schedule['schedule_mode'])) {
                return 'Runs weekly (legacy midnight)';
            }
            return 'Runs ' . $day . 's at ' . $t;

        case 'monthly':
            $dom = (int) ($schedule['monthly_day'] ?? 1);
            if ($dom < 1 && empty($schedule['schedule_mode'])) {
                $dom = 1;
            }
            $t = trim((string) ($schedule['monthly_time'] ?? '08:00'));
            return 'Runs monthly on day ' . $dom . ' at ' . $t;

        case 'once':
            $raw = trim((string) ($schedule['once_datetime'] ?? ''));
            if ($raw !== '') {
                $dt = jrs_schedule_parse_datetime_local($raw);
                if ($dt !== null) {
                    return 'Runs once at ' . $dt->format('Y-m-d H:i');
                }
            }
            if ($cron === 'now') {
                return 'Runs immediately on save';
            }
            return 'Runs once';

        case 'custom':
            $expr = trim((string) ($schedule['cron_custom'] ?? ''));
            return $expr !== '' ? 'Custom: ' . $expr : 'Custom schedule';

        default:
            return ucfirst($cron);
    }
}

/**
 * Build systemd timer directives from a schedule row.
 *
 * @param array<string, mixed> $schedule
 * @return array{type:string, calendar?:string, interval?:string, error?:string}
 */
function jrs_schedule_timer_spec(array $schedule): array
{
    $mode = jrs_schedule_effective_mode($schedule);
    if ($mode === 'never') {
        return ['type' => 'none', 'error' => 'no schedule'];
    }

    switch ($mode) {
        case 'repeat_minutes':
            $n = (int) ($schedule['repeat_minutes'] ?? 0);
            if ($n < 1 || $n > 1440) {
                return ['type' => 'none', 'error' => 'invalid repeat_minutes'];
            }
            return ['type' => 'interval', 'interval' => $n . 'min'];

        case 'hourly':
            if (!array_key_exists('hourly_minute', $schedule) && ($schedule['schedule_mode'] ?? '') === '') {
                return ['type' => 'calendar', 'calendar' => '*-*-* *:00:00'];
            }
            $min = (int) ($schedule['hourly_minute'] ?? 0);
            if ($min < 0 || $min > 59) {
                return ['type' => 'none', 'error' => 'invalid hourly_minute'];
            }
            return ['type' => 'calendar', 'calendar' => sprintf('*-*-* *:%02d:00', $min)];

        case 'daily':
            $dailyTime = trim((string) ($schedule['daily_time'] ?? ''));
            if ($dailyTime === '' && ($schedule['schedule_mode'] ?? '') === '') {
                return ['type' => 'calendar', 'calendar' => '*-*-* 00:00:00'];
            }
            $parsed = jrs_schedule_parse_time_hm($dailyTime !== '' ? $dailyTime : '08:00');
            if ($parsed === null) {
                $parsed = ['hour' => 8, 'minute' => 0];
            }
            return [
                'type' => 'calendar',
                'calendar' => sprintf('*-*-* %02d:%02d:00', $parsed['hour'], $parsed['minute']),
            ];

        case 'weekly':
            if (trim((string) ($schedule['weekly_day'] ?? '')) === '' && ($schedule['schedule_mode'] ?? '') === '') {
                return ['type' => 'calendar', 'calendar' => 'Mon *-*-* 00:00:00'];
            }
            $day = jrs_schedule_weekly_day_to_systemd((string) ($schedule['weekly_day'] ?? 'Monday'));
            $parsed = jrs_schedule_parse_time_hm(trim((string) ($schedule['weekly_time'] ?? '08:00')));
            if ($parsed === null) {
                $parsed = ['hour' => 8, 'minute' => 0];
            }
            return [
                'type' => 'calendar',
                'calendar' => sprintf(
                    '%s *-*-* %02d:%02d:00',
                    $day,
                    $parsed['hour'],
                    $parsed['minute']
                ),
            ];

        case 'monthly':
            if (($schedule['schedule_mode'] ?? '') === '' && !array_key_exists('monthly_day', $schedule)) {
                return ['type' => 'calendar', 'calendar' => '*-*-01 00:00:00'];
            }
            $dom = (int) ($schedule['monthly_day'] ?? 1);
            if ($dom < 1 || $dom > 31) {
                return ['type' => 'none', 'error' => 'invalid monthly_day'];
            }
            $parsed = jrs_schedule_parse_time_hm(trim((string) ($schedule['monthly_time'] ?? '08:00')));
            if ($parsed === null) {
                $parsed = ['hour' => 8, 'minute' => 0];
            }
            return [
                'type' => 'calendar',
                'calendar' => sprintf('*-*-%02d %02d:%02d:00', $dom, $parsed['hour'], $parsed['minute']),
            ];

        case 'once':
            $raw = trim((string) ($schedule['once_datetime'] ?? ''));
            $dt = jrs_schedule_parse_datetime_local($raw);
            if ($dt === null) {
                return ['type' => 'none', 'error' => 'invalid once_datetime'];
            }
            return [
                'type' => 'calendar',
                'calendar' => $dt->format('Y-m-d H:i:s'),
            ];

        case 'custom':
            $cron = trim((string) ($schedule['cron_custom'] ?? ''));
            if ($cron === '') {
                return ['type' => 'none', 'error' => 'empty cron'];
            }
            $calendar = jrs_schedule_cron_to_systemd($cron);
            if ($calendar === '') {
                return ['type' => 'none', 'error' => 'invalid cron'];
            }
            return ['type' => 'calendar', 'calendar' => $calendar];

        default:
            return ['type' => 'none', 'error' => 'unknown mode'];
    }
}

function jrs_schedule_cron_to_systemd(string $cron): string
{
    $cron = trim($cron);
    if ($cron === '') {
        return '';
    }
    $parts = preg_split('/\s+/', $cron) ?: [];
    if (count($parts) !== 5) {
        return $cron;
    }
    [$min, $hour, $dom, $mon, $dow] = $parts;

    if (preg_match('#^\*/(\d+)$#', $min, $m) && $hour === '*' && $dom === '*' && $mon === '*' && $dow === '*') {
        return '*:0/' . (int) $m[1];
    }
    if (preg_match('#^(\d+)$#', $min, $mMin) && preg_match('#^(\d+)$#', $hour, $mHour)
        && $dom === '*' && $mon === '*' && $dow === '*') {
        return sprintf('*-*-* %02d:%02d:00', (int) $mHour[1], (int) $mMin[1]);
    }

    return sprintf('%s %s %s %s %s', $min, $hour, $dom, $mon, $dow);
}
