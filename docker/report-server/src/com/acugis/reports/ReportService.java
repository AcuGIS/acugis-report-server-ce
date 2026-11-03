package com.acugis.reports;

import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.PrintWriter;
import java.lang.reflect.Method;
import java.sql.Connection;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import java.util.UUID;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import net.sf.jasperreports.engine.DefaultJasperReportsContext;
import net.sf.jasperreports.engine.JRDataSource;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JasperFillManager;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.engine.JasperReport;
import net.sf.jasperreports.engine.SimpleJasperReportsContext;
import net.sf.jasperreports.engine.export.HtmlExporter;
import net.sf.jasperreports.engine.export.ooxml.JRXlsxExporter;
import net.sf.jasperreports.engine.util.JRLoader;
import net.sf.jasperreports.export.SimpleExporterInput;
import net.sf.jasperreports.export.SimpleHtmlExporterOutput;
import net.sf.jasperreports.export.SimpleHtmlReportConfiguration;
import net.sf.jasperreports.export.SimpleOutputStreamExporterOutput;
import net.sf.jasperreports.export.SimpleXlsxReportConfiguration;
import net.sf.jasperreports.pdf.JRPdfExporter;
import net.sf.jasperreports.repo.FileRepositoryService;
import net.sf.jasperreports.repo.RepositoryService;

public class ReportService {

    private static final String DEFAULT_REPORTS_DIR =
            "/home/tomcat/apache-tomcat-9.0.113/jasper_reports/reports";

    public static String getReportsDir() {
        String dir = System.getenv("REPORTS_DIR");
        if (dir != null && !dir.isEmpty()) {
            return new File(dir).getAbsolutePath();
        }
        dir = System.getProperty("reports.dir");
        if (dir != null && !dir.isEmpty()) {
            return new File(dir).getAbsolutePath();
        }
        return new File(DEFAULT_REPORTS_DIR).getAbsolutePath();
    }

    public static File resolveReportFile(File reportsDir, String repName) throws FileNotFoundException {
        String rep = repName.trim();
        rep = rep.replace("\\", "/");
        if ((rep = rep.substring(rep.lastIndexOf(47) + 1)).contains("..")) {
            throw new FileNotFoundException("Report name contains invalid path traversal: " + repName);
        }
        ArrayList<String> candidates = new ArrayList<>();
        String lowerRep = rep.toLowerCase();
        if (lowerRep.endsWith(".jasper") || lowerRep.endsWith(".jrxml")) {
            candidates.add(rep);
        } else {
            candidates.add(rep + ".jasper");
            candidates.add(rep + ".jrxml");
            candidates.add(rep);
        }
        ArrayList<String> attemptedPaths = new ArrayList<>();
        for (String candidate : candidates) {
            File f = new File(reportsDir, candidate);
            attemptedPaths.add(f.getAbsolutePath());
            if (f.isFile()) {
                return f;
            }
        }
        throw new FileNotFoundException("Report file not found. Reports directory: "
                + reportsDir.getAbsolutePath() + ". Attempted paths: "
                + String.join(", ", attemptedPaths));
    }

    public static JasperReport loadReport(String reportName) throws JRException {
        String reportsDir = getReportsDir();
        File reportsDirFile = new File(reportsDir);
        try {
            File reportFile = resolveReportFile(reportsDirFile, reportName);
            return (JasperReport) JRLoader.loadObject(reportFile);
        } catch (FileNotFoundException e) {
            throw new JRException(e.getMessage(), e);
        }
    }

    public static JasperPrint fillReport(File reportFile, Map<String, Object> parameters, Connection connection)
            throws JRException {
        File reportDir = reportFile.getParentFile();
        SimpleJasperReportsContext context = new SimpleJasperReportsContext();
        context.setParent(DefaultJasperReportsContext.getInstance());
        FileRepositoryService repo = new FileRepositoryService(context, reportDir.getAbsolutePath(), false);
        context.setExtensions(RepositoryService.class, Collections.singletonList(repo));
        try {
            return JasperFillManager.getInstance(context)
                    .fill(reportFile.getAbsolutePath(), parameters, connection);
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
        return fillWithContext(report, parameters, reportDir, connection, null);
    }

    public static JasperPrint fillReport(
            JasperReport report,
            Map<String, Object> parameters,
            JRDataSource dataSource,
            File reportDir) throws JRException {
        return fillWithContext(report, parameters, reportDir, null, dataSource);
    }

    public static JasperPrint fillReport(
            File reportFile,
            Map<String, Object> parameters,
            JRDataSource dataSource) throws JRException {
        JasperReport report = ReportParameterBinder.loadOrCompileReport(reportFile);
        return fillReport(report, parameters, dataSource, reportFile.getParentFile());
    }

    private static JasperPrint fillWithContext(
            JasperReport report,
            Map<String, Object> parameters,
            File reportDir,
            Connection connection,
            JRDataSource dataSource) throws JRException {
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
        FileRepositoryService repo = new FileRepositoryService(ctx, reportDir.getAbsolutePath(), false);
        ctx.setExtensions(RepositoryService.class, Collections.singletonList(repo));
        if (dataSource != null) {
            return JasperFillManager.getInstance(ctx).fill(report, parameters, dataSource);
        }
        return JasperFillManager.getInstance(ctx).fill(report, parameters, connection);
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
        String runId = UUID.randomUUID().toString();
        HashMap<String, byte[]> imagesMap = new HashMap<>();
        HtmlExporter exporter = new HtmlExporter();
        exporter.setExporterInput(new SimpleExporterInput(jasperPrint));
        PrintWriter writer = response.getWriter();
        SimpleHtmlExporterOutput htmlOut = new SimpleHtmlExporterOutput(writer);
        String imageUri = "/report_image.php?image={0}&uuid=" + runId;
        htmlOut.setImageHandler(new CapturingHtmlResourceHandler(imageUri, runId, imagesMap));
        SimpleHtmlReportConfiguration config = new SimpleHtmlReportConfiguration();
        config.setRemoveEmptySpaceBetweenRows(Boolean.TRUE);
        config.setWhitePageBackground(Boolean.FALSE);
        exporter.setExporterOutput(htmlOut);
        exporter.setConfiguration(config);
        exporter.exportReport();
        for (Map.Entry<String, byte[]> entry : imagesMap.entrySet()) {
            ReportImageStore.put(runId, entry.getKey(), entry.getValue());
        }
    }
}
