package com.acugis.reports;

import java.io.File;
import java.io.FileInputStream;
import java.io.InputStream;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Properties;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * Configuration loader with priority: environment → CATALINA_BASE/conf/acugis.properties
 * → WEB-INF/classes/application.properties → built-in defaults.
 */
public final class AcugisConfig {

    private static final Logger LOG = Logger.getLogger(AcugisConfig.class.getName());
    private static final Object LOCK = new Object();
    private static volatile AcugisConfig INSTANCE;

    private final Properties properties = new Properties();
    private final Map<String, String> resolved = new LinkedHashMap<>();

    private AcugisConfig() {
    }

    public static AcugisConfig get() {
        AcugisConfig local = INSTANCE;
        if (local == null) {
            synchronized (LOCK) {
                local = INSTANCE;
                if (local == null) {
                    local = new AcugisConfig();
                    local.loadDefaults();
                    INSTANCE = local;
                }
            }
        }
        return local;
    }

    /**
     * Reload after ServletContext is available so classpath application.properties can be read.
     */
    public static AcugisConfig init(ClassLoader classLoader) {
        synchronized (LOCK) {
            AcugisConfig cfg = new AcugisConfig();
            cfg.loadDefaults();
            cfg.loadClasspath(classLoader);
            cfg.loadExternalFile();
            cfg.applyEnvironmentOverrides();
            cfg.resolveDirectories();
            INSTANCE = cfg;
            return cfg;
        }
    }

    public String get(String key) {
        return resolved.containsKey(key) ? resolved.get(key) : properties.getProperty(key);
    }

    public String get(String key, String defaultValue) {
        String value = get(key);
        return value == null || value.trim().isEmpty() ? defaultValue : value.trim();
    }

    public File getDir(String key) {
        String value = get(key);
        if (value == null || value.trim().isEmpty()) {
            return null;
        }
        return new File(expand(value.trim()));
    }

    public Map<String, String> snapshot() {
        return Collections.unmodifiableMap(new LinkedHashMap<>(resolved));
    }

    public File reportsDir() {
        return requireDir("reports.dir");
    }

    public File uploadsDir() {
        return requireDir("uploads.dir");
    }

    public File logsDir() {
        return requireDir("logs.dir");
    }

    public File tempDir() {
        return requireDir("temp.dir");
    }

    public File fontsDir() {
        return requireDir("fonts.dir");
    }

    public File driversDir() {
        return requireDir("drivers.dir");
    }

    public File licensesDir() {
        return requireDir("licenses.dir");
    }

    public String htmlImageUriPattern() {
        // {contextPath} and {uuid} are substituted at export time.
        return get("html.image.uri.pattern", "{contextPath}/acugis_report_image?image={0}&uuid={uuid}");
    }

    public File resourcePathsDir() {
        File dir = getDir("resource.paths.dir");
        if (dir == null) {
            return new File(CatalinaPaths.acugisRoot(), "resource-paths");
        }
        return dir;
    }

    private File requireDir(String key) {
        File dir = getDir(key);
        if (dir == null) {
            throw new IllegalStateException("Missing configuration for " + key);
        }
        return dir;
    }

    private void loadDefaults() {
        File acugis = CatalinaPaths.acugisRoot();
        putDefault("reports.dir", new File(acugis, "reports").getAbsolutePath());
        putDefault("uploads.dir", new File(acugis, "uploads").getAbsolutePath());
        putDefault("logs.dir", CatalinaPaths.logsDir().getAbsolutePath());
        putDefault("temp.dir", CatalinaPaths.tempDir().getAbsolutePath());
        putDefault("fonts.dir", new File(acugis, "fonts").getAbsolutePath());
        putDefault("drivers.dir", new File(acugis, "drivers").getAbsolutePath());
        putDefault("licenses.dir", new File(acugis, "licenses").getAbsolutePath());
        putDefault("resource.paths.dir", new File(acugis, "resource-paths").getAbsolutePath());
        putDefault("html.image.uri.pattern", "{contextPath}/acugis_report_image?image={0}&uuid={uuid}");
        putDefault("license.file", "");
        putDefault("ignore.missing.font", "true");
        putDefault("perf.debug", "false");
    }

    private void putDefault(String key, String value) {
        properties.setProperty(key, value);
        resolved.put(key, value);
    }

    private void loadClasspath(ClassLoader classLoader) {
        if (classLoader == null) {
            classLoader = Thread.currentThread().getContextClassLoader();
        }
        if (classLoader == null) {
            return;
        }
        try (InputStream in = classLoader.getResourceAsStream("application.properties")) {
            if (in != null) {
                Properties p = new Properties();
                p.load(in);
                merge(p);
                LOG.info("Loaded classpath application.properties");
            }
        } catch (Exception e) {
            LOG.log(Level.WARNING, "Failed to load application.properties from classpath", e);
        }
    }

    private void loadExternalFile() {
        File external = new File(CatalinaPaths.confDir(), "acugis.properties");
        if (!external.isFile()) {
            return;
        }
        try (InputStream in = new FileInputStream(external)) {
            Properties p = new Properties();
            p.load(in);
            merge(p);
            LOG.info("Loaded external config: " + external.getAbsolutePath());
        } catch (Exception e) {
            LOG.log(Level.WARNING, "Failed to load " + external.getAbsolutePath(), e);
        }
    }

    private void applyEnvironmentOverrides() {
        overlayEnv("REPORTS_DIR", "reports.dir");
        overlayEnv("ACUGIS_REPORTS_DIR", "reports.dir");
        overlayEnv("UPLOADS_DIR", "uploads.dir");
        overlayEnv("ACUGIS_UPLOADS_DIR", "uploads.dir");
        overlayEnv("LOGS_DIR", "logs.dir");
        overlayEnv("ACUGIS_LOGS_DIR", "logs.dir");
        overlayEnv("TEMP_DIR", "temp.dir");
        overlayEnv("ACUGIS_TEMP_DIR", "temp.dir");
        overlayEnv("FONTS_DIR", "fonts.dir");
        overlayEnv("ACUGIS_FONTS_DIR", "fonts.dir");
        overlayEnv("DRIVERS_DIR", "drivers.dir");
        overlayEnv("ACUGIS_DRIVERS_DIR", "drivers.dir");
        overlayEnv("LICENSES_DIR", "licenses.dir");
        overlayEnv("ACUGIS_LICENSES_DIR", "licenses.dir");
        overlayEnv("RESOURCE_PATHS_DIR", "resource.paths.dir");
        overlayEnv("ACUGIS_RESOURCE_PATHS_DIR", "resource.paths.dir");
        overlayEnv("JRS_RESOURCE_PATHS_DIR", "resource.paths.dir");
        overlayEnv("ACUGIS_LICENSE_FILE", "license.file");
        overlayEnv("LICENSE_FILE", "license.file");
        overlayEnv("ACUGIS_HTML_IMAGE_URI_PATTERN", "html.image.uri.pattern");
        overlayEnv("ACUGIS_PERF_DEBUG", "perf.debug");
        overlayEnv("JRS_PERF_DEBUG", "perf.debug");

        // Generic: ACUGIS_REPORTS_DIR style already covered; also map dotted sysprops.
        for (String key : new String[] {
                "reports.dir", "uploads.dir", "logs.dir", "temp.dir",
                "fonts.dir", "drivers.dir", "licenses.dir", "resource.paths.dir",
                "license.file",
                "html.image.uri.pattern", "ignore.missing.font", "perf.debug"
        }) {
            String sys = System.getProperty(key);
            if (sys != null && !sys.trim().isEmpty()) {
                resolved.put(key, sys.trim());
                properties.setProperty(key, sys.trim());
            }
            String envKey = "ACUGIS_" + key.toUpperCase().replace('.', '_');
            overlayEnv(envKey, key);
        }
    }

    private void overlayEnv(String envName, String propertyKey) {
        String value = System.getenv(envName);
        if (value != null && !value.trim().isEmpty()) {
            resolved.put(propertyKey, value.trim());
            properties.setProperty(propertyKey, value.trim());
        }
    }

    private void merge(Properties incoming) {
        for (String name : incoming.stringPropertyNames()) {
            String value = incoming.getProperty(name);
            if (value != null) {
                properties.setProperty(name, value);
                resolved.put(name, value);
            }
        }
    }

    private void resolveDirectories() {
        for (String key : new String[] {
                "reports.dir", "uploads.dir", "logs.dir", "temp.dir",
                "fonts.dir", "drivers.dir", "licenses.dir", "resource.paths.dir"
        }) {
            String raw = get(key, "");
            if (!raw.isEmpty()) {
                String expanded = expand(raw);
                resolved.put(key, new File(expanded).getAbsolutePath());
                properties.setProperty(key, resolved.get(key));
            }
        }
    }

    static String expand(String value) {
        if (value == null) {
            return null;
        }
        String out = value;
        out = out.replace("${catalina.base}", CatalinaPaths.catalinaBase());
        out = out.replace("${catalina.home}", CatalinaPaths.catalinaHome());
        out = out.replace("${java.io.tmpdir}",
                System.getProperty("java.io.tmpdir", CatalinaPaths.tempDir().getAbsolutePath()));
        return out;
    }
}
