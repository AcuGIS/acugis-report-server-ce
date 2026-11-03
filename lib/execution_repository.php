<?php
declare(strict_types=1);

function jrs_execution_create($conn, int $scheduleId): int
{
    if (!$conn) {
        return 0;
    }
    $result = @pg_query_params(
        $conn,
        'INSERT INTO ' . jrs_table('jasper_schedule_execution') . ' (schedule_id, status) VALUES ($1, $2) RETURNING id',
        [$scheduleId > 0 ? $scheduleId : null, 'running']
    );
    if ($result === false) {
        return 0;
    }
    $row = pg_fetch_assoc($result);
    pg_free_result($result);
    return (int) ($row['id'] ?? 0);
}

function jrs_execution_finish(
    $conn,
    int $executionId,
    string $status,
    string $outputFile,
    string $emailStatus,
    string $errorMessage
): void {
    if (!$conn || $executionId <= 0) {
        return;
    }
    @pg_query_params(
        $conn,
        'UPDATE ' . jrs_table('jasper_schedule_execution') . ' SET
            status = $2,
            finished_at = now(),
            output_file = $3,
            email_status = $4,
            error_message = $5
        WHERE id = $1',
        [$executionId, $status, $outputFile, $emailStatus, $errorMessage]
    );
}

/**
 * @return list<array<string, mixed>>
 */
function jrs_execution_list($conn, ?int $scheduleId = null, int $limit = 200): array
{
    if (!$conn || !jrs_has_table($conn, 'jasper_schedule_execution')) {
        return [];
    }
    if ($scheduleId !== null && $scheduleId > 0) {
        $sql = 'SELECT e.*, s.report_name, s.filename AS schedule_filename
            FROM ' . jrs_table('jasper_schedule_execution') . ' e
            LEFT JOIN ' . jrs_table('jasper_schedule') . ' s ON s.id = e.schedule_id
            WHERE e.schedule_id = $1
            ORDER BY e.started_at DESC
            LIMIT $2';
        $result = @pg_query_params($conn, $sql, [$scheduleId, $limit]);
    } else {
        $sql = 'SELECT e.*, s.report_name, s.filename AS schedule_filename
            FROM ' . jrs_table('jasper_schedule_execution') . ' e
            LEFT JOIN ' . jrs_table('jasper_schedule') . ' s ON s.id = e.schedule_id
            ORDER BY e.started_at DESC
            LIMIT $1';
        $result = @pg_query_params($conn, $sql, [$limit]);
    }
    if ($result === false) {
        return [];
    }
    $rows = [];
    while ($row = pg_fetch_assoc($result)) {
        $rows[] = $row;
    }
    pg_free_result($result);
    return $rows;
}

function jrs_execution_latest_for_schedule($conn, int $scheduleId): ?array
{
    $list = jrs_execution_list($conn, $scheduleId, 1);
    return $list[0] ?? null;
}
