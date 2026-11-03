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

import com.fasterxml.jackson.databind.node.ObjectNode;

import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServlet;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import net.sf.jasperreports.engine.JRException;
import net.sf.jasperreports.engine.JasperReport;

/**
 * Executes a report's main dataset query and returns rows/metadata as JSON.
 * Does not fill or export report layout.
 */
public class ReportQueryServlet extends HttpServlet {
    private static final long serialVersionUID = 1L;

    @Override
    protected void doGet(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        doPost(request, response);
    }

    @Override
    protected void doPost(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        String repName = request.getParameter("_repName");
        PerformanceProfiler perf = new PerformanceProfiler("jasper_query", repName);
        try {
            String dataSource = request.getParameter("_dataSource");
            if (repName == null || repName.trim().isEmpty()) {
                sendError(response, 400, "Missing required parameter: _repName");
                return;
            }
            if (dataSource == null || dataSource.trim().isEmpty()) {
                sendError(response, 400, "Missing required parameter: _dataSource");
                return;
            }
            perf.attr("datasource", dataSource);

            Integer maxRows = null;
            String maxRowsParam = request.getParameter("_maxRows");
            if (maxRowsParam != null && !maxRowsParam.trim().isEmpty()) {
                try {
                    maxRows = Integer.valueOf(maxRowsParam.trim());
                } catch (NumberFormatException e) {
                    sendError(response, 400, "Invalid _maxRows: " + maxRowsParam);
                    return;
                }
            }

            File reportsDir = new File(ReportService.getReportsDir());
            File reportFile;
            PerformanceProfiler.Span lookupSpan = perf.start("report_lookup");
            try {
                reportFile = ReportService.resolveReportFile(reportsDir, repName);
            } catch (FileNotFoundException e) {
                lookupSpan.done();
                sendError(response, 404, e, "Report not found");
                return;
            }
            lookupSpan.done();

            ReportParameterBinder.LoadResult loaded;
            PerformanceProfiler.Span loadSpan = perf.start("report_compilation");
            try {
                loaded = ReportParameterBinder.loadOrCompileReportDetailed(reportFile);
            } catch (JRException e) {
                loadSpan.done();
                sendError(response, 500, e, "Failed to load report");
                return;
            }
            loadSpan.done();
            JasperReport jasperReport = loaded.report;
            perf.attr("compiled_report_cache", loaded.source);

            Map<String, Object> jasperParams;
            PerformanceProfiler.Span bindSpan = perf.start("parameter_initialization");
            // Data Explorer / report-query: do not bake JRXML defaults into the result
            // set — callers filter afterward. Empty/$X{IN} match-all stays available.
            jasperParams = ReportParameterBinder.bindParameters(request, jasperReport, false);
            bindSpan.done();

            Connection connection = null;
            PerformanceProfiler.Span jdbcSpan = perf.start("jdbc_acquisition");
            try {
                DataSource ds = (DataSource) new InitialContext().lookup("java:comp/env/jdbc/" + dataSource);
                connection = ds.getConnection();
            } catch (NamingException e) {
                jdbcSpan.done();
                sendError(response, 500, e, "Failed to lookup DataSource: jdbc/" + dataSource);
                return;
            } catch (SQLException e) {
                jdbcSpan.done();
                sendError(response, 500, e, "Failed to obtain JDBC connection");
                return;
            }
            jdbcSpan.done();

            try {
                PerformanceProfiler.Span querySpan = perf.start("dataset_query");
                ObjectNode result = ReportQueryRunner.execute(jasperReport, jasperParams, connection, maxRows);
                querySpan.done();
                if (result != null && result.has("rowCount")) {
                    perf.attr("row_count", result.get("rowCount").asLong());
                } else if (result != null && result.has("rows") && result.get("rows").isArray()) {
                    perf.attr("row_count", result.get("rows").size());
                }
                result.put("report", repName.trim());
                result.put("datasource", dataSource.trim());
                response.setStatus(200);
                response.setContentType("application/json; charset=UTF-8");
                response.setHeader("Cache-Control", "no-store");
                PrintWriter out = response.getWriter();
                out.write(result.toString());
                out.flush();
            } catch (JRException e) {
                sendError(response, 500, e, "Failed to execute report query");
            } catch (SQLException e) {
                sendError(response, 500, e, "Failed to read query result");
            } finally {
                try {
                    connection.close();
                } catch (SQLException ignored) {
                    // ignore
                }
            }
        } catch (Throwable t) {
            sendError(response, 500, t, "Unexpected error");
        } finally {
            perf.logSummary();
        }
    }

    private void sendError(HttpServletResponse response, int statusCode, String message) throws IOException {
        sendError(response, statusCode, null, message);
    }

    private void sendError(HttpServletResponse response, int statusCode, Throwable exception, String message)
            throws IOException {
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
            getServletContext().log("acugis-report-query exception: " + sw);
        }
        response.setStatus(statusCode);
        response.setContentType("application/json");
        PrintWriter out = response.getWriter();
        StringBuilder json = new StringBuilder();
        json.append("{");
        if (rootCause != null) {
            json.append("\"exceptionClass\":\"").append(escapeJson(rootCause.getClass().getName())).append("\",");
            json.append("\"exceptionMessage\":\"");
            String rootMessage = rootCause.getMessage();
            if (rootMessage != null) {
                json.append(escapeJson(rootMessage));
            } else {
                json.append("(no message)");
            }
            json.append("\"");
        } else {
            json.append("\"error\":\"");
            if (message != null) {
                json.append(escapeJson(message));
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
}
