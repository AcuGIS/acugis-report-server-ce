package com.acugis.reports;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;
import static org.junit.Assert.fail;

import java.awt.image.BufferedImage;
import java.io.ByteArrayInputStream;
import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import javax.imageio.ImageIO;

import org.junit.BeforeClass;
import org.junit.Test;

import jakarta.servlet.http.HttpServletRequest;
import net.sf.jasperreports.charts.JRChart;
import net.sf.jasperreports.engine.DefaultJasperReportsContext;
import net.sf.jasperreports.engine.JRElement;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JRPrintElement;
import net.sf.jasperreports.engine.JRPrintImage;
import net.sf.jasperreports.engine.JRPrintText;
import net.sf.jasperreports.engine.JRTextField;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.data.JRMapCollectionDataSource;
import net.sf.jasperreports.engine.type.HorizontalTextAlignEnum;

/**
 * Production component export. The fixture and the field/chart assertions come
 * from the embed-component probe: one fill through {@link ReportService#fillReport},
 * then {@link EmbedComponentExporter} isolates the printed element.
 */
public class EmbedComponentExportTest {

    static final String FIELD_ID = "total-units";
    static final String CHART_ID = "monthly-occupancy";
    static final int FIELD_WIDTH = 220;
    static final int FIELD_HEIGHT = 28;
    static final int CHART_WIDTH = 320;
    static final int CHART_HEIGHT = 180;

    private static JasperReport report;
    private static JasperPrint print;

    @BeforeClass
    public static void fillOnce() throws Exception {
        System.setProperty("java.awt.headless", "true");
        System.setProperty("net.sf.jasperreports.awt.ignore.missing.font", "true");
        DefaultJasperReportsContext.getInstance().setProperty(
                "net.sf.jasperreports.awt.ignore.missing.font", "true");
        DefaultJasperReportsContext.getInstance().setProperty(
                "net.sf.jasperreports.compiler.class",
                "net.sf.jasperreports.jdt.JRJdtCompiler");

        File dir = new File("target/embed-component-export");
        if (!dir.isDirectory() && !dir.mkdirs()) {
            throw new IllegalStateException("cannot create " + dir);
        }
        File jrxml = new File(dir, "embed-component-export.jrxml");
        Files.write(jrxml.toPath(), fixtureJrxml().getBytes(StandardCharsets.UTF_8));

        report = ReportParameterBinder.loadOrCompileReport(jrxml);
        Map<String, Object> params = new HashMap<String, Object>();
        params.put("account_id", "A-17");
        params.put(net.sf.jasperreports.engine.JRParameter.REPORT_LOCALE, Locale.US);
        List<Map<String, ?>> rows = new ArrayList<Map<String, ?>>();
        rows.add(row("Jan", 800));
        rows.add(row("Feb", 400));
        rows.add(row("Mar", 34));
        print = ReportService.fillReport(
                report,
                params,
                new JRMapCollectionDataSource(rows),
                dir,
                "embed-component-export",
                null);
        ReportService.endChartRenderSession();
        assertNotNull(print);
    }

    @Test
    public void fieldExportsOnlyItsFilledValue() throws Exception {
        JRElement design = onlyDesign(FIELD_ID);
        assertTrue(design instanceof JRTextField);
        assertEquals("net.sf.jasperreports.engine.base.JRBaseTextField", design.getClass().getName());

        JRPrintText text = (JRPrintText) onlyPrint(FIELD_ID);
        assertEquals("1,234", text.getFullText());
        assertEquals("#,##0", text.getPattern());
        assertEquals("DejaVu Sans", text.getFontName());
        assertEquals(16f, text.getFontSize(), 0.01f);
        assertTrue(text.isBold());
        assertEquals(HorizontalTextAlignEnum.RIGHT, text.getHorizontalTextAlign());
        assertEquals(FIELD_WIDTH, text.getWidth());
        assertEquals(FIELD_HEIGHT, text.getHeight());
        int x = text.getX();
        int y = text.getY();

        String html = EmbedComponentExporter.exportHtml(report, print, FIELD_ID, null);
        assertEquals("1,234", text.getFullText());
        assertEquals(x, text.getX());
        assertEquals(y, text.getY());
        assertTrue(html.contains("1,234"));
        assertTrue(html.contains("id=\"" + FIELD_ID + "\""));
        assertOnlyThisComponent(html, FIELD_ID);
        String lower = html.toLowerCase(Locale.US);
        String around = snippet(html, FIELD_ID).toLowerCase(Locale.US);
        assertTrue(around, around.contains("text-align: right"));
        assertTrue(around, around.contains("#123456"));
        assertTrue(around, around.contains("#fff4e5"));
        assertTrue(around, around.contains("dejavu sans"));
        assertTrue(around, around.contains("font-size: 16"));
        assertTrue(around, around.contains("font-weight: bold"));
        assertTrue(around, around.contains("padding-top: 4px"));
        assertTrue(around, around.contains("padding-left: 6px"));
        assertTrue(around, around.contains("padding-bottom: 4px"));
        assertTrue(around, around.contains("padding-right: 6px"));
        assertTrue(around, around.contains("border: 1px solid #112233"));
        assertFalse(lower.contains("acugis_report_image"));
        Dimension box = pageBox(html);
        assertEquals(FIELD_WIDTH, box.width);
        assertEquals(FIELD_HEIGHT, box.height);
    }

    @Test
    public void chartExportsTheFilledImageThroughTheReportImageStore() throws Exception {
        JRElement design = onlyDesign(CHART_ID);
        assertTrue(design instanceof JRChart);
        assertEquals("net.sf.jasperreports.charts.base.JRBaseChart", design.getClass().getName());

        JRPrintImage image = (JRPrintImage) onlyPrint(CHART_ID);
        Object renderer = image.getRenderer();
        assertNotNull(renderer);
        assertEquals(CHART_WIDTH, image.getWidth());
        assertEquals(CHART_HEIGHT, image.getHeight());

        String html = EmbedComponentExporter.exportHtml(report, print, CHART_ID, null);
        assertTrue(renderer == image.getRenderer());
        assertTrue(html.contains("id=\"" + CHART_ID + "\""));
        assertOnlyThisComponent(html, CHART_ID);
        assertFalse(html.contains("1,234"));

        Matcher src = Pattern.compile("acugis_report_image\\?image=([^\"&]+)&(?:amp;)?uuid=([^\"&]+)").matcher(html);
        assertTrue("chart HTML has no image URI:\n" + html, src.find());
        String imageName = java.net.URLDecoder.decode(src.group(1), "UTF-8");
        String uuid = java.net.URLDecoder.decode(src.group(2), "UTF-8");
        byte[] bytes = ReportImageStore.get(uuid, imageName);
        assertNotNull(bytes);
        assertTrue(bytes.length > 1000);
        String kind = imageKind(bytes);
        assertTrue(kind.equals("png") || kind.equals("svg"));
        if ("png".equals(kind)) {
            BufferedImage raster = ImageIO.read(new ByteArrayInputStream(bytes));
            assertNotNull(raster);
            assertTrue(raster.getWidth() >= CHART_WIDTH / 2);
            assertTrue(raster.getHeight() >= CHART_HEIGHT / 2);
            assertTrue(distinctColors(raster) >= 4);
        }
        Dimension box = pageBox(html);
        assertEquals(CHART_WIDTH, box.width);
        assertEquals(CHART_HEIGHT, box.height);
        assertTrue(html.contains("style=\"height: " + CHART_HEIGHT + "px\""));
        assertTrue(ReportImageStore.countFor(uuid) >= 1);
    }

    @Test
    public void nestedFrameElementIsExportedAlone() throws Exception {
        assertEquals(0, topLevelPrint("nested-note"));
        assertEquals(1, EmbedComponentExporter.printElements(print, "nested-note").size());
        String html = EmbedComponentExporter.exportHtml(report, print, "nested-note", null);
        assertTrue(html.contains("nested"));
        assertOnlyThisComponent(html, "nested-note");
    }

    @Test
    public void unknownIdIsNotFound() {
        assertEquals(0, EmbedComponentExporter.designElements(report, "not-a-component").size());
        EmbedComponentExporter.ComponentExportException err = exportError("not-a-component");
        assertEquals(EmbedComponentExporter.STATUS_NOT_FOUND, err.status());
        assertEquals(EmbedComponentExporter.CODE_NOT_FOUND, err.code());
    }

    @Test
    public void suppressedElementIsNotPrinted() {
        assertEquals(1, EmbedComponentExporter.designElements(report, "hidden-units").size());
        assertEquals(0, EmbedComponentExporter.printElements(print, "hidden-units").size());
        EmbedComponentExporter.ComponentExportException hidden = exportError("hidden-units");
        assertEquals(EmbedComponentExporter.STATUS_NOT_FOUND, hidden.status());
        assertEquals(EmbedComponentExporter.CODE_NOT_PRINTED, hidden.code());

        assertEquals(1, EmbedComponentExporter.designElements(report, "absent-band").size());
        assertEquals(0, EmbedComponentExporter.printElements(print, "absent-band").size());
        EmbedComponentExporter.ComponentExportException absent = exportError("absent-band");
        assertEquals(EmbedComponentExporter.CODE_NOT_PRINTED, absent.code());
    }

    @Test
    public void repeatedElementIsAmbiguous() {
        assertEquals(1, EmbedComponentExporter.designElements(report, "month-row").size());
        assertEquals(3, EmbedComponentExporter.printElements(print, "month-row").size());
        EmbedComponentExporter.ComponentExportException err = exportError("month-row");
        assertEquals(EmbedComponentExporter.STATUS_AMBIGUOUS, err.status());
        assertEquals(EmbedComponentExporter.CODE_AMBIGUOUS, err.code());
    }

    @Test
    public void imageAndStaticTextAreUnsupported() {
        JRElement image = onlyDesign("logo-mark");
        assertFalse(image instanceof JRChart);
        assertFalse(image instanceof JRTextField);
        JRPrintElement printedImage = onlyPrint("logo-mark");
        assertTrue("print type " + printedImage.getClass().getName(), printedImage instanceof JRPrintImage);
        assertFalse(printedImage instanceof JRPrintText);
        EmbedComponentExporter.ComponentExportException imageErr = exportError("logo-mark");
        assertEquals(EmbedComponentExporter.STATUS_UNSUPPORTED, imageErr.status());
        assertEquals(EmbedComponentExporter.CODE_UNSUPPORTED, imageErr.code());

        JRElement label = onlyDesign("static-label");
        assertFalse(label instanceof JRTextField);
        JRPrintElement printedLabel = onlyPrint("static-label");
        assertTrue(printedLabel instanceof JRPrintText);
        EmbedComponentExporter.ComponentExportException labelErr = exportError("static-label");
        assertEquals(EmbedComponentExporter.STATUS_UNSUPPORTED, labelErr.status());
        assertEquals(EmbedComponentExporter.CODE_UNSUPPORTED, labelErr.code());
    }

    @Test
    public void componentModeRequiresTheGatewayHeader() {
        assertFalse(EmbedComponentExporter.componentMode((String) null));
        assertFalse(EmbedComponentExporter.componentMode(""));
        assertFalse(EmbedComponentExporter.componentMode("true"));
        assertFalse(EmbedComponentExporter.componentMode("0"));
        assertTrue(EmbedComponentExporter.componentMode(EmbedComponentExporter.HEADER_VALUE));
        assertFalse(EmbedComponentExporter.componentMode((HttpServletRequest) null));

        assertNull(EmbedComponentExporter.trustedComponentId((String) null, FIELD_ID));
        assertNull(EmbedComponentExporter.trustedComponentId("", FIELD_ID));
        assertNull(EmbedComponentExporter.trustedComponentId("true", FIELD_ID));
        assertNull(EmbedComponentExporter.trustedComponentId(EmbedComponentExporter.HEADER_VALUE, null));
        assertNull(EmbedComponentExporter.trustedComponentId(EmbedComponentExporter.HEADER_VALUE, ""));
        assertEquals(FIELD_ID, EmbedComponentExporter.trustedComponentId(EmbedComponentExporter.HEADER_VALUE, FIELD_ID));
        assertEquals("Monthly Occupancy", EmbedComponentExporter.trustedComponentId(
                EmbedComponentExporter.HEADER_VALUE, "Monthly Occupancy"));
        assertNull(EmbedComponentExporter.trustedComponentId((HttpServletRequest) null));
    }

    private static void assertOnlyThisComponent(String html, String embedId) {
        String[] others = {
            "acct=A-17", "should-not-print", ">decoy<", ">Static<", "id=\"hidden-units\"",
            "id=\"cover-label\"", "id=\"month-row\"", "id=\"logo-mark\"", "id=\"static-label\"",
            "id=\"notes-frame\"", "id=\"decoy-key\""
        };
        for (String other : others) {
            if (html.contains(other)) {
                fail("component " + embedId + " HTML contains unrelated text \"" + other + "\":\n" + html);
            }
        }
        if (!"nested-note".equals(embedId)) {
            assertFalse(html.contains("id=\"nested-note\""));
            assertFalse(html.contains(">nested<"));
        }
        if (!FIELD_ID.equals(embedId)) {
            assertFalse(html.contains("id=\"" + FIELD_ID + "\""));
            assertFalse(html.contains("1,234"));
        }
        if (!CHART_ID.equals(embedId)) {
            assertFalse(html.contains("id=\"" + CHART_ID + "\""));
            assertFalse(html.contains("acugis_report_image"));
        }
    }

    private static EmbedComponentExporter.ComponentExportException exportError(String embedId) {
        try {
            EmbedComponentExporter.exportHtml(report, print, embedId, null);
            fail("exported " + embedId);
            return null;
        } catch (EmbedComponentExporter.ComponentExportException ex) {
            return ex;
        } catch (JRException ex) {
            fail(ex.getMessage());
            return null;
        }
    }

    private static JRElement onlyDesign(String embedId) {
        List<JRElement> hits = EmbedComponentExporter.designElements(report, embedId);
        assertEquals(embedId, 1, hits.size());
        return hits.get(0);
    }

    private static JRPrintElement onlyPrint(String embedId) {
        List<JRPrintElement> hits = EmbedComponentExporter.printElements(print, embedId);
        assertEquals(embedId, 1, hits.size());
        return hits.get(0);
    }

    private static int topLevelPrint(String embedId) {
        int count = 0;
        for (net.sf.jasperreports.engine.JRPrintPage page : print.getPages()) {
            for (JRPrintElement element : page.getElements()) {
                if (embedId.equals(element.getKey())) {
                    count++;
                }
            }
        }
        return count;
    }

    private static String snippet(String html, String embedId) {
        int at = html.toLowerCase(Locale.US).indexOf("id=\"" + embedId + "\"");
        if (at < 0) {
            return html;
        }
        int from = Math.max(0, at - 500);
        int to = Math.min(html.length(), at + 700);
        return html.substring(from, to);
    }

    private static Dimension pageBox(String html) {
        Matcher width = Pattern.compile("class=\"jrPage\"[^>]*style=\"[^\"]*width:\\s*(\\d+)px").matcher(html);
        Matcher height = Pattern.compile("data-jr-height=\"(\\d+)\"").matcher(html);
        if (!width.find() || !height.find()) {
            fail("page box not found in HTML:\n" + html);
        }
        Dimension box = new Dimension();
        box.width = Integer.parseInt(width.group(1));
        box.height = Integer.parseInt(height.group(1));
        return box;
    }

    private static String imageKind(byte[] bytes) {
        if (bytes.length >= 8 && (bytes[0] & 0xFF) == 0x89 && bytes[1] == 'P' && bytes[2] == 'N' && bytes[3] == 'G') {
            return "png";
        }
        String head = new String(bytes, 0, Math.min(bytes.length, 200), StandardCharsets.UTF_8).toLowerCase(Locale.US);
        if (head.contains("<svg")) {
            return "svg";
        }
        return "other";
    }

    private static int distinctColors(BufferedImage image) {
        Map<Integer, Boolean> colors = new LinkedHashMap<Integer, Boolean>();
        int stepX = Math.max(1, image.getWidth() / 40);
        int stepY = Math.max(1, image.getHeight() / 40);
        for (int y = 0; y < image.getHeight(); y += stepY) {
            for (int x = 0; x < image.getWidth(); x += stepX) {
                colors.put(image.getRGB(x, y), Boolean.TRUE);
                if (colors.size() >= 8) {
                    return colors.size();
                }
            }
        }
        return colors.size();
    }

    private static Map<String, Object> row(String month, int units) {
        Map<String, Object> row = new HashMap<String, Object>();
        row.put("month", month);
        row.put("units", Integer.valueOf(units));
        return row;
    }

    private static String fixtureJrxml() {
        return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
                + "<jasperReport name=\"embed_component_export\" language=\"java\" pageWidth=\"360\" pageHeight=\"280\" "
                + "columnWidth=\"360\" leftMargin=\"0\" rightMargin=\"0\" topMargin=\"0\" bottomMargin=\"0\" "
                + "summaryNewPage=\"true\" uuid=\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\">\n"
                + "  <parameter name=\"account_id\" class=\"java.lang.String\"/>\n"
                + "  <field name=\"month\" class=\"java.lang.String\"/>\n"
                + "  <field name=\"units\" class=\"java.lang.Integer\"/>\n"
                + "  <variable name=\"TOTAL_UNITS\" class=\"java.lang.Integer\" calculation=\"Sum\">\n"
                + "    <expression><![CDATA[$F{units}]]></expression>\n"
                + "  </variable>\n"
                + "  <group name=\"never\">\n"
                + "    <expression><![CDATA[\"constant\"]]></expression>\n"
                + "    <groupHeader>\n"
                + "      <band height=\"16\">\n"
                + "        <printWhenExpression><![CDATA[Boolean.FALSE]]></printWhenExpression>\n"
                + "        <element kind=\"textField\" key=\"absent-band\" uuid=\"33333333-3333-4333-8333-333333333333\" x=\"0\" y=\"0\" width=\"120\" height=\"14\">\n"
                + "          <property name=\"net.sf.jasperreports.export.html.id\" value=\"absent-band\"/>\n"
                + "          <expression><![CDATA[\"should-not-print\"]]></expression>\n"
                + "        </element>\n"
                + "      </band>\n"
                + "    </groupHeader>\n"
                + "  </group>\n"
                + "  <title height=\"56\">\n"
                + "    <element kind=\"textField\" key=\"cover-label\" uuid=\"44444444-4444-4444-8444-444444444444\" x=\"0\" y=\"0\" width=\"180\" height=\"16\">\n"
                + "      <expression><![CDATA[\"acct=\" + $P{account_id}]]></expression>\n"
                + "    </element>\n"
                + "    <element kind=\"textField\" key=\"hidden-units\" uuid=\"55555555-5555-4555-8555-555555555555\" x=\"0\" y=\"18\" width=\"120\" height=\"14\">\n"
                + "      <property name=\"net.sf.jasperreports.export.html.id\" value=\"hidden-units\"/>\n"
                + "      <printWhenExpression><![CDATA[Boolean.FALSE]]></printWhenExpression>\n"
                + "      <expression><![CDATA[\"hidden\"]]></expression>\n"
                + "    </element>\n"
                + "    <element kind=\"image\" key=\"logo-mark\" uuid=\"abababab-abab-4aba-8aba-abababababab\" x=\"190\" y=\"0\" width=\"48\" height=\"24\" onErrorType=\"Icon\">\n"
                + "      <expression><![CDATA[new java.awt.image.BufferedImage(48, 24, java.awt.image.BufferedImage.TYPE_INT_RGB)]]></expression>\n"
                + "    </element>\n"
                + "    <element kind=\"staticText\" key=\"static-label\" uuid=\"cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd\" x=\"190\" y=\"28\" width=\"90\" height=\"16\">\n"
                + "      <text><![CDATA[Static]]></text>\n"
                + "    </element>\n"
                + "  </title>\n"
                + "  <detail>\n"
                + "    <band height=\"16\">\n"
                + "      <element kind=\"textField\" key=\"month-row\" uuid=\"66666666-6666-4666-8666-666666666666\" x=\"0\" y=\"0\" width=\"80\" height=\"14\">\n"
                + "        <expression><![CDATA[$F{month}]]></expression>\n"
                + "      </element>\n"
                + "    </band>\n"
                + "  </detail>\n"
                + "  <summary height=\"270\">\n"
                + "    <element kind=\"textField\" key=\"total-units\" uuid=\"11111111-1111-4111-8111-111111111111\" mode=\"Opaque\" x=\"10\" y=\"8\" width=\"" + FIELD_WIDTH + "\" height=\"" + FIELD_HEIGHT + "\" forecolor=\"#123456\" backcolor=\"#FFF4E5\" fontName=\"SansSerif\" fontSize=\"16.0\" bold=\"true\" hTextAlign=\"Right\" vTextAlign=\"Middle\" pattern=\"#,##0\">\n"
                + "      <property name=\"net.sf.jasperreports.export.html.id\" value=\"total-units\"/>\n"
                + "      <box topPadding=\"4\" leftPadding=\"6\" bottomPadding=\"4\" rightPadding=\"6\">\n"
                + "        <pen lineWidth=\"1.0\" lineColor=\"#112233\"/>\n"
                + "      </box>\n"
                + "      <expression><![CDATA[$V{TOTAL_UNITS}]]></expression>\n"
                + "    </element>\n"
                + "    <element kind=\"textField\" key=\"decoy-key\" uuid=\"77777777-7777-4777-8777-777777777777\" x=\"10\" y=\"40\" width=\"80\" height=\"14\">\n"
                + "      <property name=\"net.sf.jasperreports.export.html.id\" value=\"total-units\"/>\n"
                + "      <expression><![CDATA[\"decoy\"]]></expression>\n"
                + "    </element>\n"
                + "    <element kind=\"chart\" key=\"monthly-occupancy\" uuid=\"22222222-2222-4222-8222-222222222222\" x=\"10\" y=\"60\" width=\"" + CHART_WIDTH + "\" height=\"" + CHART_HEIGHT + "\" chartType=\"bar\" evaluationTime=\"Report\">\n"
                + "      <property name=\"net.sf.jasperreports.export.html.id\" value=\"monthly-occupancy\"/>\n"
                + "      <dataset kind=\"category\">\n"
                + "        <series>\n"
                + "          <seriesExpression><![CDATA[\"Occupancy\"]]></seriesExpression>\n"
                + "          <categoryExpression><![CDATA[$F{month}]]></categoryExpression>\n"
                + "          <valueExpression><![CDATA[$F{units}]]></valueExpression>\n"
                + "        </series>\n"
                + "      </dataset>\n"
                + "      <plot/>\n"
                + "    </element>\n"
                + "    <element kind=\"frame\" key=\"notes-frame\" uuid=\"88888888-8888-4888-8888-888888888888\" x=\"10\" y=\"246\" width=\"200\" height=\"20\">\n"
                + "      <element kind=\"textField\" key=\"nested-note\" uuid=\"99999999-9999-4999-8999-999999999999\" x=\"2\" y=\"2\" width=\"160\" height=\"14\">\n"
                + "        <property name=\"net.sf.jasperreports.export.html.id\" value=\"nested-note\"/>\n"
                + "        <expression><![CDATA[\"nested\"]]></expression>\n"
                + "      </element>\n"
                + "    </element>\n"
                + "  </summary>\n"
                + "</jasperReport>\n";
    }

    private static final class Dimension {
        int width;
        int height;
    }
}
