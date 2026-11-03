package com.acugis.map;

/**
 * Opt-in flags for map fill profiling and verbose geometry tracing.
 *
 * <ul>
 *   <li>{@code ACUGIS_MAP_PERF=1} / {@code JRS_MAP_PERF=1} — per-map and summary logs</li>
 *   <li>{@code ACUGIS_PERF_DEBUG=1} / {@code JRS_PERF_DEBUG=1} — also enables map perf</li>
 *   <li>{@code ACUGIS_MAP_GEOMETRY_TRACE=1} — per-feature geometry console spam</li>
 * </ul>
 */
final class MapPerfFlags {

    private static final Object LOCK = new Object();
    private static volatile Boolean PERF;
    private static volatile Boolean GEOM_TRACE;

    private MapPerfFlags() {}

    static boolean perfEnabled() {
        Boolean local = PERF;
        if (local != null) {
            return local;
        }
        synchronized (LOCK) {
            if (PERF != null) {
                return PERF;
            }
            PERF = truthyEnv("ACUGIS_MAP_PERF")
                    || truthyEnv("JRS_MAP_PERF")
                    || truthyEnv("ACUGIS_PERF_DEBUG")
                    || truthyEnv("JRS_PERF_DEBUG")
                    || truthyProp("map.perf")
                    || truthyProp("perf.debug");
            return PERF;
        }
    }

    static boolean geometryTraceEnabled() {
        Boolean local = GEOM_TRACE;
        if (local != null) {
            return local;
        }
        synchronized (LOCK) {
            if (GEOM_TRACE != null) {
                return GEOM_TRACE;
            }
            GEOM_TRACE = truthyEnv("ACUGIS_MAP_GEOMETRY_TRACE")
                    || truthyEnv("JRS_MAP_GEOMETRY_TRACE")
                    || truthyProp("map.geometry.trace");
            return GEOM_TRACE;
        }
    }

    /** Test hook. */
    static void reset() {
        synchronized (LOCK) {
            PERF = null;
            GEOM_TRACE = null;
        }
    }

    private static boolean truthyEnv(String name) {
        return truthy(System.getenv(name));
    }

    private static boolean truthyProp(String name) {
        return truthy(System.getProperty(name));
    }

    private static boolean truthy(String raw) {
        if (raw == null || raw.trim().isEmpty()) {
            return false;
        }
        String v = raw.trim().toLowerCase();
        return "1".equals(v) || "true".equals(v) || "yes".equals(v) || "on".equals(v);
    }
}
