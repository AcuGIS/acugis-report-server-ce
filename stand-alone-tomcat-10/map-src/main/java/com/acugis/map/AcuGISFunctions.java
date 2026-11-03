package com.acugis.map;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.Locale;
import org.locationtech.jts.geom.Coordinate;
import org.locationtech.jts.geom.Envelope;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.geom.LineString;
import org.locationtech.jts.geom.MultiPolygon;
import org.locationtech.jts.geom.Point;
import org.locationtech.jts.geom.Polygon;
import net.postgis.jdbc.PGgeometry;
import org.postgresql.util.PGobject;
import net.sf.geographiclib.Geodesic;
import net.sf.geographiclib.PolygonArea;
import net.sf.geographiclib.PolygonResult;

/**
 * Jasper expression helpers for GIS fields.
 *
 * <p>Uses the same geometry conversion pipeline as map rendering:
 * {@link GeometryCodec} (shared with {@code GeometryLoader}).
 *
 * <p>Geometry columns must be declared as {@code java.lang.Object} in JRXML so
 * JDBC drivers (PostGIS {@code PGobject}, etc.) can deliver values. Pass
 * {@code $P{REPORT_CONNECTION}} when available so Db2 / SQL Server adapters
 * match the Map component path.
 *
 * <p>Example (CRS units):
 * <pre>com.acugis.map.AcuGISFunctions.area($F{geom}, (java.sql.Connection)$P{REPORT_CONNECTION})</pre>
 *
 * <p>Example (square meters):
 * <pre>com.acugis.map.AcuGISFunctions.areaSquareMeters($F{geom}, (java.sql.Connection)$P{REPORT_CONNECTION})</pre>
 */
public final class AcuGISFunctions {

    private AcuGISFunctions() {}

    /** Polygon / multipolygon area in CRS units (square degrees for EPSG:4326). */
    public static Double area(Object geometry) {
        return area(geometry, null);
    }

    public static Double area(Object geometry, Connection connection) {
        return Double.valueOf(require(geometry, connection).getArea());
    }

    /**
     * Polygon area in square meters.
     *
     * <p>For geographic CRS (EPSG:4326 and similar), computes geodetic area
     * in-process via GeographicLib (no per-call JDBC round-trip). PostGIS
     * {@code ST_Area(geom::geography)} is only a fallback if GeographicLib is
     * unavailable. For projected meter-based CRS, uses planar JTS area.
     */
    public static Double areaSquareMeters(Object geometry, Connection connection) {
        Geometry g = require(geometry, connection);
        int srid = resolveSrid(g, geometry);
        boolean geographic = isGeographicSrid(srid) || (srid <= 0 && looksGeographic(g));
        if (geographic && srid <= 0) {
            srid = 4326;
        }

        if (geographic) {
            try {
                return Double.valueOf(geodeticAreaSquareMeters(g));
            } catch (Throwable t) {
                // Fall through to PostGIS when GeographicLib is missing / fails.
                if (connection != null && isPostgres(connection)) {
                    Double fromDb = postgisAreaMeters(connection, geometry, g, srid, true);
                    if (fromDb != null) {
                        return fromDb;
                    }
                }
                throw new IllegalArgumentException(
                        "areaSquareMeters geodetic calculation failed for SRID "
                                + srid
                                + ": "
                                + t.getMessage(),
                        t);
            }
        }

        if (connection != null && isPostgres(connection)) {
            Double fromDb = postgisAreaMeters(connection, geometry, g, srid, false);
            if (fromDb != null) {
                return fromDb;
            }
        }
        // Projected CRS: assume linear units are meters (UTM, Web Mercator, etc.).
        return Double.valueOf(g.getArea());
    }

    /**
     * Geodetic area on WGS84 using GeographicLib (same ellipsoid PostGIS geography uses).
     * Avoids one {@code ST_Area(?::geography)} round-trip per Jasper expression evaluation.
     */
    static double geodeticAreaSquareMeters(Geometry g) {
        if (g == null || g.isEmpty()) {
            return 0d;
        }
        if (g instanceof MultiPolygon) {
            double sum = 0d;
            MultiPolygon mp = (MultiPolygon) g;
            for (int i = 0; i < mp.getNumGeometries(); i++) {
                sum += geodeticAreaSquareMeters(mp.getGeometryN(i));
            }
            return sum;
        }
        if (g instanceof Polygon) {
            Polygon p = (Polygon) g;
            double area = Math.abs(ringGeodeticArea(p.getExteriorRing()));
            for (int i = 0; i < p.getNumInteriorRing(); i++) {
                area -= Math.abs(ringGeodeticArea(p.getInteriorRingN(i)));
            }
            return Math.abs(area);
        }
        // Non-polygonal: no area.
        return 0d;
    }

    private static double ringGeodeticArea(LineString ring) {
        if (ring == null || ring.isEmpty()) {
            return 0d;
        }
        Coordinate[] coords = ring.getCoordinates();
        if (coords == null || coords.length < 3) {
            return 0d;
        }
        int n = coords.length;
        // Drop duplicate closing vertex — PolygonArea closes implicitly.
        if (n > 1 && coords[0].equals2D(coords[n - 1])) {
            n--;
        }
        PolygonArea pa = new PolygonArea(Geodesic.WGS84, false);
        for (int i = 0; i < n; i++) {
            // GeographicLib expects (lat, lon); JTS geographic coords are (x=lon, y=lat).
            pa.AddPoint(coords[i].y, coords[i].x);
        }
        PolygonResult result = pa.Compute();
        return result == null ? 0d : result.area;
    }

    /** Line length or polygon perimeter (CRS units). */
    public static Double length(Object geometry) {
        return length(geometry, null);
    }

    public static Double length(Object geometry, Connection connection) {
        return Double.valueOf(require(geometry, connection).getLength());
    }

    /** Distance between two geometries (CRS units). */
    public static Double distance(Object a, Object b) {
        return distance(a, b, null);
    }

    public static Double distance(Object a, Object b, Connection connection) {
        Geometry ga = require(a, connection);
        Geometry gb = require(b, connection);
        return Double.valueOf(ga.distance(gb));
    }

    /** Centroid point. */
    public static Object centroid(Object geometry) {
        return centroid(geometry, null);
    }

    public static Object centroid(Object geometry, Connection connection) {
        return require(geometry, connection).getCentroid();
    }

    /** Geometry type name (Point, Polygon, …). */
    public static String geometryType(Object geometry) {
        return geometryType(geometry, null);
    }

    public static String geometryType(Object geometry, Connection connection) {
        return require(geometry, connection).getGeometryType();
    }

    /** Buffer geometry by distance (CRS units). */
    public static Object buffer(Object geometry, double distance) {
        return buffer(geometry, distance, null);
    }

    public static Object buffer(Object geometry, double distance, Connection connection) {
        return require(geometry, connection).buffer(distance);
    }

    /** True if geometries intersect. */
    public static Boolean intersects(Object a, Object b) {
        return intersects(a, b, null);
    }

    public static Boolean intersects(Object a, Object b, Connection connection) {
        Geometry ga = require(a, connection);
        Geometry gb = require(b, connection);
        return Boolean.valueOf(ga.intersects(gb));
    }

    /** Bounding envelope geometry. */
    public static Object envelope(Object geometry) {
        return envelope(geometry, null);
    }

    public static Object envelope(Object geometry, Connection connection) {
        return require(geometry, connection).getEnvelope();
    }

    /** SRID, or 0 when unset. */
    public static Integer srid(Object geometry) {
        return srid(geometry, null);
    }

    public static Integer srid(Object geometry, Connection connection) {
        return Integer.valueOf(require(geometry, connection).getSRID());
    }

    /** X of first coordinate / centroid fallback. */
    public static Double x(Object geometry) {
        return x(geometry, null);
    }

    public static Double x(Object geometry, Connection connection) {
        Geometry g = require(geometry, connection);
        if (g.getCoordinate() != null) {
            return Double.valueOf(g.getCoordinate().x);
        }
        Point c = g.getCentroid();
        return c == null ? null : Double.valueOf(c.getX());
    }

    /** Y of first coordinate / centroid fallback. */
    public static Double y(Object geometry) {
        return y(geometry, null);
    }

    public static Double y(Object geometry, Connection connection) {
        Geometry g = require(geometry, connection);
        if (g.getCoordinate() != null) {
            return Double.valueOf(g.getCoordinate().y);
        }
        Point c = g.getCentroid();
        return c == null ? null : Double.valueOf(c.getY());
    }

    /**
     * Decode raw JDBC geometry to JTS using the Map component pipeline
     * ({@link GeometryCodec}).
     */
    public static Object toGeometry(Object geometry) {
        return toGeometry(geometry, null);
    }

    public static Object toGeometry(Object geometry, Connection connection) {
        if (geometry == null) {
            return null;
        }
        try {
            return GeometryCodec.toGeometry(geometry, connection);
        } catch (SQLException e) {
            throw new IllegalArgumentException("Unable to decode geometry: " + e.getMessage(), e);
        }
    }

    private static Geometry require(Object geometry, Connection connection) {
        if (geometry == null) {
            throw new IllegalArgumentException("Geometry is null");
        }
        Geometry g;
        try {
            g = GeometryCodec.toGeometry(geometry, connection);
        } catch (SQLException e) {
            throw new IllegalArgumentException("Unable to decode geometry: " + e.getMessage(), e);
        }
        if (g == null) {
            throw new IllegalArgumentException(
                    "Unable to decode geometry from " + geometry.getClass().getName()
                            + " (same pipeline as Map / GeometryCodec)");
        }
        return g;
    }

    private static Double postgisAreaMeters(
            Connection connection, Object raw, Geometry g, int srid, boolean geographic) {
        // Prefer binding the original JDBC value when it is a PostGIS type.
        if (raw instanceof PGobject || raw instanceof PGgeometry) {
            String sql = geographic
                    ? "SELECT ST_Area(?::geography)"
                    : "SELECT ST_Area(?::geometry)";
            try {
                return queryDouble(connection, sql, raw);
            } catch (SQLException ignored) {
                // Fall through to WKT path.
            }
        }

        String wkt = g.toText();
        String sql = geographic
                ? "SELECT ST_Area(ST_SetSRID(ST_GeomFromText(?), ?)::geography)"
                : "SELECT ST_Area(ST_SetSRID(ST_GeomFromText(?), ?))";
        try {
            PreparedStatement ps = connection.prepareStatement(sql);
            try {
                ps.setString(1, wkt);
                ps.setInt(2, srid > 0 ? srid : 4326);
                ResultSet rs = ps.executeQuery();
                try {
                    if (!rs.next()) {
                        return null;
                    }
                    double v = rs.getDouble(1);
                    return rs.wasNull() ? null : Double.valueOf(v);
                } finally {
                    rs.close();
                }
            } finally {
                ps.close();
            }
        } catch (SQLException e) {
            throw new IllegalArgumentException(
                    "PostGIS areaSquareMeters failed: " + e.getMessage(), e);
        }
    }

    private static Double queryDouble(Connection connection, String sql, Object bind)
            throws SQLException {
        PreparedStatement ps = connection.prepareStatement(sql);
        try {
            ps.setObject(1, bind);
            ResultSet rs = ps.executeQuery();
            try {
                if (!rs.next()) {
                    return null;
                }
                double v = rs.getDouble(1);
                return rs.wasNull() ? null : Double.valueOf(v);
            } finally {
                rs.close();
            }
        } finally {
            ps.close();
        }
    }

    private static int resolveSrid(Geometry g, Object raw) {
        if (g != null && g.getSRID() > 0) {
            return g.getSRID();
        }
        if (raw instanceof PGgeometry) {
            int s = ((PGgeometry) raw).getGeometry().getSrid();
            if (s > 0) {
                return s;
            }
        }
        if (raw instanceof PGobject) {
            String value = ((PGobject) raw).getValue();
            if (value != null) {
                String u = value.trim().toUpperCase(Locale.US);
                if (u.startsWith("SRID=")) {
                    int semi = u.indexOf(';');
                    if (semi > 5) {
                        try {
                            return Integer.parseInt(u.substring(5, semi).trim());
                        } catch (NumberFormatException ignored) {
                            /* fall through */
                        }
                    }
                }
            }
        }
        return 0;
    }

    private static boolean isGeographicSrid(int srid) {
        // Common lon/lat geographic CRS codes.
        return srid == 4326
                || srid == 4269
                || srid == 4258
                || srid == 4019
                || srid == 4267
                || srid == 4121
                || srid == 4283;
    }

    private static boolean looksGeographic(Geometry g) {
        if (g == null || g.isEmpty()) {
            return false;
        }
        Envelope env = g.getEnvelopeInternal();
        if (env.getWidth() > 360 || env.getHeight() > 180) {
            return false;
        }
        Coordinate c = g.getCoordinate();
        if (c == null) {
            return env.getMinX() >= -180
                    && env.getMaxX() <= 180
                    && env.getMinY() >= -90
                    && env.getMaxY() <= 90;
        }
        return c.x >= -180 && c.x <= 180 && c.y >= -90 && c.y <= 90;
    }

    private static boolean isPostgres(Connection connection) {
        try {
            String product = connection.getMetaData().getDatabaseProductName();
            if (product == null) {
                return false;
            }
            String p = product.toLowerCase(Locale.US);
            return p.contains("postgres") || p.contains("postgis");
        } catch (SQLException e) {
            return false;
        }
    }

    // ── Type conversion helpers (calculated-field TO_* macros) ─────────────

    /**
     * Null-safe cast to Integer. Accepts Number or numeric String ("5", "5.0").
     * Returns null for null/blank/unparseable values (does not throw).
     */
    public static Integer toInteger(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Number) {
            return Integer.valueOf(((Number) value).intValue());
        }
        String s = value.toString().trim();
        if (s.isEmpty()) {
            return null;
        }
        try {
            return Integer.valueOf(s);
        } catch (NumberFormatException e) {
            try {
                return Integer.valueOf((int) Double.parseDouble(s));
            } catch (NumberFormatException e2) {
                return null;
            }
        }
    }

    /** Null-safe cast to Long. */
    public static Long toLong(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Number) {
            return Long.valueOf(((Number) value).longValue());
        }
        String s = value.toString().trim();
        if (s.isEmpty()) {
            return null;
        }
        try {
            return Long.valueOf(s);
        } catch (NumberFormatException e) {
            try {
                return Long.valueOf((long) Double.parseDouble(s));
            } catch (NumberFormatException e2) {
                return null;
            }
        }
    }

    /** Null-safe cast to Double. */
    public static Double toDouble(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Number) {
            return Double.valueOf(((Number) value).doubleValue());
        }
        String s = value.toString().trim();
        if (s.isEmpty()) {
            return null;
        }
        try {
            return Double.valueOf(s);
        } catch (NumberFormatException e) {
            return null;
        }
    }

    /** Null-safe cast to BigDecimal. */
    public static java.math.BigDecimal toDecimal(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof java.math.BigDecimal) {
            return (java.math.BigDecimal) value;
        }
        if (value instanceof Number) {
            return java.math.BigDecimal.valueOf(((Number) value).doubleValue());
        }
        String s = value.toString().trim();
        if (s.isEmpty()) {
            return null;
        }
        try {
            return new java.math.BigDecimal(s);
        } catch (NumberFormatException e) {
            return null;
        }
    }

    /** Null-safe cast to Boolean. */
    public static Boolean toBoolean(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Boolean) {
            return (Boolean) value;
        }
        if (value instanceof Number) {
            return Boolean.valueOf(((Number) value).doubleValue() != 0.0);
        }
        String s = value.toString().trim();
        if (s.isEmpty()) {
            return null;
        }
        return Boolean.valueOf(s);
    }

    /** Null-safe toString. */
    public static String toStringValue(Object value) {
        return value == null ? null : value.toString();
    }

    /**
     * Null-safe cast to {@link java.util.Date}.
     * Accepts Date, or yyyy-MM-dd / yyyy-MM-dd HH:mm:ss strings.
     */
    public static java.util.Date toDate(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof java.util.Date) {
            return (java.util.Date) value;
        }
        String s = value.toString().trim();
        if (s.isEmpty()) {
            return null;
        }
        String[] patterns = {"yyyy-MM-dd", "yyyy-MM-dd HH:mm:ss", "MM/dd/yyyy", "dd/MM/yyyy"};
        for (String p : patterns) {
            try {
                java.text.SimpleDateFormat fmt = new java.text.SimpleDateFormat(p);
                fmt.setLenient(false);
                return fmt.parse(s);
            } catch (Exception ignored) {
                // try next
            }
        }
        return null;
    }

    /** Null-safe cast to {@link java.sql.Timestamp}. */
    public static java.sql.Timestamp toTimestamp(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof java.sql.Timestamp) {
            return (java.sql.Timestamp) value;
        }
        if (value instanceof java.util.Date) {
            return new java.sql.Timestamp(((java.util.Date) value).getTime());
        }
        String s = value.toString().trim();
        if (s.isEmpty()) {
            return null;
        }
        try {
            return java.sql.Timestamp.valueOf(s);
        } catch (IllegalArgumentException e) {
            java.util.Date d = toDate(value);
            return d == null ? null : new java.sql.Timestamp(d.getTime());
        }
    }
}
