package com.acugis.reports;

import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.PrintWriter;
import java.io.StringWriter;
import java.sql.Connection;
import java.sql.SQLException;
import java.util.Map;

import javax.naming.InitialContext;
import javax.naming.NamingException;
import javax.sql.DataSource;

import jakarta.servlet.ServletException;
import jakarta.servlet.annotation.MultipartConfig;
import jakarta.servlet.http.HttpServlet;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JasperCompileManager;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.engine.JasperReport;

@MultipartConfig(maxFileSize = 64L * 1024 * 1024, maxRequestSize = 64L * 1024 * 1024)
public class ReportServlet extends HttpServlet {
    private static final long serialVersionUID = 1L;

    @Override
    public void init() throws ServletException {
        super.init();
        System.out.println("JasperReports loaded from: "
                + JasperCompileManager.class.getProtectionDomain().getCodeSource().getLocation());
        CollectionQueryExecuterFactory.register();
        FontSupport.configure();
        CompilerSupport.configure();
        // Legacy Designer/Server JRXML may still say language="duckdb"; treat as JDBC SQL.
        System.setProperty(
                "net.sf.jasperreports.query.executer.factory.duckdb",
                "net.sf.jasperreports.engine.query.JRJdbcQueryExecuterFactory");
        System.setProperty(
                "net.sf.jasperreports.query.executer.factory.DuckDB",
                "net.sf.jasperreports.engine.query.JRJdbcQueryExecuterFactory");
        logAuthConnectionsFile("com.acugis.map.GeoServerAuth", "GeoServer");
        logAuthConnectionsFile("com.acugis.map.QGISAuth", "QGIS");
        logAuthConnectionsFile("com.acugis.map.ArcGISAuth", "ArcGIS");
    }

    private void logAuthConnectionsFile(String className, String label) {
        try {
            Class<?> auth = Class.forName(className);
            Object path = auth.getMethod("resolveConnectionsFile").invoke(null);
            String resolved = path == null ? "" : String.valueOf(path).trim();
            if (resolved.isEmpty()) {
                System.out.println(label + "Auth: no connections file resolved (anonymous only)");
            } else {
                File f = new File(resolved);
                System.out.println(label + "Auth: connections file=" + resolved
                        + (f.isFile() ? " (ok)" : " (MISSING)"));
            }
        } catch (Throwable ignored) {
            // Map jar is optional until it is packed into the WAR.
        }
    }

    @Override
    protected void doGet(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        this.doPost(request, response);
    }

    @Override
    protected void doPost(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        String repName = request.getParameter("_repName");
        PerformanceProfiler perf = new PerformanceProfiler("jasper_execute", repName);
        try {
            String repFormat = request.getParameter("_repFormat");
            String dataSource = request.getParameter("_dataSource");
            String outFilename = request.getParameter("_outFilename");
            if (repName == null || repName.trim().isEmpty()) {
                this.sendError(response, request, 400, "Missing required parameter: _repName");
                return;
            }
            if (repFormat == null || repFormat.trim().isEmpty()) {
                this.sendError(response, request, 400, "Missing required parameter: _repFormat");
                return;
            }
            if (dataSource == null || dataSource.trim().isEmpty()) {
                this.sendError(response, request, 400, "Missing required parameter: _dataSource");
                return;
            }
            String format = FormatUtil.normalizeFormat(repFormat);
            if (!FormatUtil.isSupported(format)) {
                this.sendError(response, request, 400,
                        "Unsupported format: " + repFormat + ". Supported: PDF, HTML, XLSX, DOCX");
                return;
            }
            perf.attr("format", format);
            perf.attr("datasource", dataSource);

            String reportsDirAbs = ReportService.getReportsDir();
            this.getServletContext().log("acugis-report-server reportsDir=" + reportsDirAbs
                    + " contextPath=" + request.getContextPath());
            this.getServletContext().log("acugis-report-server repName(raw)=" + repName);

            File reportsDir = new File(reportsDirAbs);
            File reportFile;
            PerformanceProfiler.Span lookupSpan = perf.start("report_lookup");
            try {
                reportFile = ReportService.resolveReportFile(reportsDir, repName);
            } catch (FileNotFoundException e) {
                lookupSpan.done();
                this.sendError(response, request, 404, e, "Report not found");
                return;
            }
            lookupSpan.done();

            ReportParameterBinder.LoadResult loaded;
            PerformanceProfiler.Span loadSpan = perf.start("report_compilation");
            try {
                loaded = ReportParameterBinder.loadOrCompileReportDetailed(reportFile);
            } catch (JRException e) {
                loadSpan.done();
                this.sendError(response, request, 500, e, "Failed to load report");
                return;
            }
            loadSpan.done();
            JasperReport jasperReport = loaded.report;
            perf.attr("compiled_report_cache", loaded.source);
            // Distinguish load vs compile in stage naming for summaries.
            if ("memory_hit".equals(loaded.source) || "jasper_file".equals(loaded.source)) {
                perf.attr("jrxml_load", loaded.source);
            }

            Map<String, Object> jasperParams;
            PerformanceProfiler.Span bindSpan = perf.start("parameter_initialization");
            jasperParams = ReportParameterBinder.bindParameters(request, jasperReport);
            bindSpan.done();
            perf.attr("param_count", jasperParams.size());

            String duckdbSession = request.getParameter("_duckdbSession");
            String collectionData = request.getParameter("_collectionData");
            JasperPrint jasperPrint;
            String mode = fillMode(duckdbSession, collectionData);
            if ("duckdb-jdbc".equals(mode)) {
                perf.attr("fill_mode", "duckdb-jdbc");
                this.getServletContext().log("acugis-report-server fill mode=duckdb-jdbc datasource=" + dataSource);
                Connection connection = null;
                PerformanceProfiler.Span jdbcSpan = perf.start("duckdb_jdbc_acquisition");
                try {
                    connection = DuckDBSession.open(duckdbSession);
                } catch (Exception e) {
                    jdbcSpan.done();
                    this.sendError(response, request, 500, e, "Failed to open DuckDB JDBC connection");
                    return;
                }
                jdbcSpan.done();
                PerformanceProfiler.Span fillSpan = perf.start("jasper_fill");
                try {
                    jasperPrint = ReportService.fillReport(
                            jasperReport,
                            jasperParams,
                            connection,
                            reportFile.getParentFile(),
                            repName,
                            request.getParameter("_acugisResourceBase"));
                } catch (JRException e) {
                    fillSpan.done();
                    this.sendError(response, request, 500, e, "Failed to fill report");
                    return;
                } finally {
                    this.closeConnection(connection);
                }
                fillSpan.done();
            } else if ("collection".equals(mode)) {
                CollectionFill.ParseResult parsed;
                PerformanceProfiler.Span parseSpan = perf.start("collection_parse");
                try {
                    parsed = CollectionFill.parse(collectionData, jasperReport);
                } catch (Exception e) {
                    parseSpan.done();
                    this.sendError(response, request, 400, e, "Failed to parse _collectionData");
                    return;
                }
                parseSpan.done();
                perf.attr("fill_mode", "collection");
                perf.attr("row_count", parsed.getRowCount());
                this.getServletContext().log("acugis-report-server fill mode=collection datasource="
                        + dataSource + " rows=" + parsed.getRowCount());

                PerformanceProfiler.Span fillSpan = perf.start("jasper_fill");
                try {
                    jasperPrint = ReportService.fillReport(
                            jasperReport,
                            jasperParams,
                            parsed.getDataSource(),
                            reportFile.getParentFile(),
                            repName,
                            request.getParameter("_acugisResourceBase"));
                } catch (JRException e) {
                    fillSpan.done();
                    this.sendError(response, request, 500, e, "Failed to fill report");
                    return;
                }
                fillSpan.done();
            } else {
                perf.attr("fill_mode", "jdbc");
                this.getServletContext().log("acugis-report-server fill mode=jdbc datasource=" + dataSource);

                Connection connection = null;
                PerformanceProfiler.Span jdbcSpan = perf.start("jdbc_acquisition");
                try {
                    DataSource ds = (DataSource) new InitialContext().lookup("java:comp/env/jdbc/" + dataSource);
                    connection = ds.getConnection();
                } catch (NamingException e) {
                    jdbcSpan.done();
                    this.sendError(response, request, 500, e,
                            "Failed to lookup DataSource: jdbc/" + dataSource
                                    + ". If this is Apache Cassandra / DataStax Astra or Google BigQuery, it must run through Report Server collection fill "
                                    + "(not context.xml). Set the datasource Database Type accordingly, restart the Go Report Server, "
                                    + "and re-run the report — do not rely on Generate Context XML for collection providers."
                                    + " If this is DuckDB, the Go gateway must attach _duckdbSession (DuckDB JDBC fill).");
                    return;
                } catch (SQLException e) {
                    jdbcSpan.done();
                    this.sendError(response, request, 500, e, "Failed to get database connection");
                    return;
                }
                jdbcSpan.done();

                PerformanceProfiler.Span fillSpan = perf.start("jasper_fill");
                try {
                    jasperPrint = ReportService.fillReport(
                            jasperReport,
                            jasperParams,
                            connection,
                            reportFile.getParentFile(),
                            repName,
                            request.getParameter("_acugisResourceBase"));
                } catch (JRException e) {
                    fillSpan.done();
                    this.sendError(response, request, 500, e, "Failed to fill report");
                    return;
                } finally {
                    this.closeConnection(connection);
                }
                fillSpan.done();
            }
            if (jasperPrint != null) {
                perf.attr("page_count", jasperPrint.getPages() == null ? 0 : jasperPrint.getPages().size());
            }

            String filename = outFilename;
            if (filename == null || filename.trim().isEmpty()) {
                filename = repName + FormatUtil.getExtension(format);
            } else {
                String ext = FormatUtil.getExtension(format);
                if (!filename.toLowerCase().endsWith(ext.toLowerCase())) {
                    filename = filename + ext;
                }
            }
            // Component mode is armed only when the Go embed-component route set
            // X-Acugis-Embed-Component. A client _componentId on /reports/execute
            // has no header, so this branch is skipped and the full report exports.
            if (EmbedComponentExporter.componentMode(request)) {
                String componentId = EmbedComponentExporter.trustedComponentId(request);
                if (componentId == null || !"HTML".equals(format) || jasperPrint == null) {
                    this.sendComponentError(response, 500, EmbedComponentExporter.CODE_SERVER_ERROR,
                            "component export failed");
                    return;
                }
                String html;
                PerformanceProfiler.Span exportSpan = perf.start("export_html_component");
                try {
                    html = EmbedComponentExporter.exportHtml(jasperReport, jasperPrint, componentId, request);
                } catch (EmbedComponentExporter.ComponentExportException e) {
                    exportSpan.done();
                    this.sendComponentError(response, e.status(), e.code(), e.detail());
                    return;
                } catch (JRException e) {
                    exportSpan.done();
                    this.getServletContext().log("acugis-report-server component export failed", e);
                    this.sendComponentError(response, 500, EmbedComponentExporter.CODE_SERVER_ERROR,
                            "component export failed");
                    return;
                }
                exportSpan.done();
                response.setContentType(FormatUtil.getContentType(format));
                response.setHeader("Content-Disposition", "inline; filename=\"" + filename + "\"");
                response.setHeader("Cache-Control", "no-store, no-cache, must-revalidate");
                response.setHeader("Pragma", "no-cache");
                response.setDateHeader("Expires", 0L);
                response.getWriter().write(html);
            } else {
                response.setContentType(FormatUtil.getContentType(format));
                String disposition = "HTML".equals(format) ? "inline" : "attachment";
                response.setHeader("Content-Disposition", disposition + "; filename=\"" + filename + "\"");
                if ("HTML".equals(format)) {
                    response.setHeader("Cache-Control", "no-store, no-cache, must-revalidate");
                    response.setHeader("Pragma", "no-cache");
                    response.setDateHeader("Expires", 0L);
                }
                PerformanceProfiler.Span exportSpan = perf.start("export_" + format.toLowerCase());
                try {
                    ReportService.exportReport(jasperPrint, format, response, request);
                } catch (JRException e) {
                    exportSpan.done();
                    this.sendError(response, request, 500, e, "Failed to export report");
                    return;
                }
                exportSpan.done();
            }
        } catch (Exception e) {
            this.sendError(response, request, 500, e, "Unexpected error");
        } finally {
            ReportService.endChartRenderSession();
            perf.logSummary();
        }
    }

    /**
     * Chooses the Tomcat fill path. Collection data wins over JDBC so a
     * language="collection" report never looks up java:comp/env/jdbc when
     * Go attached _collectionData.
     */
    static String fillMode(String duckdbSession, String collectionData) {
        if (duckdbSession != null && !duckdbSession.trim().isEmpty()) {
            return "duckdb-jdbc";
        }
        if (collectionData != null && !collectionData.trim().isEmpty()) {
            return "collection";
        }
        return "jdbc";
    }

    /**
     * Plain-text {@code CODE: detail} body. The Go content bridge turns that
     * into an iframe error event. Lookup failures are reported here, before
     * any HTML is written.
     */
    private void sendComponentError(HttpServletResponse response, int statusCode, String code, String detail)
            throws IOException {
        if (code == null || code.isEmpty()) {
            code = EmbedComponentExporter.CODE_SERVER_ERROR;
        }
        String text = detail == null || detail.isEmpty() ? code : code + ": " + detail;
        this.getServletContext().log("acugis-report-server component export " + text);
        response.setStatus(statusCode);
        response.setContentType("text/plain; charset=UTF-8");
        response.setHeader("Cache-Control", "no-store");
        response.getWriter().write(text);
    }

    private void sendError(HttpServletResponse response, HttpServletRequest request, int statusCode, String message)
            throws IOException {
        this.sendError(response, statusCode, null, message, request);
    }

    private void sendError(
            HttpServletResponse response,
            HttpServletRequest request,
            int statusCode,
            Throwable exception,
            String message) throws IOException {
        this.sendError(response, statusCode, exception, message, request);
    }

    private void sendError(
            HttpServletResponse response,
            int statusCode,
            Throwable exception,
            String message,
            HttpServletRequest request) throws IOException {
        Throwable rootCause = exception;
        if (rootCause != null) {
            while (rootCause.getCause() != null) {
                rootCause = rootCause.getCause();
            }
        }
        if (exception != null) {
            StringWriter sw = new StringWriter();
            PrintWriter pw = new PrintWriter(sw);
            exception.printStackTrace(pw);
            this.getServletContext().log("acugis-report-server exception: " + sw);
        }
        String fillMode = "unknown";
        if (request != null) {
            fillMode = fillMode(request.getParameter("_duckdbSession"), request.getParameter("_collectionData"));
        }
        response.setStatus(statusCode);
        response.setContentType("application/json");
        PrintWriter out = response.getWriter();
        StringBuilder json = new StringBuilder();
        json.append("{");
        json.append("\"fillMode\":\"").append(this.escapeJson(fillMode)).append("\",");
        if (rootCause != null) {
            json.append("\"exceptionClass\":\"").append(this.escapeJson(rootCause.getClass().getName())).append("\",");
            json.append("\"exceptionMessage\":\"");
            String rootMessage = rootCause.getMessage();
            if (rootMessage != null) {
                json.append(this.escapeJson(rootMessage));
            } else {
                json.append("(no message)");
            }
            json.append("\"");
        } else {
            json.append("\"error\":\"");
            if (message != null) {
                json.append(this.escapeJson(message));
            } else {
                json.append("Unknown error");
            }
            json.append("\"");
        }
        json.append("}");
        out.print(json.toString());
        out.flush();
    }

    private String escapeJson(String str) {
        if (str == null) {
            return "";
        }
        return str.replace("\\", "\\\\").replace("\"", "\\\"")
                .replace("\n", "\\n").replace("\r", "\\r").replace("\t", "\\t");
    }

    private void closeConnection(Connection connection) {
        if (connection != null) {
            try {
                connection.close();
            } catch (SQLException e) {
                e.printStackTrace();
            }
        }
    }
}
