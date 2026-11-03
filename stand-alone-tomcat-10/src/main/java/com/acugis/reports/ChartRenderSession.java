package com.acugis.reports;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * Fill/export-scoped chart timing aggregates.
 * Opened around Jasper fill; also records HTML image-capture timings for chart PNGs.
 */
public final class ChartRenderSession {

    private static final Logger LOG = Logger.getLogger(ChartRenderSession.class.getName());
    private static final ThreadLocal<ChartRenderSession> CURRENT = new ThreadLocal<ChartRenderSession>();

    private final String reportName;
    private final ConcurrentHashMap<String, AtomicInteger> evaluationCounts =
            new ConcurrentHashMap<String, AtomicInteger>();

    private final AtomicInteger chartCount = new AtomicInteger();
    private final AtomicInteger createCount = new AtomicInteger();
    private final AtomicInteger drawCount = new AtomicInteger();
    private final AtomicInteger htmlImageCount = new AtomicInteger();
    private final AtomicLong createChartMs = new AtomicLong();
    private final AtomicLong customizeMs = new AtomicLong();
    private final AtomicLong factoryMs = new AtomicLong();
    private final AtomicLong drawMs = new AtomicLong();
    private final AtomicLong htmlCaptureMs = new AtomicLong();
    private final AtomicLong htmlBytes = new AtomicLong();
    private final AtomicLong totalChartMs = new AtomicLong();

    private ChartRenderSession(String reportName) {
        this.reportName = reportName == null ? "" : reportName;
    }

    public static void begin(String reportName) {
        if (CURRENT.get() != null) {
            return;
        }
        CURRENT.set(new ChartRenderSession(reportName));
    }

    public static void end() {
        ChartRenderSession session = CURRENT.get();
        if (session == null) {
            return;
        }
        CURRENT.remove();
        session.logSummary();
    }

    public static ChartRenderSession current() {
        return CURRENT.get();
    }

    public static ChartRenderSession currentOrBegin(String reportName) {
        ChartRenderSession session = CURRENT.get();
        if (session != null) {
            return session;
        }
        session = new ChartRenderSession(reportName);
        CURRENT.set(session);
        return session;
    }

    String reportName() {
        return reportName;
    }

    int noteEvaluation(String evalKey) {
        AtomicInteger counter = evaluationCounts.get(evalKey);
        if (counter == null) {
            AtomicInteger created = new AtomicInteger(0);
            AtomicInteger prev = evaluationCounts.putIfAbsent(evalKey, created);
            counter = prev == null ? created : prev;
        }
        return counter.incrementAndGet();
    }

    void recordCreate(long createMs, long customizeMs, long factoryMs, long totalMs) {
        chartCount.incrementAndGet();
        createCount.incrementAndGet();
        createChartMs.addAndGet(Math.max(0, createMs));
        this.customizeMs.addAndGet(Math.max(0, customizeMs));
        this.factoryMs.addAndGet(Math.max(0, factoryMs));
        totalChartMs.addAndGet(Math.max(0, totalMs));
    }

    void recordDraw(long ms) {
        drawCount.incrementAndGet();
        drawMs.addAndGet(Math.max(0, ms));
    }

    void recordHtmlCapture(long ms, int bytes) {
        htmlImageCount.incrementAndGet();
        htmlCaptureMs.addAndGet(Math.max(0, ms));
        htmlBytes.addAndGet(Math.max(0, bytes));
    }

    private void logSummary() {
        if (!PerformanceProfiler.isEnabled()) {
            return;
        }
        int dupKeys = 0;
        int dupEvals = 0;
        for (Map.Entry<String, AtomicInteger> e : evaluationCounts.entrySet()) {
            int n = e.getValue().get();
            if (n > 1) {
                dupKeys++;
                dupEvals += n;
            }
        }
        StringBuilder sb = new StringBuilder(384);
        sb.append("chart_render_summary")
                .append(" report=").append(reportName)
                .append(" chart_count=").append(chartCount.get())
                .append(" create_count=").append(createCount.get())
                .append(" draw_count=").append(drawCount.get())
                .append(" html_image_count=").append(htmlImageCount.get())
                .append(" create_chart_ms=").append(createChartMs.get())
                .append(" customize_ms=").append(customizeMs.get())
                .append(" factory_ms=").append(factoryMs.get())
                .append(" draw_ms=").append(drawMs.get())
                .append(" html_capture_ms=").append(htmlCaptureMs.get())
                .append(" html_bytes=").append(htmlBytes.get())
                .append(" total_chart_ms=").append(totalChartMs.get())
                .append(" duplicate_eval_keys=").append(dupKeys)
                .append(" duplicate_eval_total=").append(dupEvals);
        LOG.log(Level.INFO, sb.toString());
    }
}
