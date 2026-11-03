package com.acugis.reports;

import java.io.IOException;
import jakarta.servlet.Filter;
import jakarta.servlet.FilterChain;
import jakarta.servlet.FilterConfig;
import jakarta.servlet.ServletException;
import jakarta.servlet.ServletRequest;
import jakarta.servlet.ServletResponse;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletRequestWrapper;
import jakarta.servlet.http.HttpServletResponse;
import jakarta.servlet.http.HttpSession;

public class ImageServletDebugFilter
implements Filter {
    public void init(FilterConfig filterConfig) throws ServletException {
        System.out.println("[ImageServletDebugFilter] Filter initialized successfully");
        System.out.println("[ImageServletDebugFilter] Filter name: " + filterConfig.getFilterName());
    }

    public void doFilter(ServletRequest request, ServletResponse response, FilterChain chain) throws IOException, ServletException {
        if (request instanceof HttpServletRequest) {
            HttpServletRequest httpRequest = (HttpServletRequest)request;
            String requestURI = httpRequest.getRequestURI();
            String queryString = httpRequest.getQueryString();
            if (requestURI.contains("/reportImage") || requestURI.contains("/report_image")) {
                System.out.println("========================================");
                System.out.println("[ImageServletDebugFilter] >>> FILTER HIT FOR /report_image <<<");
                System.out.println("[ImageServletDebugFilter] Request URI: " + requestURI);
                System.out.println("[ImageServletDebugFilter] Query String: " + queryString);
                String imageParam = httpRequest.getParameter("image");
                String uuidParam = httpRequest.getParameter("uuid");
                HttpSession session = httpRequest.getSession(false);
                String sessionId = session != null ? session.getId() : "no-session";
                String cookieHeader = httpRequest.getHeader("Cookie");
                System.out.println("[ImageServletDebugFilter] Image param: " + imageParam);
                System.out.println("[ImageServletDebugFilter] UUID param: " + uuidParam);
                System.out.println("[ImageServletDebugFilter] Cookie header: " + (cookieHeader != null ? cookieHeader : "none"));
                System.out.println("[ImageServletDebugFilter] JSESSIONID: " + sessionId);
                if (session != null) {
                    Object jasperPrint = session.getAttribute("net.sf.jasperreports.jakarta.jasper_print");
                    System.out.println("[ImageServletDebugFilter] Has JasperPrint: " + (jasperPrint != null));
                    if (jasperPrint != null) {
                        System.out.println("[ImageServletDebugFilter] JasperPrint class: " + jasperPrint.getClass().getName());
                    }
                } else {
                    System.out.println("[ImageServletDebugFilter] WARNING: No session found!");
                }
                if (imageParam != null && imageParam.endsWith(".png")) {
                    final String imageNameWithoutExt = imageParam.substring(0, imageParam.length() - 4);
                    System.out.println("[ImageServletDebugFilter] Stripping .png extension: " + imageParam + " -> " + imageNameWithoutExt);
                    HttpServletRequestWrapper wrappedRequest = new HttpServletRequestWrapper(httpRequest){

                        public String getParameter(String name) {
                            if ("image".equals(name)) {
                                return imageNameWithoutExt;
                            }
                            return super.getParameter(name);
                        }
                    };
                    chain.doFilter((ServletRequest)wrappedRequest, response);
                    System.out.println("[ImageServletDebugFilter] Response status: " + ((HttpServletResponse)response).getStatus());
                    System.out.println("========================================");
                    return;
                }
                System.out.println("[ImageServletDebugFilter] Image param does not have .png extension, passing through");
                System.out.println("========================================");
            }
        }
        chain.doFilter(request, response);
    }

    public void destroy() {
    }
}
