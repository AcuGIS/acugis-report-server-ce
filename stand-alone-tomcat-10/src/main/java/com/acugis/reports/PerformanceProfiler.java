package com.acugis.reports;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * Opt-in stage timing for the Jasper fill/export pipeline.
 * Enable with ACUGIS_PERF_DEBUG=1 / JRS_PERF_DEBUG=1 or perf.debug=true in acugis.properties.
 * When disabled, all methods are cheap no-ops and no logs are emitted.
 */
public final class PerformanceProfiler {

    private static final Logger LOG = Logger.getLogger(PerformanceProfiler.class.getName());
    private static final Object LOCK = new Object();
    private static volatile Boolean ENABLED;

    private final boolean enabled;
    private final String pipeline;
    private final String reportName;
    private final long startedAtNanos;
    private final Map<String, Long> stagesMs = new LinkedHashMap<>();
    private final Map<String, Object> attrs = new LinkedHashMap<>();

    public PerformanceProfiler(String pipeline, String reportName) {
        this.enabled = isEnabled();
        this.pipeline = pipeline == null ? "execute" : pipeline;
        this.reportName = reportName == null ? "" : reportName;
        this.startedAtNanos = System.nanoTime();
    }

    public static boolean isEnabled() {
        Boolean local = ENABLED;
        if (local != null) {
            return local;
        }
        synchronized (LOCK) {
            if (ENABLED != null) {
                return ENABLED;
            }
            ENABLED = resolveEnabled();
            return ENABLED;
        }
    }

    /** Test / admin hook to force re-read of env. */
    public static void resetEnabledCache() {
        synchronized (LOCK) {
            ENABLED = null;
        }
    }

    private static boolean resolveEnabled() {
        if (truthy(System.getenv("ACUGIS_PERF_DEBUG")) || truthy(System.getenv("JRS_PERF_DEBUG"))) {
            return true;
        }
        try {
            String prop = AcugisConfig.get().get("perf.debug", "");
            if (truthy(prop)) {
                return true;
            }
        } catch (Throwable ignored) {
            // Config may not be initialized during early class load.
        }
        return truthy(System.getProperty("perf.debug"));
    }

    private static boolean truthy(String raw) {
        if (raw == null || raw.trim().isEmpty()) {
            return false;
        }
        String v = raw.trim().toLowerCase();
        return "1".equals(v) || "true".equals(v) || "yes".equals(v) || "on".equals(v);
    }

    public boolean enabled() {
        return enabled;
    }

    public void attr(String key, Object value) {
        if (!enabled || key == null) {
            return;
        }
        attrs.put(key, value);
    }

    public void record(String stage, long elapsedMs) {
        if (!enabled || stage == null) {
            return;
        }
        stagesMs.put(stage, elapsedMs);
    }

    public Span start(String stage) {
        return new Span(stage);
    }

    public final class Span {
        private final String stage;
        private final long startNanos;
        private final boolean active;

        private Span(String stage) {
            this.stage = stage;
            this.active = enabled;
            this.startNanos = System.nanoTime();
        }

        public void done() {
            if (!active) {
                return;
            }
            long ms = (System.nanoTime() - startNanos) / 1_000_000L;
            record(stage, ms);
        }
    }

    public void logSummary() {
        if (!enabled) {
            return;
        }
        long totalMs = (System.nanoTime() - startedAtNanos) / 1_000_000L;
        StringBuilder sb = new StringBuilder(256);
        sb.append("report_perf pipeline=").append(pipeline)
                .append(" report=").append(reportName)
                .append(" total_ms=").append(totalMs)
                .append(" stages_ms={");
        boolean first = true;
        for (Map.Entry<String, Long> e : stagesMs.entrySet()) {
            if (!first) {
                sb.append(", ");
            }
            first = false;
            sb.append(e.getKey()).append('=').append(e.getValue());
        }
        sb.append('}');
        for (Map.Entry<String, Object> e : attrs.entrySet()) {
            sb.append(' ').append(e.getKey()).append('=').append(e.getValue());
        }
        LOG.log(Level.INFO, sb.toString());
    }
}
