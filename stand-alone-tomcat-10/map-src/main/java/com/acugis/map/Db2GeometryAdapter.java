package com.acugis.map;

import java.lang.reflect.Method;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Struct;
import java.util.Arrays;
import java.util.Locale;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.io.ParseException;
import org.locationtech.jts.io.WKBReader;
import org.locationtech.jts.io.WKTReader;

/**
 * Converts IBM Db2 Spatial Extender values to JTS {@link Geometry}.
 *
 * <p>IBM JCC returns ST_* columns as vendor UDT / {@link Struct} objects via {@code
 * ResultSet.getObject}. Calling {@code getBytes}/{@code getString} on those columns causes
 * SQLCODE=-4461. This adapter inspects the runtime object first, then falls back to Spatial
 * Extender {@code ST_AsBinary}/{@code ST_AsText} conversion when needed.
 */
final class Db2GeometryAdapter {

    private static final String LOG = "[acugis-map-db2] ";

    private Db2GeometryAdapter() {}

    static boolean isDb2Geometry(Object raw) {
        if (raw == null) {
            return false;
        }
        if (raw instanceof Struct) {
            return isDb2Struct((Struct) raw);
        }
        String cn = raw.getClass().getName().toLowerCase(Locale.US);
        return cn.contains("st_geometry")
                || cn.contains("st_point")
                || cn.contains("db2gse")
                || (cn.contains("db2") && cn.contains("geom"))
                || (cn.contains("db2") && cn.contains("struct"));
    }

    static boolean isDb2GeometryTypeName(String typeName) {
        if (typeName == null || typeName.isEmpty()) {
            return false;
        }
        String u = normalizeTypeName(typeName);
        int dot = u.lastIndexOf('.');
        if (dot >= 0) {
            u = u.substring(dot + 1);
        }
        return u.startsWith("ST_")
                && (u.contains("GEOM") || u.contains("POINT") || u.contains("LINE") || u.contains("POLY"));
    }

    /** Uppercase, trim, strip quotes, replace {@code ::} with {@code .}. */
    static String normalizeTypeName(String typeName) {
        if (typeName == null) {
            return "";
        }
        String u = typeName.trim().toUpperCase(Locale.US);
        u = u.replace("\"", "").replace("'", "");
        u = u.replace("::", ".");
        int paren = u.indexOf('(');
        if (paren >= 0) {
            u = u.substring(0, paren).trim();
        }
        return u.trim();
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
        if (raw instanceof Struct || raw.getClass().getName().toUpperCase(Locale.US).contains("STRUCT")) {
            Geometry fromStruct = tryStruct(raw, wkbReader, wktReader);
            if (fromStruct != null) {
                return fromStruct;
            }
        }

        Geometry reflected = tryReflectBinaryOrText(raw, wkbReader, wktReader);
        if (reflected != null) {
            return reflected;
        }

        // Driver-required conversion: opaque UDT → WKB/WKT via Spatial Extender.
        if (connection != null) {
            Geometry viaFn = viaSpatialFunction(connection, raw, wkbReader, wktReader);
            if (viaFn != null) {
                return viaFn;
            }
        }

        String asText = String.valueOf(raw).trim();
        // Object.toString() for Struct/UDT is usually "pkg.Class@hash" — never WKT.
        if (looksLikeWkt(asText)) {
            return fromText(asText, wkbReader, wktReader);
        }
        if (MapPerfFlags.geometryTraceEnabled()) {
            System.err.println(LOG + "unable to convert geometry object of class "
                    + raw.getClass().getName()
                    + " (refusing toString WKT fallback)");
        }
        return null;
    }

    private static void logObject(Object raw) {
        if (!MapPerfFlags.geometryTraceEnabled()) {
            return;
        }
        try {
            String cn = raw.getClass().getName();
            String typeName = "";
            if (raw instanceof Struct) {
                try {
                    typeName = ((Struct) raw).getSQLTypeName();
                } catch (SQLException ignored) {
                    // ignore
                }
            }
            System.err.println(LOG + "getObject class=" + cn
                    + (typeName == null || typeName.isEmpty() ? "" : " sqlType=" + typeName));
        } catch (Exception ignored) {
            // never fail rendering due to logging
        }
    }

    private static Geometry tryStruct(Object raw, WKBReader wkbReader, WKTReader wktReader)
            throws SQLException, ParseException {
        Struct st;
        try {
            st = (Struct) raw;
        } catch (ClassCastException e) {
            return null;
        }
        Object[] attrs = st.getAttributes();
        if (attrs == null) {
            return null;
        }
        for (Object attr : attrs) {
            if (attr == null) {
                continue;
            }
            if (attr instanceof byte[]) {
                Geometry g = fromBytes((byte[]) attr, wkbReader);
                if (g != null) {
                    return g;
                }
            }
            if (attr instanceof String) {
                Geometry g = fromText((String) attr, wkbReader, wktReader);
                if (g != null) {
                    return g;
                }
            }
            if (attr instanceof Struct) {
                Geometry g = tryStruct(attr, wkbReader, wktReader);
                if (g != null) {
                    return g;
                }
            }
        }
        return null;
    }

    private static Geometry tryReflectBinaryOrText(Object raw, WKBReader wkbReader, WKTReader wktReader) {
        // Some JCC wrappers expose getBytes()/getString() on the UDT object itself.
        for (String method : new String[] {"getBytes", "toBytes", "asBytes", "getWKB"}) {
            try {
                Method m = raw.getClass().getMethod(method);
                Object v = m.invoke(raw);
                if (v instanceof byte[]) {
                    Geometry g = fromBytes((byte[]) v, wkbReader);
                    if (g != null) {
                        return g;
                    }
                }
            } catch (ReflectiveOperationException | ParseException ignored) {
                // try next
            }
        }
        for (String method : new String[] {"getString", "asText", "getWKT", "toWKT"}) {
            try {
                Method m = raw.getClass().getMethod(method);
                Object v = m.invoke(raw);
                if (v instanceof String) {
                    Geometry g = fromText((String) v, wkbReader, wktReader);
                    if (g != null) {
                        return g;
                    }
                }
            } catch (ReflectiveOperationException | ParseException ignored) {
                // try next
            }
        }
        return null;
    }

    /**
     * Converts an opaque Db2 geometry UDT using Spatial Extender functions. This avoids
     * ResultSet.getBytes/getString on the ST_* column (SQLCODE=-4461).
     */
    private static Geometry viaSpatialFunction(
            Connection connection, Object raw, WKBReader wkbReader, WKTReader wktReader) {
        String[] binarySql = new String[] {
                "SELECT db2gse.ST_AsBinary(?) FROM SYSIBM.SYSDUMMY1",
                "VALUES db2gse.ST_AsBinary(?)",
        };
        for (String sql : binarySql) {
            Geometry g = viaSql(connection, sql, raw, true, wkbReader, wktReader);
            if (g != null) {
                return g;
            }
        }
        String[] textSql = new String[] {
                "SELECT db2gse.ST_AsText(?) FROM SYSIBM.SYSDUMMY1",
                "VALUES db2gse.ST_AsText(?)",
        };
        for (String sql : textSql) {
            Geometry g = viaSql(connection, sql, raw, false, wkbReader, wktReader);
            if (g != null) {
                return g;
            }
        }
        return null;
    }

    private static Geometry viaSql(
            Connection connection,
            String sql,
            Object raw,
            boolean binary,
            WKBReader wkbReader,
            WKTReader wktReader) {
        try {
            PreparedStatement ps = connection.prepareStatement(sql);
            try {
                ps.setObject(1, raw);
                ResultSet rs = ps.executeQuery();
                try {
                    if (!rs.next()) {
                        return null;
                    }
                    if (binary) {
                        byte[] bytes = rs.getBytes(1);
                        if (bytes == null || bytes.length == 0) {
                            return null;
                        }
                        return fromBytes(bytes, wkbReader);
                    }
                    String text = rs.getString(1);
                    if (text == null || text.trim().isEmpty()) {
                        return null;
                    }
                    return fromText(text, wkbReader, wktReader);
                } finally {
                    rs.close();
                }
            } finally {
                ps.close();
            }
        } catch (Exception e) {
            if (MapPerfFlags.geometryTraceEnabled()) {
                System.err.println(LOG + "spatial function convert failed (" + sql + "): " + e.getMessage());
            }
            return null;
        }
    }

    private static boolean isDb2Struct(Struct st) {
        try {
            return isDb2GeometryTypeName(st.getSQLTypeName());
        } catch (SQLException ignored) {
            return false;
        }
    }

    private static Geometry fromBytes(byte[] raw, WKBReader reader) throws ParseException {
        if (raw == null || raw.length == 0) {
            return null;
        }
        // Plain OGC / EWKB (common for ST_AsBinary and some getObject payloads).
        if (looksLikeWKB(raw, 0)) {
            return reader.read(raw);
        }
        // Some drivers prepend a 4-byte SRID (little-endian), similar to MySQL.
        if (raw.length >= 9 && looksLikeWKB(raw, 4)) {
            int srid = ByteBuffer.wrap(raw, 0, 4).order(ByteOrder.LITTLE_ENDIAN).getInt();
            Geometry g = reader.read(Arrays.copyOfRange(raw, 4, raw.length));
            if (g != null && srid > 0) {
                g.setSRID(srid);
            }
            return g;
        }
        return reader.read(raw);
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
