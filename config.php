<?php
declare(strict_types=1);

return [
    'db' => [
        'host' => getenv('JRS_DB_HOST') ?: 'localhost',
        'port' => (int) (getenv('JRS_DB_PORT') ?: 5432),
        'name' => getenv('JRS_DB_NAME') ?: 'jasper_reports',
        'user' => getenv('JRS_DB_USER') ?: 'postgres',
        // No baked-in password. Runtime must set JRS_DB_PASS.
        'password' => getenv('JRS_DB_PASS') ?: '',
    ],
    'app_schema' => getenv('JRS_DB_SCHEMA') ?: 'public',
    'product_name' => 'AcuGIS Report Server',
    'app_mount' => '/acugis-report-server',
    /** Browser login page (local stand-alone auth). Env: JRS_LOGIN_URL */
    'login_url' => getenv('JRS_LOGIN_URL') ?: '/acugis-report-server/login.php',
    /** Relative to reports/ when using the bundled viewer; proxied through PHP for session checks. */
    'report_endpoint' => getenv('JRS_REPORT_ENDPOINT') ?: 'execute_report.php',
    'jasper_context_path' => getenv('JRS_JASPER_CONTEXT_PATH') ?: __DIR__ . '/docker/report-server/context.xml',
    /**
     * Properties file read by Java GeoServerAuth at fill time (connection name → url/user/pass).
     * Regenerate from reports/geoserver_connections.php after CRUD changes.
     * Env: JRS_GEOSERVER_CONNECTIONS_PATH
     */
    'geoserver_connections_path' => getenv('JRS_GEOSERVER_CONNECTIONS_PATH')
        ?: __DIR__ . '/docker/report-server/geoserver-connections.properties',
    /**
     * Properties file read by Java ArcGISAuth at fill time (connection name → url/user/pass).
     * Regenerate from reports/arcgis_connections.php after CRUD changes.
     * Env: JRS_ARCGIS_CONNECTIONS_PATH
     */
    'arcgis_connections_path' => getenv('JRS_ARCGIS_CONNECTIONS_PATH')
        ?: __DIR__ . '/docker/report-server/arcgis-connections.properties',
    /** Internal Jasper HTTP endpoint used only after server-side authorization (not exposed to browsers). */
    'jasper_internal_report_url' => getenv('JRS_JASPER_INTERNAL_REPORT_URL') ?: 'http://127.0.0.1:9099/report',
    'jasper_internal_image_url' => getenv('JRS_JASPER_INTERNAL_IMAGE_URL') ?: 'http://127.0.0.1:9099/acugis_report_image',
    /**
     * Optional Tomcat JRXML compile endpoint (CompileServlet). When empty or unavailable,
     * report_compiler.php falls back to jasperstarter (same as publish_report.php).
     * Env: JRS_JASPER_INTERNAL_COMPILE_URL
     */
    'jasper_internal_compile_url' => getenv('JRS_JASPER_INTERNAL_COMPILE_URL') ?: '',
    /**
     * Shared secret for /internal/compile (Authorization: Bearer … / X-JRS-Compile-Token).
     * Env: JRS_COMPILE_TOKEN
     */
    'jasper_compile_token' => getenv('JRS_COMPILE_TOKEN') ?: '',
    /**
     * JRXML / compiled .jasper directory. Empty → CATALINA_BASE/acugis/reports when
     * that directory exists, else {install_root}/docker/report-server/reports.
     * Native DEB: /opt/tomcat/acugis/reports (see /etc/acugis-report-server/env).
     * Env: JRS_JRXML_DIR
     */
    'jrxml_dir' => getenv('JRS_JRXML_DIR') ?: '',
    /**
     * Path Tomcat uses inside P_MAP_IMAGE. Empty → same absolute path as jrxml_dir
     * (native / shared filesystem). Set only when the Tomcat mount differs from the
     * host JRXML dir (Docker bind mounts).
     * Env: JRS_JASPER_REPORTS_CONTAINER_DIR
     */
    'jasper_reports_container_dir' => getenv('JRS_JASPER_REPORTS_CONTAINER_DIR') ?: '',
    /** Install root (default: this service directory). Env: JRS_JASPER_HOME */
    'jasper_home' => getenv('JRS_JASPER_HOME') ?: '',
    /** Per-schedule env files and run output. Env: JRS_JASPER_SCHEDULE_DIR */
    'jasper_schedule_dir' => getenv('JRS_JASPER_SCHEDULE_DIR') ?: '',
    /** HTML email templates directory (legacy email_tmpl). Env: JRS_JASPER_EMAIL_TMPL_DIR */
    'jasper_email_tmpl_dir' => getenv('JRS_JASPER_EMAIL_TMPL_DIR') ?: '',
    /** systemctl binary path. Env: JRS_JASPER_SYSTEMCTL */
    'jasper_systemctl' => getenv('JRS_JASPER_SYSTEMCTL') ?: '/bin/systemctl',
    /**
     * Prefix ln/rm/systemctl with sudo -n when unit files are not writable by www-data.
     * Env: JRS_JASPER_SYSTEMD_USE_SUDO (empty/unset => true; 0/false/no/off => false).
     */
    'jasper_systemd_use_sudo' => (static function (): bool {
        $raw = getenv('JRS_JASPER_SYSTEMD_USE_SUDO');
        if ($raw === false || trim((string) $raw) === '') {
            return true;
        }
        return !in_array(strtolower(trim((string) $raw)), ['0', 'false', 'no', 'off'], true);
    })(),
    /** systemd unit directory (writable by PHP or via sudo). Env: JRS_JASPER_SYSTEMD_UNIT_DIR */
    'jasper_systemd_unit_dir' => getenv('JRS_JASPER_SYSTEMD_UNIT_DIR') ?: '/opt/acugis-report-server/systemd',
    /** systemd unit load path (symlinks created here). Env: JRS_JASPER_SYSTEMD_SYSTEM_DIR */
    'jasper_systemd_system_dir' => getenv('JRS_JASPER_SYSTEMD_SYSTEM_DIR') ?: '/etc/systemd/system',
    /** Optional SMTP for scheduled report email. Env: JRS_SMTP_HOST, JRS_SMTP_PORT, JRS_SMTP_USER, JRS_SMTP_PASS, JRS_SMTP_FROM */
    'smtp_host' => getenv('JRS_SMTP_HOST') ?: '',
    'smtp_port' => (int) (getenv('JRS_SMTP_PORT') ?: 587),
    'smtp_user' => getenv('JRS_SMTP_USER') ?: '',
    'smtp_pass' => getenv('JRS_SMTP_PASS') ?: '',
    'smtp_from' => getenv('JRS_SMTP_FROM') ?: '',
];
