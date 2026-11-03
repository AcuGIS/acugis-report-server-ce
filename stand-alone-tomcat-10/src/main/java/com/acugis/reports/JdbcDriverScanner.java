package com.acugis.reports;

import java.io.File;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

/**
 * Detects JDBC drivers in WEB-INF/lib (Mode A) and Tomcat lib / drivers.dir (Mode B).
 */
public final class JdbcDriverScanner {

    public enum Vendor {
        POSTGRESQL("PostgreSQL", new String[]{"postgresql"}, "org.postgresql.Driver"),
        MYSQL("MySQL", new String[]{"mysql-connector", "mysql-jdbc", "mysql"}, "com.mysql.cj.jdbc.Driver"),
        MARIADB("MariaDB", new String[]{"mariadb"}, "org.mariadb.jdbc.Driver"),
        SQLSERVER("SQL Server", new String[]{"mssql-jdbc", "sqljdbc"}, "com.microsoft.sqlserver.jdbc.SQLServerDriver"),
        ORACLE("Oracle", new String[]{"ojdbc"}, "oracle.jdbc.OracleDriver"),
        SQLITE("SQLite", new String[]{"sqlite-jdbc", "sqlite"}, "org.sqlite.JDBC"),
        SNOWFLAKE("Snowflake", new String[]{"snowflake-jdbc", "snowflake"}, "net.snowflake.client.jdbc.SnowflakeDriver"),
        DB2("IBM Db2", new String[]{"db2jcc", "jcc", "db2"}, "com.ibm.db2.jcc.DB2Driver");

        public final String label;
        public final String[] nameHints;
        public final String driverClass;

        Vendor(String label, String[] nameHints, String driverClass) {
            this.label = label;
            this.nameHints = nameHints;
            this.driverClass = driverClass;
        }
    }

    public static final class Finding {
        public final Vendor vendor;
        public final String location;
        public final String jarName;
        public final boolean classLoadable;

        Finding(Vendor vendor, String location, String jarName, boolean classLoadable) {
            this.vendor = vendor;
            this.location = location;
            this.jarName = jarName;
            this.classLoadable = classLoadable;
        }
    }

    public static final class Report {
        public final List<Finding> findings;
        public final Set<Vendor> foundVendors;
        public final List<String> scannedDirs;

        Report(List<Finding> findings, Set<Vendor> foundVendors, List<String> scannedDirs) {
            this.findings = findings;
            this.foundVendors = foundVendors;
            this.scannedDirs = scannedDirs;
        }
    }

    private JdbcDriverScanner() {
    }

    public static Report scan(File webInfLib) {
        List<String> scanned = new ArrayList<>();
        List<Finding> findings = new ArrayList<>();
        Set<Vendor> found = new LinkedHashSet<>();

        List<File> dirs = new ArrayList<>();
        if (webInfLib != null) {
            dirs.add(webInfLib);
        }
        dirs.add(CatalinaPaths.tomcatLibDir());
        dirs.add(AcugisConfig.get().driversDir());

        Map<String, File> jars = new LinkedHashMap<>();
        for (File dir : dirs) {
            if (dir == null) {
                continue;
            }
            scanned.add(dir.getAbsolutePath());
            if (!dir.isDirectory()) {
                continue;
            }
            File[] files = dir.listFiles((d, name) -> name != null && name.toLowerCase(Locale.ROOT).endsWith(".jar"));
            if (files == null) {
                continue;
            }
            for (File jar : files) {
                jars.putIfAbsent(jar.getName().toLowerCase(Locale.ROOT), jar);
            }
        }

        for (Vendor vendor : Vendor.values()) {
            boolean matchedJar = false;
            for (File jar : jars.values()) {
                String name = jar.getName().toLowerCase(Locale.ROOT);
                for (String hint : vendor.nameHints) {
                    if (name.contains(hint.toLowerCase(Locale.ROOT))) {
                        boolean loadable = canLoad(vendor.driverClass);
                        findings.add(new Finding(vendor, jar.getParentFile().getAbsolutePath(), jar.getName(), loadable));
                        found.add(vendor);
                        matchedJar = true;
                        break;
                    }
                }
                if (matchedJar) {
                    break;
                }
            }
            if (!matchedJar && canLoad(vendor.driverClass)) {
                findings.add(new Finding(vendor, "(classpath)", "(classloader)", true));
                found.add(vendor);
            }
        }

        return new Report(Collections.unmodifiableList(findings),
                Collections.unmodifiableSet(found),
                Collections.unmodifiableList(scanned));
    }

    private static boolean canLoad(String className) {
        try {
            Class.forName(className);
            return true;
        } catch (Throwable ignored) {
            return false;
        }
    }
}
