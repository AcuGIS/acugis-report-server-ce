package com.acugis.map;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.sql.SQLException;
import java.util.Arrays;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.io.ParseException;
import org.locationtech.jts.io.WKBReader;
import org.locationtech.jts.io.WKTReader;

/**
 * Converts MySQL / MariaDB spatial values to JTS Geometry.
 *
 * <p>MySQL stores geometries as {@code 4-byte little-endian SRID + OGC WKB}. JDBC returns that
 * blob as {@code byte[]}. Parsing the whole blob as plain WKB is wrong: an SRID of {@code 0}
 * begins with {@code 00 00 00 00}, which JTS reads as big-endian WKB type {@code Point}, so
 * polygons collapse to a single bogus point.
 */
final class MySQLGeometryAdapter {

    private MySQLGeometryAdapter() {}

    static boolean isMySQLGeometryBytes(byte[] raw) {
        return raw != null && raw.length >= 9 && looksLikeWKB(raw, 4);
    }

    static Geometry toGeometry(Object raw, WKBReader wkbReader, WKTReader wktReader)
            throws ParseException, SQLException {
        if (raw == null) {
            return null;
        }
        if (raw instanceof Geometry) {
            return (Geometry) raw;
        }
        if (raw instanceof byte[]) {
            return fromBytes((byte[]) raw, wkbReader);
        }
        if (raw instanceof String) {
            String s = ((String) raw).trim();
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
            // Hex WKB / EWKB from some drivers.
            if (looksLikeHex(s)) {
                return fromBytes(WKBReader.hexToBytes(stripHexPrefix(s)), wkbReader);
            }
            return wktReader.read(s);
        }
        return null;
    }

    private static Geometry fromBytes(byte[] raw, WKBReader reader) throws ParseException {
        if (raw == null || raw.length == 0) {
            return null;
        }
        // MySQL/MariaDB internal storage: SRID + WKB (detect WKB header at offset 4).
        if (isMySQLGeometryBytes(raw)) {
            int srid = littleEndianInt(raw, 0);
            byte[] payload = Arrays.copyOfRange(raw, 4, raw.length);
            Geometry g = reader.read(payload);
            if (g != null && srid > 0) {
                g.setSRID(srid);
            }
            return g;
        }
        // Plain OGC WKB (e.g. ST_AsBinary without SRID prefix).
        if (looksLikeWKB(raw, 0)) {
            return reader.read(raw);
        }
        // Last resort: try SRID-stripped then plain.
        if (raw.length >= 5) {
            try {
                Geometry g = reader.read(Arrays.copyOfRange(raw, 4, raw.length));
                if (g != null && !g.isEmpty()) {
                    int srid = littleEndianInt(raw, 0);
                    if (srid > 0) {
                        g.setSRID(srid);
                    }
                    return g;
                }
            } catch (ParseException ignored) {
                // fall through
            }
        }
        return reader.read(raw);
    }

    /**
     * True when {@code bytes[offset]} looks like an OGC/EWKB header: byte order 0/1 and a
     * geometry type code in 1..7 (optionally with EWKB flag bits in the high word).
     */
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
        // Strip common EWKB / ISO flag bits; MySQL uses 1..7 for core types.
        int baseType = type & 0xFF;
        if (baseType == 0) {
            baseType = type & 0x1FFFFFFF;
        }
        return baseType >= 1 && baseType <= 7;
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

    private static int littleEndianInt(byte[] bytes, int offset) {
        return ByteBuffer.wrap(bytes, offset, 4).order(ByteOrder.LITTLE_ENDIAN).getInt();
    }

    private static int parseIntSafe(String raw) {
        try {
            return Integer.parseInt(raw.trim());
        } catch (NumberFormatException e) {
            return 0;
        }
    }
}
