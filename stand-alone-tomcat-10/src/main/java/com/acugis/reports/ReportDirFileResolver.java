package com.acugis.reports;

import java.io.File;

public class ReportDirFileResolver {
    private final File baseDir;

    public ReportDirFileResolver(File baseDir) {
        this.baseDir = baseDir;
    }

    public File resolveFile(String filename) {
        if (filename == null || filename.trim().isEmpty()) {
            return null;
        }
        filename = ReportsFileRepositoryService.normalizeLocation(filename);
        File f = new File(filename);
        if (f.isAbsolute() && f.exists()) {
            return f;
        }
        File rel = new File(this.baseDir, filename);
        if (rel.exists()) {
            return rel;
        }
        return null;
    }
}
