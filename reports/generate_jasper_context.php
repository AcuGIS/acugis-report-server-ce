<?php
declare(strict_types=1);

require_once __DIR__ . '/../lib/bootstrap.php';

$cconn = jrs_auth_require_central_db();
jrs_require_login($cconn);
jrs_require_service_access($cconn, 'jasper_reports', 'edit');

function xml_attr(string $value): string
{
    return htmlspecialchars($value, ENT_QUOTES | ENT_XML1, 'UTF-8');
}

$datasources = jrs_datasources();
if ($datasources === []) {
    http_response_code(500);
    echo 'No datasources found. Add rows to public.jasper_datasource first.' . PHP_EOL;
    exit(1);
}

$resources = [];
foreach ($datasources as $row) {
    $name = trim((string) ($row['name'] ?? ''));
    $username = (string) ($row['username'] ?? '');
    $password = (string) ($row['password'] ?? '');
    $jdbc = jrs_jdbc_settings_for_datasource($row);

    if ($name === '' || $jdbc === null) {
        continue;
    }

    $resources[] =
        '  <Resource name="jdbc/' . xml_attr($name) . '"' . PHP_EOL .
        '            auth="Container"' . PHP_EOL .
        '            type="javax.sql.DataSource"' . PHP_EOL .
        '            factory="org.apache.tomcat.jdbc.pool.DataSourceFactory"' . PHP_EOL .
        '            driverClassName="' . xml_attr($jdbc['driverClassName']) . '"' . PHP_EOL .
        '            url="' . xml_attr($jdbc['url']) . '"' . PHP_EOL .
        '            username="' . xml_attr($username) . '"' . PHP_EOL .
        '            password="' . xml_attr($password) . '"' . PHP_EOL .
        '            initialSize="0"' . PHP_EOL .
        '            maxTotal="20"' . PHP_EOL .
        '            maxIdle="10"' . PHP_EOL .
        '            maxWaitMillis="10000"' . PHP_EOL .
        '            validationQuery="' . xml_attr($jdbc['validationQuery']) . '"' . PHP_EOL .
        '            testOnBorrow="true"/>';
}

$xml = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Context>\n";
if ($resources !== []) {
    $xml .= implode("\n", $resources) . "\n";
}
$xml .= "</Context>\n";

$outputPath = (string) jrs_config('jasper_context_path', jrs_base_dir() . '/docker/report-server/context.xml');
$outputDirectory = dirname($outputPath);
if (!jrs_ensure_dir($outputDirectory) || !is_writable($outputDirectory)) {
    http_response_code(500);
    echo 'Output directory is not writable: ' . $outputDirectory . PHP_EOL;
    exit(1);
}

if (file_put_contents($outputPath, $xml) === false) {
    http_response_code(500);
    echo 'Failed to write: ' . $outputPath . PHP_EOL;
    exit(1);
}

echo 'Context XML generated at ' . $outputPath . PHP_EOL;
