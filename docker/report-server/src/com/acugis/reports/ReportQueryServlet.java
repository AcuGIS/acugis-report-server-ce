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
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import javax.sql.DataSource;

import com.fasterxml.jackson.databind.node.ObjectNode;

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
        try {
            String repName = request.getParameter("_repName");
            String dataSource = request.getParameter("_dataSource");
            if (repName == null || repName.trim().isEmpty()) {
                sendError(response, 400, "Missing required parameter: _repName");
                return;
            }
            if (dataSource == null || dataSource.trim().isEmpty()) {
                sendError(response, 400, "Missing required parameter: _dataSource");
                return;
            }

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
            try {
                reportFile = ReportService.resolveReportFile(reportsDir, repName);
            } catch (FileNotFoundException e) {
                sendError(response, 404, e, "Report not found");
                return;
            }

            JasperReport jasperReport;
            try {
                jasperReport = ReportParameterBinder.loadOrCompileReport(reportFile);
            } catch (JRException e) {
                sendError(response, 500, e, "Failed to load report");
                return;
            }

            Map<String, Object> jasperParams = ReportParameterBinder.bindParameters(request, jasperReport, false);

            Connection connection = null;
            try {
                DataSource ds = (DataSource) new InitialContext().lookup("java:comp/env/jdbc/" + dataSource);
                connection = ds.getConnection();
            } catch (NamingException e) {
                sendError(response, 500, e, "Failed to lookup DataSource: jdbc/" + dataSource);
                return;
            } catch (SQLException e) {
                sendError(response, 500, e, "Failed to obtain JDBC connection");
                return;
            }

            try {
                ObjectNode result = ReportQueryRunner.execute(jasperReport, jasperParams, connection, maxRows);
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
                closeConnection(connection);
            }
        } catch (Exception e) {
            sendError(response, 500, e, "Unexpected error");
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
            getServletContext().log("acugis-report-server reportQuery exception: " + sw.toString());
        }
        response.setStatus(statusCode);
        response.setContentType("application/json; charset=UTF-8");
        response.setHeader("Cache-Control", "no-store");
        PrintWriter out = response.getWriter();
        StringBuilder json = new StringBuilder();
        json.append("{");
        if (rootCause != null) {
            json.append("\"error\":\"").append(escapeJson(message != null ? message : "error")).append("\",");
            json.append("\"exceptionClass\":\"").append(escapeJson(rootCause.getClass().getName())).append("\",");
            json.append("\"exceptionMessage\":\"");
            String rootMessage = rootCause.getMessage();
            json.append(escapeJson(rootMessage != null ? rootMessage : "(no message)"));
            json.append("\"");
        } else {
            json.append("\"error\":\"").append(escapeJson(message != null ? message : "Unknown error")).append("\"");
        }
        json.append("}");
        out.print(json.toString());
        out.flush();
    }

    private String escapeJson(String str) {
        if (str == null) {
            return "";
        }
        return str.replace("\\", "\\\\").replace("\"", "\\\"").replace("\n", "\\n").replace("\r", "\\r")
                .replace("\t", "\\t");
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
