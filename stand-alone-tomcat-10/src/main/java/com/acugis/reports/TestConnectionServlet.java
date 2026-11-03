package com.acugis.reports;

import java.io.IOException;
import java.io.PrintWriter;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.ResultSet;
import java.sql.Statement;
import java.util.Properties;

import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServlet;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;

/**
 * Admin helper: load a JDBC driver and open a connection (Test Connection).
 *
 * POST application/x-www-form-urlencoded:
 *   driverClassName, url, username, password, validationQuery, connectionProperties
 */
public class TestConnectionServlet extends HttpServlet {

    private static final long serialVersionUID = 1L;

    @Override
    protected void doPost(HttpServletRequest req, HttpServletResponse resp)
            throws ServletException, IOException {
        resp.setContentType("application/json;charset=UTF-8");
        resp.setHeader("Cache-Control", "no-store");

        String driver = text(req, "driverClassName");
        String url = text(req, "url");
        String user = text(req, "username");
        String pass = req.getParameter("password");
        if (pass == null) {
            pass = "";
        }
        String validationQuery = text(req, "validationQuery");
        if (validationQuery.isEmpty()) {
            validationQuery = "SELECT 1";
        }
        String connProps = text(req, "connectionProperties");

        PrintWriter out = resp.getWriter();
        if (driver.isEmpty() || url.isEmpty()) {
            resp.setStatus(HttpServletResponse.SC_BAD_REQUEST);
            out.print("{\"ok\":false,\"message\":\"driverClassName and url are required\"}");
            return;
        }

        try {
            Class<?> cls = Class.forName(driver);
            try {
                ClassLoader cl = cls.getClassLoader();
                String loader = cl == null ? "bootstrap" : cl.getClass().getName();
                String loc = "(unknown)";
                if (cls.getProtectionDomain() != null
                        && cls.getProtectionDomain().getCodeSource() != null
                        && cls.getProtectionDomain().getCodeSource().getLocation() != null) {
                    loc = String.valueOf(cls.getProtectionDomain().getCodeSource().getLocation());
                }
                getServletContext().log("testConnection driver OK: " + driver
                        + " classLoader=" + loader + " location=" + loc);
            } catch (Throwable ignored) {
                // ignore location probe failures
            }
        } catch (ClassNotFoundException e) {
            resp.setStatus(HttpServletResponse.SC_BAD_REQUEST);
            out.print(jsonError("JDBC driver failed to load: " + driver + " (" + e.getMessage() + ")"));
            return;
        }

        Properties props = new Properties();
        if (!user.isEmpty()) {
            props.setProperty("user", user);
        }
        if (!pass.isEmpty()) {
            props.setProperty("password", pass);
        }
        applyConnectionProperties(props, connProps);

        Connection conn = null;
        Statement stmt = null;
        ResultSet rs = null;
        try {
            DriverManager.setLoginTimeout(15);
            if (url.startsWith("jdbc:sqlite:")) {
                conn = DriverManager.getConnection(url);
            } else {
                conn = DriverManager.getConnection(url, props);
            }
            if (conn == null) {
                resp.setStatus(HttpServletResponse.SC_BAD_REQUEST);
                out.print(jsonError("Driver returned a null connection"));
                return;
            }
            stmt = conn.createStatement();
            rs = stmt.executeQuery(validationQuery);
            if (rs != null && rs.next()) {
                // ok
            }
            out.print("{\"ok\":true,\"message\":\"Connection successful. Driver loaded and database is reachable.\"}");
        } catch (Exception e) {
            resp.setStatus(HttpServletResponse.SC_BAD_REQUEST);
            out.print(jsonError(e.getMessage() == null ? e.toString() : e.getMessage()));
        } finally {
            closeQuietly(rs);
            closeQuietly(stmt);
            closeQuietly(conn);
        }
    }

    private static void applyConnectionProperties(Properties props, String raw) {
        if (raw == null || raw.trim().isEmpty()) {
            return;
        }
        String[] parts = raw.split(";");
        for (String part : parts) {
            part = part.trim();
            if (part.isEmpty()) {
                continue;
            }
            int eq = part.indexOf('=');
            if (eq <= 0) {
                continue;
            }
            String k = part.substring(0, eq).trim();
            String v = part.substring(eq + 1).trim();
            if (!k.isEmpty()) {
                props.setProperty(k, v);
            }
        }
    }

    private static String text(HttpServletRequest req, String name) {
        String v = req.getParameter(name);
        return v == null ? "" : v.trim();
    }

    private static String jsonError(String message) {
        if (message == null) {
            message = "Connection failed";
        }
        String escaped = message
                .replace("\\", "\\\\")
                .replace("\"", "\\\"")
                .replace("\r", "\\r")
                .replace("\n", "\\n");
        return "{\"ok\":false,\"message\":\"" + escaped + "\"}";
    }

    private static void closeQuietly(AutoCloseable c) {
        if (c == null) {
            return;
        }
        try {
            c.close();
        } catch (Exception ignored) {
            // ignore
        }
    }
}
