package com.acugis.reports;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.Base64;
import java.util.Iterator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

/**
 * DuckDB JDBC session initialization for Report Server fill — mirrors AcuGIS Report Designer
 * {@code DuckDBSession}.
 *
 * <p>Always apply on the <em>same</em> {@link Connection} passed to
 * {@code JasperFillManager} ({@code REPORT_CONNECTION}).
 *
 * <p>Local database mode opens {@code jdbc:duckdb:/abs/path}. Remote Data Files mode opens
 * an in-memory DuckDB, loads httpfs, applies HTTP auth secrets, and registers file views.
 * Credentials are never logged.
 */
public final class DuckDBSession {
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final String SECRET_NAME = "acugis_http";
    private static final String DRIVER = "org.duckdb.DuckDBDriver";

    private DuckDBSession() {}

    /**
     * Open a DuckDB JDBC connection and configure session state from {@code _duckdbSession} JSON.
     */
    public static Connection open(String sessionJson) throws Exception {
        if (sessionJson == null || sessionJson.trim().isEmpty()) {
            throw new IllegalArgumentException("_duckdbSession is required");
        }
        JsonNode req = MAPPER.readTree(sessionJson);
        List<String> secrets = new ArrayList<String>();
        collectSecrets(req, secrets);
        try {
            Class.forName(DRIVER);
        } catch (ClassNotFoundException e) {
            throw new IllegalStateException(
                    "DuckDB JDBC driver not found (org.duckdb.DuckDBDriver). "
                            + "Install duckdb_jdbc-*-{platform}.jar into CATALINA_HOME/lib "
                            + "(see stand-alone-tomcat-10/sync-duckdb-jdbc.ps1).",
                    e);
        }
        String jdbcUrl = buildJdbcUrl(req);
        Connection conn = DriverManager.getConnection(jdbcUrl);
        try {
            configure(conn, req, secrets);
            return conn;
        } catch (Exception e) {
            try {
                conn.close();
            } catch (Exception ignored) {
                // ignore close after failed init
            }
            throw redactException(e, secrets);
        }
    }

    /**
     * Configure a DuckDB connection for the given session JSON.
     */
    public static void configure(Connection conn, JsonNode req, List<String> secrets) throws Exception {
        if (conn == null || req == null || req.isNull()) {
            return;
        }
        String mode = text(req, "mode").toLowerCase(Locale.ROOT);
        if (mode.isEmpty()) {
            mode = "database";
        }
        if (!"files".equals(mode)) {
            return;
        }
        loadHttpfs(conn, req);
        configureAuth(conn, req, secrets);
        registerFileViews(conn, req);
    }

    public static void collectSecrets(JsonNode req, List<String> secrets) {
        if (req == null || secrets == null) {
            return;
        }
        String pass = text(req, "password");
        if (!pass.isEmpty()) {
            secrets.add(pass);
            try {
                JsonNode obj = MAPPER.readTree(pass);
                if (obj != null && obj.isObject()) {
                    Iterator<Map.Entry<String, JsonNode>> it = obj.fields();
                    while (it.hasNext()) {
                        Map.Entry<String, JsonNode> e = it.next();
                        if (e.getValue() != null && e.getValue().isTextual()) {
                            String v = e.getValue().asText();
                            if (v != null && !v.isEmpty()) {
                                secrets.add(v);
                            }
                        }
                    }
                }
            } catch (Exception ignored) {
                // not JSON header map
            }
        }
    }

    public static String buildJdbcUrl(JsonNode req) {
        String mode = text(req, "mode").toLowerCase(Locale.ROOT);
        if ("files".equals(mode)) {
            return "jdbc:duckdb:";
        }
        String path = text(req, "databasePath");
        if (path.isEmpty()) {
            throw new IllegalStateException("DuckDB database file is required");
        }
        Path p = Paths.get(path);
        if (!Files.isRegularFile(p)) {
            throw new IllegalStateException("DuckDB database not found: " + p.toAbsolutePath());
        }
        return "jdbc:duckdb:" + p.toAbsolutePath().toString().replace('\\', '/');
    }

    private static void loadHttpfs(Connection conn, JsonNode req) throws Exception {
        String extPath = text(req, "httpfsExtensionPath");
        if (!extPath.isEmpty()) {
            Path p = Paths.get(extPath);
            if (!Files.isRegularFile(p)) {
                throw new IllegalStateException(
                        "httpfs extension not found at " + p.toAbsolutePath());
            }
            String loadSql =
                    "LOAD '" + escapeSql(p.toAbsolutePath().toString().replace('\\', '/')) + "'";
            try (Statement st = conn.createStatement()) {
                st.execute(loadSql);
            }
            return;
        }
        // Server fallback: INSTALL/LOAD into a writable home (same idea as Go duckdb.ensureHTTPFS).
        // Use separate statements — a failed LOAD may close the JDBC Statement.
        ensureHomeDirectory(conn, req);
        try (Statement st = conn.createStatement()) {
            st.execute("LOAD httpfs");
            return;
        } catch (Exception loadFirst) {
            try (Statement st = conn.createStatement()) {
                st.execute("INSTALL httpfs");
            }
            try (Statement st = conn.createStatement()) {
                st.execute("LOAD httpfs");
            }
        }
    }

    private static void ensureHomeDirectory(Connection conn, JsonNode req) throws Exception {
        String home = text(req, "homeDirectory");
        if (home.isEmpty()) {
            String env = System.getenv("ACUGIS_DUCKDB_HOME");
            if (env == null || env.trim().isEmpty()) {
                env = System.getenv("DUCKDB_HOME");
            }
            if (env != null && !env.trim().isEmpty()) {
                home = env.trim();
            } else {
                String userHome = System.getProperty("user.home");
                if (userHome != null && !userHome.trim().isEmpty()) {
                    home = Paths.get(userHome, ".acugis", "duckdb").toString();
                } else {
                    home = Paths.get(System.getProperty("java.io.tmpdir"), "acugis-duckdb").toString();
                }
            }
        }
        Path homePath = Paths.get(home);
        Files.createDirectories(homePath);
        try (Statement st = conn.createStatement()) {
            st.execute(
                    "SET home_directory='"
                            + escapeSql(homePath.toAbsolutePath().toString().replace('\\', '/'))
                            + "'");
        }
    }

    private static void configureAuth(Connection conn, JsonNode req, List<String> secrets)
            throws Exception {
        String authType = text(req, "authType").toLowerCase(Locale.ROOT);
        if (authType.isEmpty()
                || "none".equals(authType)
                || "anonymous".equals(authType)
                || "public".equals(authType)) {
            return;
        }
        String scope = firstScope(req);
        String secretSql;
        if ("bearer".equals(authType) || "token".equals(authType) || "bearer_token".equals(authType)) {
            String token = text(req, "password");
            if (token.isEmpty()) {
                throw new IllegalStateException("Bearer Token is required");
            }
            secrets.add(token);
            secretSql =
                    "CREATE OR REPLACE SECRET "
                            + SECRET_NAME
                            + " (TYPE HTTP, BEARER_TOKEN '"
                            + escapeSql(token)
                            + "'"
                            + (scope.isEmpty() ? "" : ", SCOPE '" + escapeSql(scope) + "'")
                            + ")";
        } else if ("basic".equals(authType) || "basic_auth".equals(authType)) {
            String user = text(req, "username");
            String pass = text(req, "password");
            if (user.isEmpty() || pass.isEmpty()) {
                throw new IllegalStateException("Basic Auth requires a username and password");
            }
            secrets.add(pass);
            String basic =
                    Base64.getEncoder()
                            .encodeToString((user + ":" + pass).getBytes(StandardCharsets.UTF_8));
            secrets.add(basic);
            secretSql =
                    "CREATE OR REPLACE SECRET "
                            + SECRET_NAME
                            + " (TYPE HTTP, EXTRA_HTTP_HEADERS MAP {"
                            + "'Authorization': 'Basic "
                            + basic
                            + "'}"
                            + (scope.isEmpty() ? "" : ", SCOPE '" + escapeSql(scope) + "'")
                            + ")";
        } else if ("headers".equals(authType)
                || "custom".equals(authType)
                || "custom_headers".equals(authType)) {
            Map<String, String> headers = parseHeaderSecrets(req);
            if (headers.isEmpty()) {
                throw new IllegalStateException("at least one custom HTTP header is required");
            }
            StringBuilder map = new StringBuilder();
            boolean first = true;
            List<String> keys = new ArrayList<String>(headers.keySet());
            java.util.Collections.sort(keys);
            for (String k : keys) {
                String v = headers.get(k);
                secrets.add(v);
                if (!first) {
                    map.append(", ");
                }
                first = false;
                map.append("'").append(escapeSql(k)).append("': '").append(escapeSql(v)).append("'");
            }
            secretSql =
                    "CREATE OR REPLACE SECRET "
                            + SECRET_NAME
                            + " (TYPE HTTP, EXTRA_HTTP_HEADERS MAP {"
                            + map
                            + "}"
                            + (scope.isEmpty() ? "" : ", SCOPE '" + escapeSql(scope) + "'")
                            + ")";
        } else {
            throw new IllegalStateException("unsupported DuckDB authType: " + authType);
        }

        try (Statement st = conn.createStatement()) {
            st.execute("DROP SECRET IF EXISTS " + SECRET_NAME);
            st.execute(secretSql);
        }
    }

    private static Map<String, String> parseHeaderSecrets(JsonNode req) {
        Map<String, String> out = new LinkedHashMap<String, String>();
        JsonNode keysNode = req.get("httpHeaderKeys");
        List<String> keys = new ArrayList<String>();
        if (keysNode != null && keysNode.isArray()) {
            for (JsonNode k : keysNode) {
                if (k != null && k.isTextual() && !k.asText().trim().isEmpty()) {
                    keys.add(k.asText().trim());
                }
            }
        }
        String secretJson = text(req, "password");
        Map<String, String> values = new LinkedHashMap<String, String>();
        if (!secretJson.isEmpty()) {
            try {
                JsonNode obj = MAPPER.readTree(secretJson);
                if (obj != null && obj.isObject()) {
                    Iterator<Map.Entry<String, JsonNode>> it = obj.fields();
                    while (it.hasNext()) {
                        Map.Entry<String, JsonNode> e = it.next();
                        if (e.getValue() != null && e.getValue().isTextual()) {
                            values.put(e.getKey(), e.getValue().asText());
                        }
                    }
                }
            } catch (Exception ignored) {
                // not JSON
            }
        }
        for (String key : keys) {
            String v = values.get(key);
            if (v == null) {
                for (Map.Entry<String, String> e : values.entrySet()) {
                    if (e.getKey().equalsIgnoreCase(key)) {
                        v = e.getValue();
                        break;
                    }
                }
            }
            if (v == null || v.trim().isEmpty()) {
                throw new IllegalStateException("missing secret value for header(s): " + key);
            }
            out.put(key, v);
        }
        return out;
    }

    private static void registerFileViews(Connection conn, JsonNode req) throws Exception {
        JsonNode files = req.get("files");
        if (files == null || !files.isArray() || files.size() == 0) {
            throw new IllegalStateException("remote source URL is required");
        }
        for (JsonNode f : files) {
            String alias = text(f, "alias");
            String source = text(f, "source");
            if (source.isEmpty()) {
                source = text(f, "path");
            }
            String format = text(f, "format");
            if (alias.isEmpty() || source.isEmpty()) {
                continue;
            }
            if (!source.toLowerCase(Locale.ROOT).startsWith("http://")
                    && !source.toLowerCase(Locale.ROOT).startsWith("https://")) {
                throw new IllegalStateException(
                        "Data Files requires an HTTP or HTTPS URL (local paths are not supported in this phase)");
            }
            String reader = readerFor(source, format);
            String sql =
                    "CREATE OR REPLACE VIEW "
                            + quoteIdent(alias)
                            + " AS SELECT * FROM "
                            + reader
                            + "('"
                            + escapeSql(source)
                            + "')";
            try (Statement st = conn.createStatement()) {
                st.execute(sql);
            }
        }
    }

    private static String firstScope(JsonNode req) {
        JsonNode files = req.get("files");
        if (files == null || !files.isArray() || files.size() == 0) {
            return "";
        }
        String source = text(files.get(0), "source");
        if (source.isEmpty()) {
            source = text(files.get(0), "path");
        }
        try {
            java.net.URI u = java.net.URI.create(source);
            if (u.getScheme() == null || u.getAuthority() == null) {
                return "";
            }
            return u.getScheme().toLowerCase(Locale.ROOT) + "://" + u.getAuthority() + "/";
        } catch (Exception e) {
            return "";
        }
    }

    static String readerFor(String source, String format) {
        String resolved = resolvedFormat(source, format);
        if ("csv".equals(resolved)) {
            return "read_csv_auto";
        }
        if ("json".equals(resolved)) {
            return "read_json_auto";
        }
        return "read_parquet";
    }

    static String resolvedFormat(String source, String format) {
        String f = format == null ? "" : format.trim().toLowerCase(Locale.ROOT);
        if (f.isEmpty() || "auto".equals(f) || "detect".equals(f) || "autodetect".equals(f)) {
            String lower = source.toLowerCase(Locale.ROOT);
            int q = lower.indexOf('?');
            if (q >= 0) {
                lower = lower.substring(0, q);
            }
            if (lower.endsWith(".csv") || lower.endsWith(".tsv") || lower.endsWith(".txt")) {
                return "csv";
            }
            if (lower.endsWith(".json") || lower.endsWith(".jsonl") || lower.endsWith(".ndjson")) {
                return "json";
            }
            return "parquet";
        }
        if ("parq".equals(f) || "pq".equals(f)) {
            return "parquet";
        }
        return f;
    }

    static String quoteIdent(String name) {
        return "\"" + name.replace("\"", "\"\"") + "\"";
    }

    static String escapeSql(String s) {
        return s.replace("'", "''");
    }

    static String text(JsonNode n, String field) {
        if (n == null) {
            return "";
        }
        JsonNode v = n.get(field);
        if (v == null || v.isNull()) {
            return "";
        }
        return v.asText("").trim();
    }

    private static Exception redactException(Exception e, List<String> secrets) {
        if (e == null) {
            return null;
        }
        String msg = e.getMessage();
        if (msg == null || secrets == null || secrets.isEmpty()) {
            return e;
        }
        String out = msg;
        for (String secret : secrets) {
            if (secret != null && secret.length() > 0) {
                out = out.replace(secret, "***");
            }
        }
        if (out.equals(msg)) {
            return e;
        }
        return new IllegalStateException(out, e);
    }
}
