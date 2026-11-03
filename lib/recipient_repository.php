<?php
declare(strict_types=1);

/**
 * @return list<array<string, mixed>>
 */
function jrs_recipient_list($conn): array
{
    if (!$conn || !jrs_has_table($conn, 'jasper_recipient')) {
        return [];
    }
    $result = @pg_query(
        $conn,
        'SELECT id, name, email, notes FROM ' . jrs_table('jasper_recipient') . ' ORDER BY name, email'
    );
    if ($result === false) {
        return [];
    }
    $rows = [];
    while ($row = pg_fetch_assoc($result)) {
        $rows[] = [
            'id' => (int) ($row['id'] ?? 0),
            'name' => (string) ($row['name'] ?? ''),
            'email' => (string) ($row['email'] ?? ''),
            'notes' => (string) ($row['notes'] ?? ''),
        ];
    }
    pg_free_result($result);
    return $rows;
}

function jrs_recipient_get($conn, int $id): ?array
{
    if (!$conn || $id <= 0) {
        return null;
    }
    $result = @pg_query_params(
        $conn,
        'SELECT id, name, email, notes FROM ' . jrs_table('jasper_recipient') . ' WHERE id = $1',
        [$id]
    );
    if ($result === false) {
        return null;
    }
    $row = pg_fetch_assoc($result);
    pg_free_result($result);
    if ($row === false) {
        return null;
    }
    return [
        'id' => (int) ($row['id'] ?? 0),
        'name' => (string) ($row['name'] ?? ''),
        'email' => (string) ($row['email'] ?? ''),
        'notes' => (string) ($row['notes'] ?? ''),
    ];
}

function jrs_recipient_save($conn, ?int $id, string $name, string $email, string $notes): bool
{
    if (!$conn || trim($email) === '') {
        return false;
    }
    if ($id !== null && $id > 0) {
        $result = @pg_query_params(
            $conn,
            'UPDATE ' . jrs_table('jasper_recipient') . ' SET name = $2, email = $3, notes = $4, updated_at = now() WHERE id = $1',
            [$id, $name, $email, $notes]
        );
    } else {
        $result = @pg_query_params(
            $conn,
            'INSERT INTO ' . jrs_table('jasper_recipient') . ' (name, email, notes) VALUES ($1,$2,$3)',
            [$name, $email, $notes]
        );
    }
    if ($result === false) {
        return false;
    }
    pg_free_result($result);
    return true;
}

function jrs_recipient_delete($conn, int $id): bool
{
    if (!$conn || $id <= 0) {
        return false;
    }
    $result = @pg_query_params($conn, 'DELETE FROM ' . jrs_table('jasper_recipient') . ' WHERE id = $1', [$id]);
    if ($result === false) {
        return false;
    }
    $ok = pg_affected_rows($result) > 0;
    pg_free_result($result);
    return $ok;
}
