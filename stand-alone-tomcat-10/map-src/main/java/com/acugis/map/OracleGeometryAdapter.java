package com.acugis.map;

import java.math.BigDecimal;
import java.sql.Array;
import java.sql.SQLException;
import java.sql.Struct;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import org.locationtech.jts.geom.Coordinate;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.geom.GeometryCollection;
import org.locationtech.jts.geom.GeometryFactory;
import org.locationtech.jts.geom.LineString;
import org.locationtech.jts.geom.LinearRing;
import org.locationtech.jts.geom.MultiLineString;
import org.locationtech.jts.geom.MultiPoint;
import org.locationtech.jts.geom.MultiPolygon;
import org.locationtech.jts.geom.Point;
import org.locationtech.jts.geom.Polygon;
import org.locationtech.jts.io.ParseException;
import org.locationtech.jts.io.WKBReader;
import org.locationtech.jts.io.WKTReader;

/**
 * Converts Oracle Spatial {@code MDSYS.SDO_GEOMETRY} values to JTS {@link Geometry}.
 * Preserves SRID and Z/M ordinates when present. Used by {@link GeometryLoader} so
 * the map renderer stays database-independent after conversion.
 */
final class OracleGeometryAdapter {

    private static final GeometryFactory GF = new GeometryFactory();

    private OracleGeometryAdapter() {}

    static boolean isOracleGeometry(Object raw) {
        if (raw == null) {
            return false;
        }
        if (raw instanceof Struct) {
            return isSdoStruct((Struct) raw);
        }
        String cn = raw.getClass().getName();
        return cn.contains("SDO_GEOMETRY") || cn.contains("oracle.sql.STRUCT");
    }

    static Geometry toGeometry(Object raw, WKBReader wkbReader, WKTReader wktReader)
            throws SQLException, ParseException {
        if (raw == null) {
            return null;
        }
        if (raw instanceof Geometry) {
            return (Geometry) raw;
        }
        if (raw instanceof Struct) {
            return fromStruct((Struct) raw);
        }
        if (raw instanceof byte[]) {
            return wkbReader.read((byte[]) raw);
        }
        if (raw instanceof String) {
            String s = ((String) raw).trim();
            if (s.isEmpty()) {
                return null;
            }
            return new WKTReader(GF).read(s);
        }
        // JDBC may return oracle.sql.STRUCT without compile-time type.
        if (raw.getClass().getName().contains("STRUCT")) {
            try {
                Struct st = (Struct) raw;
                return fromStruct(st);
            } catch (ClassCastException ignored) {
                // fall through
            }
        }
        return null;
    }

    private static boolean isSdoStruct(Struct st) {
        try {
            String type = st.getSQLTypeName();
            if (type != null) {
                String u = type.toUpperCase(Locale.US);
                if (u.contains("SDO_GEOMETRY")) {
                    return true;
                }
            }
        } catch (SQLException ignored) {
            // ignore
        }
        return false;
    }

    private static Geometry fromStruct(Struct st) throws SQLException {
        Object[] attrs = st.getAttributes();
        if (attrs == null || attrs.length < 1) {
            return null;
        }
        int gtype = toInt(attrs[0]);
        int srid = attrs.length > 1 ? toInt(attrs[1]) : 0;
        Struct point = attrs.length > 2 && attrs[2] instanceof Struct ? (Struct) attrs[2] : null;
        Number[] elemInfo = attrs.length > 3 ? toNumberArray(attrs[3]) : null;
        Number[] ordinates = attrs.length > 4 ? toNumberArray(attrs[4]) : null;

        Geometry g;
        if (elemInfo == null || elemInfo.length == 0) {
            g = pointFromStruct(point);
        } else {
            g = fromElemInfo(gtype, elemInfo, ordinates);
        }
        if (g == null) {
            g = pointFromStruct(point);
        }
        if (g == null) {
            return null;
        }
        if (srid > 0) {
            g.setSRID(srid);
        }
        return g;
    }

    private static Geometry pointFromStruct(Struct point) throws SQLException {
        if (point == null) {
            return null;
        }
        Object[] p = point.getAttributes();
        if (p == null || p.length < 2) {
            return null;
        }
        double x = toDouble(p[0]);
        double y = toDouble(p[1]);
        if (Double.isNaN(x) || Double.isNaN(y)) {
            return null;
        }
        if (p.length > 2 && p[2] != null) {
            double z = toDouble(p[2]);
            return GF.createPoint(new Coordinate(x, y, z));
        }
        return GF.createPoint(new Coordinate(x, y));
    }

    private static Geometry fromElemInfo(int gtype, Number[] elemInfo, Number[] ordinates) {
        if (elemInfo == null || ordinates == null || elemInfo.length < 3) {
            return null;
        }
        int type = gtype % 10;
        List<Geometry> parts = new ArrayList<Geometry>();
        int ordIdx = 0;
        for (int i = 0; i + 2 < elemInfo.length; i += 3) {
            int interpretation = elemInfo[i + 2].intValue();
            int nPairs = interpretation == 1 ? 1 : interpretation == 2 ? countPairs(elemInfo, ordinates, ordIdx) : 0;
            if (interpretation == 3) {
                // rectangle: 2 points (lower-left, upper-right)
                nPairs = 2;
            }
            if (nPairs <= 0) {
                continue;
            }
            Coordinate[] coords = readOrdinates(ordinates, ordIdx, nPairs, interpretation == 3);
            ordIdx += coordCount(ordinates, ordIdx, nPairs, interpretation == 3);
            if (coords.length == 0) {
                continue;
            }
            Geometry part = geometryForType(type, coords, interpretation);
            if (part != null) {
                parts.add(part);
            }
        }
        if (parts.isEmpty()) {
            return null;
        }
        if (parts.size() == 1) {
            return parts.get(0);
        }
        return GF.createGeometryCollection(parts.toArray(new Geometry[0]));
    }

    private static int countPairs(Number[] elemInfo, Number[] ordinates, int startOrdIdx) {
        // For line strings / polygons the third elem of a triplet is pair count when interp=2.
        return Math.max(0, (ordinates.length - startOrdIdx) / dimension(ordinates, startOrdIdx));
    }

    private static int dimension(Number[] ordinates, int start) {
        if (ordinates.length - start >= 3) {
            return 3;
        }
        return 2;
    }

    private static int coordCount(Number[] ordinates, int start, int nPairs, boolean rectangle) {
        int dim = dimension(ordinates, start);
        if (rectangle) {
            return dim * 2;
        }
        return dim * nPairs;
    }

    private static Coordinate[] readOrdinates(Number[] ordinates, int start, int nPairs, boolean rectangle) {
        int dim = dimension(ordinates, start);
        List<Coordinate> out = new ArrayList<Coordinate>();
        int idx = start;
        if (rectangle && ordinates.length >= start + dim * 2) {
            double x1 = toDouble(ordinates[idx]);
            double y1 = toDouble(ordinates[idx + 1]);
            double x2 = toDouble(ordinates[idx + dim]);
            double y2 = toDouble(ordinates[idx + dim + 1]);
            out.add(new Coordinate(x1, y1));
            out.add(new Coordinate(x2, y1));
            out.add(new Coordinate(x2, y2));
            out.add(new Coordinate(x1, y2));
            out.add(new Coordinate(x1, y1));
            return out.toArray(new Coordinate[0]);
        }
        for (int i = 0; i < nPairs && idx + dim - 1 < ordinates.length; i++) {
            double x = toDouble(ordinates[idx]);
            double y = toDouble(ordinates[idx + 1]);
            if (dim >= 3) {
                double z = toDouble(ordinates[idx + 2]);
                out.add(new Coordinate(x, y, z));
                idx += 3;
            } else {
                out.add(new Coordinate(x, y));
                idx += 2;
            }
        }
        return out.toArray(new Coordinate[0]);
    }

    private static Geometry geometryForType(int type, Coordinate[] coords, int interpretation) {
        if (coords.length == 0) {
            return null;
        }
        switch (type) {
            case 1: // POINT
                return GF.createPoint(coords[0]);
            case 2: // LINESTRING
                return GF.createLineString(coords);
            case 3: // POLYGON
                return polygonFromCoords(coords);
            case 5: // MULTIPOINT
                return GF.createMultiPointFromCoords(coords);
            case 6: // MULTILINESTRING
                return GF.createMultiLineString(new LineString[] {GF.createLineString(coords)});
            case 7: // MULTIPOLYGON
                Polygon poly = polygonFromCoords(coords);
                return poly == null ? null : GF.createMultiPolygon(new Polygon[] {poly});
            case 4: // COLLECTION
            default:
                if (interpretation == 1) {
                    return GF.createPoint(coords[0]);
                }
                if (interpretation == 2) {
                    return GF.createLineString(coords);
                }
                return polygonFromCoords(coords);
        }
    }

    private static Polygon polygonFromCoords(Coordinate[] coords) {
        if (coords.length < 4) {
            return null;
        }
        LinearRing shell = GF.createLinearRing(coords);
        return GF.createPolygon(shell, null);
    }

    private static Number[] toNumberArray(Object raw) throws SQLException {
        if (raw == null) {
            return null;
        }
        if (raw instanceof Number[]) {
            return (Number[]) raw;
        }
        if (raw instanceof BigDecimal[]) {
            BigDecimal[] bd = (BigDecimal[]) raw;
            Number[] out = new Number[bd.length];
            System.arraycopy(bd, 0, out, 0, bd.length);
            return out;
        }
        if (raw instanceof Array) {
            Object arr = ((Array) raw).getArray();
            if (arr instanceof BigDecimal[]) {
                BigDecimal[] bd = (BigDecimal[]) arr;
                Number[] out = new Number[bd.length];
                System.arraycopy(bd, 0, out, 0, bd.length);
                return out;
            }
            if (arr instanceof Object[]) {
                Object[] oa = (Object[]) arr;
                Number[] out = new Number[oa.length];
                for (int i = 0; i < oa.length; i++) {
                    out[i] = (Number) oa[i];
                }
                return out;
            }
        }
        return null;
    }

    private static int toInt(Object v) {
        if (v == null) {
            return 0;
        }
        if (v instanceof Number) {
            return ((Number) v).intValue();
        }
        try {
            return Integer.parseInt(String.valueOf(v));
        } catch (NumberFormatException e) {
            return 0;
        }
    }

    private static double toDouble(Object v) {
        if (v == null) {
            return Double.NaN;
        }
        if (v instanceof Number) {
            return ((Number) v).doubleValue();
        }
        try {
            return Double.parseDouble(String.valueOf(v));
        } catch (NumberFormatException e) {
            return Double.NaN;
        }
    }
}
