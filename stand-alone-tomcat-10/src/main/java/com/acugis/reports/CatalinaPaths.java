package com.acugis.reports;

import java.io.File;

/**
 * Resolves Apache Tomcat CATALINA_BASE / CATALINA_HOME without assuming Docker paths.
 */
public final class CatalinaPaths {

    private CatalinaPaths() {
    }

    public static String catalinaBase() {
        String base = firstNonBlank(
                System.getProperty("catalina.base"),
                System.getenv("CATALINA_BASE"),
                System.getProperty("catalina.home"),
                System.getenv("CATALINA_HOME"));
        if (base != null) {
            return new File(base).getAbsolutePath();
        }
        return new File(".").getAbsolutePath();
    }

    public static String catalinaHome() {
        String home = firstNonBlank(
                System.getProperty("catalina.home"),
                System.getenv("CATALINA_HOME"),
                System.getProperty("catalina.base"),
                System.getenv("CATALINA_BASE"));
        if (home != null) {
            return new File(home).getAbsolutePath();
        }
        return catalinaBase();
    }

    public static File acugisRoot() {
        return new File(catalinaBase(), "acugis");
    }

    public static File confDir() {
        return new File(catalinaBase(), "conf");
    }

    public static File logsDir() {
        return new File(catalinaBase(), "logs");
    }

    public static File tempDir() {
        File catalinaTemp = new File(catalinaBase(), "temp");
        if (catalinaTemp.isDirectory()) {
            return catalinaTemp;
        }
        String tmp = System.getProperty("java.io.tmpdir");
        if (tmp != null && !tmp.trim().isEmpty()) {
            return new File(tmp);
        }
        return catalinaTemp;
    }

    public static File tomcatLibDir() {
        File baseLib = new File(catalinaBase(), "lib");
        if (baseLib.isDirectory()) {
            return baseLib;
        }
        return new File(catalinaHome(), "lib");
    }

    private static String firstNonBlank(String... values) {
        if (values == null) {
            return null;
        }
        for (String value : values) {
            if (value != null && !value.trim().isEmpty()) {
                return value.trim();
            }
        }
        return null;
    }
}
