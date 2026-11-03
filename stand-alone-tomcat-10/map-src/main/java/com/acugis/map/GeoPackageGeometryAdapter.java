package com.acugis.map;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.Arrays;
import java.util.Locale;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.geom.GeometryCollection;
import org.locationtech.jts.geom.LineString;
import org.locationtech.jts.geom.MultiLineString;
import org.locationtech.jts.geom.MultiPoint;
import org.locationtech.jts.geom.MultiPolygon;
import org.locationtech.jts.geom.Point;
import org.locationtech.jts.geom.Polygon;
import org.locationtech.jts.io.ParseException;
import org.locationtech.jts.io.WKBReader;

/**
 * Decodes GeoPackage Binary (GPB) geometry blobs to JTS Geometry.
 *
 * <p>GeoPackage Binary layout (OGC 12-128r15 §I.1.1.1):
 *
 * <pre>
 *   magic: 'G' 'P' (2 bytes)
 *   version: 0 (1 byte)
 *   flags: see bit layout below (1 byte)
 *   srs_id: int32 (4 bytes, endianness from flags bit B)
 *   envelope: 0 / 32 / 48 / 64 bytes
 *   WKB: standard OGC WKB payload
 * </pre>
 *
 * <p>Flags bit layout (bit 7 … bit 0): {@code R R X Y E E E B}
 *
 * <ul>
 *   <li>B (bit 0): header byte order (0=BE, 1=LE)
 *   <li>E (bits 1–3): envelope indicator (0–4)
 *   <li>Y (bit 4): empty geometry
 *   <li>X (bit 5): 0=Standard, 1=Extended
 *   <li>R (bits 6–7): reserved
 * </ul>
 *
 * The decoded WKB is converted via {@link WKBReader} into the shared JTS model used by
 * {@link MapPainter} — no GeoPackage-specific rendering.
 */
final class GeoPackageGeometryAdapter {

    private static final byte MAGIC_G = 0x47;
    private static final byte MAGIC_P = 0x50;

    private GeoPackageGeometryAdapter() {}

    static boolean isGeoPackageBinary(byte[] raw) {
        return raw != null
                && raw.length >= 8
                && raw[0] == MAGIC_G
                && raw[1] == MAGIC_P;
    }

    static Geometry toGeometry(byte[] raw, WKBReader wkbReader) throws ParseException {
        if (raw == null || raw.length == 0) {
            return null;
        }
        if (!isGeoPackageBinary(raw)) {
            // Plain OGC WKB fallback (some tools strip the GPB header).
            if (MySQLGeometryAdapter.looksLikeWKB(raw, 0)) {
                return wkbReader.read(raw);
            }
            return null;
        }

        int version = raw[2] & 0xFF;
        if (version != 0) {
            throw new ParseException("Unsupported GeoPackage Binary version: " + version);
        }

        int flags = raw[3] & 0xFF;
        // OGC bit layout: R R X Y E E E B  (bit7 … bit0)
        boolean littleEndian = (flags & 0x01) == 1;
        int envelopeIndicator = (flags >> 1) & 0x07;
        boolean empty = ((flags >> 4) & 0x01) == 1;

        if (empty) {
            return null;
        }

        ByteOrder order = littleEndian ? ByteOrder.LITTLE_ENDIAN : ByteOrder.BIG_ENDIAN;
        int srid = ByteBuffer.wrap(raw, 4, 4).order(order).getInt();

        int envelopeBytes = envelopeByteCount(envelopeIndicator);
        int wkbOffset = 8 + envelopeBytes;
        if (wkbOffset >= raw.length) {
            // Empty / malformed header-only blob — try plain WKB after the 8-byte header,
            // then fail with a clear message.
            if (raw.length > 8 && MySQLGeometryAdapter.looksLikeWKB(raw, 8)) {
                Geometry g = wkbReader.read(Arrays.copyOfRange(raw, 8, raw.length));
                if (g != null && srid != 0) {
                    g.setSRID(srid);
                }
                assertSupported(g);
                return g;
            }
            throw new ParseException("GeoPackage Binary truncated: missing WKB payload");
        }

        byte[] wkb = Arrays.copyOfRange(raw, wkbOffset, raw.length);
        if (!MySQLGeometryAdapter.looksLikeWKB(wkb, 0) && MySQLGeometryAdapter.looksLikeWKB(raw, 8)) {
            // Envelope indicator may be wrong in non-compliant files; fall back to no-envelope.
            wkb = Arrays.copyOfRange(raw, 8, raw.length);
        }
        Geometry g = wkbReader.read(wkb);
        if (g == null) {
            return null;
        }
        if (srid != 0) {
            g.setSRID(srid);
        }
        assertSupported(g);
        return g;
    }

    /** Envelope byte sizes from OGC envelope contents indicator codes 0–4. */
    static int envelopeByteCount(int envelopeIndicator) throws ParseException {
        switch (envelopeIndicator) {
            case 0:
                return 0;
            case 1:
                return 32; // minx, maxx, miny, maxy
            case 2:
            case 3:
                return 48; // + z or m
            case 4:
                return 64; // xyzm
            default:
                throw new ParseException(
                        "Unsupported GeoPackage geometry envelope indicator: " + envelopeIndicator);
        }
    }

    private static void assertSupported(Geometry g) throws ParseException {
        if (g == null || g.isEmpty()) {
            return;
        }
        if (isSupported(g)) {
            return;
        }
        if (g instanceof GeometryCollection) {
            GeometryCollection gc = (GeometryCollection) g;
            for (int i = 0; i < gc.getNumGeometries(); i++) {
                assertSupported(gc.getGeometryN(i));
            }
            return;
        }
        String type = g.getGeometryType() == null ? "unknown" : g.getGeometryType();
        throw new ParseException(
                "Unsupported GeoPackage geometry type: " + type.toUpperCase(Locale.US));
    }

    private static boolean isSupported(Geometry g) {
        return g instanceof Point
                || g instanceof MultiPoint
                || g instanceof LineString
                || g instanceof MultiLineString
                || g instanceof Polygon
                || g instanceof MultiPolygon
                || g instanceof GeometryCollection;
    }
}
