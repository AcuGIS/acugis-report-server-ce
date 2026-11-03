package com.acugis.reports;

import java.io.IOException;
import java.io.PrintWriter;

import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServlet;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;

/**
 * Explicit compiled-report cache maintenance.
 *
 * <pre>
 *   POST /clearReportCache              — clear all in-memory compiled reports
 *   POST /clearReportCache?_repName=x   — invalidate one report
 * </pre>
 *
 * Does not delete on-disk {@code .jasper} files; those are regenerated when JRXML is newer.
 */
public class ClearReportCacheServlet extends HttpServlet {
    private static final long serialVersionUID = 1L;

    @Override
    protected void doPost(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        handle(request, response);
    }

    @Override
    protected void doGet(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        handle(request, response);
    }

    private void handle(HttpServletRequest request, HttpServletResponse response) throws IOException {
        String repName = request.getParameter("_repName");
        if (repName == null || repName.trim().isEmpty()) {
            repName = request.getParameter("report");
        }
        int before = CompiledReportCache.size();
        if (repName != null && !repName.trim().isEmpty()) {
            CompiledReportCache.invalidateByName(new java.io.File(ReportService.getReportsDir()), repName.trim());
        } else {
            CompiledReportCache.clear();
        }
        int after = CompiledReportCache.size();

        response.setStatus(200);
        response.setContentType("application/json; charset=UTF-8");
        response.setHeader("Cache-Control", "no-store");
        PrintWriter out = response.getWriter();
        out.print("{\"ok\":true,\"before\":" + before + ",\"after\":" + after
                + ",\"action\":\""
                + (repName != null && !repName.trim().isEmpty()
                    ? "invalidate"
                    : "clear")
                + "\"");
        if (repName != null && !repName.trim().isEmpty()) {
            out.print(",\"report\":\"" + escape(repName.trim()) + "\"");
        }
        out.print("}");
        out.flush();
    }

    private static String escape(String s) {
        return s.replace("\\", "\\\\").replace("\"", "\\\"");
    }
}
