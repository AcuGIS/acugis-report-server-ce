<?php
declare(strict_types=1);

require_once __DIR__ . '/native_paths.php';
jrs_load_dotenv_files(dirname(__DIR__));

$GLOBALS['JRS_CONFIG'] = require dirname(__DIR__) . '/config.php';

function jrs_config(string $key, $default = null)
{
    return $GLOBALS['JRS_CONFIG'][$key] ?? $default;
}

function jrs_base_dir(): string
{
    return dirname(__DIR__);
}

function jrs_app_schema(): string
{
    $schema = (string) jrs_config('app_schema', 'public');
    return preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $schema) === 1 ? $schema : 'public';
}

function jrs_ident(string $identifier): string
{
    return '"' . str_replace('"', '""', $identifier) . '"';
}

function jrs_table(string $table): string
{
    return jrs_ident(jrs_app_schema()) . '.' . jrs_ident($table);
}

/**
 * Normalize datasource engine to one of: postgresql, mysql, oracle.
 */
function jrs_normalize_db_type($type): string
{
    $normalized = strtolower(trim((string) $type));
    if ($normalized === 'postgres' || $normalized === 'pgsql' || $normalized === 'pg') {
        return 'postgresql';
    }
    if ($normalized === 'mariadb') {
        return 'mysql';
    }
    if ($normalized === 'mssql' || $normalized === 'sql server' || $normalized === 'microsoft sql server') {
        return 'sqlserver';
    }
    if ($normalized === 'geopackage' || $normalized === 'gpkg') {
        return 'sqlite';
    }
    if ($normalized === 'ibm db2' || $normalized === 'ibmdb2') {
        return 'db2';
    }
    if ($normalized === 'apache cassandra' || $normalized === 'datastax' || $normalized === 'datastax astra' || $normalized === 'astra') {
        return 'cassandra';
    }
    if (in_array($normalized, ['postgresql', 'mysql', 'oracle', 'sqlserver', 'db2', 'snowflake', 'sqlite', 'cassandra'], true)) {
        return $normalized;
    }
    return 'postgresql';
}

/**
 * @return array{label:string,port:int,schema:string,dbname_label:string}
 */
function jrs_db_type_meta(string $dbType): array
{
    switch (jrs_normalize_db_type($dbType)) {
        case 'mysql':
            return [
                'label' => 'MySQL',
                'port' => 3306,
                'schema' => '',
                'dbname_label' => 'Database',
            ];
        case 'oracle':
            return [
                'label' => 'Oracle',
                'port' => 1521,
                'schema' => '',
                'dbname_label' => 'Service name / SID',
            ];
        case 'sqlserver':
            return [
                'label' => 'Microsoft SQL Server',
                'port' => 1433,
                'schema' => '',
                'dbname_label' => 'Database',
            ];
        case 'db2':
            return [
                'label' => 'IBM Db2',
                'port' => 50000,
                'schema' => '',
                'dbname_label' => 'Database',
            ];
        case 'snowflake':
            return [
                'label' => 'Snowflake',
                'port' => 443,
                'schema' => 'PUBLIC',
                'dbname_label' => 'Database',
            ];
        case 'sqlite':
            return [
                'label' => 'SQLite / GeoPackage',
                'port' => 0,
                'schema' => '',
                'dbname_label' => 'Database file path',
            ];
        case 'cassandra':
            return [
                'label' => 'Apache Cassandra',
                'port' => 9042,
                'schema' => '',
                'dbname_label' => 'Keyspace',
            ];
        default:
            return [
                'label' => 'PostgreSQL',
                'port' => 5432,
                'schema' => 'public',
                'dbname_label' => 'Database',
            ];
    }
}

/**
 * JDBC settings for Tomcat JNDI Resource generation.
 *
 * @return array{driverClassName:string,url:string,validationQuery:string}|null
 */
function jrs_jdbc_settings_for_datasource(array $row): ?array
{
    $host = trim((string) ($row['host'] ?? ''));
    $port = (int) ($row['port'] ?? 0);
    $dbName = trim((string) ($row['dbname'] ?? ''));
    $dbType = jrs_normalize_db_type($row['db_type'] ?? 'postgresql');

    if ($dbType === 'cassandra') {
        // Cassandra / Astra use Go collection fill (gocql / Data API), not JDBC JNDI.
        return null;
    }

    if ($dbType === 'sqlite') {
        $path = $dbName !== '' ? $dbName : $host;
        if ($path === '') {
            return null;
        }
        return [
            'driverClassName' => 'org.sqlite.JDBC',
            'url' => 'jdbc:sqlite:' . str_replace('\\', '/', $path),
            'validationQuery' => 'SELECT 1',
        ];
    }
    if ($host === '' || $port <= 0 || $dbName === '') {
        return null;
    }

    switch ($dbType) {
        case 'mysql':
            return [
                'driverClassName' => 'com.mysql.cj.jdbc.Driver',
                'url' => 'jdbc:mysql://' . $host . ':' . $port . '/' . $dbName
                    . '?useSSL=false&allowPublicKeyRetrieval=true&serverTimezone=UTC',
                'validationQuery' => 'SELECT 1',
            ];
        case 'oracle':
            // Prefer service name (@//host:port/service). Prefix dbname with "sid:" for SID syntax.
            if (stripos($dbName, 'sid:') === 0) {
                $sid = trim(substr($dbName, 4));
                $url = 'jdbc:oracle:thin:@' . $host . ':' . $port . ':' . $sid;
            } else {
                $url = 'jdbc:oracle:thin:@//' . $host . ':' . $port . '/' . $dbName;
            }
            return [
                'driverClassName' => 'oracle.jdbc.OracleDriver',
                'url' => $url,
                'validationQuery' => 'SELECT 1 FROM DUAL',
            ];
        case 'sqlserver':
            return [
                'driverClassName' => 'com.microsoft.sqlserver.jdbc.SQLServerDriver',
                'url' => 'jdbc:sqlserver://' . $host . ':' . $port
                    . ';databaseName=' . $dbName . ';encrypt=true;trustServerCertificate=false;',
                'validationQuery' => 'SELECT 1',
            ];
        case 'db2':
            // Db2 is not packaged / not offered for Tomcat JNDI — skip context.xml generation.
            return null;
        case 'snowflake':
            $accountHost = $host;
            $lower = strtolower($accountHost);
            if (!str_contains($lower, 'snowflakecomputing.com')) {
                $accountHost = preg_replace('#^https?://#i', '', $accountHost) . '.snowflakecomputing.com';
            }
            $url = 'jdbc:snowflake://' . $accountHost . '/?db=' . rawurlencode($dbName);
            $schema = trim((string) ($row['schema_name'] ?? ''));
            if ($schema !== '') {
                $url .= '&schema=' . rawurlencode($schema);
            }
            return [
                'driverClassName' => 'net.snowflake.client.jdbc.SnowflakeDriver',
                'url' => $url,
                'validationQuery' => 'SELECT 1',
            ];
        default:
            return [
                'driverClassName' => 'org.postgresql.Driver',
                'url' => 'jdbc:postgresql://' . $host . ':' . $port . '/' . $dbName,
                'validationQuery' => 'SELECT 1',
            ];
    }
}

function jrs_ensure_datasource_db_type_column($conn): void
{
    if (!$conn || !jrs_has_table($conn, 'jasper_datasource')) {
        return;
    }

    @pg_query(
        $conn,
        'ALTER TABLE ' . jrs_table('jasper_datasource')
        . " ADD COLUMN IF NOT EXISTS db_type varchar(20) NOT NULL DEFAULT 'postgresql'"
    );
}

function jrs_pg_connect_from_settings(array $settings)
{
    if (!extension_loaded('pgsql')) {
        return false;
    }

    $quote = static function ($value): string {
        return "'" . str_replace(["\\", "'"], ["\\\\", "\\'"], (string) $value) . "'";
    };

    $parts = [
        'host=' . $quote($settings['host'] ?? 'localhost'),
        'port=' . (int) ($settings['port'] ?? 5432),
        'dbname=' . $quote($settings['dbname'] ?? $settings['name'] ?? ''),
        'user=' . $quote($settings['username'] ?? $settings['user'] ?? ''),
    ];

    $password = (string) ($settings['password'] ?? '');
    if ($password !== '') {
        $parts[] = 'password=' . $quote($password);
    }

    return @pg_connect(implode(' ', $parts));
}

function jrs_mysql_connect_from_settings(array $settings)
{
    $host = (string) ($settings['host'] ?? 'localhost');
    $port = (int) ($settings['port'] ?? 3306);
    $dbname = (string) ($settings['dbname'] ?? '');
    $username = (string) ($settings['username'] ?? $settings['user'] ?? '');
    $password = (string) ($settings['password'] ?? '');

    if (extension_loaded('mysqli')) {
        $mysqli = @mysqli_connect($host, $username, $password, $dbname !== '' ? $dbname : null, $port);
        return $mysqli ?: false;
    }

    if (extension_loaded('pdo_mysql')) {
        try {
            $dsn = sprintf('mysql:host=%s;port=%d;dbname=%s;charset=utf8mb4', $host, $port, $dbname);
            return new PDO($dsn, $username, $password, [
                PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION,
            ]);
        } catch (Throwable $e) {
            return false;
        }
    }

    return false;
}

function jrs_oracle_connect_from_settings(array $settings)
{
    $host = (string) ($settings['host'] ?? 'localhost');
    $port = (int) ($settings['port'] ?? 1521);
    $dbname = (string) ($settings['dbname'] ?? '');
    $username = (string) ($settings['username'] ?? $settings['user'] ?? '');
    $password = (string) ($settings['password'] ?? '');

    if (stripos($dbname, 'sid:') === 0) {
        $sid = trim(substr($dbname, 4));
        $easyConnect = $host . ':' . $port . '/' . $sid;
        $tns = '(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=' . $host . ')(PORT=' . $port . '))(CONNECT_DATA=(SID=' . $sid . ')))';
    } else {
        $easyConnect = $host . ':' . $port . '/' . $dbname;
        $tns = '(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=' . $host . ')(PORT=' . $port . '))(CONNECT_DATA=(SERVICE_NAME=' . $dbname . ')))';
    }

    if (extension_loaded('oci8') || extension_loaded('oci8_19') || function_exists('oci_connect')) {
        $conn = @oci_connect($username, $password, $easyConnect);
        if ($conn === false) {
            $conn = @oci_connect($username, $password, $tns);
        }
        return $conn ?: false;
    }

    if (extension_loaded('pdo_oci')) {
        try {
            return new PDO('oci:dbname=' . $tns, $username, $password, [
                PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION,
            ]);
        } catch (Throwable $e) {
            return false;
        }
    }

    return false;
}

function jrs_app_db()
{
    if (array_key_exists('JRS_APP_DB', $GLOBALS) && $GLOBALS['JRS_APP_DB'] !== false) {
        return $GLOBALS['JRS_APP_DB'];
    }
    $conn = jrs_pg_connect_from_settings((array) jrs_config('db', []));
    $GLOBALS['JRS_APP_DB'] = $conn ?: false;
    return $conn ?: false;
}

/** PostgreSQL boolean literal for pg_query_params (PHP false becomes "" otherwise). */
function jrs_pg_bool($value): string
{
    if ($value === true || $value === 't' || $value === 'true' || $value === '1' || $value === 1 || $value === 'on') {
        return 't';
    }
    return 'f';
}

/** True for PHP/PG boolean-ish values (pg returns 't'/'f' strings). */
function jrs_pg_is_true($value): bool
{
    if ($value === true || $value === 1 || $value === '1') {
        return true;
    }
    if (!is_string($value)) {
        return false;
    }
    $v = strtolower(trim($value));

    return in_array($v, ['t', 'true', 'yes', 'on', 'y'], true);
}

function jrs_has_table($conn, string $table): bool
{
    if (!$conn) {
        return false;
    }

    $qualified = jrs_app_schema() . '.' . $table;
    $result = @pg_query_params($conn, 'SELECT to_regclass($1) AS table_name', [$qualified]);
    if ($result === false) {
        return false;
    }

    $row = pg_fetch_assoc($result);
    pg_free_result($result);
    return !empty($row['table_name']);
}

function jrs_json_dir(): string
{
    return jrs_base_dir() . '/reports/json';
}

function jrs_report_files_dir(): string
{
    return jrs_resolve_jrxml_dir(jrs_base_dir());
}

function jrs_report_assets_dir(): string
{
    return jrs_base_dir() . '/assets/reports';
}

function jrs_ensure_dir(string $path): bool
{
    return is_dir($path) || mkdir($path, 0775, true);
}

function jrs_html(string $value): string
{
    return htmlspecialchars($value, ENT_QUOTES, 'UTF-8');
}

require_once __DIR__ . '/auth.php';
require_once __DIR__ . '/service_acl.php';
require_once __DIR__ . '/resource_sync.php';
require_once __DIR__ . '/schedule_admin.php';
require_once __DIR__ . '/schedule_config.php';
require_once __DIR__ . '/schedule_repository.php';
require_once __DIR__ . '/email_template_repository.php';
require_once __DIR__ . '/recipient_repository.php';
require_once __DIR__ . '/execution_repository.php';
require_once __DIR__ . '/schedule_systemd.php';
require_once __DIR__ . '/schedule_mail.php';
require_once __DIR__ . '/schedule_runner.php';
// Professional Map Report. The Community export omits lib/map_report.php.
if (is_file(__DIR__ . '/map_report.php')) {
    require_once __DIR__ . '/map_report.php';
}

function jrs_report_key(string $value): string
{
    $value = strtolower(trim($value));
    $value = preg_replace('/\s+/', '-', $value) ?? '';
    $value = preg_replace('/[^a-z0-9_-]/', '', $value) ?? '';
    return trim($value, '-_');
}

function jrs_is_valid_report_key(string $value): bool
{
    return preg_match('/^[a-z0-9_-]+$/', $value) === 1;
}

function jrs_report_json_path(string $report): string
{
    return jrs_json_dir() . '/' . $report . '.report.json';
}

function jrs_report_jrxml_path(string $report): string
{
    return jrs_report_files_dir() . '/' . $report . '.jrxml';
}

function jrs_report_jasper_path(string $report): string
{
    return jrs_report_files_dir() . '/' . $report . '.jasper';
}

function jrs_report_asset_path(string $report): string
{
    return jrs_report_assets_dir() . '/' . $report . '.png';
}

function jrs_datasources($conn = null): array
{
    if (!$conn) {
        $conn = jrs_app_db();
    }

    if (!$conn) {
        return [];
    }

    $rows = [];
    if (jrs_has_table($conn, 'jasper_datasource')) {
        jrs_ensure_datasource_db_type_column($conn);
        $sql = 'SELECT id, name, db_type, host, port, dbname, username, password, schema_name FROM '
            . jrs_table('jasper_datasource') . ' WHERE enabled = true ORDER BY name';
        $result = @pg_query($conn, $sql);
        if ($result === false) {
            // Pre-migration schema without db_type
            $sql = 'SELECT id, name, host, port, dbname, username, password, schema_name FROM '
                . jrs_table('jasper_datasource') . ' WHERE enabled = true ORDER BY name';
            $result = @pg_query($conn, $sql);
        }
    } elseif (jrs_has_table($conn, 'pg_store')) {
        $sql = 'SELECT svc_name AS name, host, port, dbname, username, password, schema AS schema_name FROM '
            . jrs_table('pg_store') . ' ORDER BY svc_name';
        $result = @pg_query($conn, $sql);
    } else {
        $result = false;
    }

    if ($result !== false) {
        while ($row = pg_fetch_assoc($result)) {
            $name = trim((string) ($row['name'] ?? ''));
            if ($name !== '') {
                $row['db_type'] = jrs_normalize_db_type($row['db_type'] ?? 'postgresql');
                $rows[] = $row;
            }
        }
        pg_free_result($result);
    }

    return $rows;
}

function jrs_datasource_names($conn = null): array
{
    return array_values(array_map(static fn(array $row): string => (string) $row['name'], jrs_datasources($conn)));
}

function jrs_find_datasource(string $name, $conn = null): ?array
{
    foreach (jrs_datasources($conn) as $row) {
        if ((string) $row['name'] === $name) {
            return $row;
        }
    }
    return null;
}

/**
 * Enabled GeoServer connections (Designer publish lookup by stable name).
 *
 * @return list<array<string,mixed>>
 */
function jrs_geoserver_connections($conn = null): array
{
    if (!$conn) {
        $conn = jrs_app_db();
    }
    if (!$conn || !jrs_has_table($conn, 'jasper_geoserver')) {
        return [];
    }
    $sql = 'SELECT id, name, url, username, password FROM '
        . jrs_table('jasper_geoserver') . ' WHERE enabled = true ORDER BY name';
    $result = @pg_query($conn, $sql);
    if ($result === false) {
        return [];
    }
    $rows = [];
    while ($row = pg_fetch_assoc($result)) {
        $name = trim((string) ($row['name'] ?? ''));
        if ($name !== '') {
            $rows[] = $row;
        }
    }
    pg_free_result($result);
    return $rows;
}

function jrs_find_geoserver_connection(string $name, $conn = null): ?array
{
    foreach (jrs_geoserver_connections($conn) as $row) {
        if ((string) $row['name'] === $name) {
            return $row;
        }
    }
    return null;
}

/**
 * Write geoserver-connections.properties for Java GeoServerAuth (Tomcat / Report Server WAR).
 * Returns true on success.
 */
function jrs_write_geoserver_connections_file($conn = null): bool
{
    $path = (string) jrs_config(
        'geoserver_connections_path',
        jrs_base_dir() . '/docker/report-server/geoserver-connections.properties'
    );
    $dir = dirname($path);
    if (!is_dir($dir) && !@mkdir($dir, 0755, true) && !is_dir($dir)) {
        return false;
    }
    $rows = jrs_geoserver_connections($conn);
    $lines = [
        '# Generated by AcuGIS Report Server — do not edit by hand',
        '# Java: com.acugis.map.GeoServerAuth (system property acugis.geoserver.connections.file)',
        '# Keys are stable connection names matching Designer Connection Manager / JRXML connectionName',
        '',
    ];
    foreach ($rows as $row) {
        $name = trim((string) ($row['name'] ?? ''));
        if ($name === '') {
            continue;
        }
        $url = trim((string) ($row['url'] ?? ''));
        $user = (string) ($row['username'] ?? '');
        $pass = (string) ($row['password'] ?? '');
        $lines[] = '# ' . $name;
        $lines[] = jrs_geoserver_prop_key($name, 'url') . '=' . jrs_geoserver_prop_escape($url);
        $lines[] = jrs_geoserver_prop_key($name, 'username') . '=' . jrs_geoserver_prop_escape($user);
        $lines[] = jrs_geoserver_prop_key($name, 'password') . '=' . jrs_geoserver_prop_escape($pass);
        $lines[] = '';
    }
    $body = implode("\n", $lines) . "\n";
    return @file_put_contents($path, $body) !== false;
}

function jrs_geoserver_prop_key(string $name, string $field): string
{
    // Exact connection name (must match Designer / JRXML connectionName). Avoid '.' in names.
    return $name . '.' . $field;
}

function jrs_geoserver_prop_escape(string $value): string
{
    // Java Properties load: escape backslash and line breaks in values.
    $value = str_replace(['\\', "\n", "\r", "\t"], ['\\\\', '\\n', '\\r', '\\t'], $value);
    return $value;
}

/**
 * Open a PHP connection for SQL-backed report parameters.
 *
 * @return array{driver:string,conn:mixed}|false
 */
function jrs_connect_datasource(string $name)
{
    $row = jrs_find_datasource($name);
    if ($row === null) {
        return false;
    }

    $dbType = jrs_normalize_db_type($row['db_type'] ?? 'postgresql');
    if ($dbType === 'mysql') {
        $conn = jrs_mysql_connect_from_settings($row);
        return $conn ? ['driver' => 'mysql', 'conn' => $conn] : false;
    }
    if ($dbType === 'oracle') {
        $conn = jrs_oracle_connect_from_settings($row);
        return $conn ? ['driver' => 'oracle', 'conn' => $conn] : false;
    }

    $conn = jrs_pg_connect_from_settings($row);
    return $conn ? ['driver' => 'pgsql', 'conn' => $conn] : false;
}

/**
 * @param array{driver:string,conn:mixed} $handle
 * @return list<array{label:mixed,value:mixed}>
 */
function jrs_fetch_sql_option_rows(array $handle, string $sql): array
{
    $driver = $handle['driver'];
    $conn = $handle['conn'];
    $options = [];

    if ($driver === 'pgsql') {
        $result = @pg_query($conn, $sql);
        if ($result === false) {
            return [];
        }
        while ($row = pg_fetch_assoc($result)) {
            $values = array_values($row);
            if (count($values) === 1) {
                $options[] = ['label' => $values[0], 'value' => $values[0]];
            } elseif (count($values) >= 2) {
                $options[] = ['label' => $values[0], 'value' => $values[1]];
            }
        }
        pg_free_result($result);
        return $options;
    }

    if ($driver === 'mysql') {
        if ($conn instanceof PDO) {
            try {
                $stmt = $conn->query($sql);
                if ($stmt === false) {
                    return [];
                }
                while ($row = $stmt->fetch(PDO::FETCH_NUM)) {
                    if (count($row) === 1) {
                        $options[] = ['label' => $row[0], 'value' => $row[0]];
                    } elseif (count($row) >= 2) {
                        $options[] = ['label' => $row[0], 'value' => $row[1]];
                    }
                }
            } catch (Throwable $e) {
                return [];
            }
            return $options;
        }

        $result = @mysqli_query($conn, $sql);
        if ($result === false) {
            return [];
        }
        while ($row = mysqli_fetch_row($result)) {
            if (count($row) === 1) {
                $options[] = ['label' => $row[0], 'value' => $row[0]];
            } elseif (count($row) >= 2) {
                $options[] = ['label' => $row[0], 'value' => $row[1]];
            }
        }
        mysqli_free_result($result);
        return $options;
    }

    if ($driver === 'oracle') {
        if ($conn instanceof PDO) {
            try {
                $stmt = $conn->query($sql);
                if ($stmt === false) {
                    return [];
                }
                while ($row = $stmt->fetch(PDO::FETCH_NUM)) {
                    if (count($row) === 1) {
                        $options[] = ['label' => $row[0], 'value' => $row[0]];
                    } elseif (count($row) >= 2) {
                        $options[] = ['label' => $row[0], 'value' => $row[1]];
                    }
                }
            } catch (Throwable $e) {
                return [];
            }
            return $options;
        }

        $stid = @oci_parse($conn, $sql);
        if ($stid === false || !@oci_execute($stid)) {
            return [];
        }
        while (($row = oci_fetch_array($stid, OCI_NUM + OCI_RETURN_NULLS)) !== false) {
            if (count($row) === 1) {
                $options[] = ['label' => $row[0], 'value' => $row[0]];
            } elseif (count($row) >= 2) {
                $options[] = ['label' => $row[0], 'value' => $row[1]];
            }
        }
        oci_free_statement($stid);
        return $options;
    }

    return [];
}

function jrs_close_datasource_handle(array $handle): void
{
    $driver = $handle['driver'] ?? '';
    $conn = $handle['conn'] ?? null;
    if ($conn === null) {
        return;
    }

    if ($driver === 'pgsql') {
        @pg_close($conn);
        return;
    }
    if ($driver === 'mysql') {
        if ($conn instanceof PDO) {
            return;
        }
        @mysqli_close($conn);
        return;
    }
    if ($driver === 'oracle' && !($conn instanceof PDO) && function_exists('oci_close')) {
        @oci_close($conn);
    }
}

/**
 * Access-group names for report publish/edit UI.
 * Prefer auth_groups (Users & Groups); fall back to legacy jasper_access_group.
 *
 * @param resource|null $conn App/auth DB when available
 * @return list<string>
 */
function jrs_access_groups($conn = null): array
{
    $db = $conn ?: jrs_app_db();
    if ($db) {
        $fromAuth = jrs_auth_all_group_names($db);
        if ($fromAuth !== []) {
            return $fromAuth;
        }
    }

    if (!$db || !jrs_has_table($db, 'jasper_access_group')) {
        return [];
    }

    $groups = [];
    $result = @pg_query($db, 'SELECT name FROM ' . jrs_table('jasper_access_group') . ' ORDER BY name');
    if ($result !== false) {
        while ($row = pg_fetch_assoc($result)) {
            $name = trim((string) ($row['name'] ?? ''));
            if ($name !== '') {
                $groups[] = $name;
            }
        }
        pg_free_result($result);
    }

    return $groups;
}

function jrs_expand_sql_parameter_options(array $definition): array
{
    $params = $definition['parameters'] ?? $definition['params'] ?? [];
    if (!is_array($params)) {
        return $definition;
    }

    $datasource = trim((string) ($definition['datasource'] ?? $definition['dataSource'] ?? ''));
    if ($datasource === '') {
        return $definition;
    }

    $handle = jrs_connect_datasource($datasource);
    if ($handle === false) {
        return $definition;
    }

    foreach ($params as $idx => $param) {
        if (!is_array($param)) {
            continue;
        }

        $source = trim((string) ($param['source'] ?? ''));
        if ($source === '' || strpos($source, 'sql:') !== 0) {
            continue;
        }

        $sql = trim(substr($source, 4));
        if ($sql === '') {
            continue;
        }

        $params[$idx]['options'] = jrs_fetch_sql_option_rows($handle, $sql);
    }

    jrs_close_datasource_handle($handle);
    $definition['parameters'] = $params;
    unset($definition['params']);
    return $definition;
}

function jrs_read_report(string $report, bool $expandSqlOptions = false): ?array
{
    if (!jrs_is_valid_report_key($report)) {
        return null;
    }

    $path = jrs_report_json_path($report);
    if (!is_file($path)) {
        return null;
    }

    $data = json_decode((string) file_get_contents($path), true);
    if (!is_array($data)) {
        return null;
    }

    return $expandSqlOptions ? jrs_expand_sql_parameter_options($data) : $data;
}

function jrs_all_reports(bool $expandSqlOptions = false): array
{
    $reports = [];
    if (!is_dir(jrs_json_dir())) {
        return $reports;
    }

    foreach (glob(jrs_json_dir() . '/*.report.json') ?: [] as $path) {
        $data = json_decode((string) file_get_contents($path), true);
        if (!is_array($data)) {
            continue;
        }
        if (trim((string) ($data['report'] ?? '')) === '') {
            $data['report'] = basename($path, '.report.json');
        }
        $reports[] = $expandSqlOptions ? jrs_expand_sql_parameter_options($data) : $data;
    }

    usort($reports, static function (array $a, array $b): int {
        return strcasecmp((string) ($a['name'] ?? ''), (string) ($b['name'] ?? ''));
    });

    return $reports;
}

function jrs_report_url(string $report, string $datasource, string $format = 'html', array $params = []): string
{
    $query = array_merge([
        '_repName' => $report,
        '_dataSource' => $datasource,
        '_repFormat' => $format,
    ], $params);

    return (string) jrs_config('report_endpoint', 'execute_report.php') . '?' . http_build_query($query);
}

function jrs_material_nav_item(string $href, string $icon, string $label, string $activePage): string
{
    $page = basename((string) (parse_url($href, PHP_URL_PATH) ?: $href));
    $active = $page === $activePage ? ' class="active"' : '';

    return '<li' . $active . '><a href="' . jrs_html($href) . '" class="waves-effect">'
        . '<i class="material-icons">' . jrs_html($icon) . '</i><span>' . jrs_html($label) . '</span></a></li>';
}

function jrs_app_mount(): string
{
    $mount = trim((string) jrs_config('app_mount', '/acugis-report-server'));
    if ($mount === '' || $mount[0] !== '/') {
        $mount = '/' . ltrim($mount, '/');
    }
    return rtrim($mount, '/') ?: '/acugis-report-server';
}

function jrs_product_name(): string
{
    $name = trim((string) jrs_config('product_name', 'AcuGIS Report Server'));
    return $name !== '' ? $name : 'AcuGIS Report Server';
}

function jrs_shell_critical_style(): string
{
    // Inline so desktop/mobile shell visibility is correct before any JS/CSS fetch.
    return '<style id="fed-shell-critical">'
        . '.fed-shell-menu-btn{display:none!important}'
        . '.fed-shell-backdrop{display:none!important}'
        . 'body.fed-shell-body{margin:0;padding-left:var(--fed-shell-sidebar-width,250px);box-sizing:border-box}'
        . '.fed-shell-sidebar{position:fixed;top:0;left:0;bottom:0;width:var(--fed-shell-sidebar-width,250px);z-index:1000;overflow:hidden;background:#f0f4f9}'
        . '@media (max-width:992px){'
        . 'body.fed-shell-body{padding-left:0}'
        . '.fed-shell-sidebar{transform:translateX(-100%)}'
        . 'body.fed-shell-open .fed-shell-sidebar{transform:translateX(0)}'
        . 'body.fed-shell-open .fed-shell-backdrop{display:block!important}'
        . '.fed-shell-menu-btn{display:flex!important;align-items:center;justify-content:center;position:fixed;top:12px;left:12px;z-index:1002;width:44px;height:44px;border:none;border-radius:50%;background:#fff;cursor:pointer}'
        . '}'
        . '</style>';
}

function jrs_shell_bootstrap_tag(): string
{
    return jrs_shell_critical_style()
        . '<link rel="stylesheet" href="/shared/ui/shell.css?v=20260727f" id="fed-shell-css">'
        . '<script src="/shared/ui/shell-bootstrap.js?v=20261011b"></script>';
}

function jrs_shell_chrome_html(): string
{
    $brand = jrs_html(jrs_product_name());
    $home = jrs_html(jrs_app_mount() . '/hubs/reports.php');

    return '
<aside id="fed-shell-sidebar" class="fed-shell-sidebar" aria-label="Application navigation">
    <a class="fed-shell-sidebar-brand" href="' . $home . '"><i class="material-icons">assessment</i><span>' . $brand . '</span></a>
    <div class="fed-shell-nav-scroll" data-fed-shell-nav-mount></div>
</aside>
<button type="button" class="fed-shell-menu-btn" aria-label="Open menu" hidden><i class="material-icons">menu</i></button>
<div class="fed-shell-backdrop"></div>';
}

function jrs_material_workspace_page(): void
{
    $title = jrs_html(jrs_product_name());
    echo '<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>' . $title . '</title>
    ' . jrs_shell_bootstrap_tag() . '
    <link href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" rel="stylesheet">
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
    <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap" rel="stylesheet">
</head>
<body data-federation-app="jasper" class="fed-shell-body">
' . jrs_shell_chrome_html() . '
<main class="fed-shell-main" data-federation-workspace></main>
</body>
</html>';
}

function jrs_material_hub_page(string $hubId, string $title): void
{
    $safeHub = preg_replace('/[^a-z0-9_-]/i', '', $hubId) ?? '';
    $safeTitle = jrs_html($title);
    $product = jrs_html(jrs_product_name());

    echo '<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>' . $safeTitle . ' - ' . $product . '</title>
    ' . jrs_shell_bootstrap_tag() . '
    <link href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" rel="stylesheet">
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
    <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap" rel="stylesheet">
</head>
<body data-federation-app="jasper" data-federation-hub="' . jrs_html($safeHub) . '" class="fed-shell-body">
' . jrs_shell_chrome_html() . '
<main class="fed-shell-main" data-federation-hub="' . jrs_html($safeHub) . '"></main>
</body>
</html>';
}

/**
 * @param array{no_sidebar?:bool,no_back?:bool} $options
 */
function jrs_material_page_header(string $title, array $options = []): void
{
    $safeTitle = jrs_html($title);
    $noSidebar = !empty($options['no_sidebar']);
    $noBack = !empty($options['no_back']);
    $bodyAttrs = 'data-federation-app="jasper" class="fed-shell-body"';
    if ($noSidebar) {
        $bodyAttrs .= ' data-federation-no-sidebar';
    }
    if ($noBack) {
        $bodyAttrs .= ' data-federation-no-back';
    }
    $chrome = $noSidebar ? '' : jrs_shell_chrome_html();

    echo '<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>' . $safeTitle . ' - ' . jrs_html(jrs_product_name()) . '</title>
    ' . jrs_shell_bootstrap_tag() . '
    <link href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" rel="stylesheet">
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
    <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --content-bg: #f8f9fa;
            --header-color: #202124;
            --subtle-text: #5f6368;
        }

        html,
        body {
            background: var(--content-bg);
            color: #263238;
            min-height: 100vh;
        }

        body {
            padding-bottom: 0;
        }

        .main-content {
            padding: 32px 40px;
            min-height: 100vh;
            box-sizing: border-box;
        }

        .page-header {
            display: flex;
            flex-wrap: wrap;
            align-items: flex-start;
            justify-content: space-between;
            gap: 12px;
            margin-bottom: 28px;
        }

        .page-header .page-title {
            font-size: 2.2rem;
            font-weight: 500;
            color: var(--header-color);
            margin: 0 0 6px;
            line-height: 1.2;
        }

        .page-header .page-subtitle {
            margin: 0;
            color: var(--subtle-text);
            font-size: 0.95rem;
        }

        .main-content .card {
            border-radius: 6px;
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06), 0 1px 2px rgba(0, 0, 0, 0.04);
            margin: 0 0 24px;
        }

        .main-content .card .card-content {
            padding: 28px;
        }

        .main-content .card .card-title {
            font-size: 1.15rem;
            font-weight: 500;
            color: var(--header-color);
            margin-bottom: 18px;
            display: block;
        }

        .main-content .card .input-field {
            margin-top: 1rem;
        }

        .btn,
        .btn-small,
        .btn-flat {
            text-transform: none;
            font-weight: 500;
            letter-spacing: 0.2px;
        }

        .header-actions,
        .card-actions-row,
        .form-actions {
            display: flex;
            flex-wrap: wrap;
            align-items: center;
            gap: 8px;
        }

        .form-actions {
            justify-content: flex-end;
        }

        .report-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
            gap: 20px;
        }

        .report-card {
            overflow: hidden;
        }

        .report-card-thumb,
        .report-card-placeholder {
            width: 100%;
            height: 150px;
            object-fit: cover;
            background: #eceff1;
        }

        .report-card-placeholder {
            display: flex;
            align-items: center;
            justify-content: center;
            color: #90a4ae;
        }

        .report-card-description {
            margin: 0;
            color: var(--subtle-text);
            font-size: 0.95rem;
            line-height: 1.45;
        }

        .report-meta {
            font-size: 0.88rem;
            color: var(--subtle-text);
        }

        .report-meta .meta-row {
            margin-bottom: 6px;
            word-break: break-word;
        }

        .report-meta .meta-label {
            font-weight: 600;
            color: #455a64;
            margin-right: 6px;
        }

        .report-meta .meta-value,
        code {
            font-family: ui-monospace, "SFMono-Regular", Menlo, Consolas, monospace;
            color: #102a43;
            background: #eceff1;
            border-radius: 3px;
            padding: 1px 4px;
        }

        .empty-state {
            text-align: center;
            padding: 56px 20px;
            color: var(--subtle-text);
        }

        .empty-state i.material-icons {
            font-size: 56px;
            color: #b0bec5;
            margin-bottom: 12px;
        }

        .connections-table-wrap,
        .table-scroll {
            overflow-x: auto;
            max-width: 100%;
            -webkit-overflow-scrolling: touch;
        }

        .connections-table {
            font-size: 0.86rem;
            margin: 0;
        }

        .connections-table thead th {
            background: #eceff1;
            color: #263238;
            white-space: nowrap;
            padding: 10px 12px;
        }

        .connections-table td {
            padding: 8px 12px;
            vertical-align: middle;
        }

        .inline-form {
            display: inline;
            margin: 0;
        }

        .actions-cell .btn-small {
            margin: 2px 0 2px 4px;
        }

        .parameter-row {
            border: 1px solid #cfd8dc;
            border-radius: 6px;
            padding: 18px;
            margin-bottom: 16px;
            background: #ffffff;
        }

        .parameter-row .remove-parameter-btn {
            margin-top: 22px;
        }

        .section-divider {
            border: 0;
            border-top: 1px solid #e0e0e0;
            margin: 28px 0;
        }

        .jrs-url-output {
            white-space: pre-wrap;
            word-break: break-all;
            background: #eceff1;
            border: 1px solid #cfd8dc;
            border-radius: 4px;
            padding: 14px;
            margin: 0;
            min-height: 52px;
            user-select: all;
        }

        .report-frame {
            width: 100%;
            height: 420px;
            border: 1px solid #cfd8dc;
            border-radius: 4px;
            background: #ffffff;
        }

        .is-hidden,
        .d-none {
            display: none !important;
        }

        .stack-sm > * + * {
            margin-top: 12px;
        }

        .muted-small {
            color: var(--subtle-text);
            font-size: 0.9rem;
        }

        .card-panel ul {
            margin-bottom: 0;
        }

        @media only screen and (max-width: 992px) {
            .main-content {
                padding: 72px 20px 32px;
            }

            .page-header .page-title {
                font-size: 1.8rem;
            }
        }
    </style>
</head>
<body ' . $bodyAttrs . '>
' . $chrome . '
<main class="main-content">';
}

function jrs_material_page_footer(): void
{
    echo '</main>
<script src="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/js/materialize.min.js"></script>
<script>
document.addEventListener("DOMContentLoaded", function () {
    M.Sidenav.init(document.querySelectorAll(".sidenav"));
    M.FormSelect.init(document.querySelectorAll("select"));
    M.updateTextFields();
});

window.jrsInitMaterialize = function (root) {
    root = root || document;
    if (!window.M) {
        return;
    }
    M.FormSelect.init(root.querySelectorAll("select"));
    M.updateTextFields();
};
</script>
</body>
</html>';
}

function jrs_page_header(string $title): void
{
    $safeTitle = jrs_html($title);
    echo '<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>' . $safeTitle . ' - Jasper Report Services</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css" rel="stylesheet">
    <style>
        body { background-color: #f8f9fa; }
        .app-shell { max-width: 1180px; }
        .report-card-thumb { height: 150px; object-fit: cover; background: #e9ecef; }
        .nav-pills .nav-link { color: #495057; }
    </style>
</head>
<body>
<nav class="navbar navbar-expand-lg bg-white border-bottom mb-4">
    <div class="container app-shell">
        <a class="navbar-brand fw-semibold" href="index.php">Jasper Report Services</a>
        <div class="navbar-nav">
            <a class="nav-link" href="index.php">Reports</a>
            <a class="nav-link" href="datasources.php">Datasources</a>
            <a class="nav-link" href="geoserver_connections.php">GeoServer</a>
            <a class="nav-link" href="publish_report.php">Publish</a>
            <a class="nav-link" href="report_runner.php">URL Builder</a>
        </div>
    </div>
</nav>
<main class="container app-shell pb-5">';
}

function jrs_page_footer(): void
{
    echo '</main>
<script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/js/bootstrap.bundle.min.js"></script>
</body>
</html>';
}
