package com.acugis.reports;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertTrue;
import static org.junit.Assert.fail;

import java.awt.image.BufferedImage;
import java.io.ByteArrayInputStream;
import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import javax.imageio.ImageIO;

import org.junit.BeforeClass;
import org.junit.Test;

import net.sf.jasperreports.engine.DefaultJasperReportsContext;
import net.sf.jasperreports.engine.JRElement;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JRPrintElement;
import net.sf.jasperreports.engine.JRPrintImage;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.data.JRMapCollectionDataSource;

/**
 * Map elements use the same component export as charts. Classification is the
 * design property {@code acugis.component.type=map}. An ordinary image with a
 * key is still rejected.
 */
public class EmbedComponentMapExportTest {

    static final String MAP_ID = "property-map";
    static final String REGIONAL_ID = "regional-map";
    static final int MAP_WIDTH = 400;
    static final int MAP_HEIGHT = 300;
    static final int REGIONAL_WIDTH = 180;
    static final int REGIONAL_HEIGHT = 120;

    private static JasperReport report;
    private static JasperPrint print;
    private static File outDir;

    @BeforeClass
    public static void fillOnce() throws Exception {
        System.setProperty("java.awt.headless", "true");
        System.setProperty("net.sf.jasperreports.awt.ignore.missing.font", "true");
        DefaultJasperReportsContext.getInstance().setProperty(
                "net.sf.jasperreports.awt.ignore.missing.font", "true");
        DefaultJasperReportsContext.getInstance().setProperty(
                "net.sf.jasperreports.compiler.class",
                "net.sf.jasperreports.jdt.JRJdtCompiler");

        outDir = new File("target/embed-component-map");
        if (!outDir.isDirectory() && !outDir.mkdirs()) {
            throw new IllegalStateException("cannot create " + outDir);
        }
        File jrxml = new File(outDir, "embed-component-map.jrxml");
        Files.write(jrxml.toPath(), fixtureJrxml().getBytes(StandardCharsets.UTF_8));

        report = ReportParameterBinder.loadOrCompileReport(jrxml);
        Map<String, Object> params = new HashMap<String, Object>();
        params.put("account_id", "A-17");
        params.put(net.sf.jasperreports.engine.JRParameter.REPORT_LOCALE, Locale.US);
        List<Map<String, ?>> rows = new ArrayList<Map<String, ?>>();
        rows.add(row("Jan"));
        rows.add(row("Feb"));
        print = ReportService.fillReport(
                report,
                params,
                new JRMapCollectionDataSource(rows),
                outDir,
                "embed-component-map",
                null);
        ReportService.endChartRenderSession();
        assertNotNull(print);
    }

    @Test
    public void configuredMapExportsThroughTheReportImageStore() throws Exception {
        JRElement design = onlyDesign(MAP_ID);
        assertTrue(design instanceof net.sf.jasperreports.engine.JRImage);
        assertFalse(design instanceof net.sf.jasperreports.charts.JRChart);
        assertTrue(EmbedComponentExporter.markedMapImage(design));
        assertEquals(MAP_ID, design.getPropertiesMap().getProperty("net.sf.jasperreports.export.html.id"));
        assertEquals(EmbedComponentExporter.MAP_TYPE_VALUE,
                design.getPropertiesMap().getProperty(EmbedComponentExporter.MAP_TYPE_PROPERTY));

        JRPrintElement printed = onlyPrint(MAP_ID);
        assertTrue(printed instanceof JRPrintImage);
        assertEquals("net.sf.jasperreports.engine.fill.JRTemplatePrintImage", printed.getClass().getName());
        assertEquals(MAP_WIDTH, printed.getWidth());
        assertEquals(MAP_HEIGHT, printed.getHeight());

        String html = EmbedComponentExporter.exportHtml(report, print, MAP_ID, null);
        Files.write(new File(outDir, "property-map.html").toPath(), html.getBytes(StandardCharsets.UTF_8));
        assertTrue(html.contains("id=\"" + MAP_ID + "\""));
        assertFalse(html.contains("id=\"" + REGIONAL_ID + "\""));
        assertFalse(html.contains("id=\"logo-mark\""));
        assertFalse(html.contains("id=\"total-units\""));
        assertFalse(html.contains("A-17"));
        assertTrue(html.toLowerCase(Locale.US).contains("border: 1px"));
        assertTrue(html.toUpperCase(Locale.US).contains("#4A7C4A"));
        Dimension box = pageBox(html);
        assertEquals(MAP_WIDTH, box.width);
        assertEquals(MAP_HEIGHT, box.height);

        StoredImage stored = storedImage(html);
        assertEquals("png", imageKind(stored.bytes));
        BufferedImage raster = ImageIO.read(new ByteArrayInputStream(stored.bytes));
        assertNotNull(raster);
        assertEquals(MAP_WIDTH, raster.getWidth());
        assertEquals(MAP_HEIGHT, raster.getHeight());
        assertTrue(ReportImageStore.countFor(stored.uuid) >= 1);
    }

    @Test
    public void twoMapsExportIndependently() throws Exception {
        String property = EmbedComponentExporter.exportHtml(report, print, MAP_ID, null);
        String regional = EmbedComponentExporter.exportHtml(report, print, REGIONAL_ID, null);
        assertTrue(regional.contains("id=\"" + REGIONAL_ID + "\""));
        assertFalse(regional.contains("id=\"" + MAP_ID + "\""));
        StoredImage propertyImage = storedImage(property);
        StoredImage regionalImage = storedImage(regional);
        assertFalse(propertyImage.uuid.equals(regionalImage.uuid) && propertyImage.name.equals(regionalImage.name));

        BufferedImage propertyRaster = ImageIO.read(new ByteArrayInputStream(propertyImage.bytes));
        BufferedImage regionalRaster = ImageIO.read(new ByteArrayInputStream(regionalImage.bytes));
        assertEquals(MAP_WIDTH, propertyRaster.getWidth());
        assertEquals(MAP_HEIGHT, propertyRaster.getHeight());
        assertEquals(REGIONAL_WIDTH, regionalRaster.getWidth());
        assertEquals(REGIONAL_HEIGHT, regionalRaster.getHeight());

        byte[] propertyAgain = ReportImageStore.get(propertyImage.uuid, propertyImage.name);
        assertEquals(propertyImage.bytes.length, propertyAgain.length);
        Dimension regionalBox = pageBox(regional);
        assertEquals(REGIONAL_WIDTH, regionalBox.width);
        assertEquals(REGIONAL_HEIGHT, regionalBox.height);
    }

    @Test
    public void ordinaryImageAndPlaceholderAreUnsupported() {
        JRElement logo = onlyDesign("logo-mark");
        assertTrue(logo instanceof net.sf.jasperreports.engine.JRImage);
        assertFalse(EmbedComponentExporter.markedMapImage(logo));
        assertTrue(onlyPrint("logo-mark") instanceof JRPrintImage);
        EmbedComponentExporter.ComponentExportException logoErr = exportError("logo-mark");
        assertEquals(EmbedComponentExporter.STATUS_UNSUPPORTED, logoErr.status());
        assertEquals(EmbedComponentExporter.CODE_UNSUPPORTED, logoErr.code());

        JRElement wrong = onlyDesign("wrong-type");
        assertTrue(wrong instanceof net.sf.jasperreports.engine.JRImage);
        assertEquals("image", wrong.getPropertiesMap().getProperty(EmbedComponentExporter.MAP_TYPE_PROPERTY));
        assertFalse(EmbedComponentExporter.markedMapImage(wrong));
        assertEquals(EmbedComponentExporter.CODE_UNSUPPORTED, exportError("wrong-type").code());

        JRElement placeholder = onlyDesign("placeholder-map");
        assertFalse(placeholder instanceof net.sf.jasperreports.engine.JRImage);
        assertFalse(EmbedComponentExporter.markedMapImage(placeholder));
        assertEquals(EmbedComponentExporter.CODE_UNSUPPORTED, exportError("placeholder-map").code());
    }

    @Test
    public void mapThatDoesNotPrintIsNotPrinted() {
        assertEquals(1, EmbedComponentExporter.designElements(report, "hidden-map").size());
        assertTrue(EmbedComponentExporter.markedMapImage(onlyDesign("hidden-map")));
        assertEquals(0, EmbedComponentExporter.printElements(print, "hidden-map").size());
        EmbedComponentExporter.ComponentExportException err = exportError("hidden-map");
        assertEquals(EmbedComponentExporter.STATUS_NOT_FOUND, err.status());
        assertEquals(EmbedComponentExporter.CODE_NOT_PRINTED, err.code());
    }

    @Test
    public void repeatingMapIsAmbiguous() {
        assertEquals(1, EmbedComponentExporter.designElements(report, "repeating-map").size());
        assertEquals(2, EmbedComponentExporter.printElements(print, "repeating-map").size());
        EmbedComponentExporter.ComponentExportException err = exportError("repeating-map");
        assertEquals(EmbedComponentExporter.STATUS_AMBIGUOUS, err.status());
        assertEquals(EmbedComponentExporter.CODE_AMBIGUOUS, err.code());
    }

    @Test
    public void missingMapIsNotFound() {
        assertEquals(0, EmbedComponentExporter.designElements(report, "not-a-map").size());
        EmbedComponentExporter.ComponentExportException err = exportError("not-a-map");
        assertEquals(EmbedComponentExporter.STATUS_NOT_FOUND, err.status());
        assertEquals(EmbedComponentExporter.CODE_NOT_FOUND, err.code());
    }

    @Test
    public void fieldBesideAMapStillExports() throws Exception {
        JRElement design = onlyDesign("total-units");
        assertTrue(design instanceof net.sf.jasperreports.engine.JRTextField);
        assertFalse(EmbedComponentExporter.markedMapImage(design));
        String html = EmbedComponentExporter.exportHtml(report, print, "total-units", null);
        assertTrue(html.contains("A-17"));
        assertTrue(html.contains("id=\"total-units\""));
        assertFalse(html.contains("acugis_report_image"));
        assertFalse(html.contains("id=\"" + MAP_ID + "\""));
    }

    private static StoredImage storedImage(String html) throws Exception {
        Matcher src = Pattern.compile("acugis_report_image\\?image=([^\"&]+)&(?:amp;)?uuid=([^\"&]+)").matcher(html);
        assertTrue("map HTML has no image URI:\n" + html, src.find());
        StoredImage stored = new StoredImage();
        stored.name = java.net.URLDecoder.decode(src.group(1), "UTF-8");
        stored.uuid = java.net.URLDecoder.decode(src.group(2), "UTF-8");
        stored.bytes = ReportImageStore.get(stored.uuid, stored.name);
        assertNotNull(stored.bytes);
        assertTrue(stored.bytes.length > 8);
        return stored;
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
        return "other";
    }

    private static Map<String, Object> row(String month) {
        Map<String, Object> row = new HashMap<String, Object>();
        row.put("month", month);
        return row;
    }

    private static String imageElement(String key, String uuid, int x, int y, int width, int height, String marker, String printWhen) {
        StringBuilder xml = new StringBuilder();
        xml.append("        <element kind=\"image\" key=\"").append(key).append("\" uuid=\"").append(uuid)
                .append("\" x=\"").append(x).append("\" y=\"").append(y)
                .append("\" width=\"").append(width).append("\" height=\"").append(height)
                .append("\" scaleImage=\"FillFrame\" onErrorType=\"Icon\">\n");
        xml.append("          <property name=\"net.sf.jasperreports.export.html.id\" value=\"").append(key).append("\"/>\n");
        if (marker != null) {
            xml.append("          <property name=\"acugis.component.type\" value=\"").append(marker).append("\"/>\n");
        }
        if ("property-map".equals(key)) {
            xml.append("          <pen lineWidth=\"1.0\" lineColor=\"#4A7C4A\" lineStyle=\"Solid\"/>\n");
        }
        if (printWhen != null) {
            xml.append("          <printWhenExpression><![CDATA[").append(printWhen).append("]]></printWhenExpression>\n");
        }
        xml.append("          <expression><![CDATA[new java.awt.image.BufferedImage(")
                .append(width).append(", ").append(height)
                .append(", java.awt.image.BufferedImage.TYPE_INT_ARGB)]]></expression>\n");
        xml.append("        </element>\n");
        return xml.toString();
    }

    private static String fixtureJrxml() {
        return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
                + "<jasperReport name=\"embed_component_map\" language=\"java\" pageWidth=\"420\" pageHeight=\"520\" "
                + "columnWidth=\"420\" leftMargin=\"0\" rightMargin=\"0\" topMargin=\"0\" bottomMargin=\"0\" "
                + "uuid=\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab\">\n"
                + "  <parameter name=\"account_id\" class=\"java.lang.String\"/>\n"
                + "  <field name=\"month\" class=\"java.lang.String\"/>\n"
                + "  <title height=\"70\">\n"
                + "    <element kind=\"textField\" key=\"total-units\" uuid=\"11111111-1111-4111-8111-111111111111\" x=\"0\" y=\"0\" width=\"120\" height=\"16\">\n"
                + "      <property name=\"net.sf.jasperreports.export.html.id\" value=\"total-units\"/>\n"
                + "      <expression><![CDATA[$P{account_id}]]></expression>\n"
                + "    </element>\n"
                + "    <element kind=\"frame\" key=\"placeholder-map\" uuid=\"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb\" x=\"130\" y=\"0\" width=\"80\" height=\"24\">\n"
                + "      <property name=\"net.sf.jasperreports.export.html.id\" value=\"placeholder-map\"/>\n"
                + "      <element kind=\"staticText\" uuid=\"cccccccc-cccc-4ccc-8ccc-cccccccccccc\" x=\"2\" y=\"2\" width=\"70\" height=\"14\">\n"
                + "        <text><![CDATA[MAP]]></text>\n"
                + "      </element>\n"
                + "    </element>\n"
                + imageElement("logo-mark", "dddddddd-dddd-4ddd-8ddd-dddddddddddd", 220, 0, 48, 24, null, null)
                + imageElement("wrong-type", "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", 280, 0, 40, 20, "image", null)
                + imageElement("hidden-map", "ffffffff-ffff-4fff-8fff-ffffffffffff", 330, 28, 40, 20, "map", "Boolean.FALSE")
                + "  </title>\n"
                + "  <detail>\n"
                + "    <band height=\"20\">\n"
                + imageElement("repeating-map", "12121212-1212-4121-8121-121212121212", 0, 0, 80, 16, "map", null)
                + "    </band>\n"
                + "  </detail>\n"
                + "  <summary height=\"440\">\n"
                + imageElement(MAP_ID, "22222222-2222-4222-8222-222222222222", 0, 0, MAP_WIDTH, MAP_HEIGHT, "map", null)
                + imageElement(REGIONAL_ID, "33333333-3333-4333-8333-333333333333", 0, 310, REGIONAL_WIDTH, REGIONAL_HEIGHT, "map", null)
                + "  </summary>\n"
                + "</jasperReport>\n";
    }

    private static final class StoredImage {
        String uuid;
        String name;
        byte[] bytes;
    }

    private static final class Dimension {
        int width;
        int height;
    }
}
