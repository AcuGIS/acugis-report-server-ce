package com.acugis.reports;

import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.Writer;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.sql.Connection;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;

import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import net.sf.jasperreports.engine.DefaultJasperReportsContext;
import net.sf.jasperreports.engine.JRDataSource;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JRPrintElement;
import net.sf.jasperreports.engine.JRPrintImage;
import net.sf.jasperreports.engine.JRPrintPage;
import net.sf.jasperreports.engine.JasperFillManager;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.SimpleJasperReportsContext;
import net.sf.jasperreports.engine.export.HtmlExporter;
import net.sf.jasperreports.engine.export.ooxml.JRXlsxExporter;
import net.sf.jasperreports.engine.fonts.FontUtil;
import net.sf.jasperreports.export.SimpleExporterInput;
import net.sf.jasperreports.export.SimpleHtmlExporterOutput;
import net.sf.jasperreports.export.SimpleHtmlReportConfiguration;
import net.sf.jasperreports.export.SimpleOutputStreamExporterOutput;
import net.sf.jasperreports.export.SimpleXlsxReportConfiguration;
import net.sf.jasperreports.pdf.JRPdfExporter;
import net.sf.jasperreports.renderers.Renderable;
import net.sf.jasperreports.repo.FileRepositoryService;
import net.sf.jasperreports.repo.RepositoryService;

public class ReportService {

    public static String getReportsDir() {
        try {
            return AcugisConfig.get().reportsDir().getAbsolutePath();
        } catch (Exception ignored) {
            // Config may not be initialized in unit/tooling contexts.
        }
        String dir = System.getenv("REPORTS_DIR");
        if (dir == null || dir.isEmpty()) {
            dir = System.getenv("ACUGIS_REPORTS_DIR");
        }
        if (dir != null && !dir.isEmpty()) {
            return new File(dir).getAbsolutePath();
        }
        dir = System.getProperty("reports.dir");
        if (dir != null && !dir.isEmpty()) {
            return new File(AcugisConfig.expand(dir)).getAbsolutePath();
        }
        return new File(CatalinaPaths.acugisRoot(), "reports").getAbsolutePath();
    }

    public static File resolveReportFile(File reportsDir, String repName) throws FileNotFoundException {
        String rep = repName.trim();
        rep = rep.replace("\\", "/");
        if ((rep = rep.substring(rep.lastIndexOf(47) + 1)).contains("..")) {
            throw new FileNotFoundException("Report name contains invalid path traversal: " + repName);
        }
        String lowerRep = rep.toLowerCase();
        if (lowerRep.endsWith(".jasper") || lowerRep.endsWith(".jrxml")) {
            File f = new File(reportsDir, rep);
            if (f.isFile()) {
                return f;
            }
            throw new FileNotFoundException("Report file not found: " + f.getAbsolutePath());
        }

        File jasper = new File(reportsDir, rep + ".jasper");
        File jrxml = new File(reportsDir, rep + ".jrxml");
        boolean jasperOk = jasper.isFile();
        boolean jrxmlOk = jrxml.isFile();

        // Prefer JRXML when it is newer than .jasper so embedded-map / layout edits apply.
        if (jrxmlOk && jasperOk && jrxml.lastModified() > jasper.lastModified()) {
            return jrxml;
        }
        if (jasperOk) {
            return jasper;
        }
        if (jrxmlOk) {
            return jrxml;
        }
        File bare = new File(reportsDir, rep);
        if (bare.isFile()) {
            return bare;
        }
        throw new FileNotFoundException("Report file not found. Reports directory: "
                + reportsDir.getAbsolutePath() + ". Attempted paths: "
                + jasper.getAbsolutePath() + ", " + jrxml.getAbsolutePath());
    }

    public static JasperReport loadReport(String reportName) throws JRException {
        String reportsDir = ReportService.getReportsDir();
        File reportsDirFile = new File(reportsDir);
        try {
            File reportFile = ReportService.resolveReportFile(reportsDirFile, reportName);
            return ReportParameterBinder.loadOrCompileReport(reportFile);
        } catch (FileNotFoundException e) {
            throw new JRException(e.getMessage(), e);
        }
    }

    public static JasperPrint fillReport(File reportFile, Map<String, Object> parameters, Connection connection)
            throws JRException {
        File reportDir = reportFile.getParentFile();
        String repName = reportFile.getName();
        SimpleJasperReportsContext context = new SimpleJasperReportsContext();
        context.setParent(DefaultJasperReportsContext.getInstance());
        applyChartInstrumentationProperties(context);
        FileRepositoryService repo = newResourceRepo(context, reportDir, repName, null);
        context.setExtensions(RepositoryService.class, Collections.singletonList(repo));
        try {
            return JasperFillManager.getInstance(context).fill(reportFile.getAbsolutePath(), parameters, connection);
        } catch (Exception e) {
            if (e instanceof JRException) {
                throw (JRException) e;
            }
            throw new JRException("Error filling report", e);
        }
    }

    public static JasperPrint fillReport(
            JasperReport report,
            Map<String, Object> parameters,
            Connection connection,
            File reportDir) throws JRException {
        return fillReport(report, parameters, connection, reportDir, null, null);
    }

    public static JasperPrint fillReport(
            JasperReport report,
            Map<String, Object> parameters,
            Connection connection,
            File reportDir,
            String repName,
            String resourceBaseOverride) throws JRException {
        parameters.remove("SUBREPORT_DIR");
        parameters.remove("REPORT_DIR");
        String reportDirPath = reportDir.getAbsolutePath();
        if (!reportDirPath.endsWith(File.separator)) {
            reportDirPath = reportDirPath + File.separator;
        }
        parameters.put("SUBREPORT_DIR", reportDirPath);
        parameters.put("REPORT_DIR", reportDirPath);
        SimpleJasperReportsContext ctx = new SimpleJasperReportsContext();
        ctx.setParent(DefaultJasperReportsContext.getInstance());
        // Ensure chart timing hooks apply on this fill context (parent props alone
        // are not always visible to JRFillChart / ChartUtil during evaluate).
        applyChartInstrumentationProperties(ctx);
        // true: allow absolute P_MAP_IMAGE / image paths outside REPORT_DIR (embedded maps).
        FileRepositoryService repo = newResourceRepo(ctx, reportDir, repName, resourceBaseOverride);
        ctx.setExtensions(RepositoryService.class, Collections.singletonList(repo));
        // Register timing theme on this fill context (classpath extension lookup is unreliable here).
        ctx.setExtensions(net.sf.jasperreports.charts.ChartThemeBundle.class,
                Collections.singletonList(AcugisChartsExtensionsRegistryFactory.BUNDLE));
        String reportName = report == null ? "" : report.getName();
        beginMapRenderSession(reportName);
        ChartRenderSession.begin(reportName);
        SqlTimingContext sqlTiming = null;
        Connection fillConnection = connection;
        if (PerformanceProfiler.isEnabled() && connection != null) {
            sqlTiming = SqlTimingContext.install();
            fillConnection = SqlTimingContext.wrap(connection);
        }
        try {
            // Must use the context-bound manager — static fillReport() ignores our ctx
            // (chart theme/factories + FileRepositoryService would be discarded).
            return JasperFillManager.getInstance(ctx).fill(report, parameters, fillConnection);
        } finally {
            if (sqlTiming != null) {
                if (PerformanceProfiler.isEnabled()) {
                    System.out.println("fill_sql_summary report=" + reportName
                            + " sql_ms=" + sqlTiming.sqlMs()
                            + " fetch_ms=" + sqlTiming.fetchMs()
                            + " metadata_ms=" + sqlTiming.metadataMs()
                            + " queries=" + sqlTiming.queries()
                            + " rows=" + sqlTiming.rows());
                    List<SqlTimingContext.QuerySample> samples = sqlTiming.querySamples();
                    // Log slowest queries first (cap at 20 lines).
                    samples.sort((a, b) -> Long.compare(b.executeMs, a.executeMs));
                    int n = Math.min(20, samples.size());
                    for (int i = 0; i < n; i++) {
                        SqlTimingContext.QuerySample q = samples.get(i);
                        System.out.println("fill_sql_query rank=" + (i + 1)
                                + " execute_ms=" + q.executeMs
                                + " sql=" + q.sql);
                    }
                }
                SqlTimingContext.clear();
            }
            // Keep chart session open through HTML export so draw/html_capture are attributed.
            endMapRenderSession();
        }
    }

    public static JasperPrint fillReport(
            JasperReport report,
            Map<String, Object> parameters,
            JRDataSource dataSource,
            File reportDir) throws JRException {
        return fillReport(report, parameters, dataSource, reportDir, null, null);
    }

    public static JasperPrint fillReport(
            JasperReport report,
            Map<String, Object> parameters,
            JRDataSource dataSource,
            File reportDir,
            String repName,
            String resourceBaseOverride) throws JRException {
        parameters.remove("SUBREPORT_DIR");
        parameters.remove("REPORT_DIR");
        String reportDirPath = reportDir.getAbsolutePath();
        if (!reportDirPath.endsWith(File.separator)) {
            reportDirPath = reportDirPath + File.separator;
        }
        parameters.put("SUBREPORT_DIR", reportDirPath);
        parameters.put("REPORT_DIR", reportDirPath);
        SimpleJasperReportsContext ctx = new SimpleJasperReportsContext();
        ctx.setParent(DefaultJasperReportsContext.getInstance());
        applyChartInstrumentationProperties(ctx);
        FileRepositoryService repo = newResourceRepo(ctx, reportDir, repName, resourceBaseOverride);
        ctx.setExtensions(RepositoryService.class, Collections.singletonList(repo));
        ctx.setExtensions(net.sf.jasperreports.charts.ChartThemeBundle.class,
                Collections.singletonList(AcugisChartsExtensionsRegistryFactory.BUNDLE));
        String reportName = report == null ? "" : report.getName();
        beginMapRenderSession(reportName);
        ChartRenderSession.begin(reportName);
        try {
            return JasperFillManager.getInstance(ctx).fill(report, parameters, dataSource);
        } finally {
            endMapRenderSession();
        }
    }

    public static JasperPrint fillReport(
            File reportFile,
            Map<String, Object> parameters,
            JRDataSource dataSource) throws JRException {
        JasperReport report = ReportParameterBinder.loadOrCompileReport(reportFile);
        return fillReport(report, parameters, dataSource, reportFile.getParentFile());
    }

    private static FileRepositoryService newResourceRepo(
            SimpleJasperReportsContext ctx,
            File reportDir,
            String repName,
            String resourceBaseOverride) {
        File pathsRoot = null;
        try {
            pathsRoot = AcugisConfig.get().resourcePathsDir();
        } catch (Exception ignored) {
            // Config may be uninitialized in tests.
        }
        String base = ReportsFileRepositoryService.normalizeBase(resourceBaseOverride);
        if (base == null || base.isEmpty()) {
            base = ReportsFileRepositoryService.readResourceBaseFile(reportDir, repName);
        }
        return new ReportsFileRepositoryService(
                ctx,
                reportDir.getAbsolutePath(),
                true,
                pathsRoot,
                base);
    }

    private static void applyChartInstrumentationProperties(SimpleJasperReportsContext ctx) {
        if (ctx == null) {
            return;
        }
        ctx.setProperty("net.sf.jasperreports.chart.theme", TimingChartTheme.THEME_NAME);
        ctx.setProperty("net.sf.jasperreports.customizer.class.acugis_perf",
                TimingChartCustomizer.class.getName());
        ctx.setProperty("net.sf.jasperreports.chart.renderer.factory.draw",
                TimingChartRenderableFactory.Draw.class.getName());
        ctx.setProperty("net.sf.jasperreports.chart.renderer.factory.image",
                TimingChartRenderableFactory.Image.class.getName());
        ctx.setProperty("net.sf.jasperreports.chart.renderer.factory.svg",
                TimingChartRenderableFactory.Svg.class.getName());
    }

    /**
     * Opens a fill-scoped map render cache / profiler session when acugis-report-map.jar
     * is present. No-op if the map library is absent.
     */
    private static void beginMapRenderSession(String reportName) {
        try {
            Class<?> session = Class.forName("com.acugis.map.MapRenderSession");
            session.getMethod("begin", String.class).invoke(null, reportName);
        } catch (ClassNotFoundException ignored) {
            // Map jar optional.
        } catch (Throwable t) {
            System.err.println("MapRenderSession.begin failed: " + t.getMessage());
        }
    }

    private static void endMapRenderSession() {
        try {
            Class<?> session = Class.forName("com.acugis.map.MapRenderSession");
            session.getMethod("end").invoke(null);
        } catch (ClassNotFoundException ignored) {
            // Map jar optional.
        } catch (Throwable t) {
            System.err.println("MapRenderSession.end failed: " + t.getMessage());
        }
    }

    /** Ends chart timing session (call after export so deferred draw is included). */
    public static void endChartRenderSession() {
        ChartRenderSession.end();
    }

    public static void exportReport(
            JasperPrint jasperPrint,
            String format,
            HttpServletResponse response,
            HttpServletRequest request) throws JRException, IOException {
            switch (format) {
                case "PDF":
                    exportToPdf(jasperPrint, response);
                    break;
                case "HTML":
                    exportToHtml(jasperPrint, response, request);
                    break;
                case "XLSX":
                    exportToXlsx(jasperPrint, response);
                    break;
                case "DOCX":
                    exportToDocx(jasperPrint, response);
                    break;
                default:
                    throw new IllegalArgumentException("Unsupported format: " + format);
            }
    }

    private static void exportToPdf(JasperPrint jasperPrint, HttpServletResponse response)
            throws JRException, IOException {
        JRPdfExporter exporter = new JRPdfExporter();
        exporter.setExporterInput(new SimpleExporterInput(jasperPrint));
        exporter.setExporterOutput(new SimpleOutputStreamExporterOutput(response.getOutputStream()));
        exporter.exportReport();
    }

    private static void exportToXlsx(JasperPrint jasperPrint, HttpServletResponse response)
            throws JRException, IOException {
        JRXlsxExporter exporter = new JRXlsxExporter();
        exporter.setExporterInput(new SimpleExporterInput(jasperPrint));
        exporter.setExporterOutput(new SimpleOutputStreamExporterOutput(response.getOutputStream()));
        SimpleXlsxReportConfiguration configuration = new SimpleXlsxReportConfiguration();
        configuration.setDetectCellType(Boolean.TRUE);
        configuration.setRemoveEmptySpaceBetweenRows(Boolean.TRUE);
        configuration.setRemoveEmptySpaceBetweenColumns(Boolean.TRUE);
        configuration.setOnePagePerSheet(Boolean.FALSE);
        exporter.setConfiguration(configuration);
        exporter.exportReport();
    }

    private static void exportToDocx(JasperPrint jasperPrint, HttpServletResponse response) throws JRException {
        try {
            Class<?> docxExporterClass = Class.forName("net.sf.jasperreports.engine.export.ooxml.JRDocxExporter");
            Object exporter = docxExporterClass.getDeclaredConstructor().newInstance();
            Method setExporterInput = exporter.getClass().getMethod("setExporterInput", SimpleExporterInput.class);
            Method setExporterOutput = exporter.getClass()
                    .getMethod("setExporterOutput", SimpleOutputStreamExporterOutput.class);
            Method exportReport = exporter.getClass().getMethod("exportReport");
            setExporterInput.invoke(exporter, new SimpleExporterInput(jasperPrint));
            setExporterOutput.invoke(exporter, new SimpleOutputStreamExporterOutput(response.getOutputStream()));
            exportReport.invoke(exporter);
        } catch (ClassNotFoundException e) {
            throw new JRException(
                    "DOCX exporter not available in this version of JasperReports. Please use XLSX or PDF format instead.");
        } catch (Exception e) {
            throw new JRException("Failed to export DOCX: " + e.getMessage(), e);
        }
    }

    private static void exportToHtml(
            JasperPrint jasperPrint,
            HttpServletResponse response,
            HttpServletRequest request) throws JRException, IOException {
        writeHtml(jasperPrint, response.getWriter(), request);
    }

    /**
     * HTML export used by the full report and by a single-component slice.
     * Component export calls this so chart images still go through
     * {@link CapturingHtmlResourceHandler} and the report image store.
     * removeEmptySpaceBetweenRows stays true, matching the full-report exporter.
     */
    static void writeHtml(JasperPrint jasperPrint, Writer writer, HttpServletRequest request) throws JRException {
        String runId = UUID.randomUUID().toString();
        HashMap<String, byte[]> imagesMap = new HashMap<>();
        HtmlExporter exporter = new HtmlExporter();
        exporter.setExporterInput(new SimpleExporterInput(jasperPrint));
        SimpleHtmlExporterOutput htmlOut = new SimpleHtmlExporterOutput(writer);

        String contextPath = request == null ? "" : request.getContextPath();
        if (contextPath == null) {
            contextPath = "";
        }
        String pattern = AcugisConfig.get().htmlImageUriPattern();
        String imageUri = pattern
                .replace("{contextPath}", contextPath)
                .replace("{uuid}", runId);

        htmlOut.setImageHandler(new CapturingHtmlResourceHandler(imageUri, runId, imagesMap));
        SimpleHtmlReportConfiguration config = new SimpleHtmlReportConfiguration();
        config.setRemoveEmptySpaceBetweenRows(Boolean.TRUE);
        config.setWhitePageBackground(Boolean.FALSE);
        exporter.setExporterOutput(htmlOut);
        exporter.setConfiguration(config);
        exporter.exportReport();
    }

    @SuppressWarnings("unused")
    private static Map<String, byte[]> extractImages(JasperPrint jasperPrint) {
        HashMap<String, byte[]> images = new HashMap<>();
        try {
            List<?> pages = jasperPrint.getPages();
            for (int pageIndex = 0; pageIndex < pages.size(); ++pageIndex) {
                JRPrintPage page = (JRPrintPage) pages.get(pageIndex);
                List<?> elements = page.getElements();
                int imageIndex = 0;
                for (int elementIndex = 0; elementIndex < elements.size(); ++elementIndex) {
                    JRPrintElement element = (JRPrintElement) elements.get(elementIndex);
                    if (!(element instanceof JRPrintImage)) {
                        continue;
                    }
                    JRPrintImage printImage = (JRPrintImage) element;
                    try {
                        Renderable renderer = printImage.getRenderer();
                        if (renderer == null) {
                            continue;
                        }
                        byte[] imageBytes = null;
                        try {
                            Method getImageDataMethod = renderer.getClass().getMethod("getImageData");
                            Object data = getImageDataMethod.invoke(renderer);
                            if (data instanceof byte[]) {
                                imageBytes = (byte[]) data;
                            }
                        } catch (IllegalAccessException | NoSuchMethodException | InvocationTargetException ignored) {
                            // try alternate APIs below
                        }
                        if (imageBytes == null) {
                            try {
                                Method getDataMethod = renderer.getClass().getMethod("getData", FontUtil.class);
                                Object fontProvider = null;
                                try {
                                    Method getFontProviderMethod = jasperPrint.getClass()
                                            .getMethod("getDefaultFontProvider");
                                    fontProvider = getFontProviderMethod.invoke(jasperPrint);
                                } catch (Exception ignored) {
                                    // optional
                                }
                                if (fontProvider != null) {
                                    Object data = getDataMethod.invoke(renderer, fontProvider);
                                    if (data instanceof byte[]) {
                                        imageBytes = (byte[]) data;
                                    }
                                }
                            } catch (Exception ignored) {
                                // optional
                            }
                        }
                        if (imageBytes != null && imageBytes.length > 0) {
                            String imageName = String.format("img_%d_%d_%d.png", pageIndex, elementIndex, imageIndex);
                            images.put(imageName, imageBytes);
                            ++imageIndex;
                        }
                    } catch (Exception e) {
                        System.err.println("[html] Failed to extract image at page " + pageIndex
                                + ", element " + elementIndex + ": " + e.getMessage());
                    }
                }
            }
        } catch (Exception e) {
            System.err.println("[html] Error extracting images: " + e.getMessage());
        }
        return images;
    }
}
