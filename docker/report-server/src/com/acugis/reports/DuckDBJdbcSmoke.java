package com.acugis.reports;

import java.io.File;
import java.nio.file.Files;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.Statement;
import java.util.HashMap;
import net.sf.jasperreports.engine.JasperCompileManager;
import net.sf.jasperreports.engine.JasperFillManager;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.engine.JasperReport;

/**
 * Local smoke: DuckDB JDBC → DuckDBSession → JasperFillManager(Connection).
 *
 * Usage (from docker/report-server after rebuild-war):
 *   java -cp "_classes_out;_war_rebuild/WEB-INF/lib/*;../stand-alone-tomcat-10/lib/duckdb_jdbc-1.5.5.1-windows_amd64.jar"
 *        com.acugis.reports.DuckDBJdbcSmoke
 */
public final class DuckDBJdbcSmoke {
    private DuckDBJdbcSmoke() {}

    public static void main(String[] args) throws Exception {
        File dir = Files.createTempDirectory("duckdb-jdbc-smoke").toFile();
        File db = new File(dir, "typed.duckdb");
        File jrxml = new File(dir, "typed.jrxml");

        Class.forName("org.duckdb.DuckDBDriver");
        try (Connection seed = DriverManager.getConnection("jdbc:duckdb:" + db.getAbsolutePath().replace('\\', '/'));
                Statement st = seed.createStatement()) {
            st.execute("CREATE TABLE typed_demo ("
                    + "id INTEGER, name VARCHAR, amount DECIMAL(18,4), score DOUBLE, "
                    + "ok BOOLEAN, d DATE, ts TIMESTAMP, note VARCHAR)");
            st.execute("INSERT INTO typed_demo VALUES "
                    + "(1, 'alpha', 12.50, 1.5e-4, true, DATE '2024-01-15', TIMESTAMP '2024-01-15 10:30:00', NULL),"
                    + "(2, 'beta', 999999999.1234, 3.1415926535, false, DATE '2024-02-01', TIMESTAMP '2024-02-01 00:00:00', 'x')");
        }

        String xml = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
                + "<jasperReport name=\"duckdb_typed_smoke\" language=\"java\" pageWidth=\"595\" pageHeight=\"842\" "
                + "columnWidth=\"555\" leftMargin=\"20\" rightMargin=\"20\" topMargin=\"20\" bottomMargin=\"20\">\n"
                + "  <property name=\"acugis.datasource.provider\" value=\"duckdb\"/>\n"
                + "  <query language=\"SQL\"><![CDATA[SELECT id, name, amount, score, ok, d, ts, note FROM typed_demo ORDER BY id]]></query>\n"
                + "  <field name=\"id\" class=\"java.lang.Integer\"/>\n"
                + "  <field name=\"name\" class=\"java.lang.String\"/>\n"
                + "  <field name=\"amount\" class=\"java.math.BigDecimal\"/>\n"
                + "  <field name=\"score\" class=\"java.lang.Double\"/>\n"
                + "  <field name=\"ok\" class=\"java.lang.Boolean\"/>\n"
                + "  <field name=\"d\" class=\"java.sql.Date\"/>\n"
                + "  <field name=\"ts\" class=\"java.sql.Timestamp\"/>\n"
                + "  <field name=\"note\" class=\"java.lang.String\"/>\n"
                + "  <detail><band height=\"18\">\n"
                + "    <element kind=\"textField\" x=\"0\" y=\"0\" width=\"40\" height=\"16\">"
                + "<expression><![CDATA[$F{id}]]></expression></element>\n"
                + "    <element kind=\"textField\" x=\"50\" y=\"0\" width=\"80\" height=\"16\">"
                + "<expression><![CDATA[$F{name}]]></expression></element>\n"
                + "    <element kind=\"textField\" x=\"140\" y=\"0\" width=\"100\" height=\"16\">"
                + "<expression><![CDATA[$F{amount}]]></expression></element>\n"
                + "    <element kind=\"textField\" x=\"250\" y=\"0\" width=\"80\" height=\"16\">"
                + "<expression><![CDATA[$F{score}]]></expression></element>\n"
                + "  </band></detail>\n"
                + "</jasperReport>\n";
        Files.write(jrxml.toPath(), xml.getBytes("UTF-8"));

        System.setProperty(
                "net.sf.jasperreports.query.executer.factory.duckdb",
                "net.sf.jasperreports.engine.query.JRJdbcQueryExecuterFactory");

        String sessionJson = "{\"mode\":\"database\",\"databasePath\":\""
                + db.getAbsolutePath().replace('\\', '/').replace("\"", "\\\"")
                + "\"}";
        Connection conn = DuckDBSession.open(sessionJson);
        try {
            JasperReport report = JasperCompileManager.compileReport(jrxml.getAbsolutePath());
            JasperPrint print = JasperFillManager.fillReport(report, new HashMap<String, Object>(), conn);
            int pages = print.getPages() == null ? 0 : print.getPages().size();
            if (pages < 1) {
                throw new IllegalStateException("expected at least one page, got " + pages);
            }
            System.out.println("PASS duckdb-jdbc local fill pages=" + pages + " rows via REPORT_CONNECTION");
        } finally {
            conn.close();
        }
    }
}
