package com.acugis.reports;

/**
 * Thread-local bridge between chart theme (createChart), customizer, and
 * renderable factory/draw during one chart evaluation.
 */
final class ChartFillProbe {

    private static final ThreadLocal<ChartFillProbe> CURRENT = new ThreadLocal<ChartFillProbe>();

    final long startedNanos = System.nanoTime();
    String chartKey = "";
    String chartType = "";
    String renderType = "";
    String evaluationTime = "";
    int datasetItemCount = -1;
    int evaluationCount;
    long createChartMs;
    long customizeMs;
    long factoryMs;

    static void begin() {
        CURRENT.set(new ChartFillProbe());
    }

    static ChartFillProbe current() {
        return CURRENT.get();
    }

    static void clear() {
        CURRENT.remove();
    }
}
