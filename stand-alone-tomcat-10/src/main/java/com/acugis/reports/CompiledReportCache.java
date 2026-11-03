package com.acugis.reports;

import java.io.File;
import java.util.concurrent.ConcurrentHashMap;
import java.util.logging.Level;
import java.util.logging.Logger;

import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JasperCompileManager;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.util.JRLoader;
import net.sf.jasperreports.engine.util.JRSaver;

/**
 * Process-wide cache of compiled {@link JasperReport} objects.
 *
 * <p>Load order per request:
 * <ol>
 *   <li>In-memory hit when JRXML mtime is unchanged</li>
 *   <li>Load sibling {@code .jasper} when it is present and not older than JRXML</li>
 *   <li>Compile JRXML once, persist {@code .jasper}, store in memory</li>
 * </ol>
 *
 * <p>Compilation must not run on every request. Invalidate when JRXML changes
 * (mtime fingerprint), report metadata forces a clear, or {@link #clear()}/
 * {@link #invalidate(File)} is called explicitly.
 */
public final class CompiledReportCache {

    private static final Logger LOG = Logger.getLogger(CompiledReportCache.class.getName());
    private static final ConcurrentHashMap<String, Entry> CACHE = new ConcurrentHashMap<>();

    private CompiledReportCache() {
    }

    private static final class Entry {
        final JasperReport report;
        /** JRXML lastModified at cache time; 0 if no JRXML existed. */
        final long jrxmlMtime;

        Entry(JasperReport report, long jrxmlMtime) {
            this.report = report;
            this.jrxmlMtime = jrxmlMtime;
        }
    }

    /** Load result with cache diagnostics for the profiler. */
    public static final class LoadResult {
        public final JasperReport report;
        /** memory_hit | jasper_file | compiled */
        public final String source;
        public final boolean compiled;

        public LoadResult(JasperReport report, String source, boolean compiled) {
            this.report = report;
            this.source = source;
            this.compiled = compiled;
        }
    }

    /**
     * Resolve a report file (.jrxml / .jasper / bare) to a cached JasperReport.
     * Never compiles when a fresh in-memory object or on-disk .jasper is available.
     */
    public static LoadResult getOrLoad(File reportFile) throws JRException {
        if (reportFile == null) {
            throw new JRException("report file is null");
        }

        File jrxml = companion(reportFile, ".jrxml");
        File jasper = companion(reportFile, ".jasper");
        String key = cacheKey(reportFile);
        long jrxmlMtime = (jrxml != null && jrxml.isFile()) ? jrxml.lastModified() : 0L;

        Entry cached = CACHE.get(key);
        if (cached != null && cached.report != null && cached.jrxmlMtime == jrxmlMtime) {
            return new LoadResult(cached.report, "memory_hit", false);
        }

        // Prefer on-disk .jasper when it is at least as new as JRXML.
        if (jasper != null && jasper.isFile()) {
            boolean jasperFresh = jrxml == null || !jrxml.isFile()
                    || jasper.lastModified() >= jrxml.lastModified();
            if (jasperFresh) {
                JasperReport loaded = prepare(JRLoader.loadObject(jasper));
                CACHE.put(key, new Entry(loaded, jrxmlMtime));
                return new LoadResult(loaded, "jasper_file", false);
            }
        }

        if (jrxml != null && jrxml.isFile()) {
            LOG.info("Compiling JRXML once: " + jrxml.getName());
            JasperReport compiled = prepare(JasperCompileManager.compileReport(jrxml.getAbsolutePath()));
            persistJasper(jrxml, jasper, compiled);
            long version = jrxml.lastModified();
            CACHE.put(key, new Entry(compiled, version));
            return new LoadResult(compiled, "compiled", true);
        }

        // Last resort: load whatever file was passed (non-standard extension).
        if (reportFile.isFile()) {
            JasperReport loaded = prepare(JRLoader.loadObject(reportFile));
            CACHE.put(key, new Entry(loaded, jrxmlMtime));
            return new LoadResult(loaded, "jasper_file", false);
        }

        throw new JRException("Report file not found: " + reportFile.getAbsolutePath());
    }

    /** Drop one report from memory (JRXML / metadata change). */
    public static void invalidate(File reportFile) {
        if (reportFile == null) {
            return;
        }
        CACHE.remove(cacheKey(reportFile));
        File jrxml = companion(reportFile, ".jrxml");
        File jasper = companion(reportFile, ".jasper");
        if (jrxml != null) {
            CACHE.remove(cacheKey(jrxml));
        }
        if (jasper != null) {
            CACHE.remove(cacheKey(jasper));
        }
    }

    /** Invalidate by report key (no extension) under the reports directory. */
    public static void invalidateByName(File reportsDir, String reportName) {
        if (reportsDir == null || reportName == null) {
            return;
        }
        String rep = reportName.trim();
        int slash = Math.max(rep.lastIndexOf('/'), rep.lastIndexOf('\\'));
        if (slash >= 0) {
            rep = rep.substring(slash + 1);
        }
        String lower = rep.toLowerCase();
        if (lower.endsWith(".jrxml") || lower.endsWith(".jasper")) {
            rep = rep.substring(0, rep.lastIndexOf('.'));
        }
        if (rep.isEmpty() || rep.contains("..")) {
            return;
        }
        invalidate(new File(reportsDir, rep + ".jrxml"));
    }

    /** Explicit full clear of the in-memory compiled-report cache. */
    public static void clear() {
        CACHE.clear();
        LOG.info("Compiled report memory cache cleared (" + "all entries)");
    }

    public static int size() {
        return CACHE.size();
    }

    private static void persistJasper(File jrxml, File jasperTarget, JasperReport compiled) {
        File dest = jasperTarget;
        if (dest == null) {
            String path = jrxml.getAbsolutePath();
            int dot = path.lastIndexOf('.');
            if (dot < 0) {
                return;
            }
            dest = new File(path.substring(0, dot) + ".jasper");
        }
        try {
            JRSaver.saveObject(compiled, dest);
        } catch (Throwable t) {
            LOG.log(Level.WARNING,
                    "Could not write compiled .jasper beside " + jrxml.getName()
                            + "; in-memory cache still avoids recompile until restart/invalidate",
                    t);
        }
    }

    static String cacheKey(File reportFile) {
        File abs = reportFile.getAbsoluteFile();
        String path = abs.getPath();
        String lower = path.toLowerCase();
        if (lower.endsWith(".jrxml")) {
            return path.substring(0, path.length() - 6);
        }
        if (lower.endsWith(".jasper")) {
            return path.substring(0, path.length() - 7);
        }
        return path;
    }

    private static File companion(File reportFile, String extension) {
        if (reportFile == null) {
            return null;
        }
        String path = reportFile.getAbsolutePath();
        String lower = path.toLowerCase();
        String base;
        if (lower.endsWith(".jrxml")) {
            base = path.substring(0, path.length() - 6);
        } else if (lower.endsWith(".jasper")) {
            base = path.substring(0, path.length() - 7);
        } else {
            base = path;
        }
        return new File(base + extension);
    }

    /** Apply bundled font-name aliases after compile/load. */
    private static JasperReport prepare(Object loaded) {
        JasperReport report = (JasperReport) loaded;
        FontNameAliaser.apply(report);
        return report;
    }
}
