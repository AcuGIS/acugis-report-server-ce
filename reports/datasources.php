<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'admin');

$conn = jrs_app_db();
$errors = [];
$message = '';

if (!$conn) {
    $errors[] = 'Unable to connect to the application PostgreSQL database. Check config.php or JRS_DB_* environment variables.';
} elseif (!jrs_has_table($conn, 'jasper_datasource')) {
    $errors[] = 'Missing public.jasper_datasource. Load database/schema.sql into the application database.';
} else {
    jrs_ensure_datasource_db_type_column($conn);
}

$dbTypes = [
    'postgresql' => jrs_db_type_meta('postgresql'),
    'mysql' => jrs_db_type_meta('mysql'),
    'oracle' => jrs_db_type_meta('oracle'),
    'sqlserver' => jrs_db_type_meta('sqlserver'),
    'snowflake' => jrs_db_type_meta('snowflake'),
    'sqlite' => jrs_db_type_meta('sqlite'),
];

if ($_SERVER['REQUEST_METHOD'] === 'POST' && $errors === []) {
    $action = (string) ($_POST['action'] ?? '');
    if ($action === 'delete') {
        $id = (int) ($_POST['id'] ?? 0);
        if ($id > 0) {
            @pg_query_params($conn, 'DELETE FROM ' . jrs_table('jasper_datasource') . ' WHERE id = $1', [$id]);
            $message = 'Datasource deleted.';
        }
    } else {
        $name = trim((string) ($_POST['name'] ?? ''));
        $dbType = jrs_normalize_db_type($_POST['db_type'] ?? 'postgresql');
        $meta = jrs_db_type_meta($dbType);
        $host = trim((string) ($_POST['host'] ?? ''));
        $port = (int) ($_POST['port'] ?? $meta['port']);
        $dbname = trim((string) ($_POST['dbname'] ?? ''));
        $username = trim((string) ($_POST['username'] ?? ''));
        $password = (string) ($_POST['password'] ?? '');
        $schemaName = trim((string) ($_POST['schema_name'] ?? $meta['schema']));
        if ($schemaName === '' && $dbType === 'postgresql') {
            $schemaName = 'public';
        }
        if ($schemaName === '' && $dbType === 'snowflake') {
            $schemaName = 'PUBLIC';
        }

        if ($name === '' || $dbname === '') {
            $errors[] = 'Name and database/service (or file path) are required.';
        } elseif ($dbType !== 'sqlite' && ($host === '' || $username === '')) {
            $errors[] = 'Name, host, database/service, and username are required.';
        } elseif ($dbType !== 'sqlite' && $port <= 0) {
            $errors[] = 'Port must be a positive integer.';
        } else {
            $sql = 'INSERT INTO ' . jrs_table('jasper_datasource') . '
                (name, db_type, host, port, dbname, username, password, schema_name, enabled)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true)
                ON CONFLICT (name) DO UPDATE SET
                    db_type = EXCLUDED.db_type,
                    host = EXCLUDED.host,
                    port = EXCLUDED.port,
                    dbname = EXCLUDED.dbname,
                    username = EXCLUDED.username,
                    password = CASE
                        WHEN EXCLUDED.password = \'\' THEN ' . jrs_table('jasper_datasource') . '.password
                        ELSE EXCLUDED.password
                    END,
                    schema_name = EXCLUDED.schema_name,
                    enabled = true';
            $result = @pg_query_params($conn, $sql, [
                $name,
                $dbType,
                $host,
                $port,
                $dbname,
                $username,
                $password,
                $schemaName,
            ]);
            if ($result === false) {
                $errors[] = 'Failed to save datasource: ' . pg_last_error($conn);
            } else {
                pg_free_result($result);
                $message = 'Datasource saved. Regenerate Jasper context before running reports against it.';
            }
        }
    }
}

$datasources = $errors === [] && $conn ? jrs_datasources($conn) : [];

jrs_material_page_header('Datasources');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">Datasources</h1>
        <p class="page-subtitle">Manage JDBC connections used by Jasper and SQL-backed parameters.</p>
    </div>
    <div class="header-actions">
        <a class="btn waves-effect waves-light blue-grey" href="generate_jasper_context.php">
            <i class="material-icons left">sync</i>Generate Context XML
        </a>
    </div>
</header>

<?php foreach ($errors as $error): ?>
    <div class="card-panel red lighten-4 red-text text-darken-4"><?php echo jrs_html($error); ?></div>
<?php endforeach; ?>
<?php if ($message !== ''): ?>
    <div class="card-panel green lighten-4 green-text text-darken-4"><?php echo jrs_html($message); ?></div>
<?php endif; ?>

<?php if ($errors === []): ?>
    <div class="row">
        <div class="col s12 l5">
            <div class="card">
                <div class="card-content">
                    <span class="card-title" id="datasourceFormTitle">Add Datasource</span>
                    <form method="post" id="datasourceForm">
                        <div class="input-field">
                            <input type="text" id="name" name="name" required placeholder="MainPG">
                            <label for="name" class="active">Datasource Name</label>
                        </div>
                        <div class="input-field">
                            <select id="db_type" name="db_type" required>
                                <?php foreach ($dbTypes as $typeKey => $typeMeta): ?>
                                    <option value="<?php echo jrs_html($typeKey); ?>"
                                        data-port="<?php echo (int) $typeMeta['port']; ?>"
                                        data-schema="<?php echo jrs_html($typeMeta['schema']); ?>"
                                        data-dbname-label="<?php echo jrs_html($typeMeta['dbname_label']); ?>"
                                        <?php echo $typeKey === 'postgresql' ? 'selected' : ''; ?>>
                                        <?php echo jrs_html($typeMeta['label']); ?>
                                    </option>
                                <?php endforeach; ?>
                            </select>
                            <label>Database Type</label>
                        </div>
                        <div class="row" id="host_port_row">
                            <div class="input-field col s12 m8">
                                <input type="text" id="host" name="host" required placeholder="host.docker.internal">
                                <label for="host" id="host_label" class="active">Host</label>
                            </div>
                            <div class="input-field col s12 m4">
                                <input type="number" id="port" name="port" value="5432" required>
                                <label for="port" class="active">Port</label>
                            </div>
                        </div>
                        <div class="input-field">
                            <input type="text" id="dbname" name="dbname" required>
                            <label for="dbname" id="dbname_label" class="active">Database</label>
                            <span class="helper-text" id="dbname_help">For Oracle, enter the service name (or sid:ORCL for SID).</span>
                        </div>
                        <div class="input-field" id="schema_field">
                            <input type="text" id="schema_name" name="schema_name" value="public">
                            <label for="schema_name" class="active">Schema</label>
                        </div>
                        <div class="input-field" id="username_field">
                            <input type="text" id="username" name="username" required>
                            <label for="username">Username</label>
                        </div>
                        <div class="input-field">
                            <input type="password" id="password" name="password" autocomplete="new-password">
                            <label for="password">Password</label>
                            <span class="helper-text" id="password_help">Used for password authentication.</span>
                        </div>
                        <button type="submit" class="btn waves-effect waves-light teal" id="datasourceSaveBtn">
                            Save Datasource
                            <i class="material-icons right">save</i>
                        </button>
                        <button type="button" class="btn waves-effect waves-light blue-grey" id="datasourceClearBtn" style="display:none;margin-left:8px;">
                            Cancel
                            <i class="material-icons right">clear</i>
                        </button>
                    </form>
                </div>
            </div>
        </div>

        <div class="col s12 l7">
            <div class="card connections-card">
                <div class="card-content" style="padding:0;">
                    <?php if ($datasources === []): ?>
                        <div class="empty-state">
                            <i class="material-icons">storage</i>
                            <p>No datasources configured.</p>
                        </div>
                    <?php else: ?>
                        <div class="connections-table-wrap">
                            <table class="striped highlight connections-table">
                                <thead>
                                <tr>
                                    <th>Name</th>
                                    <th>Type</th>
                                    <th>Host</th>
                                    <th>Database</th>
                                    <th>User</th>
                                    <th class="right-align">Actions</th>
                                </tr>
                                </thead>
                                <tbody>
                                <?php foreach ($datasources as $row): ?>
                                    <?php
                                    $rowType = jrs_normalize_db_type($row['db_type'] ?? 'postgresql');
                                    $rowLabel = jrs_db_type_meta($rowType)['label'];
                                    $hostPort = $rowType === 'sqlite'
                                        ? '—'
                                        : ((string) $row['host'] . ':' . (string) $row['port']);
                                    $hasPassword = trim((string) ($row['password'] ?? '')) !== '';
                                    ?>
                                    <tr>
                                        <td><code><?php echo jrs_html((string) $row['name']); ?></code></td>
                                        <td><?php echo jrs_html($rowLabel); ?></td>
                                        <td><?php echo jrs_html($hostPort); ?></td>
                                        <td><?php echo jrs_html((string) $row['dbname']); ?></td>
                                        <td><?php echo jrs_html((string) $row['username']); ?></td>
                                        <td class="right-align actions-cell">
                                            <?php if (isset($row['id'])): ?>
                                                <button type="button"
                                                    class="btn-small waves-effect waves-light blue edit-ds-btn"
                                                    data-name="<?php echo jrs_html((string) $row['name']); ?>"
                                                    data-db-type="<?php echo jrs_html($rowType); ?>"
                                                    data-host="<?php echo jrs_html((string) ($row['host'] ?? '')); ?>"
                                                    data-port="<?php echo (int) ($row['port'] ?? 0); ?>"
                                                    data-dbname="<?php echo jrs_html((string) ($row['dbname'] ?? '')); ?>"
                                                    data-schema="<?php echo jrs_html((string) ($row['schema_name'] ?? '')); ?>"
                                                    data-username="<?php echo jrs_html((string) ($row['username'] ?? '')); ?>"
                                                    data-has-password="<?php echo $hasPassword ? '1' : '0'; ?>">
                                                    <i class="material-icons left">edit</i>Edit
                                                </button>
                                                <form method="post" class="inline-form" onsubmit="return confirm('Delete this datasource?');">
                                                    <input type="hidden" name="action" value="delete">
                                                    <input type="hidden" name="id" value="<?php echo (int) $row['id']; ?>">
                                                    <button type="submit" class="btn-small waves-effect waves-light red">
                                                        <i class="material-icons left">delete</i>Delete
                                                    </button>
                                                </form>
                                            <?php endif; ?>
                                        </td>
                                    </tr>
                                <?php endforeach; ?>
                                </tbody>
                            </table>
                        </div>
                    <?php endif; ?>
                </div>
            </div>
        </div>
    </div>
    <script>
    (function () {
        var typeSelect = document.getElementById('db_type');
        var portInput = document.getElementById('port');
        var hostInput = document.getElementById('host');
        var hostLabel = document.getElementById('host_label');
        var hostPortRow = document.getElementById('host_port_row');
        var usernameInput = document.getElementById('username');
        var usernameField = document.getElementById('username_field');
        var schemaField = document.getElementById('schema_field');
        var schemaInput = document.getElementById('schema_name');
        var dbnameLabel = document.getElementById('dbname_label');
        var dbnameHelp = document.getElementById('dbname_help');
        var formTitle = document.getElementById('datasourceFormTitle');
        var clearBtn = document.getElementById('datasourceClearBtn');
        var form = document.getElementById('datasourceForm');
        var passwordHelp = document.getElementById('password_help');
        var editing = false;
        if (!typeSelect || !portInput) {
            return;
        }

        function syncTypeDefaults() {
            var option = typeSelect.options[typeSelect.selectedIndex];
            if (!option) {
                return;
            }
            var port = option.getAttribute('data-port') || '5432';
            var schema = option.getAttribute('data-schema') || '';
            var label = option.getAttribute('data-dbname-label') || 'Database';
            var type = option.value;
            var isSqlite = type === 'sqlite';

            if (!portInput.dataset.touched) {
                portInput.value = port;
            }
            if (dbnameLabel) {
                dbnameLabel.textContent = label;
            }
            if (dbnameHelp) {
                if (type === 'oracle') {
                    dbnameHelp.textContent = 'For Oracle, enter the service name (or sid:ORCL for SID).';
                    dbnameHelp.style.display = '';
                } else if (type === 'snowflake') {
                    dbnameHelp.textContent = 'For Snowflake, Host is the account identifier (e.g. xy12345.us-east-1).';
                    dbnameHelp.style.display = '';
                } else if (isSqlite) {
                    dbnameHelp.textContent = 'Absolute path to a .sqlite, .db, or .gpkg file on the report server.';
                    dbnameHelp.style.display = '';
                } else {
                    dbnameHelp.style.display = 'none';
                }
            }
            if (hostLabel) {
                hostLabel.textContent = type === 'snowflake' ? 'Account' : 'Host';
            }
            if (hostPortRow) {
                hostPortRow.style.display = isSqlite ? 'none' : '';
            }
            if (usernameField) {
                usernameField.style.display = isSqlite ? 'none' : '';
            }
            if (hostInput) {
                if (isSqlite) {
                    hostInput.removeAttribute('required');
                } else {
                    hostInput.setAttribute('required', 'required');
                }
            }
            if (portInput) {
                if (isSqlite) {
                    portInput.removeAttribute('required');
                } else {
                    portInput.setAttribute('required', 'required');
                }
            }
            if (usernameInput) {
                if (isSqlite) {
                    usernameInput.removeAttribute('required');
                } else {
                    usernameInput.setAttribute('required', 'required');
                }
            }
            var showSchema = type === 'postgresql' || type === 'snowflake';
            if (schemaField) {
                schemaField.style.display = showSchema ? '' : 'none';
            }
            if (schemaInput && showSchema && schemaInput.value === '') {
                schemaInput.value = schema || (type === 'snowflake' ? 'PUBLIC' : 'public');
            }
        }

        function setEditMode(on, name) {
            editing = !!on;
            if (formTitle) {
                formTitle.textContent = editing ? ('Edit Datasource' + (name ? ': ' + name : '')) : 'Add Datasource';
            }
            if (clearBtn) {
                clearBtn.style.display = editing ? '' : 'none';
            }
            if (passwordHelp) {
                passwordHelp.textContent = editing
                    ? 'Leave blank to keep the current password.'
                    : 'Used for password authentication.';
            }
        }

        function refreshMaterialize() {
            if (window.M) {
                if (M.FormSelect) {
                    M.FormSelect.init(document.querySelectorAll('select'));
                }
                if (M.updateTextFields) {
                    M.updateTextFields();
                }
            }
        }

        function clearForm() {
            if (!form) {
                return;
            }
            form.reset();
            delete portInput.dataset.touched;
            typeSelect.value = 'postgresql';
            if (schemaInput) {
                schemaInput.value = 'public';
            }
            if (portInput) {
                portInput.value = '5432';
            }
            var passwordInput = document.getElementById('password');
            if (passwordInput) {
                passwordInput.value = '';
            }
            setEditMode(false);
            syncTypeDefaults();
            refreshMaterialize();
        }

        function fillFormFromButton(btn) {
            if (!btn || !form) {
                return;
            }
            var name = btn.getAttribute('data-name') || '';
            document.getElementById('name').value = name;
            typeSelect.value = btn.getAttribute('data-db-type') || 'postgresql';
            if (hostInput) {
                hostInput.value = btn.getAttribute('data-host') || '';
            }
            if (portInput) {
                portInput.value = btn.getAttribute('data-port') || '';
                portInput.dataset.touched = '1';
            }
            document.getElementById('dbname').value = btn.getAttribute('data-dbname') || '';
            if (schemaInput) {
                schemaInput.value = btn.getAttribute('data-schema') || '';
            }
            if (usernameInput) {
                usernameInput.value = btn.getAttribute('data-username') || '';
            }
            var passwordInput = document.getElementById('password');
            if (passwordInput) {
                passwordInput.value = '';
            }
            setEditMode(true, name);
            syncTypeDefaults();
            refreshMaterialize();
            if (formTitle) {
                formTitle.scrollIntoView({ behavior: 'smooth', block: 'start' });
            }
        }

        portInput.addEventListener('input', function () {
            portInput.dataset.touched = '1';
        });
        typeSelect.addEventListener('change', function () {
            delete portInput.dataset.touched;
            syncTypeDefaults();
        });
        document.querySelectorAll('.edit-ds-btn').forEach(function (btn) {
            btn.addEventListener('click', function () {
                fillFormFromButton(btn);
            });
        });
        if (clearBtn) {
            clearBtn.addEventListener('click', clearForm);
        }
        syncTypeDefaults();
    })();
    </script>
<?php endif; ?>
<?php
if ($conn) {
    pg_close($conn);
}
jrs_material_page_footer();
?>
