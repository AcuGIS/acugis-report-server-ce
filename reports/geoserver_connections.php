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
} elseif (!jrs_has_table($conn, 'jasper_geoserver')) {
    $errors[] = 'Missing public.jasper_geoserver. Load database/schema.sql or database/migrate_geoserver.sql into the application database.';
}

if ($_SERVER['REQUEST_METHOD'] === 'POST' && $errors === []) {
    $action = (string) ($_POST['action'] ?? '');
    if ($action === 'delete') {
        $id = (int) ($_POST['id'] ?? 0);
        if ($id > 0) {
            @pg_query_params($conn, 'DELETE FROM ' . jrs_table('jasper_geoserver') . ' WHERE id = $1', [$id]);
            jrs_write_geoserver_connections_file($conn);
            $message = 'GeoServer connection deleted. Properties file regenerated.';
        }
    } elseif ($action === 'regenerate') {
        if (jrs_write_geoserver_connections_file($conn)) {
            $message = 'GeoServer connections properties file regenerated for the Report Server WAR.';
        } else {
            $errors[] = 'Failed to write geoserver-connections.properties. Check geoserver_connections_path / permissions.';
        }
    } else {
        $name = trim((string) ($_POST['name'] ?? ''));
        $url = rtrim(trim((string) ($_POST['url'] ?? '')), '/');
        $username = trim((string) ($_POST['username'] ?? ''));
        $password = (string) ($_POST['password'] ?? '');

        if ($name === '' || $url === '') {
            $errors[] = 'Name and GeoServer base URL are required.';
        } elseif (str_contains($name, '.')) {
            $errors[] = 'Connection name must not contain a period (used as the properties key separator).';
        } else {
            $sql = 'INSERT INTO ' . jrs_table('jasper_geoserver') . '
                (name, url, username, password, enabled)
                VALUES ($1, $2, $3, $4, true)
                ON CONFLICT (name) DO UPDATE SET
                    url = EXCLUDED.url,
                    username = EXCLUDED.username,
                    password = EXCLUDED.password,
                    enabled = true';
            $result = @pg_query_params($conn, $sql, [$name, $url, $username, $password]);
            if ($result === false) {
                $errors[] = 'Failed to save GeoServer connection: ' . pg_last_error($conn);
            } else {
                pg_free_result($result);
                if (!jrs_write_geoserver_connections_file($conn)) {
                    $errors[] = 'Saved to database, but failed to write geoserver-connections.properties. Check path / permissions.';
                } else {
                    $message = 'GeoServer connection saved. Use the same Name as the Designer Connection Manager entry.';
                }
            }
        }
    }
}

$connections = $errors === [] && $conn ? jrs_geoserver_connections($conn) : [];
$propsPath = (string) jrs_config(
    'geoserver_connections_path',
    jrs_base_dir() . '/docker/report-server/geoserver-connections.properties'
);

jrs_material_page_header('GeoServer Connections');
?>
<header class="page-header">
    <div>
        <h1 class="page-title">GeoServer Connections</h1>
        <p class="page-subtitle">
            Credentials for Designer-published GeoServer maps. Match the <strong>Name</strong> to the
            Connection Manager name in Report Designer (not the local UUID). Anonymous public layers
            can omit username/password and rely on the snapshotted URL in JRXML.
        </p>
    </div>
    <div class="header-actions">
        <form method="post" class="inline-form">
            <input type="hidden" name="action" value="regenerate">
            <button type="submit" class="btn waves-effect waves-light blue-grey">
                <i class="material-icons left">sync</i>Regenerate Properties
            </button>
        </form>
        <a class="btn waves-effect waves-light blue-grey" href="datasources.php">
            <i class="material-icons left">storage</i>JDBC Datasources
        </a>
    </div>
</header>

<?php foreach ($errors as $error): ?>
    <div class="card-panel red lighten-4 red-text text-darken-4"><?php echo jrs_html($error); ?></div>
<?php endforeach; ?>
<?php if ($message !== ''): ?>
    <div class="card-panel green lighten-4 green-text text-darken-4"><?php echo jrs_html($message); ?></div>
<?php endif; ?>

<div class="card-panel blue-grey lighten-5">
    <p class="mb-0" style="margin:0;">
        Properties file (Tomcat / WAR): <code><?php echo jrs_html($propsPath); ?></code><br>
        Point the JVM at it with <code>-Dacugis.geoserver.connections.file=…</code> or mount as
        <code>conf/geoserver-connections.properties</code>. The WAR must include
        <code>acugis-report-map.jar</code>.
    </p>
</div>

<?php if ($errors === [] || $connections !== []): ?>
    <div class="row">
        <div class="col s12 l5">
            <div class="card">
                <div class="card-content">
                    <span class="card-title">Add or Update Connection</span>
                    <form method="post">
                        <div class="input-field">
                            <input type="text" id="name" name="name" required placeholder="ProdGeoServer">
                            <label for="name" class="active">Connection Name</label>
                            <span class="helper-text">Must match Designer Connection Manager name (JRXML <code>connectionName</code>).</span>
                        </div>
                        <div class="input-field">
                            <input type="url" id="url" name="url" required placeholder="https://gs.example.com/geoserver">
                            <label for="url" class="active">GeoServer Base URL</label>
                        </div>
                        <div class="input-field">
                            <input type="text" id="username" name="username" placeholder="(optional)">
                            <label for="username" class="active">Username</label>
                            <span class="helper-text">Leave empty for anonymous / public WMS.</span>
                        </div>
                        <div class="input-field">
                            <input type="password" id="password" name="password">
                            <label for="password">Password</label>
                        </div>
                        <button type="submit" class="btn waves-effect waves-light teal">
                            Save Connection
                            <i class="material-icons right">save</i>
                        </button>
                    </form>
                </div>
            </div>
        </div>

        <div class="col s12 l7">
            <div class="card connections-card">
                <div class="card-content" style="padding:0;">
                    <?php if ($connections === []): ?>
                        <div class="empty-state">
                            <i class="material-icons">public</i>
                            <p>No GeoServer connections configured.</p>
                        </div>
                    <?php else: ?>
                        <div class="connections-table-wrap">
                            <table class="striped highlight connections-table">
                                <thead>
                                <tr>
                                    <th>Name</th>
                                    <th>URL</th>
                                    <th>User</th>
                                    <th class="right-align">Actions</th>
                                </tr>
                                </thead>
                                <tbody>
                                <?php foreach ($connections as $row): ?>
                                    <tr>
                                        <td><code><?php echo jrs_html((string) $row['name']); ?></code></td>
                                        <td><?php echo jrs_html((string) $row['url']); ?></td>
                                        <td><?php echo jrs_html((string) ($row['username'] !== '' ? $row['username'] : '—')); ?></td>
                                        <td class="right-align">
                                            <?php if (isset($row['id'])): ?>
                                                <form method="post" class="inline-form" onsubmit="return confirm('Delete this GeoServer connection?');">
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
<?php endif; ?>

<?php jrs_material_page_footer(); ?>
