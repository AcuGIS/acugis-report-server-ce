package com.acugis.reports;

import java.io.IOException;
import java.lang.reflect.Method;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServlet;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import jakarta.servlet.http.HttpSession;
import net.sf.jasperreports.engine.JRPrintElement;
import net.sf.jasperreports.engine.JRPrintImage;
import net.sf.jasperreports.engine.JRPrintPage;
import net.sf.jasperreports.engine.JasperPrint;
import net.sf.jasperreports.renderers.Renderable;

/**
 * Serves HTML exporter images (charts, maps, barcodes/QR) from {@link ReportImageStore}.
 * Verbose console tracing is opt-in via {@link PerformanceProfiler} — per-image
 * System.out during normal loads adds synchronous I/O for every img_* request.
 */
public class ReportImageServlet
extends HttpServlet {
    private static final long serialVersionUID = 1L;

    public void init() throws ServletException {
        super.init();
        if (PerformanceProfiler.isEnabled()) {
            System.out.println("[report_image] ReportImageServlet initialized name=" + this.getServletName());
        }
    }

    protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        this.doPost(request, response);
    }

    protected void doPost(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        String uuid = request.getParameter("uuid");
        String image = request.getParameter("image");
        if (uuid == null || uuid.trim().isEmpty()) {
            response.sendError(400, "Missing required parameter: uuid");
            return;
        }
        if (image == null || image.trim().isEmpty()) {
            response.sendError(400, "Missing required parameter: image");
            return;
        }
        byte[] bytes = ReportImageStore.get(uuid, image);
        if (bytes == null) {
            bytes = lookupImageAlias(uuid, image);
        }
        boolean found = bytes != null;
        if (!found) {
            bytes = this.extractImageFromSession(request, image);
            found = bytes != null;
        }
        if (PerformanceProfiler.isEnabled()) {
            System.out.println("[report_image] uuid=" + uuid + " image=" + image
                    + " found=" + found + " storedCount=" + ReportImageStore.countFor(uuid));
        }
        if (!found) {
            response.sendError(404, "Image not found: uuid=" + uuid + ", image=" + image);
            return;
        }
        String contentType = this.getContentType(image);
        response.setContentType(contentType);
        response.getOutputStream().write(bytes);
        response.getOutputStream().flush();
    }

    private byte[] extractImageFromSession(HttpServletRequest request, String imageName) {
        try {
            HttpSession session = request.getSession(false);
            if (session == null) {
                return null;
            }
            Object jasperPrintObj = session.getAttribute("net.sf.jasperreports.jakarta.jasper_print");
            if (!(jasperPrintObj instanceof JasperPrint)) {
                jasperPrintObj = session.getAttribute("net.sf.jasperreports.j2ee.jasper_print");
            }
            if (!(jasperPrintObj instanceof JasperPrint)) {
                return null;
            }
            JasperPrint jasperPrint = (JasperPrint)jasperPrintObj;
            String nameWithoutExt = imageName;
            String lowerName = imageName.toLowerCase();
            if (lowerName.endsWith(".png") || lowerName.endsWith(".svg")) {
                nameWithoutExt = imageName.substring(0, imageName.length() - 4);
            }
            String[] parts = nameWithoutExt.split("_");
            if (parts.length < 4) {
                return null;
            }
            int pageIndex = Integer.parseInt(parts[1]);
            int elementIndex = Integer.parseInt(parts[2]);
            if (jasperPrint.getPages() == null || pageIndex >= jasperPrint.getPages().size()) {
                return null;
            }
            JRPrintPage page = (JRPrintPage)jasperPrint.getPages().get(pageIndex);
            if (page.getElements() == null || elementIndex >= page.getElements().size()) {
                return null;
            }
            JRPrintElement element = (JRPrintElement)page.getElements().get(elementIndex);
            if (!(element instanceof JRPrintImage)) {
                return null;
            }
            return this.extractImageBytes((JRPrintImage) element);
        }
        catch (Exception e) {
            System.err.println("[report_image] Error extracting from session: " + e.getMessage());
            return null;
        }
    }

    private byte[] extractImageBytes(JRPrintImage printImage) {
        try {
            Renderable renderer = printImage.getRenderer();
            if (renderer == null) {
                return null;
            }
            try {
                Method getImageDataMethod = renderer.getClass().getMethod("getImageData");
                Object data = getImageDataMethod.invoke(renderer);
                if (data instanceof byte[]) {
                    return (byte[]) data;
                }
            } catch (NoSuchMethodException ignored) {
                // try other methods below
            } catch (Exception e) {
                if (PerformanceProfiler.isEnabled()) {
                    System.out.println("[report_image] getImageData() failed: " + e.getMessage());
                }
            }
            Method[] methods = renderer.getClass().getMethods();
            for (Method method : methods) {
                if (method.getReturnType() != byte[].class || method.getParameterCount() != 0) continue;
                try {
                    Object data = method.invoke(renderer, new Object[0]);
                    if (data instanceof byte[]) {
                        return (byte[]) data;
                    }
                } catch (Exception ignored) {
                    // try next
                }
            }
        }
        catch (Exception e) {
            System.err.println("[report_image] Error in extractImageBytes: " + e.getMessage());
        }
        return null;
    }

    private byte[] lookupImageAlias(String uuid, String image) {
        if (image == null) {
            return null;
        }
        String lower = image.toLowerCase();
        if (lower.endsWith(".png") || lower.endsWith(".svg")) {
            String noExt = image.substring(0, image.length() - 4);
            byte[] bytes = ReportImageStore.get(uuid, noExt);
            if (bytes != null) {
                return bytes;
            }
        } else {
            byte[] bytes = ReportImageStore.get(uuid, image + ".png");
            if (bytes != null) {
                return bytes;
            }
            return ReportImageStore.get(uuid, image + ".svg");
        }
        return null;
    }

    private String getContentType(String image) {
        if (image == null) {
            return "application/octet-stream";
        }
        String lower = image.toLowerCase();
        if (lower.endsWith(".svg")) {
            return "image/svg+xml";
        }
        if (lower.endsWith(".jpg") || lower.endsWith(".jpeg")) {
            return "image/jpeg";
        }
        if (lower.endsWith(".gif")) {
            return "image/gif";
        }
        return "image/png";
    }
}
