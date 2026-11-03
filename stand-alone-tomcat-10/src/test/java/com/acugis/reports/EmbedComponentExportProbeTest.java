package com.acugis.reports;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertTrue;
import static org.junit.Assert.fail;

import java.awt.Color;
import java.awt.image.BufferedImage;
import java.io.ByteArrayInputStream;
import java.io.File;
import java.io.StringWriter;
import java.lang.reflect.Field;
import java.lang.reflect.Modifier;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.IdentityHashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.UUID;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import javax.imageio.ImageIO;

import org.junit.BeforeClass;
import org.junit.Test;

import net.sf.jasperreports.engine.DefaultJasperReportsContext;
import net.sf.jasperreports.engine.JRElement;
import net.sf.jasperreports.engine.JRElementGroup;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JRGroup;
import net.sf.jasperreports.engine.JRPrintElement;
import net.sf.jasperreports.engine.JRPrintFrame;
import net.sf.jasperreports.engine.JRPrintImage;
import net.sf.jasperreports.engine.JRPrintPage;
import net.sf.jasperreports.engine.JRPrintText;
import net.sf.jasperreports.engine.JRPropertiesUtil;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.base.JRBasePrintPage;
import net.sf.jasperreports.engine.data.JRMapCollectionDataSource;
import net.sf.jasperreports.engine.export.HtmlExporter;
import net.sf.jasperreports.engine.fill.JRTemplatePrintElement;
import net.sf.jasperreports.engine.type.HorizontalTextAlignEnum;
import net.sf.jasperreports.export.SimpleExporterInput;
import net.sf.jasperreports.export.SimpleHtmlExporterOutput;
import net.sf.jasperreports.export.SimpleHtmlReportConfiguration;

/**
 * Throwaway audit probe for component-level embedding.
 *
 * Fills one JR 7.0.1 report through {@link ReportService#fillReport} (the same
 * fill context Report Server uses) and exports a single printed element with
 * the same {@link HtmlExporter} and {@link CapturingHtmlResourceHandler} the
 * HTML pipeline uses. This is not a public route.
 */
public class EmbedComponentExportProbeTest {

    static final String HTML_ID = HtmlExporter.PROPERTY_HTML_ID;
    static final String FIELD_ID = "total-units";
    static final String CHART_ID = "monthly-occupancy";
    static final String FIELD_UUID = "11111111-1111-4111-8111-111111111111";
    static final String CHART_UUID = "22222222-2222-4222-8222-222222222222";
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

        File dir = new File("target/embed-component-probe");
        if (!dir.isDirectory() && !dir.mkdirs()) {
            throw new IllegalStateException("cannot create " + dir);
        }
        File jrxml = new File(dir, "embed-component-probe.jrxml");
        Files.write(jrxml.toPath(), probeJrxml().getBytes(StandardCharsets.UTF_8));

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
                "embed-component-probe",
                null);
        ReportService.endChartRenderSession();
        assertNotNull(print);
        System.out.println("PROBE pages=" + print.getPages().size()
                + " field=" + describe(only(findByKey(print, FIELD_ID)))
                + " chart=" + describe(only(findByKey(print, CHART_ID))));
    }

    @Test
    public void embedIdsExistOnTheCompiledReportBeforeFill() {
        JRElement field = onlyDesign(FIELD_ID);
        JRElement chart = onlyDesign(CHART_ID);
        assertEquals(FIELD_ID, field.getKey());
        assertEquals(CHART_ID, chart.getKey());
        assertEquals(FIELD_UUID, field.getUUID().toString());
        assertEquals(CHART_UUID, chart.getUUID().toString());
        assertFalse(FIELD_ID.equals(field.getUUID().toString()));
        assertEquals(FIELD_ID, field.getPropertiesMap().getProperty(HTML_ID));
        assertEquals(CHART_ID, chart.getPropertiesMap().getProperty(HTML_ID));
        assertEquals(FIELD_WIDTH, field.getWidth());
        assertEquals(FIELD_HEIGHT, field.getHeight());
        assertEquals(CHART_WIDTH, chart.getWidth());
        assertEquals(CHART_HEIGHT, chart.getHeight());
        assertEquals("net.sf.jasperreports.engine.base.JRBaseTextField", field.getClass().getName());
        assertEquals("net.sf.jasperreports.charts.base.JRBaseChart", chart.getClass().getName());
    }

    @Test
    public void embedIdsSurviveFillOnThePrintElement() {
        JRPrintElement field = only(findByKey(print, FIELD_ID));
        JRPrintElement chart = only(findByKey(print, CHART_ID));

        assertEquals(FIELD_ID, field.getKey());
        assertEquals(CHART_ID, chart.getKey());
        assertEquals(UUID.fromString(FIELD_UUID), field.getUUID());
        assertEquals(UUID.fromString(CHART_UUID), chart.getUUID());
        assertEquals(FIELD_ID, htmlId(field));
        assertEquals(CHART_ID, htmlId(chart));

        // JRTemplatePrintElement.getParentProperties() is null. The HTML id is
        // copied onto the fill template and exposed as the base of the print
        // element's property map. It is not an own property of the print element.
        assertTrue(field instanceof JRTemplatePrintElement);
        assertTrue(chart instanceof JRTemplatePrintElement);
        assertEquals(null, field.getParentProperties());
        assertEquals(null, chart.getParentProperties());
        assertEquals(FIELD_ID, ((JRTemplatePrintElement) field).getTemplate().getKey());
        assertEquals(CHART_ID, ((JRTemplatePrintElement) chart).getTemplate().getKey());
        assertNoOwnProperty(field, HTML_ID);
        assertNoOwnProperty(chart, HTML_ID);
        assertEquals(FIELD_ID, field.getPropertiesMap().getBaseProperties().getProperty(HTML_ID));
        assertEquals(CHART_ID, chart.getPropertiesMap().getBaseProperties().getProperty(HTML_ID));
        assertEquals(FIELD_ID, ((JRTemplatePrintElement) field).getTemplate().getPropertiesMap().getProperty(HTML_ID));
        assertEquals(CHART_ID, ((JRTemplatePrintElement) chart).getTemplate().getPropertiesMap().getProperty(HTML_ID));

        System.out.println("PROBE fieldClass=" + field.getClass().getName()
                + " template=" + ((JRTemplatePrintElement) field).getTemplate().getClass().getName()
                + " chartClass=" + chart.getClass().getName()
                + " chartTemplate=" + ((JRTemplatePrintElement) chart).getTemplate().getClass().getName()
                + " chartRenderer=" + ((JRPrintImage) chart).getRenderer().getClass().getName());
    }

    @Test
    public void fieldIsOnePrintTextWithFillFormatting() {
        JRPrintElement element = only(findByKey(print, FIELD_ID));
        assertTrue("print type " + element.getClass().getName(), element instanceof JRPrintText);
        assertFalse(element instanceof JRPrintImage);
        JRPrintText text = (JRPrintText) element;
        assertEquals("1,234", text.getFullText());
        assertEquals(Integer.valueOf(1234), text.getValue());
        assertEquals("#,##0", text.getPattern());
        // JRXML asks for SansSerif. The server font extension resolves that family
        // onto the print element as DejaVu Sans. Export uses this resolved name.
        assertEquals("DejaVu Sans", text.getFontName());
        assertEquals(16f, text.getFontSize(), 0.01f);
        assertTrue(text.isBold());
        assertEquals(HorizontalTextAlignEnum.RIGHT, text.getHorizontalTextAlign());
        assertEquals(new Color(0x12, 0x34, 0x56), text.getForecolor());
        assertEquals(new Color(0xFF, 0xF4, 0xE5), text.getBackcolor());
        assertEquals(Integer.valueOf(4), text.getLineBox().getTopPadding());
        assertEquals(Integer.valueOf(6), text.getLineBox().getLeftPadding());
        assertEquals(Integer.valueOf(4), text.getLineBox().getBottomPadding());
        assertEquals(Integer.valueOf(6), text.getLineBox().getRightPadding());
        assertTrue(text.getLineBox().getPen().getLineWidth().floatValue() > 0f);
        assertEquals(FIELD_WIDTH, text.getWidth());
        assertEquals(FIELD_HEIGHT, text.getHeight());
    }

    @Test
    public void chartIsOnePrintImageWhoseSeriesWereCollectedDuringFill() throws Exception {
        List<JRPrintElement> matches = findByKey(print, CHART_ID);
        assertEquals(1, matches.size());
        JRPrintElement element = matches.get(0);
        assertTrue("print type " + element.getClass().getName(), element instanceof JRPrintImage);
        assertFalse(element instanceof JRPrintText);
        assertFalse(element instanceof JRPrintFrame);
        JRPrintImage image = (JRPrintImage) element;
        assertNotNull(image.getRenderer());
        assertEquals(CHART_WIDTH, image.getWidth());
        assertEquals(CHART_HEIGHT, image.getHeight());

        Object chart = findNested(image.getRenderer(), "org.jfree.chart.JFreeChart", 0, new IdentityHashMap<Object, Boolean>());
        assertNotNull("JFreeChart inside " + image.getRenderer().getClass().getName(), chart);
        Object dataset = chart.getClass().getMethod("getCategoryPlot").invoke(chart).getClass()
                .getMethod("getDataset").invoke(chart.getClass().getMethod("getCategoryPlot").invoke(chart));
        assertNotNull(dataset);
        int columns = ((Number) dataset.getClass().getMethod("getColumnCount").invoke(dataset)).intValue();
        int rows = ((Number) dataset.getClass().getMethod("getRowCount").invoke(dataset)).intValue();
        assertEquals(3, columns);
        assertEquals(1, rows);
        assertEquals("Jan", String.valueOf(dataset.getClass().getMethod("getColumnKey", int.class).invoke(dataset, 0)));
        assertEquals("Feb", String.valueOf(dataset.getClass().getMethod("getColumnKey", int.class).invoke(dataset, 1)));
        assertEquals("Mar", String.valueOf(dataset.getClass().getMethod("getColumnKey", int.class).invoke(dataset, 2)));
        assertEquals(800, ((Number) dataset.getClass().getMethod("getValue", int.class, int.class).invoke(dataset, 0, 0)).intValue());
        assertEquals(400, ((Number) dataset.getClass().getMethod("getValue", int.class, int.class).invoke(dataset, 0, 1)).intValue());
        assertEquals(34, ((Number) dataset.getClass().getMethod("getValue", int.class, int.class).invoke(dataset, 0, 2)).intValue());
        System.out.println("PROBE chartSeries renderer=" + image.getRenderer().getClass().getName()
                + " chartObject=" + chart.getClass().getName()
                + " dataset=" + dataset.getClass().getName());
    }

    @Test
    public void lookupIsByPrintedKeyAcrossPagesAndFrames() {
        JRPrintElement field = only(findByKey(print, FIELD_ID));
        JRPrintElement chart = only(findByKey(print, CHART_ID));
        JRPrintElement cover = only(findByKey(print, "cover-label"));
        JRPrintElement nested = only(findByKey(print, "nested-note"));

        assertTrue(pageOf(print, field) > pageOf(print, cover));
        assertEquals(pageOf(print, field), pageOf(print, chart));
        assertEquals(0, findTopLevelByKey(print, "nested-note").size());
        assertEquals(1, findByKey(print, "nested-note").size());
        assertTrue(frameAncestor(print, nested));

        assertEquals(1, designHits("hidden-units").size());
        assertEquals(0, findByKey(print, "hidden-units").size());
        assertEquals(1, designHits("absent-band").size());
        assertEquals(0, findByKey(print, "absent-band").size());
        assertEquals(0, designHits("not-a-component").size());
        assertEquals(0, findByKey(print, "not-a-component").size());

        assertEquals(1, designHits("month-row").size());
        assertEquals(3, findByKey(print, "month-row").size());

        // html.id is inherited through the property resolver and is not unique.
        // The decoy reuses total-units as its HTML id while keeping another key.
        assertEquals(1, findByKey(print, FIELD_ID).size());
        assertEquals(2, findByHtmlId(print, FIELD_ID).size());
        assertEquals("acct=A-17", ((JRPrintText) cover).getFullText());

        System.out.println("PROBE pages field=" + pageOf(print, field)
                + " chart=" + pageOf(print, chart)
                + " cover=" + pageOf(print, cover)
                + " nested=" + pageOf(print, nested)
                + " monthRows=" + findByKey(print, "month-row").size());
    }

    @Test
    public void fieldExportsFromTheFilledElementWithoutRebuildingTheValue() throws Exception {
        JRPrintText text = (JRPrintText) only(findByKey(print, FIELD_ID));
        String filled = text.getFullText();
        Object filledValue = text.getValue();
        Export slice = exportElement(print, text);
        String html = slice.html;

        assertEquals(filled, text.getFullText());
        assertEquals(filledValue, text.getValue());
        assertTrue(html.contains(filled));
        assertTrue(html.contains("id=\"" + FIELD_ID + "\""));
        assertFalse(html.contains("id=\"" + CHART_ID + "\""));
        assertFalse(html.contains("acct=A-17"));
        assertFalse(html.contains("acugis_report_image"));
        String lower = html.toLowerCase(Locale.US);
        assertTrue(snippet(html, FIELD_ID), lower.contains("text-align: right"));
        assertTrue(snippet(html, FIELD_ID), lower.contains("#123456"));
        assertTrue(snippet(html, FIELD_ID), lower.contains("#fff4e5"));
        assertTrue(snippet(html, FIELD_ID), lower.contains("dejavu sans"));
        assertTrue(snippet(html, FIELD_ID), lower.contains("font-size: 16"));
        assertTrue(snippet(html, FIELD_ID), lower.contains("font-weight: bold"));
        assertTrue(snippet(html, FIELD_ID), lower.contains("padding-top: 4px"));
        assertTrue(snippet(html, FIELD_ID), lower.contains("padding-left: 6px"));
        assertTrue(snippet(html, FIELD_ID), lower.contains("padding-bottom: 4px"));
        assertTrue(snippet(html, FIELD_ID), lower.contains("padding-right: 6px"));
        assertTrue(snippet(html, FIELD_ID), lower.contains("border: 1px solid #112233"));
        Dimension box = pageBox(html);
        assertEquals(FIELD_WIDTH, box.width);
        assertEquals(FIELD_HEIGHT, box.height);
        assertTrue(html.contains("width=\"100%\""));
        assertEquals(0, ReportImageStore.countFor(slice.runId));
        System.out.println("PROBE fieldHtmlBytes=" + html.length() + " box=" + box.width + "x" + box.height);
    }

    @Test
    public void chartExportsItsFilledRendererThroughTheReportImageStore() throws Exception {
        JRPrintImage image = (JRPrintImage) only(findByKey(print, CHART_ID));
        Object renderer = image.getRenderer();
        Object chartBefore = findNested(renderer, "org.jfree.chart.JFreeChart", 0, new IdentityHashMap<Object, Boolean>());
        Export slice = exportElement(print, image);
        String html = slice.html;

        assertTrue(renderer == image.getRenderer());
        Object chartAfter = findNested(image.getRenderer(), "org.jfree.chart.JFreeChart", 0, new IdentityHashMap<Object, Boolean>());
        assertTrue(chartBefore == chartAfter);
        assertTrue(html.contains("id=\"" + CHART_ID + "\""));
        assertFalse(html.contains("id=\"" + FIELD_ID + "\""));
        assertFalse(html.contains("1,234"));

        Matcher src = Pattern.compile("acugis_report_image\\?image=([^\"&]+)&(?:amp;)?uuid=([^\"&]+)").matcher(html);
        assertTrue("chart HTML has no image URI:\n" + html, src.find());
        String imageName = java.net.URLDecoder.decode(src.group(1), "UTF-8");
        String uuid = java.net.URLDecoder.decode(src.group(2), "UTF-8");
        assertEquals(slice.runId, uuid);
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
            assertTrue("rendered chart is not a single-color image", distinctColors(raster) >= 4);
        } else {
            String svg = new String(bytes, StandardCharsets.UTF_8);
            assertTrue(svg.contains("Jan") || svg.contains("<path") || svg.contains("<rect"));
        }
        Dimension box = pageBox(html);
        assertEquals(CHART_WIDTH, box.width);
        assertEquals(CHART_HEIGHT, box.height);
        assertTrue(html.contains("width=\"100%\""));
        assertTrue(html.contains("style=\"height: " + CHART_HEIGHT + "px\""));
        System.out.println("PROBE chartImage name=" + imageName
                + " uuid=" + uuid
                + " bytes=" + bytes.length
                + " kind=" + kind
                + " box=" + box.width + "x" + box.height
                + " storeCount=" + ReportImageStore.countFor(uuid));
    }

    private static Export exportElement(JasperPrint source, JRPrintElement element) throws JRException {
        int x = element.getX();
        int y = element.getY();
        element.setX(0);
        element.setY(0);
        try {
            JasperPrint slice = new JasperPrint();
            slice.setName(source.getName());
            slice.setPageWidth(element.getWidth());
            slice.setPageHeight(element.getHeight());
            slice.setOrientation(source.getOrientation());
            slice.setDefaultStyle(source.getDefaultStyle());
            JRPrintPage page = new JRBasePrintPage();
            page.addElement(element);
            slice.addPage(page);

            StringWriter writer = new StringWriter();
            String runId = UUID.randomUUID().toString();
            Map<String, byte[]> images = new HashMap<String, byte[]>();
            HtmlExporter exporter = new HtmlExporter();
            exporter.setExporterInput(new SimpleExporterInput(slice));
            SimpleHtmlExporterOutput out = new SimpleHtmlExporterOutput(writer);
            String pattern = "/acugis_report_image?image={0}&uuid=" + runId;
            out.setImageHandler(new CapturingHtmlResourceHandler(pattern, runId, images));
            SimpleHtmlReportConfiguration config = new SimpleHtmlReportConfiguration();
            config.setRemoveEmptySpaceBetweenRows(Boolean.TRUE);
            config.setWhitePageBackground(Boolean.FALSE);
            exporter.setExporterOutput(out);
            exporter.setConfiguration(config);
            exporter.exportReport();
            Export export = new Export();
            export.html = writer.toString();
            export.runId = runId;
            return export;
        } finally {
            element.setX(x);
            element.setY(y);
        }
    }

    private static List<JRPrintElement> findByKey(JasperPrint filled, String embedId) {
        List<JRPrintElement> hits = new ArrayList<JRPrintElement>();
        List<JRPrintPage> pages = filled.getPages();
        for (int i = 0; i < pages.size(); i++) {
            walkPrint(pages.get(i).getElements(), embedId, hits);
        }
        return hits;
    }

    private static List<JRPrintElement> findTopLevelByKey(JasperPrint filled, String embedId) {
        List<JRPrintElement> hits = new ArrayList<JRPrintElement>();
        for (JRPrintPage page : filled.getPages()) {
            for (JRPrintElement element : page.getElements()) {
                if (embedId.equals(element.getKey())) {
                    hits.add(element);
                }
            }
        }
        return hits;
    }

    private static void walkPrint(List<JRPrintElement> elements, String embedId, List<JRPrintElement> hits) {
        if (elements == null) {
            return;
        }
        for (JRPrintElement element : elements) {
            if (embedId.equals(element.getKey())) {
                hits.add(element);
            }
            if (element instanceof JRPrintFrame) {
                walkPrint(((JRPrintFrame) element).getElements(), embedId, hits);
            }
        }
    }

    private static List<JRPrintElement> findByHtmlId(JasperPrint filled, String embedId) {
        List<JRPrintElement> hits = new ArrayList<JRPrintElement>();
        for (JRPrintPage page : filled.getPages()) {
            walkHtmlId(page.getElements(), embedId, hits);
        }
        return hits;
    }

    private static void walkHtmlId(List<JRPrintElement> elements, String embedId, List<JRPrintElement> hits) {
        if (elements == null) {
            return;
        }
        for (JRPrintElement element : elements) {
            if (embedId.equals(htmlId(element))) {
                hits.add(element);
            }
            if (element instanceof JRPrintFrame) {
                walkHtmlId(((JRPrintFrame) element).getElements(), embedId, hits);
            }
        }
    }

    private static int pageOf(JasperPrint filled, JRPrintElement target) {
        List<JRPrintPage> pages = filled.getPages();
        for (int i = 0; i < pages.size(); i++) {
            if (contains(pages.get(i).getElements(), target)) {
                return i;
            }
        }
        return -1;
    }

    private static boolean contains(List<JRPrintElement> elements, JRPrintElement target) {
        if (elements == null) {
            return false;
        }
        for (JRPrintElement element : elements) {
            if (element == target) {
                return true;
            }
            if (element instanceof JRPrintFrame && contains(((JRPrintFrame) element).getElements(), target)) {
                return true;
            }
        }
        return false;
    }

    private static boolean frameAncestor(JasperPrint filled, JRPrintElement target) {
        for (JRPrintPage page : filled.getPages()) {
            for (JRPrintElement element : page.getElements()) {
                if (element instanceof JRPrintFrame && contains(((JRPrintFrame) element).getElements(), target)) {
                    return true;
                }
            }
        }
        return false;
    }

    private static List<JRElement> designHits(String embedId) {
        List<JRElement> hits = new ArrayList<JRElement>();
        walkDesign(report.getBackground(), embedId, hits);
        walkDesign(report.getTitle(), embedId, hits);
        walkDesign(report.getPageHeader(), embedId, hits);
        walkDesign(report.getColumnHeader(), embedId, hits);
        if (report.getDetailSection() != null) {
            for (int i = 0; i < report.getDetailSection().getBands().length; i++) {
                walkDesign(report.getDetailSection().getBands()[i], embedId, hits);
            }
        }
        walkDesign(report.getColumnFooter(), embedId, hits);
        walkDesign(report.getPageFooter(), embedId, hits);
        walkDesign(report.getLastPageFooter(), embedId, hits);
        walkDesign(report.getSummary(), embedId, hits);
        if (report.getGroups() != null) {
            for (JRGroup group : report.getGroups()) {
                walkSection(group.getGroupHeaderSection(), embedId, hits);
                walkSection(group.getGroupFooterSection(), embedId, hits);
            }
        }
        return hits;
    }

    private static void walkSection(net.sf.jasperreports.engine.JRSection section, String embedId, List<JRElement> hits) {
        if (section == null || section.getBands() == null) {
            return;
        }
        for (int i = 0; i < section.getBands().length; i++) {
            walkDesign(section.getBands()[i], embedId, hits);
        }
    }

    private static void walkDesign(JRElementGroup group, String embedId, List<JRElement> hits) {
        if (group == null || group.getElements() == null) {
            return;
        }
        for (JRElement element : group.getElements()) {
            if (embedId.equals(element.getKey())) {
                hits.add(element);
            }
            if (element instanceof JRElementGroup) {
                walkDesign((JRElementGroup) element, embedId, hits);
            }
        }
    }

    private static JRElement onlyDesign(String embedId) {
        List<JRElement> hits = designHits(embedId);
        assertEquals(embedId, 1, hits.size());
        return hits.get(0);
    }

    private static JRPrintElement only(List<JRPrintElement> hits) {
        assertEquals(1, hits.size());
        return hits.get(0);
    }

    private static String htmlId(JRPrintElement element) {
        return JRPropertiesUtil.getInstance(DefaultJasperReportsContext.getInstance()).getProperty(element, HTML_ID);
    }

    private static void assertNoOwnProperty(JRPrintElement element, String name) {
        String[] own = element.getPropertiesMap().getOwnPropertyNames();
        if (own == null) {
            return;
        }
        for (String candidate : own) {
            if (name.equals(candidate)) {
                fail(element.getKey() + " has own property " + name);
            }
        }
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

    private static Object findNested(Object root, String className, int depth, IdentityHashMap<Object, Boolean> seen) {
        if (root == null || depth > 5 || seen.containsKey(root)) {
            return null;
        }
        seen.put(root, Boolean.TRUE);
        if (className.equals(root.getClass().getName())) {
            return root;
        }
        Class<?> type = root.getClass();
        while (type != null && type != Object.class) {
            for (Field field : type.getDeclaredFields()) {
                if (Modifier.isStatic(field.getModifiers())) {
                    continue;
                }
                field.setAccessible(true);
                Object value;
                try {
                    value = field.get(root);
                } catch (IllegalAccessException ex) {
                    continue;
                }
                if (value == null) {
                    continue;
                }
                String name = value.getClass().getName();
                if (className.equals(name)) {
                    return value;
                }
                if (name.startsWith("net.sf.jasperreports") || name.startsWith("org.jfree") || name.startsWith("com.acugis")) {
                    Object found = findNested(value, className, depth + 1, seen);
                    if (found != null) {
                        return found;
                    }
                }
            }
            type = type.getSuperclass();
        }
        return null;
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

    private static String describe(JRPrintElement element) {
        return element.getClass().getSimpleName()
                + " key=" + element.getKey()
                + " uuid=" + element.getUUID()
                + " htmlId=" + htmlId(element)
                + " " + element.getWidth() + "x" + element.getHeight()
                + "@" + element.getX() + "," + element.getY();
    }

    private static Map<String, Object> row(String month, int units) {
        Map<String, Object> row = new HashMap<String, Object>();
        row.put("month", month);
        row.put("units", Integer.valueOf(units));
        return row;
    }

    private static String probeJrxml() {
        return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
                + "<jasperReport name=\"embed_component_probe\" language=\"java\" pageWidth=\"360\" pageHeight=\"280\" "
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
                + "  <title height=\"40\">\n"
                + "    <element kind=\"textField\" key=\"cover-label\" uuid=\"44444444-4444-4444-8444-444444444444\" x=\"0\" y=\"0\" width=\"180\" height=\"16\">\n"
                + "      <expression><![CDATA[\"acct=\" + $P{account_id}]]></expression>\n"
                + "    </element>\n"
                + "    <element kind=\"textField\" key=\"hidden-units\" uuid=\"55555555-5555-4555-8555-555555555555\" x=\"0\" y=\"18\" width=\"120\" height=\"14\">\n"
                + "      <property name=\"net.sf.jasperreports.export.html.id\" value=\"hidden-units\"/>\n"
                + "      <printWhenExpression><![CDATA[Boolean.FALSE]]></printWhenExpression>\n"
                + "      <expression><![CDATA[\"hidden\"]]></expression>\n"
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
                + "    <element kind=\"textField\" key=\"total-units\" uuid=\"" + FIELD_UUID + "\" mode=\"Opaque\" x=\"10\" y=\"8\" width=\"" + FIELD_WIDTH + "\" height=\"" + FIELD_HEIGHT + "\" forecolor=\"#123456\" backcolor=\"#FFF4E5\" fontName=\"SansSerif\" fontSize=\"16.0\" bold=\"true\" hTextAlign=\"Right\" vTextAlign=\"Middle\" pattern=\"#,##0\">\n"
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
                + "    <element kind=\"chart\" key=\"monthly-occupancy\" uuid=\"" + CHART_UUID + "\" x=\"10\" y=\"60\" width=\"" + CHART_WIDTH + "\" height=\"" + CHART_HEIGHT + "\" chartType=\"bar\" evaluationTime=\"Report\">\n"
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

    private static final class Export {
        String html;
        String runId;
    }

    private static final class Dimension {
        int width;
        int height;
    }
}
