package com.acugis.reports;

import java.util.logging.Level;
import java.util.logging.Logger;

/** Per-chart timing record emitted as {@code chart_perf}. */
final class ChartPerfRecord {

    private static final Logger LOG = Logger.getLogger(ChartPerfRecord.class.getName());

    String reportName = "";
    String chartKey = "";
    String chartType = "";
    String renderType = "";
    String evaluationTime = "";
    int evaluationCount;
    int datasetItemCount = -1;
    int width;
    int height;

    long createChartMs;
    long customizeMs;
    long factoryMs;
    long drawMs;
    long totalMs;
    String phase = "fill"; // fill | draw | html_capture

    void log() {
        if (!PerformanceProfiler.isEnabled()) {
            return;
        }
        StringBuilder sb = new StringBuilder(384);
        sb.append("chart_perf")
                .append(" report=").append(nullToEmpty(reportName))
                .append(" chart_key=").append(nullToEmpty(chartKey))
                .append(" chart_type=").append(nullToEmpty(chartType))
                .append(" render_type=").append(nullToEmpty(renderType))
                .append(" evaluation_time=").append(nullToEmpty(evaluationTime))
                .append(" evaluation_count=").append(evaluationCount)
                .append(" dataset_items=").append(datasetItemCount)
                .append(" width=").append(width).append(" height=").append(height)
                .append(" phase=").append(phase)
                .append(" create_chart_ms=").append(createChartMs)
                .append(" customize_ms=").append(customizeMs)
                .append(" factory_ms=").append(factoryMs)
                .append(" draw_ms=").append(drawMs)
                .append(" total_ms=").append(totalMs);
        LOG.log(Level.INFO, sb.toString());
    }

    private static String nullToEmpty(String s) {
        return s == null ? "" : s;
    }
}
