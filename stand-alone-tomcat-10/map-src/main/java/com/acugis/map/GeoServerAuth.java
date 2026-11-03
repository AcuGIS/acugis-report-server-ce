package com.acugis.map;

import java.io.File;
import java.io.FileInputStream;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import java.util.Properties;

/**
 * GeoServer Basic-auth credentials for Preview and Report Server fill.
 *
 * <p>Resolution order for {@link #get(String)}:
 * <ol>
 *   <li>In-memory map (Designer PreviewRunner injection)</li>
 *   <li>Properties file from Report Server admin (keyed by stable connection <em>name</em>)</li>
 * </ol>
 *
 * <p>Empty / missing key → anonymous WMS. Never serialize credentials into JRXML.
 *
 * <p>File format ({@code geoserver-connections.properties}):
 * <pre>
 * MyGS.url=https://gs.example.com/geoserver
 * MyGS.username=admin
 * MyGS.password=secret
 * </pre>
 *
 * <p>Path via system property {@code acugis.geoserver.connections.file}, env
 * {@code ACUGIS_GEOSERVER_CONNECTIONS_FILE}, or Tomcat {@code conf/geoserver-connections.properties}.
 */
public final class GeoServerAuth {

    public static final String PROP_CONNECTIONS_FILE = "acugis.geoserver.connections.file";
    public static final String ENV_CONNECTIONS_FILE = "ACUGIS_GEOSERVER_CONNECTIONS_FILE";

    public static final class Credentials {
        public final String username;
        public final String password;
        /** Optional base URL from the server connection store (may be empty). */
        public final String baseUrl;

        public Credentials(String username, String password) {
            this(username, password, "");
        }

        public Credentials(String username, String password, String baseUrl) {
            this.username = username == null ? "" : username;
            this.password = password == null ? "" : password;
            this.baseUrl = baseUrl == null ? "" : baseUrl.trim();
        }

        public boolean hasUser() {
            return username != null && !username.trim().isEmpty();
        }
    }

    private static final Map<String, Credentials> BY_CONNECTION =
            Collections.synchronizedMap(new HashMap<String, Credentials>());

    private static volatile String connectionsFilePath = "";
    private static volatile long fileLoadedMtime = -1L;
    private static final Map<String, Credentials> FROM_FILE =
            Collections.synchronizedMap(new HashMap<String, Credentials>());

    private GeoServerAuth() {}

    public static void clear() {
        BY_CONNECTION.clear();
    }

    /**
     * Override the connections properties path (Report Server / tests).
     * Pass empty to clear override and use env / defaults.
     */
    public static void setConnectionsFile(String path) {
        connectionsFilePath = path == null ? "" : path.trim();
        fileLoadedMtime = -1L;
        FROM_FILE.clear();
    }

    public static void put(String connectionKey, String username, String password) {
        put(connectionKey, username, password, "");
    }

    public static void put(String connectionKey, String username, String password, String baseUrl) {
        if (connectionKey == null || connectionKey.trim().isEmpty()) {
            return;
        }
        BY_CONNECTION.put(connectionKey.trim(), new Credentials(username, password, baseUrl));
    }

    /**
     * Lookup by stable connection name (Report Server) or Designer preview key.
     * Returns null when anonymous / unknown.
     */
    public static Credentials get(String connectionKey) {
        if (connectionKey == null || connectionKey.trim().isEmpty()) {
            return null;
        }
        String key = connectionKey.trim();
        Credentials mem = BY_CONNECTION.get(key);
        if (mem != null) {
            return mem;
        }
        reloadFileIfNeeded();
        return FROM_FILE.get(key);
    }

    private static void reloadFileIfNeeded() {
        String path = resolveConnectionsFile();
        if (path.isEmpty()) {
            return;
        }
        File f = new File(path);
        if (!f.isFile()) {
            return;
        }
        long mtime = f.lastModified();
        if (mtime == fileLoadedMtime && !FROM_FILE.isEmpty()) {
            return;
        }
        synchronized (FROM_FILE) {
            if (mtime == fileLoadedMtime && !FROM_FILE.isEmpty()) {
                return;
            }
            Map<String, Credentials> next = loadPropertiesFile(f);
            FROM_FILE.clear();
            FROM_FILE.putAll(next);
            fileLoadedMtime = mtime;
        }
    }

    public static String resolveConnectionsFile() {
        if (connectionsFilePath != null && !connectionsFilePath.isEmpty()) {
            return connectionsFilePath;
        }
        String prop = System.getProperty(PROP_CONNECTIONS_FILE, "").trim();
        if (!prop.isEmpty()) {
            return prop;
        }
        String env = System.getenv(ENV_CONNECTIONS_FILE);
        if (env != null && !env.trim().isEmpty()) {
            return env.trim();
        }
        String catalina = System.getProperty("catalina.base", "");
        if (catalina != null && !catalina.isEmpty()) {
            File conf = new File(new File(catalina, "conf"), "geoserver-connections.properties");
            if (conf.isFile()) {
                return conf.getAbsolutePath();
            }
        }
        File local = new File("geoserver-connections.properties");
        if (local.isFile()) {
            return local.getAbsolutePath();
        }
        return "";
    }

    static Map<String, Credentials> loadPropertiesFile(File f) {
        Map<String, Credentials> out = new HashMap<String, Credentials>();
        Properties props = new Properties();
        try (InputStream in = new FileInputStream(f);
                InputStreamReader reader = new InputStreamReader(in, StandardCharsets.UTF_8)) {
            props.load(reader);
        } catch (Exception e) {
            System.err.println("GeoServerAuth: failed to load " + f + ": " + e.getMessage());
            return out;
        }
        // Collect connection names from *.url / *.username / *.password keys.
        Map<String, String> urls = new HashMap<String, String>();
        Map<String, String> users = new HashMap<String, String>();
        Map<String, String> passes = new HashMap<String, String>();
        for (String rawKey : props.stringPropertyNames()) {
            if (rawKey == null) {
                continue;
            }
            String key = rawKey.trim();
            String name = null;
            String field = null;
            if (key.endsWith(".url")) {
                name = key.substring(0, key.length() - 4).trim();
                field = "url";
            } else if (key.endsWith(".baseurl")) {
                name = key.substring(0, key.length() - 8).trim();
                field = "url";
            } else if (key.endsWith(".username")) {
                name = key.substring(0, key.length() - 9).trim();
                field = "username";
            } else if (key.endsWith(".user")) {
                name = key.substring(0, key.length() - 5).trim();
                field = "username";
            } else if (key.endsWith(".password")) {
                name = key.substring(0, key.length() - 9).trim();
                field = "password";
            } else if (key.endsWith(".pass")) {
                name = key.substring(0, key.length() - 5).trim();
                field = "password";
            }
            if (name == null || name.isEmpty() || field == null) {
                continue;
            }
            String val = props.getProperty(rawKey, "");
            if ("url".equals(field)) {
                urls.put(name, val == null ? "" : val.trim());
            } else if ("username".equals(field)) {
                users.put(name, val == null ? "" : val.trim());
            } else if ("password".equals(field)) {
                passes.put(name, val == null ? "" : val);
            }
        }
        for (String name : urls.keySet()) {
            out.put(name, new Credentials(users.get(name), passes.get(name), urls.get(name)));
        }
        for (String name : users.keySet()) {
            if (!out.containsKey(name)) {
                out.put(name, new Credentials(users.get(name), passes.get(name), urls.get(name)));
            }
        }
        return out;
    }
}
