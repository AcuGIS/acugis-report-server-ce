package com.acugis.map;

import java.sql.Connection;
import java.sql.SQLException;
import java.util.Locale;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.geom.GeometryFactory;
import org.locationtech.jts.io.ParseException;
import org.locationtech.jts.io.WKBReader;
import org.locationtech.jts.io.WKTReader;
import net.postgis.jdbc.PGgeometry;
import org.postgresql.util.PGobject;

/**
 * Shared geometry conversion pipeline for Map rendering and report GIS expressions.
 *
 * <p>{@code GeometryLoader} (maps) and {@link AcuGISFunctions} (expressions) both
 * decode JDBC payloads through {@link #parse} / {@link #toGeometry}. Supports
 * PostGIS ({@link PGobject}/{@link PGgeometry}), WKB/WKT, GeoPackage, MySQL,
 * Oracle, Db2, SQL Server, and Snowflake adapters.
 *
 * <p>Jasper fields must be {@code java.lang.Object} — drivers do not return JTS
 * {@link Geometry} instances directly.
 */
public final class GeometryCodec {

    private static final GeometryFactory GF = new GeometryFactory();

    private GeometryCodec() {}

    /** Decode a geometry value without a JDBC connection (PostGIS / WKB / WKT / etc.). */
    public static Geometry toGeometry(Object raw) {
        try {
            return toGeometry(raw, null);
        } catch (SQLException e) {
            throw new IllegalArgumentException("Unable to decode geometry: " + e.getMessage(), e);
        }
    }

    /** Decode a geometry value; connection enables Db2 / SQL Server adapter paths when needed. */
    public static Geometry toGeometry(Object raw, Connection connection) throws SQLException {
        if (raw == null) {
            return null;
        }
        if (raw instanceof Geometry) {
            return (Geometry) raw;
        }
        WKBReader wkbReader = new WKBReader(GF);
        WKTReader wktReader = new WKTReader(GF);
        try {
            return parse(raw, connection, false, false, false, wkbReader, wktReader);
        } catch (ParseException e) {
            throw new IllegalArgumentException("Unable to parse geometry: " + e.getMessage(), e);
        }
    }

    static Geometry parse(
            Object raw,
            Connection connection,
            boolean db2Column,
            boolean sqlServerColumn,
            boolean snowflakeColumn,
            WKBReader wkbReader,
            WKTReader wktReader)
            throws ParseException, SQLException {
        if (raw instanceof Geometry) {
            return (Geometry) raw;
        }
        if (raw instanceof java.sql.Blob) {
            java.sql.Blob blob = (java.sql.Blob) raw;
            long len = blob.length();
            if (len <= 0 || len > Integer.MAX_VALUE) {
                return null;
            }
            raw = blob.getBytes(1L, (int) len);
        }
        if (db2Column || Db2GeometryAdapter.isDb2Geometry(raw)) {
            Geometry g = Db2GeometryAdapter.toGeometry(raw, connection, wkbReader, wktReader);
            if (g != null) {
                return g;
            }
        }
        if (sqlServerColumn || SqlServerGeometryAdapter.isSqlServerGeometry(raw)) {
            Geometry g = SqlServerGeometryAdapter.toGeometry(raw, connection, wkbReader, wktReader);
            if (g != null) {
                return g;
            }
        }
        if (snowflakeColumn) {
            Geometry g = SnowflakeGeometryAdapter.toGeometry(raw, wkbReader, wktReader);
            if (g != null) {
                return g;
            }
        }
        if (OracleGeometryAdapter.isOracleGeometry(raw)) {
            Geometry g = OracleGeometryAdapter.toGeometry(raw, wkbReader, wktReader);
            if (g != null) {
                return g;
            }
        }
        if (raw instanceof byte[]) {
            byte[] bytes = (byte[]) raw;
            if (GeoPackageGeometryAdapter.isGeoPackageBinary(bytes)) {
                Geometry g = GeoPackageGeometryAdapter.toGeometry(bytes, wkbReader);
                if (g != null) {
                    return g;
                }
            }
            if (Db2GeometryAdapter.looksLikeWKB(bytes, 0)
                    || SqlServerGeometryAdapter.looksLikeWKB(bytes, 0)) {
                return wkbReader.read(bytes);
            }
            if (MySQLGeometryAdapter.isMySQLGeometryBytes(bytes)) {
                Geometry g = MySQLGeometryAdapter.toGeometry(bytes, wkbReader, wktReader);
                if (g != null) {
                    return g;
                }
            }
            return wkbReader.read(bytes);
        }
        if (raw instanceof PGgeometry) {
            return fromPostgis(((PGgeometry) raw).getGeometry(), wktReader);
        }
        if (raw instanceof net.postgis.jdbc.geometry.Geometry) {
            return fromPostgis((net.postgis.jdbc.geometry.Geometry) raw, wktReader);
        }
        if (raw instanceof PGobject) {
            PGobject pg = (PGobject) raw;
            String value = pg.getValue();
            if (value == null || value.isEmpty()) {
                return null;
            }
            String type = safe(pg.getType()).toLowerCase(Locale.US);
            if ("geometry".equals(type) || "geography".equals(type) || looksLikeWkbHex(value)) {
                if (looksLikeWkbHex(value)) {
                    return wkbReader.read(WKBReader.hexToBytes(stripHexPrefix(value)));
                }
                return readWkt(value, wktReader);
            }
            return readWkt(value, wktReader);
        }
        if (raw instanceof String) {
            String s = ((String) raw).trim();
            if (s.isEmpty()) {
                return null;
            }
            if (looksLikeWkbHex(s)) {
                return wkbReader.read(WKBReader.hexToBytes(stripHexPrefix(s)));
            }
            return readWkt(s, wktReader);
        }
        return null;
    }

    private static Geometry fromPostgis(net.postgis.jdbc.geometry.Geometry pgGeom, WKTReader wktReader)
            throws ParseException {
        if (pgGeom == null) {
            return null;
        }
        Geometry g = readWkt(pgGeom.toString(), wktReader);
        if (g != null && g.getSRID() <= 0 && pgGeom.getSrid() > 0) {
            g.setSRID(pgGeom.getSrid());
        }
        return g;
    }

    private static Geometry readWkt(String text, WKTReader wktReader) throws ParseException {
        String wkt = text.trim();
        int srid = 0;
        int semi = wkt.indexOf(';');
        if (wkt.toUpperCase(Locale.US).startsWith("SRID=") && semi > 0) {
            String sridPart = wkt.substring(5, semi).trim();
            try {
                srid = Integer.parseInt(sridPart);
            } catch (NumberFormatException ignored) {
                srid = 0;
            }
            wkt = wkt.substring(semi + 1).trim();
        }
        Geometry g = wktReader.read(wkt);
        if (g != null && srid > 0) {
            g.setSRID(srid);
        }
        return g;
    }

    private static boolean looksLikeWkbHex(String s) {
        if (s.length() < 10 || (s.length() % 2) != 0) {
            return false;
        }
        String hex = stripHexPrefix(s);
        if (hex.length() < 10) {
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

    private static String safe(String s) {
        return s == null ? "" : s;
    }
}
