package com.acugis.map;

import java.util.Locale;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.io.ParseException;
import org.locationtech.jts.io.WKBReader;
import org.locationtech.jts.io.WKTReader;

/**
 * Converts Snowflake {@code GEOGRAPHY} / {@code GEOMETRY} payloads to JTS
 * {@link Geometry}.
 *
 * <p>Do not rely on the opaque JDBC object from {@code ResultSet.getObject}. Prefer
 * map SQL that projects {@code ST_ASWKB(...)} as {@code __acugis_geometry_wkb}
 * (binary WKB). Falls back to {@code ST_ASWKT} text when necessary.
 */
final class SnowflakeGeometryAdapter {

    private static final String LOG = "[acugis-map-snowflake] ";

    private SnowflakeGeometryAdapter() {}

    static boolean isSnowflakeSpatialTypeName(String typeName) {
        if (typeName == null || typeName.isEmpty()) {
            return false;
        }
        String u = normalizeTypeName(typeName);
        return "GEOGRAPHY".equals(u) || "GEOMETRY".equals(u);
    }

    static String normalizeTypeName(String typeName) {
        if (typeName == null) {
            return "";
        }
        String u = typeName.trim().toUpperCase(Locale.ROOT);
        u = u.replace("\"", "").replace("'", "");
        u = u.replace("::", ".");
        int paren = u.indexOf('(');
        if (paren >= 0) {
            u = u.substring(0, paren).trim();
        }
        int dot = u.lastIndexOf('.');
        if (dot >= 0) {
            u = u.substring(dot + 1).trim();
        }
        return u;
    }

    static boolean isSnowflakeGeometryAlias(String columnLabel) {
        if (columnLabel == null || columnLabel.isEmpty()) {
            return false;
        }
        String u = columnLabel.trim().toLowerCase(Locale.ROOT);
        return u.equals("__acugis_geometry_wkb")
                || u.startsWith("__acugis_geometry_wkb")
                || u.equals("__acugis_geometry_wkt")
                || u.startsWith("__acugis_geometry_wkt");
    }

    static Geometry toGeometry(Object raw, WKBReader wkbReader, WKTReader wktReader)
            throws ParseException {
        if (raw == null) {
            return null;
        }
        if (MapPerfFlags.geometryTraceEnabled()) {
            try {
                System.err.println(LOG + "conversion method payload class="
                        + raw.getClass().getName());
            } catch (Exception ignored) {
                // never fail rendering due to logging
            }
        }

        if (raw instanceof Geometry) {
            return (Geometry) raw;
        }
        if (raw instanceof byte[]) {
            return fromBytes((byte[]) raw, wkbReader);
        }
        if (raw instanceof java.sql.Blob) {
            try {
                java.sql.Blob blob = (java.sql.Blob) raw;
                long len = blob.length();
                if (len <= 0 || len > Integer.MAX_VALUE) {
                    return null;
                }
                return fromBytes(blob.getBytes(1L, (int) len), wkbReader);
            } catch (Exception e) {
                if (MapPerfFlags.geometryTraceEnabled()) {
                    System.err.println(LOG + "blob read failed: " + e.getMessage());
                }
                return null;
            }
        }
        if (raw instanceof String) {
            return fromText((String) raw, wkbReader, wktReader);
        }
        if (raw instanceof java.sql.Clob) {
            try {
                java.sql.Clob clob = (java.sql.Clob) raw;
                long len = clob.length();
                if (len <= 0 || len > Integer.MAX_VALUE) {
                    return null;
                }
                return fromText(clob.getSubString(1L, (int) len), wkbReader, wktReader);
            } catch (Exception e) {
                if (MapPerfFlags.geometryTraceEnabled()) {
                    System.err.println(LOG + "clob read failed: " + e.getMessage());
                }
                return null;
            }
        }
        String asText = String.valueOf(raw).trim();
        if (looksLikeWkt(asText) || looksLikeHex(asText)) {
            return fromText(asText, wkbReader, wktReader);
        }
        if (MapPerfFlags.geometryTraceEnabled()) {
            System.err.println(LOG + "unable to convert geometry object of class "
                    + raw.getClass().getName());
        }
        return null;
    }

    private static Geometry fromBytes(byte[] raw, WKBReader reader) throws ParseException {
        if (raw == null || raw.length == 0) {
            return null;
        }
        try {
            Geometry g = reader.read(raw);
            if (MapPerfFlags.geometryTraceEnabled()) {
                System.err.println(LOG + "geometry conversion method=WKB length=" + raw.length);
            }
            return g;
        } catch (ParseException e) {
            if (MapPerfFlags.geometryTraceEnabled()) {
                System.err.println(LOG + "invalid WKB: " + e.getMessage());
            }
            return null;
        }
    }

    private static Geometry fromText(String text, WKBReader wkbReader, WKTReader wktReader)
            throws ParseException {
        String s = text.trim();
        if (s.isEmpty()) {
            return null;
        }
        if (looksLikeHex(s)) {
            if (MapPerfFlags.geometryTraceEnabled()) {
                System.err.println(LOG + "geometry conversion method=WKB-hex");
            }
            return fromBytes(WKBReader.hexToBytes(stripHexPrefix(s)), wkbReader);
        }
        int semi = s.indexOf(';');
        if (s.regionMatches(true, 0, "SRID=", 0, 5) && semi > 0) {
            int srid = parseIntSafe(s.substring(5, semi));
            Geometry g = wktReader.read(s.substring(semi + 1).trim());
            if (srid > 0 && g != null) {
                g.setSRID(srid);
            }
            if (MapPerfFlags.geometryTraceEnabled()) {
                System.err.println(LOG + "geometry conversion method=WKT srid=" + srid);
            }
            return g;
        }
        if (MapPerfFlags.geometryTraceEnabled()) {
            System.err.println(LOG + "geometry conversion method=WKT");
        }
        return wktReader.read(s);
    }

    private static boolean looksLikeWkt(String s) {
        if (s == null || s.length() < 5) {
            return false;
        }
        String u = s.toUpperCase(Locale.ROOT);
        return u.startsWith("POINT")
                || u.startsWith("LINESTRING")
                || u.startsWith("POLYGON")
                || u.startsWith("MULTI")
                || u.startsWith("GEOMETRYCOLLECTION")
                || u.startsWith("SRID=");
    }

    private static boolean looksLikeHex(String s) {
        String hex = stripHexPrefix(s);
        if (hex.length() < 10 || (hex.length() % 2) != 0) {
            return false;
        }
        for (int i = 0; i < Math.min(hex.length(), 32); i++) {
            char c = hex.charAt(i);
            boolean ok = (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F');
            if (!ok) {
                return false;
            }
        }
        return true;
    }

    private static String stripHexPrefix(String s) {
        if (s.startsWith("\\x") || s.startsWith("\\X")) {
            return s.substring(2);
        }
        if (s.startsWith("0x") || s.startsWith("0X")) {
            return s.substring(2);
        }
        return s;
    }

    private static int parseIntSafe(String raw) {
        try {
            return Integer.parseInt(raw.trim());
        } catch (NumberFormatException e) {
            return 0;
        }
    }
}
