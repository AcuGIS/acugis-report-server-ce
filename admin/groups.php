<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$conn = jrs_auth_require_central_db();
jrs_require_login($conn);
jrs_require_platform_admin($conn);

$message = '';
$error = '';
$editId = (int) ($_GET['id'] ?? 0);

if ($_SERVER['REQUEST_METHOD'] === 'POST') {
    $action = (string) ($_POST['action'] ?? 'save');
    $id = (int) ($_POST['id'] ?? 0);
    $name = trim((string) ($_POST['name'] ?? ''));
    $description = trim((string) ($_POST['description'] ?? ''));
    $memberIds = array_map('intval', (array) ($_POST['members'] ?? []));

    if ($action === 'delete' && $id > 0) {
        $nameRes = @pg_query_params($conn, 'SELECT name FROM auth_groups WHERE id = $1', [$id]);
        $gName = '';
        if ($nameRes) {
            $nr = pg_fetch_assoc($nameRes);
            pg_free_result($nameRes);
            $gName = (string) ($nr['name'] ?? '');
        }
        if (strcasecmp($gName, 'Administrators') === 0 || strcasecmp($gName, 'Public') === 0) {
            $error = 'Built-in groups Administrators and Public cannot be deleted.';
        } else {
            @pg_query_params($conn, 'DELETE FROM resource_permissions WHERE group_id = $1', [$id]);
            @pg_query_params($conn, 'DELETE FROM auth_group_members WHERE group_id = $1', [$id]);
            $ok = @pg_query_params($conn, 'DELETE FROM auth_groups WHERE id = $1', [$id]);
            $message = $ok ? 'Group deleted.' : 'Failed to delete group.';
            $editId = 0;
        }
    } elseif ($name === '') {
        $error = 'Group name is required.';
        $editId = $id;
    } else {
        if ($id > 0) {
            $ok = @pg_query_params(
                $conn,
                'UPDATE auth_groups SET name = $1, description = $2 WHERE id = $3',
                [$name, $description, $id]
            );
            if ($ok) {
                @pg_query_params($conn, 'DELETE FROM auth_group_members WHERE group_id = $1', [$id]);
                foreach ($memberIds as $uid) {
                    if ($uid <= 0) {
                        continue;
                    }
                    @pg_query_params(
                        $conn,
                        'INSERT INTO auth_group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING',
                        [$id, $uid]
                    );
                }
                $message = 'Group updated.';
                $editId = 0;
            } else {
                $error = 'Failed to update group (name may already exist).';
                $editId = $id;
            }
        } else {
            $res = @pg_query_params(
                $conn,
                'INSERT INTO auth_groups (name, description) VALUES ($1, $2) RETURNING id',
                [$name, $description]
            );
            if ($res) {
                $row = pg_fetch_assoc($res);
                pg_free_result($res);
                $newId = (int) ($row['id'] ?? 0);
                foreach ($memberIds as $uid) {
                    if ($uid <= 0 || $newId <= 0) {
                        continue;
                    }
                    @pg_query_params(
                        $conn,
                        'INSERT INTO auth_group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING',
                        [$newId, $uid]
                    );
                }
                $message = 'Group created.';
                $editId = 0;
            } else {
                $error = 'Failed to create group (name may already exist).';
            }
        }
    }
}

$allUsers = [];
$ures = @pg_query($conn, 'SELECT id, username FROM dashboard_users ORDER BY username ASC');
if ($ures) {
    while ($row = pg_fetch_assoc($ures)) {
        $allUsers[] = $row;
    }
    pg_free_result($ures);
}

$groups = [];
$gres = @pg_query(
    $conn,
    <<<'SQL'
SELECT g.id, g.name, COALESCE(g.description, '') AS description,
       COALESCE(string_agg(u.username, ', ' ORDER BY u.username), '') AS members
FROM auth_groups g
LEFT JOIN auth_group_members gm ON gm.group_id = g.id
LEFT JOIN dashboard_users u ON u.id = gm.user_id
GROUP BY g.id, g.name, g.description
ORDER BY g.name ASC
SQL
);
if ($gres) {
    while ($row = pg_fetch_assoc($gres)) {
        $groups[] = $row;
    }
    pg_free_result($gres);
}

$editGroup = null;
$editMemberIds = [];
if ($editId > 0) {
    foreach ($groups as $g) {
        if ((int) ($g['id'] ?? 0) === $editId) {
            $editGroup = $g;
            break;
        }
    }
    $mres = @pg_query_params($conn, 'SELECT user_id FROM auth_group_members WHERE group_id = $1', [$editId]);
    if ($mres) {
        while ($row = pg_fetch_assoc($mres)) {
            $editMemberIds[] = (int) ($row['user_id'] ?? 0);
        }
        pg_free_result($mres);
    }
}

jrs_material_page_header('Groups');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">Groups</h1>
        <p class="page-subtitle">Groups control report and service permissions.</p>
    </div>
    <div class="header-actions">
        <a href="groups.php" class="btn waves-effect waves-light teal"><i class="material-icons left">add</i>New group</a>
        <a href="users.php" class="btn-flat waves-effect">Users</a>
    </div>
</header>

<?php if ($message !== ''): ?>
    <div class="card-panel teal lighten-5"><?php echo jrs_html($message); ?></div>
<?php endif; ?>
<?php if ($error !== ''): ?>
    <div class="card-panel red lighten-5"><?php echo jrs_html($error); ?></div>
<?php endif; ?>

<div class="card"><div class="card-content">
    <h2 class="card-title" style="font-size:1.2rem;"><?php echo $editGroup ? 'Edit group' : 'Create group'; ?></h2>
    <form method="post" action="groups.php<?php echo $editId > 0 ? '?id=' . $editId : ''; ?>">
        <input type="hidden" name="action" value="save">
        <input type="hidden" name="id" value="<?php echo (int) ($editGroup['id'] ?? 0); ?>">
        <div class="row">
            <div class="input-field col s12 m6">
                <input id="name" name="name" type="text" required value="<?php echo jrs_html((string) ($editGroup['name'] ?? '')); ?>">
                <label for="name" class="<?php echo $editGroup ? 'active' : ''; ?>">Name</label>
            </div>
            <div class="input-field col s12 m6">
                <input id="description" name="description" type="text" value="<?php echo jrs_html((string) ($editGroup['description'] ?? '')); ?>">
                <label for="description" class="<?php echo $editGroup ? 'active' : ''; ?>">Description</label>
            </div>
        </div>
        <p><strong>Members</strong></p>
        <div class="row">
            <?php foreach ($allUsers as $u): ?>
                <div class="col s12 m4">
                    <label>
                        <input type="checkbox" name="members[]" value="<?php echo (int) $u['id']; ?>"
                            <?php echo in_array((int) $u['id'], $editMemberIds, true) ? 'checked' : ''; ?>>
                        <span><?php echo jrs_html((string) $u['username']); ?></span>
                    </label>
                </div>
            <?php endforeach; ?>
            <?php if ($allUsers === []): ?>
                <p class="grey-text">No users yet. Create users first.</p>
            <?php endif; ?>
        </div>
        <button type="submit" class="btn waves-effect waves-light teal"><?php echo $editGroup ? 'Save changes' : 'Create group'; ?></button>
        <?php if ($editGroup): ?>
            <a href="groups.php" class="btn-flat">Cancel</a>
        <?php endif; ?>
    </form>
</div></div>

<div class="card"><div class="card-content connections-table-wrap">
<?php if ($groups === []): ?>
    <div class="empty-state"><p>No groups found.</p></div>
<?php else: ?>
<table class="striped highlight connections-table">
<thead><tr><th>Name</th><th>Description</th><th>Members</th><th></th></tr></thead>
<tbody>
<?php foreach ($groups as $row): ?>
<tr>
    <td><?php echo jrs_html((string) $row['name']); ?></td>
    <td><?php echo jrs_html((string) $row['description']); ?></td>
    <td><?php echo jrs_html((string) $row['members']); ?></td>
    <td>
        <a class="btn-small" href="groups.php?id=<?php echo (int) $row['id']; ?>"><i class="material-icons">edit</i></a>
        <?php
        $n = (string) ($row['name'] ?? '');
        $protected = strcasecmp($n, 'Administrators') === 0 || strcasecmp($n, 'Public') === 0;
        if (!$protected):
        ?>
        <form method="post" action="groups.php" style="display:inline;" onsubmit="return confirm('Delete this group?');">
            <input type="hidden" name="action" value="delete">
            <input type="hidden" name="id" value="<?php echo (int) $row['id']; ?>">
            <button type="submit" class="btn-small red"><i class="material-icons">delete</i></button>
        </form>
        <?php endif; ?>
    </td>
</tr>
<?php endforeach; ?>
</tbody>
</table>
<?php endif; ?>
</div></div>
<?php jrs_material_page_footer(); ?>
