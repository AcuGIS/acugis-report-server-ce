package com.acugis.reports;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;
import static org.junit.Assert.fail;

import java.io.ByteArrayInputStream;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DriverManager;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

import org.junit.BeforeClass;
import org.junit.Test;

import net.sf.jasperreports.engine.DefaultJasperReportsContext;
import net.sf.jasperreports.engine.JREmptyDataSource;
import net.sf.jasperreports.engine.JRPrintElement;
import net.sf.jasperreports.engine.JRPrintText;
import net.sf.jasperreports.engine.JRRuntimeException;
import net.sf.jasperreports.engine.JasperCompileManager;
import net.sf.jasperreports.engine.JasperFillManager;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.SimpleJasperReportsContext;
import net.sf.jasperreports.engine.data.JRMapCollectionDataSource;
import net.sf.jasperreports.engine.query.DefaultQueryExecuterFactoryBundle;
import net.sf.jasperreports.engine.query.JRJdbcQueryExecuterFactory;
import net.sf.jasperreports.engine.query.JRQueryExecuterFactoryBundle;
import net.sf.jasperreports.engine.query.QueryExecuterFactory;
import net.sf.jasperreports.engine.util.JRQueryExecuterUtils;

/**
 * Collection languages must compile and fill from {@link JRMapCollectionDataSource}.
 * JasperReports 7 does not honor a System property for query-executer factories.
 */
public class CollectionQueryExecuterFactoryTest {

    @BeforeClass
    public static void registerCollectionLanguages() {
        System.setProperty("java.awt.headless", "true");
        System.setProperty("net.sf.jasperreports.awt.ignore.missing.font", "true");
        CollectionQueryExecuterFactory.register();
        DefaultJasperReportsContext.getInstance().setProperty(
                "net.sf.jasperreports.compiler.class",
                "net.sf.jasperreports.jdt.JRJdtCompiler");
    }

    @Test
    public void systemPropertyAloneDoesNotRegisterAQueryLanguage() throws Exception {
        String language = "collection_system_only";
        String key = QueryExecuterFactory.QUERY_EXECUTER_FACTORY_PREFIX + language;
        System.setProperty(key, CollectionQueryExecuterFactory.class.getName());
        try {
            assertNull(DefaultJasperReportsContext.getInstance().getProperty(key));
            try {
                JRQueryExecuterUtils.getInstance(DefaultJasperReportsContext.getInstance())
                        .getExecuterFactory(language);
                fail("expected no factory for a System-property-only language");
            } catch (JRRuntimeException ex) {
                assertTrue(ex.getMessage(), ex.getMessage().contains(language));
                assertTrue(ex.getMessage(), ex.getMessage().toLowerCase().contains("query executer"));
            }
        } finally {
            System.clearProperty(key);
        }
    }

    @Test
    public void contextPropertyRegistersCollectionFactory() throws Exception {
        SimpleJasperReportsContext ctx = new SimpleJasperReportsContext();
        ctx.setParent(null);
        ctx.setExtensions(
                JRQueryExecuterFactoryBundle.class,
                Collections.singletonList(DefaultQueryExecuterFactoryBundle.getInstance(ctx)));
        try {
            JRQueryExecuterUtils.getInstance(ctx).getExecuterFactory("collection");
            fail("unregistered context should reject collection");
        } catch (JRRuntimeException ex) {
            assertTrue(ex.getMessage(), ex.getMessage().contains("collection"));
        }
        ctx.setProperty(
                QueryExecuterFactory.QUERY_EXECUTER_FACTORY_PREFIX + "collection",
                CollectionQueryExecuterFactory.class.getName());
        QueryExecuterFactory factory = JRQueryExecuterUtils.getInstance(ctx).getExecuterFactory("collection");
        assertTrue(factory instanceof CollectionQueryExecuterFactory);

        QueryExecuterFactory sql = JRQueryExecuterUtils.getInstance(DefaultJasperReportsContext.getInstance())
                .getExecuterFactory("SQL");
        assertTrue(sql instanceof JRJdbcQueryExecuterFactory);
        QueryExecuterFactory collection = JRQueryExecuterUtils.getInstance(DefaultJasperReportsContext.getInstance())
                .getExecuterFactory("collection");
        assertTrue(collection instanceof CollectionQueryExecuterFactory);
        QueryExecuterFactory cassandra = JRQueryExecuterUtils.getInstance(DefaultJasperReportsContext.getInstance())
                .getExecuterFactory("cassandra");
        assertTrue(cassandra instanceof CollectionQueryExecuterFactory);
    }

    @Test
    public void arcgisCollectionFillsFromMapWithoutJdbc() throws Exception {
        assertCollectionFill(
                "arcgis",
                "collection",
                "arcgis",
                "division",
                "Mtn");
    }

    @Test
    public void cassandraCollectionFillsFromMapWithoutJdbc() throws Exception {
        assertCollectionFill(
                "cassandra",
                "cassandra",
                "cassandra",
                "id",
                "row-1");
    }

    @Test
    public void bigQueryCollectionFillsFromMapWithoutJdbc() throws Exception {
        assertCollectionFill(
                "bigquery",
                "collection",
                "bigquery",
                "sales",
                "42");
    }

    @Test
    public void geoServerCollectionFillsFromMapWithoutJdbc() throws Exception {
        assertCollectionFill(
                "geoserver",
                "collection",
                "geoserver",
                "name",
                "Main");
    }

    @Test
    public void sqlReportStillFillsThroughJdbc() throws Exception {
        JasperReport report = compile(jrxml("sql", "SQL", null, "name", "SELECT name FROM sale"));
        try (Connection connection = DriverManager.getConnection("jdbc:h2:mem:jrsjdbc;DB_CLOSE_DELAY=-1")) {
            connection.createStatement().execute("CREATE TABLE sale (name VARCHAR(40))");
            connection.createStatement().execute("INSERT INTO sale (name) VALUES ('jdbc-row')");
            JasperPrint print = JasperFillManager.fillReport(report, new HashMap<String, Object>(), connection);
            assertEquals("jdbc-row", firstText(print));
        }
    }

    @Test
    public void collectionDataSelectsCollectionFillNotJdbcLookup() {
        assertEquals("collection", ReportServlet.fillMode(null, "[{\"division\":\"Mtn\"}]"));
        assertEquals("collection", ReportServlet.fillMode("", "[{\"division\":\"Mtn\"}]"));
        assertEquals("collection", ReportServlet.fillMode("   ", "[{\"a\":1}]"));
        assertEquals("jdbc", ReportServlet.fillMode(null, null));
        assertEquals("jdbc", ReportServlet.fillMode(null, ""));
        assertEquals("jdbc", ReportServlet.fillMode(null, "   "));
        assertEquals("duckdb-jdbc", ReportServlet.fillMode("session-1", "[{\"a\":1}]"));
    }

    @Test
    public void emptyCollectionLanguageIsNotSqlFill() throws Exception {
        // No connection and no map rows: the stub executer must not open JDBC.
        JasperReport report = compile(jrxml("empty", "collection", "arcgis", "division",
                "SELECT \"division\" FROM \"layers\".\"Divisions\""));
        JasperPrint print = JasperFillManager.fillReport(report, new HashMap<String, Object>(), new JREmptyDataSource(0));
        assertTrue(print.getPages().isEmpty() || firstText(print) == null || firstText(print).isEmpty());
    }

    private static void assertCollectionFill(
            String name, String language, String provider, String field, String value) throws Exception {
        JasperReport report = compile(jrxml(name, language, provider, field,
                "SELECT \"" + field + "\" FROM \"layers\".\"t\""));
        Map<String, Object> row = new HashMap<String, Object>();
        row.put(field, value);
        JRMapCollectionDataSource data = new JRMapCollectionDataSource(Collections.singletonList(row));
        JasperPrint print = JasperFillManager.fillReport(report, new HashMap<String, Object>(), data);
        assertEquals(value, firstText(print));
    }

    private static JasperReport compile(String jrxml) throws Exception {
        return JasperCompileManager.compileReport(
                new ByteArrayInputStream(jrxml.getBytes(StandardCharsets.UTF_8)));
    }

    private static String jrxml(String name, String language, String provider, String field, String query) {
        String property = "";
        if (provider != null) {
            property = "  <property name=\"acugis.datasource.provider\" value=\"" + provider + "\"/>\n";
        }
        return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
                + "<jasperReport name=\"" + name + "\" language=\"java\" pageWidth=\"595\" pageHeight=\"842\" "
                + "columnWidth=\"555\" leftMargin=\"20\" rightMargin=\"20\" topMargin=\"20\" bottomMargin=\"20\" "
                + "uuid=\"11111111-1111-1111-1111-111111111111\">\n"
                + property
                + "  <query language=\"" + language + "\"><![CDATA[" + query + "]]></query>\n"
                + "  <field name=\"" + field + "\" class=\"java.lang.String\"/>\n"
                + "  <detail><band height=\"20\">\n"
                + "    <element kind=\"textField\" uuid=\"22222222-2222-2222-2222-222222222222\" "
                + "x=\"0\" y=\"0\" width=\"200\" height=\"20\">\n"
                + "      <expression><![CDATA[$F{" + field + "}]]></expression>\n"
                + "    </element>\n"
                + "  </band></detail>\n"
                + "</jasperReport>\n";
    }

    private static String firstText(JasperPrint print) {
        if (print.getPages() == null) {
            return null;
        }
        for (int i = 0; i < print.getPages().size(); i++) {
            for (JRPrintElement element : print.getPages().get(i).getElements()) {
                if (element instanceof JRPrintText) {
                    String text = ((JRPrintText) element).getFullText();
                    if (text != null && !text.isEmpty()) {
                        return text;
                    }
                }
            }
        }
        return null;
    }
}
