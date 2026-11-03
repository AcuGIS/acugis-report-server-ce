package com.acugis.map;

import java.awt.image.BufferedImage;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import org.locationtech.jts.geom.Envelope;
import org.locationtech.jts.geom.Geometry;

/**
 * Phase A+B+C PostGIS map renderer for AcuGIS Report Designer / Report Server.
 *
 * <pre>
 * MapRenderer.render($P{REPORT_CONNECTION}, sql, 400, 300)
 * MapRenderer.render($P{REPORT_CONNECTION}, sql, 400, 300, styleJson)
 * MapRenderer.render($P{REPORT_CONNECTION}, sql, 400, 300, styleJson, basemap, srs)
 * MapRenderer.render($P{REPORT_CONNECTION}, sql, 400, 300, styleJson, basemap, srs, latitudeField, longitudeField)
 * </pre>
 *
 * <p>Within one report fill ({@link MapRenderSession}), identical map evaluations
 * reuse a cached {@link BufferedImage} so Jasper re-evaluation during layout/pagination
 * does not re-query or re-draw.
 */
public final class MapRenderer {

    private static final Pattern GROUP_EQ = Pattern.compile(
            "(?i)(?:\\bWHERE\\b|\\bAND\\b)\\s+[\\w.\"`\\[\\]]+\\s*=\\s*(?:'([^']*)'|(\\d+(?:\\.\\d+)?)|NULL)\\s*$");

    private MapRenderer() {}

    /** Single-symbol / default render (hardcoded defaults if no style JSON). */
    public static BufferedImage render(Connection connection, String sql, int width, int height) {
        return render(connection, sql, width, height, null, null, null);
    }

    /**
     * Render with Server-aligned symbology JSON (renderer, singleSymbol, field,
     * classification, classes, colorRamp, categories, showLegend).
     * Phase C adds proportional, heatmap, and inset legend when showLegend is true.
     */
    public static BufferedImage render(
            Connection connection, String sql, int width, int height, String styleJson) {
        return render(connection, sql, width, height, styleJson, null, null);
    }

    /**
     * Render with optional XYZ basemap underlay
     * ({@code none|osm|openfreemap_light|openfreemap_dark|esri_imagery}).
     * {@code carto_light} and {@code carto_dark} render the OpenFreeMap equivalents.
     * and SRS hint for tile alignment ({@code EPSG:4326} / {@code EPSG:3857}).
     */
    public static BufferedImage render(
            Connection connection,
            String sql,
            int width,
            int height,
            String styleJson,
            String basemap,
            String srs) {
        return render(connection, sql, width, height, styleJson, basemap, srs, null, null);
    }

    /**
     * Render with optional latitude/longitude result-column names.
     * When both names are non-blank, features are numeric points (EPSG:4326).
     * Existing callers that omit the names keep the geometry-column path.
     */
    public static BufferedImage render(
            Connection connection,
            String sql,
            int width,
            int height,
            String styleJson,
            String basemap,
            String srs,
            String latitudeField,
            String longitudeField) {
        int w = Math.max(1, width);
        int h = Math.max(1, height);
        long totalStart = System.nanoTime();
        MapRenderSession session = MapRenderSession.currentOrBegin("");
        try {
            if (connection == null) {
                throw new IllegalArgumentException("Database connection is null");
            }
            if (sql == null || sql.trim().isEmpty()) {
                throw new IllegalArgumentException("Map SQL is empty");
            }
            String trimmedSql = sql.trim();
            String styleKey = styleJson == null ? "" : styleJson.trim();
            String basemapKey = BasemapTiles.normalize(basemap);
            String latKey = latitudeField == null ? "" : latitudeField.trim();
            String lonKey = longitudeField == null ? "" : longitudeField.trim();
            boolean coordinateMode = !latKey.isEmpty() && !lonKey.isEmpty();
            String srsKey = srs == null ? "" : srs.trim();
            if (coordinateMode && srsKey.isEmpty()) {
                srsKey = "EPSG:4326";
            }

            String cacheKey = cacheKey(trimmedSql, styleKey, w, h, basemapKey, srsKey, latKey, lonKey);
            String mapUuid = shortHash(elementIdentity(trimmedSql, styleKey, w, h, basemapKey, srsKey, latKey, lonKey));
            String groupValue = inferGroupValue(trimmedSql);
            String evalKey = mapUuid + "|" + groupValue;
            int evaluationCount = session.noteEvaluation(evalKey);
            int mapSequence = session.nextSequence();

            BufferedImage cached = session.cacheGet(cacheKey);
            if (cached != null) {
                session.recordHit();
                MapPerfRecord hit = new MapPerfRecord();
                hit.reportName = session.reportName();
                hit.mapUuid = mapUuid;
                hit.mapSequence = mapSequence;
                hit.groupValue = groupValue;
                hit.extentMode = inferExtentMode(trimmedSql, groupValue);
                hit.datasourceType = datasourceType(connection);
                hit.evaluationCount = evaluationCount;
                hit.cacheHit = true;
                hit.width = w;
                hit.height = h;
                hit.totalMs = (System.nanoTime() - totalStart) / 1_000_000L;
                hit.log();
                if (MapPerfFlags.perfEnabled()) {
                    System.err.println("[acugis-map] map_render_cache_hit map_uuid=" + mapUuid
                            + " group=" + groupValue
                            + " evaluation_count=" + evaluationCount);
                }
                return cached;
            }
            session.recordMiss();
            if (MapPerfFlags.perfEnabled()) {
                System.err.println("[acugis-map] map_render_cache_miss map_uuid=" + mapUuid
                        + " group=" + groupValue
                        + " evaluation_count=" + evaluationCount);
            }

            MapPerfRecord rec = new MapPerfRecord();
            rec.reportName = session.reportName();
            rec.mapUuid = mapUuid;
            rec.mapSequence = mapSequence;
            rec.groupValue = groupValue;
            rec.extentMode = inferExtentMode(trimmedSql, groupValue);
            rec.evaluationCount = evaluationCount;
            rec.cacheHit = false;
            rec.width = w;
            rec.height = h;
            rec.datasourceType = datasourceType(connection);
            // REPORT_CONNECTION is acquired by the gateway before fill.
            rec.jdbcAcquisitionMs = 0L;

            Symbology sym = Symbology.parse(styleJson);
            GeometryLoader.LoadResult loaded = coordinateMode
                    ? GeometryLoader.loadFeaturesDetailed(connection, trimmedSql, latKey, lonKey)
                    : GeometryLoader.loadFeaturesDetailed(connection, trimmedSql);
            rec.sqlPrepareMs = loaded.sqlPrepareMs;
            rec.sqlExecutionMs = loaded.sqlExecutionMs;
            rec.geometryDecodeMs = loaded.geometryDecodeMs;
            rec.queryCount = loaded.queryCount;
            rec.featureCount = loaded.features == null ? 0 : loaded.features.size();
            rec.invalidSkipped = loaded.invalidSkipped;
            rec.geometryType = loaded.geometryTypeSummary;

            List<MapFeature> features = loaded.features;
            if (features == null || features.isEmpty()) {
                if (MapPerfFlags.geometryTraceEnabled() || MapPerfFlags.perfEnabled()) {
                    System.err.println("[acugis-map] no features for SQL: " + trimmedSql);
                }
                try {
                    java.nio.file.Files.write(
                            java.nio.file.Paths.get(System.getProperty("java.io.tmpdir"), "acugis-map-sql.log"),
                            (trimmedSql + System.lineSeparator()).getBytes(StandardCharsets.UTF_8),
                            java.nio.file.StandardOpenOption.CREATE,
                            java.nio.file.StandardOpenOption.TRUNCATE_EXISTING,
                            java.nio.file.StandardOpenOption.WRITE);
                } catch (Exception ignore) {
                    // best-effort debug aid for Windows GUI preview
                }
                String snippet = trimmedSql.length() > 180 ? trimmedSql.substring(0, 180) + "…" : trimmedSql;
                String hint = emptyFeatureHint(coordinateMode, loaded.invalidSkipped);
                throw new IllegalStateException(
                        "No map features decoded from SQL. " + hint + " SQL: " + snippet);
            }

            long extentStart = System.nanoTime();
            List<Geometry> geoms = new ArrayList<Geometry>(features.size());
            for (MapFeature f : features) {
                if (f.geometry != null) {
                    geoms.add(f.geometry);
                }
            }
            Envelope envelope = MapViewport.envelopeOf(geoms);
            MapViewport viewport = MapViewport.fromEnvelope(envelope, w, h, 0.08);
            rec.extentMs = (System.nanoTime() - extentStart) / 1_000_000L;

            BufferedImage image = new BufferedImage(w, h, BufferedImage.TYPE_INT_ARGB);
            MapPainter.PaintTimings paint = MapPainter.paintTimed(image, features, viewport, sym, basemap, srsKey);
            rec.basemapMs = paint.basemapMs;
            rec.featureRenderMs = paint.featureRenderMs;
            rec.labelRenderMs = paint.labelRenderMs;
            rec.legendRenderMs = paint.legendRenderMs;
            rec.coordTransformMs = paint.coordTransformMs;
            // Skip ImageIO PNG probe during fill — it duplicates work Jasper/HTML already does
            // and inflated total_map_ms under JRS_PERF_DEBUG.

            session.cachePut(cacheKey, image);
            rec.totalMs = (System.nanoTime() - totalStart) / 1_000_000L;
            session.recordRender(rec);
            rec.log();
            return image;
        } catch (Throwable t) {
            return ErrorImage.create(w, h, t);
        } finally {
            // Standalone Designer preview: close transient session after each top-level render.
            // Fill-owned sessions (MapRenderSession.begin) stay open until MapRenderSession.end().
            MapRenderSession.endIfTransient();
        }
    }

    /**
     * Coordinate maps can fail with an empty feature list for two different reasons.
     * Geometry maps keep the existing column/filter hint.
     */
    static String emptyFeatureHint(boolean coordinateMode, int invalidSkipped) {
        if (!coordinateMode) {
            return "Check geometry column / filters.";
        }
        if (invalidSkipped > 0) {
            return "SQL returned rows but all coordinates were invalid/skipped.";
        }
        return "SQL returned zero rows.";
    }

    private static String datasourceType(Connection connection) {
        try {
            DatabaseMetaData md = connection.getMetaData();
            if (md == null) {
                return "unknown";
            }
            String product = md.getDatabaseProductName();
            return product == null ? "unknown" : product.trim();
        } catch (Throwable t) {
            return "unknown";
        }
    }

    private static String inferGroupValue(String sql) {
        if (sql == null) {
            return "";
        }
        Matcher m = GROUP_EQ.matcher(sql.trim());
        if (!m.find()) {
            return "";
        }
        if (m.group(1) != null) {
            return m.group(1);
        }
        if (m.group(2) != null) {
            return m.group(2);
        }
        return "NULL";
    }

    private static String inferExtentMode(String sql, String groupValue) {
        if (groupValue != null && !groupValue.isEmpty()) {
            return "current_group";
        }
        if (sql != null) {
            String lower = sql.toLowerCase(Locale.US);
            if (lower.contains(" where ") || lower.contains("\nwhere ")) {
                return "filtered";
            }
        }
        return "full_or_unfiltered";
    }

    private static String cacheKey(
            String sql, String style, int w, int h, String basemap, String srs,
            String latitudeField, String longitudeField) {
        String base = sql + '\u0001' + style + '\u0001' + w + 'x' + h + '\u0001' + basemap + '\u0001' + srs;
        if ((latitudeField != null && !latitudeField.isEmpty())
                || (longitudeField != null && !longitudeField.isEmpty())) {
            base += '\u0001' + latitudeField + '\u0001' + longitudeField;
        }
        return shortHash(base);
    }

    /**
     * Element identity strips the trailing group equality so the same map widget
     * across groups shares one map_uuid while still caching per group via cacheKey.
     */
    private static String elementIdentity(
            String sql, String style, int w, int h, String basemap, String srs,
            String latitudeField, String longitudeField) {
        String stripped = sql;
        Matcher m = GROUP_EQ.matcher(sql.trim());
        if (m.find()) {
            stripped = sql.trim().substring(0, m.start()).trim();
        }
        String base = stripped + '\u0001' + style + '\u0001' + w + 'x' + h + '\u0001' + basemap + '\u0001' + srs;
        if ((latitudeField != null && !latitudeField.isEmpty())
                || (longitudeField != null && !longitudeField.isEmpty())) {
            base += '\u0001' + latitudeField + '\u0001' + longitudeField;
        }
        return base;
    }

    private static String shortHash(String input) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            byte[] dig = md.digest(input.getBytes(StandardCharsets.UTF_8));
            StringBuilder sb = new StringBuilder(16);
            for (int i = 0; i < 8; i++) {
                sb.append(String.format("%02x", dig[i] & 0xff));
            }
            return sb.toString();
        } catch (Exception e) {
            return Integer.toHexString(input.hashCode());
        }
    }
}
