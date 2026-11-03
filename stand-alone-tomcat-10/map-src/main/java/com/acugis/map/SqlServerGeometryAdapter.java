package com.acugis.map;

import java.lang.reflect.Method;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.sql.Connection;
import java.sql.SQLException;
import java.util.Arrays;
import java.util.Locale;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.io.ParseException;
import org.locationtech.jts.io.WKBReader;
import org.locationtech.jts.io.WKTReader;

/**
 * Converts Microsoft SQL Server {@code geometry} / {@code geography} values to JTS
 * {@link Geometry}.
 *
 * <p>Does not assume the JDBC payload is String, Blob, byte[], or WKT. Inspects the
 * runtime object, prefers Microsoft JDBC spatial APIs when present, then falls back to
 * WKB / WKT obtained from SQL Server ({@code STAsBinary} / {@code STAsText}).
 */
final class SqlServerGeometryAdapter {

    private static final String LOG = "[acugis-map-sqlserver] ";

    private SqlServerGeometryAdapter() {}

    static boolean isSqlServerSpatialTypeName(String typeName) {
        if (typeName == null || typeName.isEmpty()) {
            return false;
        }
        String u = normalizeTypeName(typeName);
        int dot = u.lastIndexOf('.');
        if (dot >= 0) {
            u = u.substring(dot + 1);
        }
        return "GEOMETRY".equals(u) || "GEOGRAPHY".equals(u);
    }

    /** Lowercase-normalize type names before comparison (GEOMETRY / Geometry / geometry). */
    static String normalizeTypeName(String typeName) {
        if (typeName == null) {
            return "";
        }
        String u = typeName.trim().toUpperCase(Locale.US);
        u = u.replace("\"", "").replace("'", "").replace("[", "").replace("]", "");
        int paren = u.indexOf('(');
        if (paren >= 0) {
            u = u.substring(0, paren).trim();
        }
        return u.trim();
    }

    static boolean isSqlServerGeometry(Object raw) {
        if (raw == null) {
            return false;
        }
        String cn = raw.getClass().getName().toLowerCase(Locale.US);
        return cn.contains("microsoft.sql.geometry")
                || cn.contains("microsoft.sql.geography")
                || cn.endsWith(".geometry")
                || cn.endsWith(".geography")
                || (cn.contains("sqlserver") && (cn.contains("geometry") || cn.contains("geography")));
    }

    static Geometry toGeometry(Object raw, WKBReader wkbReader, WKTReader wktReader)
            throws ParseException, SQLException {
        return toGeometry(raw, null, wkbReader, wktReader);
    }

    static Geometry toGeometry(
            Object raw, Connection connection, WKBReader wkbReader, WKTReader wktReader)
            throws ParseException, SQLException {
        if (raw == null) {
            return null;
        }
        logObject(raw);

        if (raw instanceof Geometry) {
            return (Geometry) raw;
        }
        if (raw instanceof byte[]) {
            return fromBytes((byte[]) raw, wkbReader);
        }
        if (raw instanceof String) {
            return fromText((String) raw, wkbReader, wktReader);
        }
        if (raw instanceof java.sql.Blob) {
            java.sql.Blob blob = (java.sql.Blob) raw;
            long len = blob.length();
            if (len <= 0 || len > Integer.MAX_VALUE) {
                return null;
            }
            return fromBytes(blob.getBytes(1L, (int) len), wkbReader);
        }

        // Microsoft JDBC: microsoft.sql.Geometry / Geography expose serialize() / STAsBinary /
        // getWKB / STAsText via public methods — use reflection to avoid a hard compile dependency.
        Geometry reflected = tryMicrosoftSpatialApi(raw, wkbReader, wktReader);
        if (reflected != null) {
            return reflected;
        }

        if (connection != null) {
            Geometry viaFn = viaSpatialFunction(connection, raw, wkbReader, wktReader);
            if (viaFn != null) {
                return viaFn;
            }
        }

        String asText = String.valueOf(raw).trim();
        if (looksLikeWkt(asText)) {
            return fromText(asText, wkbReader, wktReader);
        }
        if (MapPerfFlags.geometryTraceEnabled()) {
            System.err.println(LOG + "unable to convert geometry object of class "
                    + raw.getClass().getName()
                    + " (invalid spatial object)");
        }
        return null;
    }

    private static void logObject(Object raw) {
        if (!MapPerfFlags.geometryTraceEnabled()) {
            return;
        }
        try {
            System.err.println(LOG + "getObject class=" + raw.getClass().getName());
        } catch (Exception ignored) {
            // never fail rendering due to logging
        }
    }

    private static Geometry tryMicrosoftSpatialApi(
            Object raw, WKBReader wkbReader, WKTReader wktReader) {
        // Preferred binary exports from microsoft.sql.Geometry / Geography.
        for (String method : new String[] {
            "serialize", "STAsBinary", "getWKB", "asBinary", "toBytes", "getBytes"
        }) {
            try {
                Method m = raw.getClass().getMethod(method);
                Object v = m.invoke(raw);
                if (v instanceof byte[]) {
                    Geometry g = fromBytes((byte[]) v, wkbReader);
                    if (g != null) {
                        applySridIfPresent(raw, g);
                        return g;
                    }
                }
            } catch (ReflectiveOperationException | ParseException ignored) {
                // try next
            }
        }
        for (String method : new String[] {"STAsText", "asText", "getWKT", "toString"}) {
            try {
                Method m = raw.getClass().getMethod(method);
                Object v = m.invoke(raw);
                if (v instanceof String) {
                    String s = ((String) v).trim();
                    if (looksLikeWkt(s)) {
                        Geometry g = fromText(s, wkbReader, wktReader);
                        if (g != null) {
                            applySridIfPresent(raw, g);
                            return g;
                        }
                    }
                }
            } catch (ReflectiveOperationException | ParseException ignored) {
                // try next
            }
        }
        return null;
    }

    private static void applySridIfPresent(Object raw, Geometry g) {
        if (g == null || g.getSRID() > 0) {
            return;
        }
        for (String method : new String[] {"getSrid", "getSTSrid", "STSrid"}) {
            try {
                Method m = raw.getClass().getMethod(method);
                Object v = m.invoke(raw);
                if (v instanceof Number) {
                    int srid = ((Number) v).intValue();
                    if (srid > 0) {
                        g.setSRID(srid);
                    }
                    return;
                }
            } catch (ReflectiveOperationException ignored) {
                // try next
            }
        }
    }

    /**
     * Connection-side conversion is best-effort. Prefer STAsBinary in the map SQL
     * and Microsoft JDBC object methods; prepared-statement method calls on UDTs
     * are not reliably supported by the driver.
     */
    private static Geometry viaSpatialFunction(
            Connection connection, Object raw, WKBReader wkbReader, WKTReader wktReader) {
        if (connection == null || raw == null) {
            return null;
        }
        // No portable T-SQL form binds an opaque UDT parameter and calls STAsBinary.
        // Reflection (tryMicrosoftSpatialApi) and provider SQL rewrites cover the
        // supported paths; skip invalid geometries rather than failing the report.
        return null;
    }

    private static Geometry fromBytes(byte[] raw, WKBReader reader) throws ParseException {
        if (raw == null || raw.length == 0) {
            return null;
        }
        // Plain OGC WKB (STAsBinary).
        if (looksLikeWKB(raw, 0)) {
            return reader.read(raw);
        }
        // Some drivers prepend a 4-byte SRID.
        if (raw.length >= 9 && looksLikeWKB(raw, 4)) {
            int srid = ByteBuffer.wrap(raw, 0, 4).order(ByteOrder.LITTLE_ENDIAN).getInt();
            Geometry g = reader.read(Arrays.copyOfRange(raw, 4, raw.length));
            if (g != null && srid > 0) {
                g.setSRID(srid);
            }
            return g;
        }
        try {
            return reader.read(raw);
        } catch (ParseException e) {
            System.err.println(LOG + "unsupported or invalid WKB: " + e.getMessage());
            return null;
        }
    }

    private static Geometry fromText(String text, WKBReader wkbReader, WKTReader wktReader)
            throws ParseException {
        String s = text.trim();
        if (s.isEmpty()) {
            return null;
        }
        int semi = s.indexOf(';');
        if (s.regionMatches(true, 0, "SRID=", 0, 5) && semi > 0) {
            int srid = parseIntSafe(s.substring(5, semi));
            Geometry g = wktReader.read(s.substring(semi + 1).trim());
            if (srid > 0 && g != null) {
                g.setSRID(srid);
            }
            return g;
        }
        if (looksLikeHex(s)) {
            return fromBytes(WKBReader.hexToBytes(stripHexPrefix(s)), wkbReader);
        }
        return wktReader.read(s);
    }

    static boolean looksLikeWKB(byte[] bytes, int offset) {
        if (bytes == null || bytes.length - offset < 5) {
            return false;
        }
        int order = bytes[offset] & 0xFF;
        if (order != 0 && order != 1) {
            return false;
        }
        ByteOrder bo = order == 1 ? ByteOrder.LITTLE_ENDIAN : ByteOrder.BIG_ENDIAN;
        int type = ByteBuffer.wrap(bytes, offset + 1, 4).order(bo).getInt();
        int baseType = type & 0xFF;
        if (baseType == 0) {
            baseType = type & 0x1FFFFFFF;
        }
        return baseType >= 1 && baseType <= 7;
    }

    private static boolean looksLikeWkt(String s) {
        if (s == null || s.length() < 5) {
            return false;
        }
        String u = s.toUpperCase(Locale.US);
        return u.startsWith("POINT")
                || u.startsWith("LINESTRING")
                || u.startsWith("POLYGON")
                || u.startsWith("MULTI")
                || u.startsWith("GEOMETRYCOLLECTION")
                || u.startsWith("CIRCULARSTRING")
                || u.startsWith("COMPOUNDCURVE")
                || u.startsWith("CURVEPOLYGON")
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
