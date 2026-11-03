package com.acugis.reports;

import java.io.File;
import java.util.logging.Logger;

import jakarta.servlet.ServletContext;
import jakarta.servlet.ServletContextEvent;
import jakarta.servlet.ServletContextListener;
import jakarta.servlet.annotation.WebListener;
import net.sf.jasperreports.engine.JasperCompileManager;

/**
 * Startup checks and summary for standalone Tomcat deployments.
 */
@WebListener
public class StartupVerifier implements ServletContextListener {

    private static final Logger LOG = Logger.getLogger(StartupVerifier.class.getName());

    @Override
    public void contextInitialized(ServletContextEvent sce) {
        ServletContext ctx = sce.getServletContext();
        AcugisConfig cfg = AcugisConfig.init(ctx.getClassLoader());

        StringBuilder summary = new StringBuilder();
        summary.append('\n');
        summary.append("======== AcuGIS Report Server startup ========\n");
        summary.append("contextPath         : ").append(ctx.getContextPath() == null || ctx.getContextPath().isEmpty()
                ? "/" : ctx.getContextPath()).append('\n');
        summary.append("catalina.base       : ").append(CatalinaPaths.catalinaBase()).append('\n');
        summary.append("catalina.home       : ").append(CatalinaPaths.catalinaHome()).append('\n');

        File reportsDir = ensureDir(cfg.reportsDir(), true);
        File uploadsDir = ensureDir(cfg.uploadsDir(), true);
        File tempDir = ensureDir(cfg.tempDir(), true);
        File fontsDir = ensureDir(cfg.fontsDir(), true);
        File driversDir = ensureDir(cfg.driversDir(), true);
        File licensesDir = ensureDir(cfg.licensesDir(), true);
        File logsDir = ensureDir(cfg.logsDir(), false);

        summary.append("reports.dir         : ").append(describeWritable(reportsDir)).append('\n');
        summary.append("uploads.dir         : ").append(describeExists(uploadsDir)).append('\n');
        summary.append("temp.dir            : ").append(describeWritable(tempDir)).append('\n');
        summary.append("fonts.dir           : ").append(describeExists(fontsDir)).append('\n');
        summary.append("drivers.dir         : ").append(describeExists(driversDir)).append('\n');
        summary.append("licenses.dir        : ").append(describeExists(licensesDir)).append('\n');
        summary.append("logs.dir            : ").append(describeExists(logsDir)).append('\n');

        // Before any JRXML compile. JR 7 will not load language="collection"
        // until this factory is on the JasperReportsContext.
        CollectionQueryExecuterFactory.register();
        FontSupport.Result fonts = FontSupport.configure();
        CompilerSupport.configure();
        summary.append("fonts               : ").append(fonts.message).append('\n');
        if (fonts.bundledFamilies != null && !fonts.bundledFamilies.isEmpty()) {
            summary.append("bundled font families: ").append(String.join(", ", fonts.bundledFamilies)).append('\n');
        }

        File webInfLib = new File(ctx.getRealPath("/WEB-INF/lib"));
        JdbcDriverScanner.Report jdbc = JdbcDriverScanner.scan(webInfLib.isDirectory() ? webInfLib : null);
        summary.append("jdbc drivers found  : ").append(jdbc.foundVendors.size()).append(" vendor(s)\n");
        if (jdbc.findings.isEmpty()) {
            summary.append("  (none detected in WEB-INF/lib, tomcat/lib, or drivers.dir)\n");
        } else {
            for (JdbcDriverScanner.Finding f : jdbc.findings) {
                summary.append("  - ").append(f.vendor.label)
                        .append(" jar=").append(f.jarName)
                        .append(" loc=").append(f.location)
                        .append(" classLoadable=").append(f.classLoadable)
                        .append('\n');
            }
        }

        File webInfLicense = null;
        try {
            String licensePath = ctx.getRealPath("/WEB-INF/license");
            if (licensePath != null) {
                webInfLicense = new File(licensePath);
            }
        } catch (Exception ignored) {
            // exploded vs packed WAR
        }
        LicenseResolver.Result license = LicenseResolver.resolve(webInfLicense);
        summary.append("license file        : ").append(license.status)
                .append(" — ").append(license.message)
                .append(" (not product entitlement)");
        if (license.file != null) {
            summary.append(" (").append(license.file.getAbsolutePath()).append(')');
        }
        summary.append('\n');

        boolean compilerOk = false;
        String compilerLoc = "(unavailable)";
        try {
            compilerLoc = String.valueOf(JasperCompileManager.class.getProtectionDomain()
                    .getCodeSource().getLocation());
            compilerOk = true;
        } catch (Throwable t) {
            compilerLoc = t.getMessage();
        }
        summary.append("report compiler     : ").append(compilerOk ? "OK" : "MISSING")
                .append(" — ").append(compilerLoc).append('\n');

        // Expression Builder GIS helpers + MapRenderer live in acugis-report-map.jar (map-lib).
        summary.append("map expression API  : ").append(describeClass(
                "com.acugis.map.AcuGISFunctions",
                "map renderer jar is missing from the WAR (Expression Builder GIS)")).append('\n');
        summary.append("map renderer API    : ").append(describeClass(
                "com.acugis.map.MapRenderer",
                "map renderer jar is missing from the WAR (embedded maps)")).append('\n');
        // Designer barcode4j:QRCode / Code128 — Jackson registers type ids via this extension jar.
        summary.append("barcode4j API       : ").append(describeClass(
                "net.sf.jasperreports.barcode4j.QRCodeComponent",
                "rebuild WAR after sync-barcode-lib.ps1 (QR/Barcode)")).append('\n');
        summary.append("==============================================");

        String text = summary.toString();
        LOG.info(text);
        ctx.log(text);
        System.out.println(text);

        ctx.setAttribute("acugis.config", cfg);
        ctx.setAttribute("acugis.license", license);
        ctx.setAttribute("acugis.jdbc", jdbc);
        ctx.setAttribute("acugis.fonts", fonts);
    }

    @Override
    public void contextDestroyed(ServletContextEvent sce) {
        // no-op
    }

    private static File ensureDir(File dir, boolean create) {
        if (dir == null) {
            return null;
        }
        if (create && !dir.exists()) {
            //noinspection ResultOfMethodCallIgnored
            dir.mkdirs();
        }
        return dir;
    }

    private static String describeWritable(File dir) {
        if (dir == null) {
            return "(null)";
        }
        boolean exists = dir.exists();
        boolean writable = exists && dir.canWrite();
        if (!exists) {
            return dir.getAbsolutePath() + " [MISSING]";
        }
        return dir.getAbsolutePath() + (writable ? " [writable]" : " [NOT WRITABLE]");
    }

    private static String describeExists(File dir) {
        if (dir == null) {
            return "(null)";
        }
        return dir.getAbsolutePath() + (dir.exists() ? " [ok]" : " [MISSING]");
    }

    /** Classpath probe for optional map-lib APIs (MapRenderer / AcuGISFunctions). */
    private static String describeClass(String className, String missingHint) {
        try {
            Class<?> cls = Class.forName(className);
            String loc = "(unknown location)";
            try {
                loc = String.valueOf(cls.getProtectionDomain().getCodeSource().getLocation());
            } catch (Throwable ignored) {
                // location optional
            }
            return "OK — " + loc;
        } catch (Throwable t) {
            String detail = t.getMessage() != null ? t.getMessage() : t.getClass().getSimpleName();
            LOG.warning(className + " not on classpath (" + detail + "). " + missingHint);
            return "MISSING — " + missingHint;
        }
    }
}
