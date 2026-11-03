package com.acugis.reports;

import java.io.File;
import java.util.ArrayList;
import java.util.List;

/**
 * Jasper startup no longer treats a file on disk as a product license.
 * Report Server Professional entitlement is an ACL1 token verified by the Go server.
 * This helper only notes whether a legacy file is present.
 */
public final class LicenseResolver {

    public enum Status {
        /** A file was found. It is not verified and does not grant Professional. */
        FILE_PRESENT,
        /** No legacy file was found. Entitlement is decided by the Go server. */
        NOT_USED
    }

    public static final class Result {
        public final Status status;
        public final File file;
        public final String message;
        public final List<String> searched;

        Result(Status status, File file, String message, List<String> searched) {
            this.status = status;
            this.file = file;
            this.message = message;
            this.searched = searched;
        }
    }

    private LicenseResolver() {
    }

    public static Result resolve(File webInfLicenseDir) {
        List<String> searched = new ArrayList<>();
        AcugisConfig cfg = AcugisConfig.get();

        String configured = cfg.get("license.file", "");
        if (!configured.isEmpty()) {
            File f = new File(AcugisConfig.expand(configured));
            searched.add(f.getAbsolutePath());
            if (f.isFile()) {
                return new Result(Status.FILE_PRESENT, f, "Legacy file present; not a Report Server Professional entitlement", searched);
            }
        }

        File licensesDir = cfg.licensesDir();
        File[] candidates = new File[] {
                new File(licensesDir, "acugis.license"),
                new File(licensesDir, "license.lic"),
                new File(licensesDir, "license.key"),
                licensesDir
        };
        for (File f : candidates) {
            searched.add(f.getAbsolutePath());
            if (f.isFile()) {
                return new Result(Status.FILE_PRESENT, f, "Legacy file present; not a Report Server Professional entitlement", searched);
            }
            if (f.isDirectory()) {
                File[] files = f.listFiles((dir, name) ->
                        name != null && (name.endsWith(".license") || name.endsWith(".lic") || name.endsWith(".key")));
                if (files != null) {
                    for (File child : files) {
                        searched.add(child.getAbsolutePath());
                        if (child.isFile()) {
                            return new Result(Status.FILE_PRESENT, child, "Legacy file present; not a Report Server Professional entitlement", searched);
                        }
                    }
                }
            }
        }

        if (webInfLicenseDir != null) {
            searched.add(webInfLicenseDir.getAbsolutePath());
            if (webInfLicenseDir.isDirectory()) {
                File[] files = webInfLicenseDir.listFiles((dir, name) ->
                        name != null && !name.startsWith(".")
                                && (name.endsWith(".license") || name.endsWith(".lic")
                                || name.endsWith(".key") || name.equalsIgnoreCase("license")));
                if (files != null) {
                    for (File child : files) {
                        searched.add(child.getAbsolutePath());
                        if (child.isFile() && child.length() > 0) {
                            return new Result(Status.FILE_PRESENT, child, "Legacy file present; not a Report Server Professional entitlement", searched);
                        }
                    }
                }
            } else if (webInfLicenseDir.isFile()) {
                return new Result(Status.FILE_PRESENT, webInfLicenseDir, "Legacy file present; not a Report Server Professional entitlement", searched);
            }
        }

        return new Result(Status.NOT_USED, null,
                "No legacy license file. Report Server entitlement is resolved by the Go server.", searched);
    }
}
