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
import javax.servlet.ServletException;
import javax.servlet.annotation.MultipartConfig;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import javax.sql.DataSource;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JasperCompileManager;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.engine.JasperReport;

@MultipartConfig(maxFileSize = 64L * 1024 * 1024, maxRequestSize = 64L * 1024 * 1024)
public class ReportServlet extends HttpServlet {
    private static final long serialVersionUID = 1L;

    public void init() throws ServletException {
        super.init();
        System.out.println("JasperReports loaded from: " + JasperCompileManager.class.getProtectionDomain().getCodeSource().getLocation());
        CollectionQueryExecuterFactory.register();
        System.setProperty("net.sf.jasperreports.awt.ignore.missing.font", "true");
        System.setProperty("net.sf.jasperreports.font.family.Arial", "Liberation Sans");
        // Legacy Designer/Server JRXML may still say language="duckdb"; treat as JDBC SQL.
        System.setProperty(
                "net.sf.jasperreports.query.executer.factory.duckdb",
                "net.sf.jasperreports.engine.query.JRJdbcQueryExecuterFactory");
        System.setProperty(
                "net.sf.jasperreports.query.executer.factory.DuckDB",
                "net.sf.jasperreports.engine.query.JRJdbcQueryExecuterFactory");
        // GeoServerAuth / ArcGISAuth read connection name → credentials for Designer-published maps.
        // Prefer -Dacugis.*.connections.file=… / ACUGIS_*_CONNECTIONS_FILE; else conf/*-connections.properties.
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
            // Map jar is optional until map-lib is populated and the WAR is rebuilt.
        }
    }

    protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        this.doPost(request, response);
    }

    protected void doPost(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        try {
            String repName = request.getParameter("_repName");
            String repFormat = request.getParameter("_repFormat");
            String dataSource = request.getParameter("_dataSource");
            String outFilename = request.getParameter("_outFilename");
            if (repName == null || repName.trim().isEmpty()) {
                this.sendError(response, 400, "Missing required parameter: _repName");
                return;
            }
            if (repFormat == null || repFormat.trim().isEmpty()) {
                this.sendError(response, 400, "Missing required parameter: _repFormat");
                return;
            }
            if (dataSource == null || dataSource.trim().isEmpty()) {
                this.sendError(response, 400, "Missing required parameter: _dataSource");
                return;
            }
            String format = FormatUtil.normalizeFormat(repFormat);
            if (!FormatUtil.isSupported(format)) {
                this.sendError(response, 400, "Unsupported format: " + repFormat + ". Supported: PDF, HTML, XLSX, DOCX");
                return;
            }

            String reportsDirAbs = ReportService.getReportsDir();
            this.getServletContext().log("acugis-report-server reportsDir=" + reportsDirAbs);
            this.getServletContext().log("acugis-report-server repName(raw)=" + repName);

            File reportsDir = new File(reportsDirAbs);
            File reportFile;
            try {
                reportFile = ReportService.resolveReportFile(reportsDir, repName);
            } catch (FileNotFoundException e) {
                this.sendError(response, 404, e, "Report not found");
                return;
            }

            JasperReport jasperReport;
            try {
                jasperReport = ReportParameterBinder.loadOrCompileReport(reportFile);
            } catch (JRException e) {
                this.sendError(response, 500, e, "Failed to load report");
                return;
            }

            Map<String, Object> jasperParams = ReportParameterBinder.bindParameters(request, jasperReport);

            String duckdbSession = request.getParameter("_duckdbSession");
            String collectionData = request.getParameter("_collectionData");
            JasperPrint jasperPrint;
            if (duckdbSession != null && !duckdbSession.trim().isEmpty()) {
                // DuckDB: DriverManager Connection → Jasper REPORT_CONNECTION (same as Designer).
                // Do not fetch rows / JRMapCollectionDataSource for DuckDB.
                this.getServletContext().log("acugis-report-server fill mode=duckdb-jdbc datasource=" + dataSource);
                Connection connection = null;
                try {
                    connection = DuckDBSession.open(duckdbSession);
                    jasperPrint = ReportService.fillReport(
                            jasperReport,
                            jasperParams,
                            connection,
                            reportFile.getParentFile());
                } catch (JRException e) {
                    this.sendError(response, 500, e, "Failed to fill report");
                    return;
                } catch (Exception e) {
                    this.sendError(response, 500, e, "Failed to open DuckDB JDBC connection");
                    return;
                } finally {
                    this.closeConnection(connection);
                }
            } else if (collectionData != null && !collectionData.trim().isEmpty()) {
                CollectionFill.ParseResult parsed;
                try {
                    parsed = CollectionFill.parse(collectionData, jasperReport);
                } catch (Exception e) {
                    this.sendError(response, 400, e, "Failed to parse _collectionData");
                    return;
                }
                this.getServletContext().log("acugis-report-server fill mode=collection datasource="
                        + dataSource + " rows=" + parsed.getRowCount());
                try {
                    jasperPrint = ReportService.fillReport(
                            jasperReport,
                            jasperParams,
                            parsed.getDataSource(),
                            reportFile.getParentFile());
                } catch (JRException e) {
                    this.sendError(response, 500, e, "Failed to fill report");
                    return;
                }
            } else {
                this.getServletContext().log("acugis-report-server fill mode=jdbc datasource=" + dataSource);

                Connection connection = null;
                try {
                    DataSource ds = (DataSource) new InitialContext().lookup("java:comp/env/jdbc/" + dataSource);
                    connection = ds.getConnection();
                } catch (NamingException e) {
                    this.sendError(response, 500, e,
                            "Failed to lookup DataSource: jdbc/" + dataSource
                                    + ". If this is Apache Cassandra / DataStax Astra, it must run through Report Server collection fill "
                                    + "(not context.xml). Set the datasource Database Type to Apache Cassandra, restart the Go Report Server, "
                                    + "and re-run the report — do not rely on Generate Context XML for Cassandra."
                                    + " If this is DuckDB, the Go gateway must attach _duckdbSession (DuckDB JDBC fill).");
                    return;
                } catch (SQLException e) {
                    this.sendError(response, 500, e, "Failed to get database connection");
                    return;
                }

                try {
                    jasperPrint = ReportService.fillReport(
                            jasperReport,
                            jasperParams,
                            connection,
                            reportFile.getParentFile());
                } catch (JRException e) {
                    this.sendError(response, 500, e, "Failed to fill report");
                    return;
                } finally {
                    this.closeConnection(connection);
                }
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
            response.setContentType(FormatUtil.getContentType(format));
            String disposition = "HTML".equals(format) ? "inline" : "attachment";
            response.setHeader("Content-Disposition", disposition + "; filename=\"" + filename + "\"");
            if ("HTML".equals(format)) {
                response.setHeader("Cache-Control", "no-store, no-cache, must-revalidate");
                response.setHeader("Pragma", "no-cache");
                response.setDateHeader("Expires", 0L);
            }
            try {
                ReportService.exportReport(jasperPrint, format, response, request);
            } catch (JRException e) {
                this.sendError(response, 500, e, "Failed to export report");
            }
        } catch (Exception e) {
            this.sendError(response, 500, e, "Unexpected error");
        }
    }

    private void sendError(HttpServletResponse response, int statusCode, String message) throws IOException {
        this.sendError(response, statusCode, null, message);
    }

    private void sendError(HttpServletResponse response, int statusCode, Throwable exception, String message) throws IOException {
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
            this.getServletContext().log("acugis-report-server exception: " + sw.toString());
        }
        response.setStatus(statusCode);
        response.setContentType("application/json");
        PrintWriter out = response.getWriter();
        StringBuilder json = new StringBuilder();
        json.append("{");
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
        return str.replace("\\", "\\\\").replace("\"", "\\\"").replace("\n", "\\n").replace("\r", "\\r").replace("\t", "\\t");
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
