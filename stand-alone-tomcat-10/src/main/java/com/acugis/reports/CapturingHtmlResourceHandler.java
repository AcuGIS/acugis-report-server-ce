package com.acugis.reports;

import java.io.UnsupportedEncodingException;
import java.net.URLEncoder;
import java.util.Map;
import net.sf.jasperreports.engine.export.HtmlResourceHandler;

/**
 * Captures HTML exporter images (charts, maps, barcodes/QR) into {@link ReportImageStore}.
 * Same delivery trick for all three: {@code img_*} URLs served by ReportImageServlet.
 */
public class CapturingHtmlResourceHandler
implements HtmlResourceHandler {
    private final String runId;
    private final Map<String, byte[]> imagesMap;
    private final String resourcePattern;

    public CapturingHtmlResourceHandler(String resourcePattern, String runId, Map<String, byte[]> imagesMap) {
        this.resourcePattern = resourcePattern;
        this.runId = runId;
        this.imagesMap = imagesMap;
    }

    public void handleResource(String id, byte[] data) {
        if (id == null || id.trim().isEmpty() || data == null || data.length == 0) {
            return;
        }
        long start = System.nanoTime();
        this.imagesMap.put(id, data);
        ReportImageStore.put(this.runId, id, data);
        String lower = id.toLowerCase();
        if (lower.endsWith(".png") || lower.endsWith(".svg")) {
            String noExt = id.substring(0, id.length() - 4);
            ReportImageStore.put(this.runId, noExt, data);
        }
        long ms = (System.nanoTime() - start) / 1_000_000L;
        ChartRenderSession session = ChartRenderSession.current();
        if (session != null) {
            session.recordHtmlCapture(ms, data.length);
        }
        if (PerformanceProfiler.isEnabled()) {
            System.out.println("[html] handleResource stored id=" + id
                    + " bytes=" + data.length
                    + " capture_ms=" + ms
                    + " runId=" + this.runId);
        } else {
            System.out.println("[html] handleResource stored id=" + id + " bytes=" + data.length + " runId=" + this.runId);
        }
    }

    public String getResourcePath(String id) {
        try {
            String encodedId = URLEncoder.encode(id, "UTF-8");
            return this.resourcePattern.replace("{0}", encodedId);
        }
        catch (UnsupportedEncodingException e) {
            return this.resourcePattern.replace("{0}", id);
        }
    }
}
