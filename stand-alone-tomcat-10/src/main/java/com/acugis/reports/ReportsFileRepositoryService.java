package com.acugis.reports;

import java.io.File;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;

import net.sf.jasperreports.engine.JasperReportsContext;
import net.sf.jasperreports.repo.FileRepositoryService;
import net.sf.jasperreports.repo.RepositoryContext;

/**
 * File repository rooted at the reports directory that also resolves Image
 * Resources under {@code resource.paths.dir}.
 *
 * <p>JRXML expressions like {@code images/family.png} resolve relative to the
 * report's catalog folder (e.g. {@code finance} →
 * {@code resource-paths/finance/images/family.png}).
 */
public class ReportsFileRepositoryService extends FileRepositoryService {

    private final File resourcePathsRoot;
    private final String reportFolderBase; // e.g. "finance" (no leading slash)

    public ReportsFileRepositoryService(JasperReportsContext context, String root, boolean resolveAbsolutePath) {
        this(context, root, resolveAbsolutePath, null, null);
    }

    public ReportsFileRepositoryService(
            JasperReportsContext context,
            String root,
            boolean resolveAbsolutePath,
            File resourcePathsRoot,
            String reportFolderBase) {
        super(context, root, resolveAbsolutePath);
        this.resourcePathsRoot = resourcePathsRoot;
        this.reportFolderBase = normalizeBase(reportFolderBase);
    }

    @Override
    public File getFile(RepositoryContext context, String location) {
        File mapped = resolveResourcePath(location);
        if (mapped != null) {
            return mapped;
        }
        return super.getFile(context, location);
    }

    @Override
    protected File locateFile(String location) {
        File mapped = resolveResourcePath(location);
        if (mapped != null) {
            return mapped;
        }
        String norm = normalizeLocation(location);
        if (norm != null && norm.startsWith("/")) {
            // Avoid treating "/finance/…" as a Linux absolute filesystem path first.
            File underReports = super.locateFile(stripLeadingSlashes(norm));
            if (underReports != null && underReports.isFile()) {
                return underReports;
            }
        }
        return super.locateFile(norm);
    }

    File resolveResourcePath(String location) {
        if (resourcePathsRoot == null || !resourcePathsRoot.isDirectory()) {
            return null;
        }
        String norm = normalizeLocation(location);
        if (norm == null || norm.isEmpty() || norm.indexOf("://") >= 0) {
            return null;
        }
        String rel = stripLeadingSlashes(norm);
        if (rel.isEmpty() || rel.contains("..")) {
            return null;
        }

        // 1) Relative to report catalog folder: finance + images/family.png
        if (reportFolderBase != null && !reportFolderBase.isEmpty()) {
            File underFolder = safeFile(resourcePathsRoot, reportFolderBase + "/" + rel);
            if (underFolder != null && underFolder.isFile()) {
                return underFolder;
            }
        }
        // 2) Absolute from resource-paths root: finance/images/family.png
        File absolute = safeFile(resourcePathsRoot, rel);
        if (absolute != null && absolute.isFile()) {
            return absolute;
        }
        return null;
    }

    static String normalizeLocation(String location) {
        if (location == null || location.isEmpty()) {
            return location;
        }
        if (location.indexOf("://") >= 0) {
            return location;
        }
        return location.replace('\\', '/');
    }

    static String stripLeadingSlashes(String path) {
        if (path == null) {
            return "";
        }
        int i = 0;
        while (i < path.length() && path.charAt(i) == '/') {
            i++;
        }
        return path.substring(i);
    }

    static String normalizeBase(String base) {
        if (base == null) {
            return null;
        }
        base = stripLeadingSlashes(base.replace('\\', '/').trim());
        if (base.isEmpty() || base.contains("..")) {
            return null;
        }
        return base;
    }

    static File safeFile(File root, String relative) {
        try {
            File candidate = new File(root, relative.replace('/', File.separatorChar));
            String rootCanon = root.getCanonicalPath();
            String candCanon = candidate.getCanonicalPath();
            if (!candCanon.equals(rootCanon) && !candCanon.startsWith(rootCanon + File.separator)) {
                return null;
            }
            return candidate;
        } catch (IOException e) {
            return null;
        }
    }

    /**
     * Reads {@code {repName}.resource-base} next to the JRXML (written by Go when
     * the report is filed in a catalog folder).
     */
    public static String readResourceBaseFile(File reportDir, String repName) {
        if (reportDir == null || repName == null || repName.trim().isEmpty()) {
            return null;
        }
        String key = repName.trim();
        int slash = Math.max(key.lastIndexOf('/'), key.lastIndexOf('\\'));
        if (slash >= 0) {
            key = key.substring(slash + 1);
        }
        if (key.toLowerCase().endsWith(".jasper")) {
            key = key.substring(0, key.length() - 7);
        } else if (key.toLowerCase().endsWith(".jrxml")) {
            key = key.substring(0, key.length() - 6);
        }
        File f = new File(reportDir, key + ".resource-base");
        if (!f.isFile()) {
            return null;
        }
        try {
            String raw = new String(Files.readAllBytes(f.toPath()), StandardCharsets.UTF_8).trim();
            return normalizeBase(raw);
        } catch (IOException e) {
            return null;
        }
    }
}
