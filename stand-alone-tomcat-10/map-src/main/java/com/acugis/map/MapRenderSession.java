package com.acugis.map;

import java.awt.image.BufferedImage;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * Report-run scoped map render cache and aggregate timers.
 * Bound to the fill thread via {@link #begin(String)} / {@link #end()}.
 * If no session is open, {@link #currentOrBegin(String)} starts a transient one
 * so Designer preview still benefits from de-dup within a single expression tree.
 */
public final class MapRenderSession {

    private static final Logger LOG = Logger.getLogger(MapRenderSession.class.getName());
    private static final ThreadLocal<MapRenderSession> CURRENT = new ThreadLocal<MapRenderSession>();

    private final String reportName;
    private final boolean owned;
    private final ConcurrentHashMap<String, BufferedImage> imageCache =
            new ConcurrentHashMap<String, BufferedImage>();
    private final ConcurrentHashMap<String, AtomicInteger> evaluationCounts =
            new ConcurrentHashMap<String, AtomicInteger>();

    private final AtomicInteger mapSeq = new AtomicInteger();
    private final AtomicInteger mapCount = new AtomicInteger();
    private final AtomicInteger queryCount = new AtomicInteger();
    private final AtomicInteger cacheHits = new AtomicInteger();
    private final AtomicInteger cacheMisses = new AtomicInteger();
    private final AtomicLong totalFeatures = new AtomicLong();
    private final AtomicLong sqlMs = new AtomicLong();
    private final AtomicLong geometryDecodeMs = new AtomicLong();
    private final AtomicLong extentMs = new AtomicLong();
    private final AtomicLong basemapMs = new AtomicLong();
    private final AtomicLong featureRenderMs = new AtomicLong();
    private final AtomicLong labelRenderMs = new AtomicLong();
    private final AtomicLong legendRenderMs = new AtomicLong();
    private final AtomicLong pngEncodeMs = new AtomicLong();
    private final AtomicLong totalMapMs = new AtomicLong();
    private final AtomicLong coordTransformMs = new AtomicLong();

    private MapRenderSession(String reportName, boolean owned) {
        this.reportName = reportName == null ? "" : reportName;
        this.owned = owned;
    }

    /** Open a session for one Jasper fill (Tomcat / preview fill). */
    public static void begin(String reportName) {
        MapRenderSession existing = CURRENT.get();
        if (existing != null) {
            // Nested fill: keep outer session.
            return;
        }
        CURRENT.set(new MapRenderSession(reportName, true));
    }

    /** End session and emit {@code map_render_summary} when map perf is enabled. */
    public static void end() {
        MapRenderSession session = CURRENT.get();
        if (session == null || !session.owned) {
            return;
        }
        CURRENT.remove();
        session.logSummary();
    }

    static MapRenderSession current() {
        return CURRENT.get();
    }

    static boolean isOwned(MapRenderSession session) {
        return session != null && session.owned;
    }

    static MapRenderSession currentOrBegin(String reportName) {
        MapRenderSession session = CURRENT.get();
        if (session != null) {
            return session;
        }
        session = new MapRenderSession(reportName, false);
        CURRENT.set(session);
        return session;
    }

    /** Discard a transient (unowned) session after a standalone render. */
    static void endIfTransient() {
        MapRenderSession session = CURRENT.get();
        if (session != null && !session.owned) {
            CURRENT.remove();
            session.logSummary();
        }
    }

    String reportName() {
        return reportName;
    }

    int nextSequence() {
        return mapSeq.incrementAndGet();
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

    BufferedImage cacheGet(String key) {
        return imageCache.get(key);
    }

    void cachePut(String key, BufferedImage image) {
        if (key != null && image != null) {
            imageCache.put(key, image);
        }
    }

    void recordHit() {
        cacheHits.incrementAndGet();
    }

    void recordMiss() {
        cacheMisses.incrementAndGet();
    }

    void recordRender(MapPerfRecord rec) {
        mapCount.incrementAndGet();
        if (rec.queryCount > 0) {
            queryCount.addAndGet(rec.queryCount);
        }
        totalFeatures.addAndGet(Math.max(0, rec.featureCount));
        sqlMs.addAndGet(rec.sqlPrepareMs + rec.sqlExecutionMs);
        geometryDecodeMs.addAndGet(rec.geometryDecodeMs);
        extentMs.addAndGet(rec.extentMs);
        coordTransformMs.addAndGet(rec.coordTransformMs);
        basemapMs.addAndGet(rec.basemapMs);
        featureRenderMs.addAndGet(rec.featureRenderMs);
        labelRenderMs.addAndGet(rec.labelRenderMs);
        legendRenderMs.addAndGet(rec.legendRenderMs);
        pngEncodeMs.addAndGet(rec.pngEncodeMs);
        totalMapMs.addAndGet(rec.totalMs);
    }

    private void logSummary() {
        if (!MapPerfFlags.perfEnabled()) {
            return;
        }
        StringBuilder sb = new StringBuilder(384);
        sb.append("map_render_summary")
                .append(" report=").append(reportName)
                .append(" map_count=").append(mapCount.get())
                .append(" map_query_count=").append(queryCount.get())
                .append(" total_features=").append(totalFeatures.get())
                .append(" map_render_cache_hit=").append(cacheHits.get())
                .append(" map_render_cache_miss=").append(cacheMisses.get())
                .append(" sql_ms=").append(sqlMs.get())
                .append(" geometry_decode_ms=").append(geometryDecodeMs.get())
                .append(" extent_ms=").append(extentMs.get())
                .append(" coord_transform_ms=").append(coordTransformMs.get())
                .append(" basemap_ms=").append(basemapMs.get())
                .append(" render_ms=").append(
                        featureRenderMs.get() + labelRenderMs.get() + legendRenderMs.get())
                .append(" feature_render_ms=").append(featureRenderMs.get())
                .append(" label_render_ms=").append(labelRenderMs.get())
                .append(" legend_render_ms=").append(legendRenderMs.get())
                .append(" png_encode_ms=").append(pngEncodeMs.get())
                .append(" total_map_ms=").append(totalMapMs.get());

        int dupKeys = 0;
        int dupEvals = 0;
        for (Map.Entry<String, AtomicInteger> e : evaluationCounts.entrySet()) {
            int n = e.getValue().get();
            if (n > 1) {
                dupKeys++;
                dupEvals += n;
            }
        }
        sb.append(" duplicate_eval_keys=").append(dupKeys)
                .append(" duplicate_eval_total=").append(dupEvals);
        LOG.log(Level.INFO, sb.toString());
    }
}
