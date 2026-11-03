<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$conn = jrs_auth_require_central_db();
jrs_require_login($conn);
jrs_require_platform_admin($conn);

$message = '';
$error = '';
$editId = (int) ($_GET['id'] ?? 0);

function jrs_admin_all_groups($conn): array
{
    $res = @pg_query($conn, 'SELECT id, name FROM auth_groups ORDER BY name ASC');
    if ($res === false) {
        return [];
    }
    $out = [];
    while ($row = pg_fetch_assoc($res)) {
        $out[] = [
            'id' => (int) ($row['id'] ?? 0),
            'name' => (string) ($row['name'] ?? ''),
        ];
    }
    pg_free_result($res);
    return $out;
}

function jrs_admin_user_group_ids($conn, int $userId): array
{
    if ($userId <= 0) {
        return [];
    }
    $res = @pg_query_params(
        $conn,
        'SELECT group_id FROM auth_group_members WHERE user_id = $1',
        [$userId]
    );
    if ($res === false) {
        return [];
    }
    $ids = [];
    while ($row = pg_fetch_assoc($res)) {
        $ids[] = (int) ($row['group_id'] ?? 0);
    }
    pg_free_result($res);
    return $ids;
}

function jrs_admin_set_user_groups($conn, int $userId, array $groupIds): void
{
    @pg_query_params($conn, 'DELETE FROM auth_group_members WHERE user_id = $1', [$userId]);
    foreach ($groupIds as $gid) {
        $gid = (int) $gid;
        if ($gid <= 0) {
            continue;
        }
        @pg_query_params(
            $conn,
            'INSERT INTO auth_group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING',
            [$gid, $userId]
        );
    }
}

if ($_SERVER['REQUEST_METHOD'] === 'POST') {
    $action = (string) ($_POST['action'] ?? 'save');
    $id = (int) ($_POST['id'] ?? 0);
    $username = trim((string) ($_POST['username'] ?? ''));
    $email = trim((string) ($_POST['email'] ?? ''));
    $password = (string) ($_POST['password'] ?? '');
    $isAdmin = isset($_POST['is_admin']);
    $enabled = isset($_POST['enabled']);
    $groupIds = array_map('intval', (array) ($_POST['groups'] ?? []));

    if ($action === 'delete' && $id > 0) {
        $current = jrs_get_current_user();
        if ($id === (int) ($current['id'] ?? 0)) {
            $error = 'You cannot delete your own account.';
        } else {
            @pg_query_params($conn, 'DELETE FROM dashboard_sessions WHERE username = (SELECT username FROM dashboard_users WHERE id = $1)', [$id]);
            @pg_query_params($conn, 'DELETE FROM auth_group_members WHERE user_id = $1', [$id]);
            $ok = @pg_query_params($conn, 'DELETE FROM dashboard_users WHERE id = $1', [$id]);
            $message = $ok ? 'User deleted.' : 'Failed to delete user.';
            $editId = 0;
        }
    } elseif ($username === '') {
        $error = 'Username is required.';
        $editId = $id;
    } elseif ($id <= 0 && $password === '') {
        $error = 'Password is required for new users.';
    } else {
        if ($id > 0) {
            if ($password !== '') {
                $hash = password_hash($password, PASSWORD_DEFAULT);
                $ok = @pg_query_params(
                    $conn,
                    'UPDATE dashboard_users SET username = $1, email = $2, password_hash = $3, is_admin = $4, enabled = $5 WHERE id = $6',
                    [$username, $email, $hash, jrs_pg_bool($isAdmin), jrs_pg_bool($enabled), $id]
                );
            } else {
                $ok = @pg_query_params(
                    $conn,
                    'UPDATE dashboard_users SET username = $1, email = $2, is_admin = $3, enabled = $4 WHERE id = $5',
                    [$username, $email, jrs_pg_bool($isAdmin), jrs_pg_bool($enabled), $id]
                );
            }
            if ($ok) {
                jrs_admin_set_user_groups($conn, $id, $groupIds);
                $message = 'User updated.';
                $editId = 0;
            } else {
                $error = 'Failed to update user (username may already exist).';
                $editId = $id;
            }
        } else {
            $hash = password_hash($password, PASSWORD_DEFAULT);
            $res = @pg_query_params(
                $conn,
                'INSERT INTO dashboard_users (username, email, password_hash, is_admin, enabled) VALUES ($1, $2, $3, $4, $5) RETURNING id',
                [$username, $email, $hash, jrs_pg_bool($isAdmin), jrs_pg_bool($enabled)]
            );
            if ($res) {
                $row = pg_fetch_assoc($res);
                pg_free_result($res);
                $newId = (int) ($row['id'] ?? 0);
                if ($newId > 0) {
                    jrs_admin_set_user_groups($conn, $newId, $groupIds);
                }
                $message = 'User created.';
                $editId = 0;
            } else {
                $error = 'Failed to create user (username may already exist).';
            }
        }
    }
}

$groups = jrs_admin_all_groups($conn);
$users = [];
$res = @pg_query(
    $conn,
    <<<'SQL'
SELECT u.id, u.username, u.email, u.is_admin, u.enabled, u.created_at,
       COALESCE(string_agg(g.name, ', ' ORDER BY g.name), '') AS group_names
FROM dashboard_users u
LEFT JOIN auth_group_members gm ON gm.user_id = u.id
LEFT JOIN auth_groups g ON g.id = gm.group_id
GROUP BY u.id, u.username, u.email, u.is_admin, u.enabled, u.created_at
ORDER BY u.username ASC
SQL
);
if ($res) {
    while ($row = pg_fetch_assoc($res)) {
        $users[] = $row;
    }
    pg_free_result($res);
}

$editUser = null;
$editGroupIds = [];
if ($editId > 0) {
    foreach ($users as $u) {
        if ((int) ($u['id'] ?? 0) === $editId) {
            $editUser = $u;
            break;
        }
    }
    $editGroupIds = jrs_admin_user_group_ids($conn, $editId);
}

jrs_material_page_header('Users');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">Users</h1>
        <p class="page-subtitle">Local accounts for AcuGIS Report Server.</p>
    </div>
    <div class="header-actions">
        <a href="users.php" class="btn waves-effect waves-light teal"><i class="material-icons left">add</i>New user</a>
        <a href="groups.php" class="btn-flat waves-effect">Groups</a>
        <a href="<?php echo jrs_html(jrs_app_mount() . '/logout.php'); ?>" class="btn-flat waves-effect">Sign out</a>
    </div>
</header>

<?php if ($message !== ''): ?>
    <div class="card-panel teal lighten-5"><?php echo jrs_html($message); ?></div>
<?php endif; ?>
<?php if ($error !== ''): ?>
    <div class="card-panel red lighten-5"><?php echo jrs_html($error); ?></div>
<?php endif; ?>

<div class="card"><div class="card-content">
    <h2 class="card-title" style="font-size:1.2rem;"><?php echo $editUser ? 'Edit user' : 'Create user'; ?></h2>
    <form method="post" action="users.php<?php echo $editId > 0 ? '?id=' . $editId : ''; ?>">
        <input type="hidden" name="action" value="save">
        <input type="hidden" name="id" value="<?php echo (int) ($editUser['id'] ?? 0); ?>">
        <div class="row">
            <div class="input-field col s12 m4">
                <input id="username" name="username" type="text" required value="<?php echo jrs_html((string) ($editUser['username'] ?? '')); ?>">
                <label for="username" class="<?php echo $editUser ? 'active' : ''; ?>">Username</label>
            </div>
            <div class="input-field col s12 m4">
                <input id="email" name="email" type="email" value="<?php echo jrs_html((string) ($editUser['email'] ?? '')); ?>">
                <label for="email" class="<?php echo $editUser ? 'active' : ''; ?>">Email</label>
            </div>
            <div class="input-field col s12 m4">
                <input id="password" name="password" type="password" autocomplete="new-password" <?php echo $editUser ? '' : 'required'; ?>>
                <label for="password"><?php echo $editUser ? 'New password (optional)' : 'Password'; ?></label>
            </div>
        </div>
        <p>
            <label><input type="checkbox" name="is_admin" <?php echo jrs_pg_is_true($editUser['is_admin'] ?? false) ? 'checked' : ''; ?>><span>Administrator flag</span></label>
            &nbsp;&nbsp;
            <label><input type="checkbox" name="enabled" <?php echo !$editUser || jrs_pg_is_true($editUser['enabled'] ?? true) ? 'checked' : ''; ?>><span>Enabled</span></label>
        </p>
        <p style="margin-top:16px;"><strong>Groups</strong></p>
        <div class="row">
            <?php foreach ($groups as $g): ?>
                <div class="col s12 m4">
                    <label>
                        <input type="checkbox" name="groups[]" value="<?php echo (int) $g['id']; ?>"
                            <?php echo in_array((int) $g['id'], $editGroupIds, true) ? 'checked' : ''; ?>>
                        <span><?php echo jrs_html($g['name']); ?></span>
                    </label>
                </div>
            <?php endforeach; ?>
        </div>
        <button type="submit" class="btn waves-effect waves-light teal"><?php echo $editUser ? 'Save changes' : 'Create user'; ?></button>
        <?php if ($editUser): ?>
            <a href="users.php" class="btn-flat">Cancel</a>
        <?php endif; ?>
    </form>
</div></div>

<div class="card"><div class="card-content connections-table-wrap">
<?php if ($users === []): ?>
    <div class="empty-state"><p>No users yet. Create one above or run <code>scripts/seed_admin.php</code>.</p></div>
<?php else: ?>
<table class="striped highlight connections-table">
<thead><tr><th>Username</th><th>Email</th><th>Groups</th><th>Flags</th><th></th></tr></thead>
<tbody>
<?php foreach ($users as $row): ?>
<tr>
    <td><?php echo jrs_html((string) $row['username']); ?></td>
    <td><?php echo jrs_html((string) $row['email']); ?></td>
    <td><?php echo jrs_html((string) $row['group_names']); ?></td>
    <td>
        <?php if (jrs_pg_is_true($row['is_admin'] ?? false)): ?>admin <?php endif; ?>
        <?php echo jrs_pg_is_true($row['enabled'] ?? true) ? 'enabled' : 'disabled'; ?>
    </td>
    <td>
        <a class="btn-small" href="users.php?id=<?php echo (int) $row['id']; ?>"><i class="material-icons">edit</i></a>
        <form method="post" action="users.php" style="display:inline;" onsubmit="return confirm('Delete this user?');">
            <input type="hidden" name="action" value="delete">
            <input type="hidden" name="id" value="<?php echo (int) $row['id']; ?>">
            <button type="submit" class="btn-small red"><i class="material-icons">delete</i></button>
        </form>
    </td>
</tr>
<?php endforeach; ?>
</tbody>
</table>
<?php endif; ?>
</div></div>
<?php jrs_material_page_footer(); ?>
