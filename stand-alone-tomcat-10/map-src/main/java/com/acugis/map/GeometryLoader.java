package com.acugis.map;

import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.ResultSetMetaData;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import org.locationtech.jts.geom.Coordinate;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.geom.GeometryCollection;
import org.locationtech.jts.geom.GeometryFactory;
import org.locationtech.jts.geom.LineString;
import org.locationtech.jts.geom.MultiLineString;
import org.locationtech.jts.geom.MultiPoint;
import org.locationtech.jts.geom.MultiPolygon;
import org.locationtech.jts.geom.Point;
import org.locationtech.jts.geom.Polygon;
import org.locationtech.jts.io.ParseException;
import org.locationtech.jts.io.WKBReader;
import org.locationtech.jts.io.WKTReader;

/**
 * Loads geometries (+ attribute columns) from a SQL query into map features.
 * Supports PostGIS, Oracle Spatial, MySQL/MariaDB spatial, IBM Db2 Spatial Extender,
 * Microsoft SQL Server Spatial (geometry/geography), Snowflake Spatial
 * (GEOGRAPHY/GEOMETRY via ST_ASWKB), and SQLite / GeoPackage (GeoPackage Binary).
 */
final class GeometryLoader {

    private static final GeometryFactory GF = new GeometryFactory();

    private GeometryLoader() {}

    static final class LoadResult {
        final List<MapFeature> features;
        final long sqlPrepareMs;
        final long sqlExecutionMs;
        final long geometryDecodeMs;
        final int invalidSkipped;
        final String geometryTypeSummary;
        final int queryCount;

        LoadResult(
                List<MapFeature> features,
                long sqlPrepareMs,
                long sqlExecutionMs,
                long geometryDecodeMs,
                int invalidSkipped,
                String geometryTypeSummary) {
            this.features = features;
            this.sqlPrepareMs = sqlPrepareMs;
            this.sqlExecutionMs = sqlExecutionMs;
            this.geometryDecodeMs = geometryDecodeMs;
            this.invalidSkipped = invalidSkipped;
            this.geometryTypeSummary = geometryTypeSummary == null ? "" : geometryTypeSummary;
            this.queryCount = 1;
        }
    }

    static List<MapFeature> loadFeatures(Connection connection, String sql)
            throws SQLException, ParseException {
        return loadFeaturesDetailed(connection, sql).features;
    }

    static LoadResult loadFeaturesDetailed(Connection connection, String sql)
            throws SQLException, ParseException {
        return loadFeaturesDetailed(connection, sql, null, null);
    }

    /**
     * Load features from {@code sql}. When both coordinate column names are non-blank,
     * each row becomes a JTS Point ({@code x = longitude}, {@code y = latitude}) in
     * EPSG:4326. Otherwise the existing geometry-column decoder is used unchanged.
     */
    static LoadResult loadFeaturesDetailed(
            Connection connection, String sql, String latitudeField, String longitudeField)
            throws SQLException, ParseException {
        boolean coordinateMode = hasText(latitudeField) && hasText(longitudeField);
        long prepareStart = System.nanoTime();
        Statement stmt = connection.createStatement();
        long sqlPrepareMs = (System.nanoTime() - prepareStart) / 1_000_000L;
        try {
            long execStart = System.nanoTime();
            ResultSet rs = stmt.executeQuery(sql);
            long sqlExecutionMs = (System.nanoTime() - execStart) / 1_000_000L;
            try {
                long decodeStart = System.nanoTime();
                ResultSetMetaData meta = rs.getMetaData();
                if (coordinateMode) {
                    return readCoordinateFeatures(
                            rs, meta, latitudeField.trim(), longitudeField.trim(),
                            sqlPrepareMs, sqlExecutionMs, decodeStart);
                }
                int geomIndex = findGeometryColumn(meta);
                if (geomIndex < 1) {
                    throw new IllegalArgumentException("No geometry column found in map SQL result");
                }
                boolean db2Column = isDb2GeometryColumn(meta, geomIndex);
                boolean sqlServerColumn = isSqlServerGeometryColumn(meta, geomIndex);
                boolean snowflakeColumn = isSnowflakeGeometryColumn(meta, geomIndex);

                WKBReader wkbReader = new WKBReader(GF);
                WKTReader wktReader = new WKTReader(GF);
                List<MapFeature> out = new ArrayList<MapFeature>();
                int cols = meta.getColumnCount();
                int invalidCount = 0;
                Set<String> geomTypes = new LinkedHashSet<String>();

                while (rs.next()) {
                    Object raw = readGeometryObject(rs, geomIndex, meta);
                    if (raw == null || rs.wasNull()) {
                        continue;
                    }
                    Geometry g = GeometryCodec.parse(raw, connection, db2Column, sqlServerColumn,
                            snowflakeColumn, wkbReader, wktReader);
                    if (g == null || g.isEmpty()) {
                        invalidCount++;
                        if (MapPerfFlags.geometryTraceEnabled()) {
                            System.err.println("[acugis-map] skipped undecoded geometry class="
                                    + (raw == null ? "null" : raw.getClass().getName()));
                        }
                        continue;
                    }
                    if (MapPerfFlags.geometryTraceEnabled()) {
                        System.err.println("[acugis-map] loaded geometry type=" + g.getGeometryType()
                                + " env=" + g.getEnvelopeInternal());
                    }
                    geomTypes.add(g.getGeometryType());
                    Map<String, Object> attrs = new LinkedHashMap<String, Object>();
                    for (int i = 1; i <= cols; i++) {
                        if (i == geomIndex) {
                            continue;
                        }
                        String label = meta.getColumnLabel(i);
                        if (label == null || label.isEmpty()) {
                            label = meta.getColumnName(i);
                        }
                        Object val = rs.getObject(i);
                        if (!rs.wasNull()) {
                            attrs.put(label, val);
                        }
                    }
                    List<Geometry> parts = new ArrayList<Geometry>();
                    collectSupported(g, parts);
                    for (Geometry part : parts) {
                        out.add(new MapFeature(part, attrs));
                    }
                }
                long geometryDecodeMs = (System.nanoTime() - decodeStart) / 1_000_000L;
                String typeSummary = geomTypes.isEmpty() ? "" : joinTypes(geomTypes);
                // One summary line per map (not per feature).
                System.err.println("[acugis-map] features loaded=" + out.size()
                        + " invalid skipped=" + invalidCount
                        + " geometry type=" + (typeSummary.isEmpty() ? "(none)" : typeSummary));
                return new LoadResult(
                        out, sqlPrepareMs, sqlExecutionMs, geometryDecodeMs, invalidCount, typeSummary);
            } finally {
                rs.close();
            }
        } finally {
            stmt.close();
        }
    }

    /**
     * Build point features from numeric latitude/longitude columns.
     * A bad row is skipped. Null is never replaced with 0.
     */
    private static LoadResult readCoordinateFeatures(
            ResultSet rs,
            ResultSetMetaData meta,
            String latitudeField,
            String longitudeField,
            long sqlPrepareMs,
            long sqlExecutionMs,
            long decodeStart)
            throws SQLException {
        int latIndex = findNamedColumn(meta, latitudeField);
        int lonIndex = findNamedColumn(meta, longitudeField);
        if (latIndex < 1) {
            throw new IllegalArgumentException("Latitude column not found in map SQL result: " + latitudeField);
        }
        if (lonIndex < 1) {
            throw new IllegalArgumentException("Longitude column not found in map SQL result: " + longitudeField);
        }
        int cols = meta.getColumnCount();
        List<MapFeature> out = new ArrayList<MapFeature>();
        int invalidCount = 0;
        while (rs.next()) {
            Object latRaw = rs.getObject(latIndex);
            if (rs.wasNull()) {
                latRaw = null;
            }
            Object lonRaw = rs.getObject(lonIndex);
            if (rs.wasNull()) {
                lonRaw = null;
            }
            Double lat = coordinateValue(latRaw);
            Double lon = coordinateValue(lonRaw);
            if (lat == null || lon == null) {
                invalidCount++;
                continue;
            }
            double latV = lat.doubleValue();
            double lonV = lon.doubleValue();
            if (!validLatitude(latV) || !validLongitude(lonV)) {
                invalidCount++;
                continue;
            }
            Point point = GF.createPoint(new Coordinate(lonV, latV));
            // Ordinary longitude/latitude. Do not read a database geometry SRID.
            point.setSRID(4326);
            Map<String, Object> attrs = new LinkedHashMap<String, Object>();
            for (int i = 1; i <= cols; i++) {
                if (i == latIndex || i == lonIndex) {
                    continue;
                }
                String label = meta.getColumnLabel(i);
                if (label == null || label.isEmpty()) {
                    label = meta.getColumnName(i);
                }
                Object val = rs.getObject(i);
                if (!rs.wasNull() && val != null) {
                    attrs.put(label, val);
                }
            }
            out.add(new MapFeature(point, attrs));
        }
        long geometryDecodeMs = (System.nanoTime() - decodeStart) / 1_000_000L;
        System.err.println("[acugis-map] features loaded=" + out.size()
                + " invalid skipped=" + invalidCount
                + " geometry type=" + (out.isEmpty() ? "(none)" : "Point"));
        String typeSummary = out.isEmpty() ? "" : "Point";
        return new LoadResult(
                out, sqlPrepareMs, sqlExecutionMs, geometryDecodeMs, invalidCount, typeSummary);
    }

    static Double coordinateValue(Object raw) {
        if (raw == null || raw instanceof Boolean) {
            return null;
        }
        if (raw instanceof Number) {
            double v = ((Number) raw).doubleValue();
            if (Double.isNaN(v) || Double.isInfinite(v)) {
                return null;
            }
            return Double.valueOf(v);
        }
        if (raw instanceof CharSequence) {
            String s = raw.toString().trim();
            if (s.isEmpty()) {
                return null;
            }
            try {
                double v = Double.parseDouble(s);
                if (Double.isNaN(v) || Double.isInfinite(v)) {
                    return null;
                }
                return Double.valueOf(v);
            } catch (NumberFormatException e) {
                return null;
            }
        }
        return null;
    }

    private static boolean validLatitude(double lat) {
        return lat >= -90.0 && lat <= 90.0;
    }

    private static boolean validLongitude(double lon) {
        return lon >= -180.0 && lon <= 180.0;
    }

    private static boolean hasText(String s) {
        return s != null && !s.trim().isEmpty();
    }

    private static int findNamedColumn(ResultSetMetaData meta, String name) throws SQLException {
        String want = name == null ? "" : name.trim().toLowerCase(Locale.US);
        if (want.isEmpty()) {
            return -1;
        }
        int cols = meta.getColumnCount();
        for (int i = 1; i <= cols; i++) {
            String label = safe(meta.getColumnLabel(i));
            if (label.isEmpty()) {
                label = safe(meta.getColumnName(i));
            }
            if (label.toLowerCase(Locale.US).equals(want)) {
                return i;
            }
            String col = safe(meta.getColumnName(i));
            if (!col.isEmpty() && col.toLowerCase(Locale.US).equals(want)) {
                return i;
            }
        }
        return -1;
    }

    private static String joinTypes(Set<String> types) {
        StringBuilder b = new StringBuilder();
        for (String t : types) {
            if (b.length() > 0) {
                b.append(',');
            }
            b.append(t);
        }
        return b.toString();
    }

    /**
     * Read geometry cell as a concrete payload. Db2 {@code ST_AsBinary} returns BLOB;
     * JCC {@code getObject} often yields {@link java.sql.Blob} rather than {@code byte[]}.
     * Prefer {@code getBytes} for binary SQL types (safe — not the ST_* UDT that causes -4461).
     */
    private static Object readGeometryObject(ResultSet rs, int geomIndex, ResultSetMetaData meta)
            throws SQLException {
        int jdbcType = meta.getColumnType(geomIndex);
        String typeName = safe(meta.getColumnTypeName(geomIndex)).toLowerCase(Locale.US);
        boolean binarySqlType = jdbcType == java.sql.Types.BLOB
                || jdbcType == java.sql.Types.BINARY
                || jdbcType == java.sql.Types.VARBINARY
                || jdbcType == java.sql.Types.LONGVARBINARY
                || typeName.contains("blob")
                || typeName.contains("binary")
                || typeName.contains("for bit data");

        if (binarySqlType) {
            byte[] bytes = rs.getBytes(geomIndex);
            if (bytes != null && !rs.wasNull() && bytes.length > 0) {
                if (MapPerfFlags.geometryTraceEnabled()) {
                    System.err.println("[acugis-map] geometry bytes length=" + bytes.length
                            + " jdbcType=" + jdbcType + " typeName=" + typeName);
                }
                return bytes;
            }
        }

        Object raw = rs.getObject(geomIndex);
        if (raw instanceof java.sql.Blob) {
            java.sql.Blob blob = (java.sql.Blob) raw;
            long len = blob.length();
            if (len <= 0) {
                return null;
            }
            if (len > Integer.MAX_VALUE) {
                throw new SQLException("Geometry BLOB too large: " + len);
            }
            byte[] bytes = blob.getBytes(1L, (int) len);
            if (MapPerfFlags.geometryTraceEnabled()) {
                System.err.println("[acugis-map] unwrapped java.sql.Blob length=" + bytes.length);
            }
            return bytes;
        }
        if (raw instanceof java.sql.Clob) {
            java.sql.Clob clob = (java.sql.Clob) raw;
            long len = clob.length();
            if (len <= 0) {
                return null;
            }
            if (len > Integer.MAX_VALUE) {
                throw new SQLException("Geometry CLOB too large: " + len);
            }
            return clob.getSubString(1L, (int) len);
        }
        if (raw != null && MapPerfFlags.geometryTraceEnabled()) {
            System.err.println("[acugis-map] geometry getObject class=" + raw.getClass().getName()
                    + " jdbcType=" + jdbcType + " typeName=" + typeName);
        }
        return raw;
    }

    private static boolean isDb2GeometryColumn(ResultSetMetaData meta, int index) throws SQLException {
        return Db2GeometryAdapter.isDb2GeometryTypeName(safe(meta.getColumnTypeName(index)));
    }

    private static boolean isSqlServerGeometryColumn(ResultSetMetaData meta, int index)
            throws SQLException {
        return SqlServerGeometryAdapter.isSqlServerSpatialTypeName(safe(meta.getColumnTypeName(index)));
    }

    private static boolean isSnowflakeGeometryColumn(ResultSetMetaData meta, int index)
            throws SQLException {
        String type = safe(meta.getColumnTypeName(index));
        String label = safe(meta.getColumnLabel(index));
        if (label == null || label.isEmpty()) {
            label = safe(meta.getColumnName(index));
        }
        return SnowflakeGeometryAdapter.isSnowflakeSpatialTypeName(type)
                || SnowflakeGeometryAdapter.isSnowflakeGeometryAlias(label);
    }

    private static int findGeometryColumn(ResultSetMetaData meta) throws SQLException {
        int cols = meta.getColumnCount();
        int fallback = -1;
        for (int i = 1; i <= cols; i++) {
            String type = safe(meta.getColumnTypeName(i)).toLowerCase(Locale.US);
            String name = safe(meta.getColumnLabel(i)).toLowerCase(Locale.US);
            if (SnowflakeGeometryAdapter.isSnowflakeGeometryAlias(name)) {
                return i;
            }
            if (Db2GeometryAdapter.isDb2GeometryTypeName(type)) {
                return i;
            }
            if (SqlServerGeometryAdapter.isSqlServerSpatialTypeName(type)) {
                return i;
            }
            if (SnowflakeGeometryAdapter.isSnowflakeSpatialTypeName(type)) {
                return i;
            }
            if (type.contains("geometry") || type.contains("geography")
                    || type.contains("point") || type.contains("linestring")
                    || type.contains("polygon") || type.contains("multipoint")
                    || type.contains("multilinestring") || type.contains("multipolygon")) {
                return i;
            }
            if (type.contains("sdo_geometry")) {
                return i;
            }
            // Prefer explicit WKB/WKT aliases from provider SQL rewrites.
            if (name.equals("geometry_wkb") || name.equals("geometry_wkt")
                    || name.equals("geom_wkb") || name.equals("geom_wkt")
                    || name.equals("__acugis_geometry_wkb") || name.equals("__acugis_geometry_wkt")) {
                return i;
            }
            if (fallback < 0 && (name.contains("geom") || name.contains("shape") || name.equals("wkb_geometry")
                    || name.equals("the_geom") || name.equals("way") || name.equals("location")
                    || name.contains("geometry"))) {
                fallback = i;
            }
        }
        if (fallback > 0) {
            return fallback;
        }
        return cols == 1 ? 1 : -1;
    }

    private static void collectSupported(Geometry g, List<Geometry> out) {
        if (g == null || g.isEmpty()) {
            return;
        }
        if (isSupported(g)) {
            out.add(g);
            return;
        }
        if (g instanceof GeometryCollection) {
            GeometryCollection gc = (GeometryCollection) g;
            for (int i = 0; i < gc.getNumGeometries(); i++) {
                collectSupported(gc.getGeometryN(i), out);
            }
        }
    }

    private static boolean isSupported(Geometry g) {
        return g instanceof Point
                || g instanceof MultiPoint
                || g instanceof LineString
                || g instanceof MultiLineString
                || g instanceof Polygon
                || g instanceof MultiPolygon;
    }

    private static String safe(String s) {
        return s == null ? "" : s;
    }
}
