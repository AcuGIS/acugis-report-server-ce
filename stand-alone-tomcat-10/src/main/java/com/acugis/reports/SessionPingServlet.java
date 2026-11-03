package com.acugis.reports;

import java.io.IOException;
import java.io.PrintWriter;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServlet;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import jakarta.servlet.http.HttpSession;

public class SessionPingServlet
extends HttpServlet {
    private static final long serialVersionUID = 1L;

    protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        response.setContentType("text/plain");
        PrintWriter out = response.getWriter();
        String cookieHeader = request.getHeader("Cookie");
        out.println("Cookie header: " + (cookieHeader != null ? cookieHeader : "none"));
        HttpSession session = request.getSession(false);
        if (session == null) {
            out.println("No session found");
            out.println("Note: Session is required for ImageServlet to work");
            return;
        }
        String sessionId = session.getId();
        Object jasperPrint = session.getAttribute("net.sf.jasperreports.jakarta.jasper_print");
        out.println("JSESSIONID=" + sessionId);
        out.println("hasJasperPrint=" + (jasperPrint != null));
        if (jasperPrint != null) {
            out.println("jasperPrintClass=" + jasperPrint.getClass().getName());
        } else {
            out.println("WARNING: JasperPrint not found in session!");
            out.println("Expected attribute: net.sf.jasperreports.jakarta.jasper_print");
        }
    }
}
