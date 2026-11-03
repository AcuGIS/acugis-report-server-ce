package com.acugis.map;

import java.awt.BasicStroke;
import java.awt.Color;
import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.geom.Path2D;
import java.awt.image.BufferedImage;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.zip.GZIPInputStream;
import com.wdtinc.mapbox_vector_tile.adapt.jts.MvtReader;
import com.wdtinc.mapbox_vector_tile.adapt.jts.TagKeyValueMapConverter;
import com.wdtinc.mapbox_vector_tile.adapt.jts.model.JtsLayer;
import com.wdtinc.mapbox_vector_tile.adapt.jts.model.JtsMvt;
import org.json.JSONArray;
import org.json.JSONObject;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.geom.GeometryFactory;
import org.locationtech.jts.geom.LineString;
import org.locationtech.jts.geom.MultiLineString;
import org.locationtech.jts.geom.MultiPolygon;
import org.locationtech.jts.geom.Polygon;

/**
 * Rasterizes OpenFreeMap's public MapLibre styles into XYZ images.
 *
 * <p>Verified public endpoints (no API key):
 *
 * <ul>
 *   <li>{@code https://tiles.openfreemap.org/styles/positron} — light, Positron
 *   <li>{@code https://tiles.openfreemap.org/styles/dark} — dark
 *   <li>vector tiles from the style's {@code openmaptiles} TileJSON
 *       ({@code https://tiles.openfreemap.org/planet}), falling back to
 *       {@code https://tiles.openfreemap.org/planet/latest/{z}/{x}/{y}.pbf}
 * </ul>
 *
 * Labels are omitted. Land, water, and roads are painted from the style's
 * fill and line layers so the print basemap matches the published colors.
 */
final class OpenFreeMapBasemap {

    static final String LIGHT_STYLE = "https://tiles.openfreemap.org/styles/positron";
    static final String DARK_STYLE = "https://tiles.openfreemap.org/styles/dark";
    static final String TILE_FALLBACK = "https://tiles.openfreemap.org/planet/latest/{z}/{x}/{y}.pbf";

    /** Planet TileJSON maxzoom observed on the public instance. */
    static final int DEFAULT_MAX_ZOOM = 14;

    private static final int TILE_PX = 512;
    private static final int CONNECT_TIMEOUT_MS = 8000;
    private static final int READ_TIMEOUT_MS = 12000;
    private static final int MAX_BYTES = 4_000_000;
    private static final String HOST = "tiles.openfreemap.org";

    private static final Map<String, ParsedStyle> STYLES = new HashMap<String, ParsedStyle>();
    private static final Map<String, BufferedImage> TILES =
            Collections.synchronizedMap(new LinkedHashMap<String, BufferedImage>(64, 0.75f, true) {
                @Override
                protected boolean removeEldestEntry(Map.Entry<String, BufferedImage> eldest) {
                    return size() > 48;
                }
            });

    private OpenFreeMapBasemap() {}

    static void prepare(String styleUrl) {
        style(styleUrl);
    }

    static int maxZoom(String styleUrl) {
        ParsedStyle parsed = cached(styleUrl);
        if (parsed == null) {
            return DEFAULT_MAX_ZOOM;
        }
        return parsed.maxZoom;
    }

    static String tileTemplate(String styleUrl) {
        ParsedStyle parsed = cached(styleUrl);
        if (parsed == null) {
            return null;
        }
        return parsed.tileTemplate;
    }

    static String tileURL(String styleUrl, int z, int x, int y) {
        ParsedStyle parsed = style(styleUrl);
        String template = parsed == null ? TILE_FALLBACK : parsed.tileTemplate;
        if (template == null || template.isEmpty()) {
            template = TILE_FALLBACK;
        }
        return template
                .replace("{z}", Integer.toString(z))
                .replace("{x}", Integer.toString(x))
                .replace("{y}", Integer.toString(y));
    }

    static BufferedImage renderTile(String styleUrl, int z, int x, int y) {
        String cacheKey = styleUrl + "|" + z + "/" + x + "/" + y;
        BufferedImage cachedTile = TILES.get(cacheKey);
        if (cachedTile != null) {
            return cachedTile;
        }
        ParsedStyle parsed = style(styleUrl);
        if (parsed == null) {
            return null;
        }
        String url = tileURL(styleUrl, z, x, y);
        byte[] body = httpGet(url, "application/vnd.mapbox-vector-tile,application/x-protobuf,*/*");
        if (body == null || body.length == 0) {
            return null;
        }
        body = gunzipIfNeeded(body);
        try {
            BufferedImage image = paintTile(parsed, body, z);
            if (image != null) {
                TILES.put(cacheKey, image);
            }
            return image;
        } catch (Exception e) {
            return null;
        }
    }

    static Color parseCssColor(String raw) {
        if (raw == null) {
            return null;
        }
        String s = raw.trim().toLowerCase(Locale.US);
        if (s.isEmpty() || "transparent".equals(s) || "none".equals(s)) {
            return null;
        }
        try {
            if (s.charAt(0) == '#') {
                return parseHex(s.substring(1));
            }
            if (s.startsWith("rgb")) {
                return parseRgb(s);
            }
            if (s.startsWith("hsl")) {
                return parseHsl(s);
            }
        } catch (Exception e) {
            return null;
        }
        return null;
    }

    static boolean featureMatches(Object filter, Map<String, Object> attrs, String geomType) {
        if (isNull(filter)) {
            return true;
        }
        if (!(filter instanceof JSONArray)) {
            return false;
        }
        JSONArray f = (JSONArray) filter;
        if (f.length() == 0) {
            return true;
        }
        String op = f.optString(0, "");
        if ("all".equals(op)) {
            for (int i = 1; i < f.length(); i++) {
                if (!featureMatches(f.opt(i), attrs, geomType)) {
                    return false;
                }
            }
            return true;
        }
        if ("any".equals(op)) {
            for (int i = 1; i < f.length(); i++) {
                if (featureMatches(f.opt(i), attrs, geomType)) {
                    return true;
                }
            }
            return false;
        }
        if ("!".equals(op)) {
            return !featureMatches(f.opt(1), attrs, geomType);
        }
        if ("==".equals(op)) {
            return objectsEqual(resolve(f.opt(1), attrs, geomType), resolve(f.opt(2), attrs, geomType));
        }
        if ("!=".equals(op)) {
            return !objectsEqual(resolve(f.opt(1), attrs, geomType), resolve(f.opt(2), attrs, geomType));
        }
        if ("has".equals(op)) {
            String key = propertyName(f.opt(1));
            return key != null && attrs != null && attrs.containsKey(key) && !isNull(attrs.get(key));
        }
        if ("match".equals(op)) {
            return matchExpr(f, attrs, geomType);
        }
        if ("in".equals(op)) {
            Object needle = resolve(f.opt(1), attrs, geomType);
            for (int i = 2; i < f.length(); i++) {
                if (objectsEqual(needle, resolve(f.opt(i), attrs, geomType))) {
                    return true;
                }
            }
            return false;
        }
        return false;
    }

    private static ParsedStyle cached(String styleUrl) {
        synchronized (STYLES) {
            return STYLES.get(styleUrl);
        }
    }

    private static ParsedStyle style(String styleUrl) {
        if (!allowed(styleUrl)) {
            return null;
        }
        synchronized (STYLES) {
            ParsedStyle hit = STYLES.get(styleUrl);
            if (hit != null) {
                return hit;
            }
            try {
                ParsedStyle loaded = loadStyle(styleUrl);
                if (loaded != null) {
                    STYLES.put(styleUrl, loaded);
                }
                return loaded;
            } catch (Exception e) {
                return null;
            }
        }
    }

    private static ParsedStyle loadStyle(String styleUrl) throws Exception {
        byte[] raw = httpGet(styleUrl, "application/json,*/*");
        if (raw == null) {
            throw new IllegalStateException("empty style");
        }
        JSONObject root = new JSONObject(new String(gunzipIfNeeded(raw), StandardCharsets.UTF_8));
        JSONObject sources = root.optJSONObject("sources");
        String vectorId = "openmaptiles";
        JSONObject vector = sources == null ? null : sources.optJSONObject(vectorId);
        if (vector == null && sources != null) {
            for (String name : sources.keySet()) {
                JSONObject src = sources.optJSONObject(name);
                if (src != null && "vector".equals(src.optString("type"))) {
                    vector = src;
                    vectorId = name;
                    break;
                }
            }
        }
        String template = TILE_FALLBACK;
        int maxZoom = DEFAULT_MAX_ZOOM;
        if (vector != null) {
            if (vector.has("tiles")) {
                JSONArray tiles = vector.optJSONArray("tiles");
                if (tiles != null && tiles.length() > 0 && allowed(tiles.optString(0))) {
                    template = tiles.optString(0);
                }
                maxZoom = vector.optInt("maxzoom", DEFAULT_MAX_ZOOM);
            } else {
                String tilejsonURL = vector.optString("url", "");
                if (allowed(tilejsonURL)) {
                    byte[] tjRaw = httpGet(tilejsonURL, "application/json,*/*");
                    if (tjRaw != null) {
                        JSONObject tj = new JSONObject(new String(gunzipIfNeeded(tjRaw), StandardCharsets.UTF_8));
                        JSONArray tiles = tj.optJSONArray("tiles");
                        if (tiles != null && tiles.length() > 0 && allowed(tiles.optString(0))) {
                            template = tiles.optString(0);
                        }
                        maxZoom = tj.optInt("maxzoom", DEFAULT_MAX_ZOOM);
                    }
                }
            }
        }
        if (!allowed(template)) {
            template = TILE_FALLBACK;
        }
        if (maxZoom < 0) {
            maxZoom = 0;
        }
        if (maxZoom > 22) {
            maxZoom = 22;
        }

        ParsedStyle parsed = new ParsedStyle();
        parsed.tileTemplate = template;
        parsed.maxZoom = maxZoom;
        parsed.vectorSource = vectorId;
        parsed.background = new Color(242, 243, 240);
        JSONArray layers = root.optJSONArray("layers");
        if (layers != null) {
            for (int i = 0; i < layers.length(); i++) {
                JSONObject layer = layers.optJSONObject(i);
                if (layer == null) {
                    continue;
                }
                String type = layer.optString("type", "");
                JSONObject paint = layer.optJSONObject("paint");
                if ("background".equals(type)) {
                    Color bg = evalColor(paint == null ? null : paint.opt("background-color"), 0);
                    if (bg != null) {
                        parsed.background = bg;
                    }
                    continue;
                }
                if (!"fill".equals(type) && !"line".equals(type)) {
                    continue;
                }
                if (!vectorId.equals(layer.optString("source"))) {
                    continue;
                }
                String sourceLayer = layer.optString("source-layer", "");
                if (sourceLayer.isEmpty() || paint == null) {
                    continue;
                }
                String colorKey = "fill".equals(type) ? "fill-color" : "line-color";
                if (!paint.has(colorKey) || isNull(paint.opt(colorKey))) {
                    continue;
                }
                LayerPaint lp = new LayerPaint();
                lp.type = type;
                lp.sourceLayer = sourceLayer;
                lp.colorExpr = paint.opt(colorKey);
                lp.opacityExpr = paint.opt("fill".equals(type) ? "fill-opacity" : "line-opacity");
                lp.widthExpr = paint.opt("line-width");
                lp.outlineExpr = paint.opt("fill-outline-color");
                lp.filter = layer.has("filter") ? layer.opt("filter") : null;
                lp.minZoom = layer.has("minzoom") ? layer.optDouble("minzoom", 0) : 0;
                lp.maxZoom = layer.has("maxzoom") ? layer.optDouble("maxzoom", 24) : 24;
                parsed.layers.add(lp);
            }
        }
        return parsed;
    }

    private static BufferedImage paintTile(ParsedStyle parsed, byte[] pbf, int zoom) throws Exception {
        JtsMvt mvt = MvtReader.loadMvt(
                new ByteArrayInputStream(pbf),
                new GeometryFactory(),
                new TagKeyValueMapConverter(),
                MvtReader.RING_CLASSIFIER_V1);
        Map<String, List<Geometry>> byLayer = new HashMap<String, List<Geometry>>();
        Map<String, Integer> extents = new HashMap<String, Integer>();
        for (Object layerObj : mvt.getLayers()) {
            JtsLayer tileLayer = (JtsLayer) layerObj;
            List<Geometry> bucket = new ArrayList<Geometry>();
            for (Object geomObj : tileLayer.getGeometries()) {
                if (geomObj instanceof Geometry) {
                    bucket.add((Geometry) geomObj);
                }
            }
            byLayer.put(tileLayer.getName(), bucket);
            extents.put(tileLayer.getName(), Integer.valueOf(tileLayer.getExtent()));
        }

        BufferedImage image = new BufferedImage(TILE_PX, TILE_PX, BufferedImage.TYPE_INT_RGB);
        Graphics2D g = image.createGraphics();
        try {
            g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
            g.setRenderingHint(RenderingHints.KEY_STROKE_CONTROL, RenderingHints.VALUE_STROKE_PURE);
            g.setColor(parsed.background);
            g.fillRect(0, 0, TILE_PX, TILE_PX);
            double z = zoom;
            for (LayerPaint layer : parsed.layers) {
                if (z < layer.minZoom || z >= layer.maxZoom) {
                    continue;
                }
                List<Geometry> bucket = byLayer.get(layer.sourceLayer);
                Integer extent = extents.get(layer.sourceLayer);
                if (bucket == null || bucket.isEmpty() || extent == null || extent.intValue() <= 0) {
                    continue;
                }
                Color color = withOpacity(evalColor(layer.colorExpr, z), evalOpacity(layer.opacityExpr, z));
                if (color == null || color.getAlpha() == 0) {
                    continue;
                }
                Color outline = withOpacity(evalColor(layer.outlineExpr, z), evalOpacity(layer.opacityExpr, z));
                Float width = null;
                if ("line".equals(layer.type)) {
                    Double w = evalNumber(layer.widthExpr, z);
                    float px = w == null ? 1f : w.floatValue();
                    if (px <= 0.2f) {
                        continue;
                    }
                    width = Float.valueOf(px);
                    g.setStroke(new BasicStroke(px, BasicStroke.CAP_ROUND, BasicStroke.JOIN_ROUND));
                }
                g.setColor(color);
                double scale = TILE_PX / (double) extent.intValue();
                for (Geometry geom : bucket) {
                    if (geom == null || geom.isEmpty()) {
                        continue;
                    }
                    if (!featureMatches(layer.filter, attrsOf(geom), geom.getGeometryType())) {
                        continue;
                    }
                    if ("line".equals(layer.type)) {
                        Path2D path = new Path2D.Double();
                        if (appendLines(path, geom, scale)) {
                            g.draw(path);
                        }
                    } else {
                        Path2D path = new Path2D.Double(Path2D.WIND_EVEN_ODD);
                        if (appendPolygons(path, geom, scale)) {
                            g.fill(path);
                            if (outline != null) {
                                g.setColor(outline);
                                g.setStroke(new BasicStroke(1f, BasicStroke.CAP_ROUND, BasicStroke.JOIN_ROUND));
                                g.draw(path);
                                g.setColor(color);
                            }
                        }
                    }
                }
                if (width != null) {
                    g.setStroke(new BasicStroke(1f));
                }
            }
            return image;
        } finally {
            g.dispose();
        }
    }

    @SuppressWarnings("unchecked")
    private static Map<String, Object> attrsOf(Geometry geom) {
        Object user = geom.getUserData();
        if (user instanceof Map) {
            return (Map<String, Object>) user;
        }
        return Collections.emptyMap();
    }

    private static boolean appendPolygons(Path2D path, Geometry geom, double scale) {
        if (geom instanceof Polygon) {
            Polygon polygon = (Polygon) geom;
            boolean any = appendRing(path, polygon.getExteriorRing(), scale, true);
            for (int i = 0; i < polygon.getNumInteriorRing(); i++) {
                any |= appendRing(path, polygon.getInteriorRingN(i), scale, true);
            }
            return any;
        }
        if (geom instanceof MultiPolygon) {
            boolean any = false;
            for (int i = 0; i < geom.getNumGeometries(); i++) {
                any |= appendPolygons(path, geom.getGeometryN(i), scale);
            }
            return any;
        }
        return false;
    }

    private static boolean appendLines(Path2D path, Geometry geom, double scale) {
        if (geom instanceof LineString) {
            return appendRing(path, (LineString) geom, scale, false);
        }
        if (geom instanceof MultiLineString) {
            boolean any = false;
            for (int i = 0; i < geom.getNumGeometries(); i++) {
                any |= appendLines(path, geom.getGeometryN(i), scale);
            }
            return any;
        }
        return false;
    }

    private static boolean appendRing(Path2D path, LineString line, double scale, boolean close) {
        if (line == null) {
            return false;
        }
        org.locationtech.jts.geom.Coordinate[] cs = line.getCoordinates();
        if (cs == null || cs.length == 0) {
            return false;
        }
        path.moveTo(cs[0].x * scale, cs[0].y * scale);
        for (int i = 1; i < cs.length; i++) {
            path.lineTo(cs[i].x * scale, cs[i].y * scale);
        }
        if (close) {
            path.closePath();
        }
        return cs.length > 1;
    }

    private static boolean matchExpr(JSONArray f, Map<String, Object> attrs, String geomType) {
        Object input = resolve(f.opt(1), attrs, geomType);
        int last = f.length() - 1;
        int i = 2;
        while (i < last) {
            if (labelMatches(f.opt(i), input)) {
                return truthy(f.opt(i + 1), attrs, geomType);
            }
            i += 2;
        }
        if (i == last) {
            return truthy(f.opt(last), attrs, geomType);
        }
        return false;
    }

    private static boolean labelMatches(Object label, Object input) {
        if (label instanceof JSONArray) {
            JSONArray labels = (JSONArray) label;
            for (int i = 0; i < labels.length(); i++) {
                if (objectsEqual(input, labels.opt(i))) {
                    return true;
                }
            }
            return false;
        }
        return objectsEqual(input, label);
    }

    private static boolean truthy(Object v, Map<String, Object> attrs, String geomType) {
        if (v instanceof Boolean) {
            return ((Boolean) v).booleanValue();
        }
        if (v instanceof Number) {
            return ((Number) v).doubleValue() != 0d;
        }
        if (v instanceof String) {
            return !"false".equalsIgnoreCase((String) v) && !((String) v).isEmpty();
        }
        if (v instanceof JSONArray) {
            return featureMatches(v, attrs, geomType);
        }
        return !isNull(v);
    }

    private static Object resolve(Object v, Map<String, Object> attrs, String geomType) {
        if (isNull(v) || v instanceof String || v instanceof Number || v instanceof Boolean) {
            return isNull(v) ? null : v;
        }
        if (v instanceof JSONArray) {
            JSONArray a = (JSONArray) v;
            String op = a.optString(0, "");
            if ("get".equals(op)) {
                if (attrs == null) {
                    return null;
                }
                return attrs.get(a.optString(1, ""));
            }
            if ("geometry-type".equals(op)) {
                return geomType;
            }
            if ("literal".equals(op)) {
                return a.opt(1);
            }
        }
        return null;
    }

    private static String propertyName(Object v) {
        if (v instanceof String) {
            return (String) v;
        }
        if (v instanceof JSONArray) {
            JSONArray a = (JSONArray) v;
            if ("get".equals(a.optString(0)) && a.length() > 1) {
                return a.optString(1);
            }
        }
        return null;
    }

    private static boolean objectsEqual(Object a, Object b) {
        if (a == b) {
            return true;
        }
        if (isNull(a) || isNull(b)) {
            return false;
        }
        if (a instanceof Number && b instanceof Number) {
            return Math.abs(((Number) a).doubleValue() - ((Number) b).doubleValue()) < 1e-6;
        }
        if (a instanceof Number && b instanceof String) {
            return numberEquals((Number) a, (String) b);
        }
        if (b instanceof Number && a instanceof String) {
            return numberEquals((Number) b, (String) a);
        }
        return String.valueOf(a).equals(String.valueOf(b));
    }

    private static boolean numberEquals(Number n, String s) {
        try {
            return Math.abs(n.doubleValue() - Double.parseDouble(s.trim())) < 1e-6;
        } catch (NumberFormatException e) {
            return false;
        }
    }

    private static Color evalColor(Object expr, double zoom) {
        if (isNull(expr)) {
            return null;
        }
        if (expr instanceof String) {
            return parseCssColor((String) expr);
        }
        if (!(expr instanceof JSONArray)) {
            return null;
        }
        JSONArray a = (JSONArray) expr;
        if (!"interpolate".equals(a.optString(0)) || a.length() < 5) {
            return null;
        }
        Color prev = null;
        Double prevZ = null;
        for (int i = 3; i + 1 < a.length(); i += 2) {
            if (!(a.opt(i) instanceof Number)) {
                return null;
            }
            double z = ((Number) a.opt(i)).doubleValue();
            Color c = evalColor(a.opt(i + 1), zoom);
            if (c == null) {
                return null;
            }
            if (zoom <= z) {
                if (prev == null || prevZ == null) {
                    return c;
                }
                float t = (float) ((zoom - prevZ.doubleValue()) / (z - prevZ.doubleValue()));
                return lerp(prev, c, t);
            }
            prev = c;
            prevZ = Double.valueOf(z);
        }
        return prev;
    }

    private static Double evalNumber(Object expr, double zoom) {
        if (isNull(expr)) {
            return null;
        }
        if (expr instanceof Number) {
            return Double.valueOf(((Number) expr).doubleValue());
        }
        if (!(expr instanceof JSONArray)) {
            return null;
        }
        JSONArray a = (JSONArray) expr;
        if (!"interpolate".equals(a.optString(0)) || a.length() < 5) {
            return null;
        }
        Double prevV = null;
        Double prevZ = null;
        for (int i = 3; i + 1 < a.length(); i += 2) {
            if (!(a.opt(i) instanceof Number) || !(a.opt(i + 1) instanceof Number)) {
                return null;
            }
            double z = ((Number) a.opt(i)).doubleValue();
            double v = ((Number) a.opt(i + 1)).doubleValue();
            if (zoom <= z) {
                if (prevZ == null || prevV == null) {
                    return Double.valueOf(v);
                }
                double t = (zoom - prevZ.doubleValue()) / (z - prevZ.doubleValue());
                return Double.valueOf(prevV.doubleValue() + t * (v - prevV.doubleValue()));
            }
            prevZ = Double.valueOf(z);
            prevV = Double.valueOf(v);
        }
        return prevV;
    }

    private static float evalOpacity(Object expr, double zoom) {
        if (isNull(expr)) {
            return 1f;
        }
        Double n = evalNumber(expr, zoom);
        if (n == null) {
            return 1f;
        }
        double v = n.doubleValue();
        if (v < 0) {
            return 0f;
        }
        if (v > 1) {
            return 1f;
        }
        return (float) v;
    }

    private static Color withOpacity(Color c, float opacity) {
        if (c == null) {
            return null;
        }
        int a = Math.round(c.getAlpha() * opacity);
        if (a < 0) {
            a = 0;
        }
        if (a > 255) {
            a = 255;
        }
        return new Color(c.getRed(), c.getGreen(), c.getBlue(), a);
    }

    private static Color lerp(Color a, Color b, float t) {
        if (t < 0) {
            t = 0;
        }
        if (t > 1) {
            t = 1;
        }
        return new Color(
                mix(a.getRed(), b.getRed(), t),
                mix(a.getGreen(), b.getGreen(), t),
                mix(a.getBlue(), b.getBlue(), t),
                mix(a.getAlpha(), b.getAlpha(), t));
    }

    private static int mix(int a, int b, float t) {
        return Math.max(0, Math.min(255, Math.round(a + (b - a) * t)));
    }

    private static Color parseHex(String h) {
        if (h.length() == 3 || h.length() == 4) {
            StringBuilder b = new StringBuilder();
            for (int i = 0; i < h.length(); i++) {
                b.append(h.charAt(i)).append(h.charAt(i));
            }
            h = b.toString();
        }
        if (h.length() == 6) {
            int rgb = Integer.parseInt(h, 16);
            return new Color((rgb >> 16) & 255, (rgb >> 8) & 255, rgb & 255);
        }
        if (h.length() == 8) {
            int rgb = Integer.parseInt(h.substring(0, 6), 16);
            int a = Integer.parseInt(h.substring(6, 8), 16);
            return new Color((rgb >> 16) & 255, (rgb >> 8) & 255, rgb & 255, a);
        }
        return null;
    }

    private static Color parseRgb(String s) {
        int open = s.indexOf('(');
        int close = s.indexOf(')');
        if (open < 0 || close < open) {
            return null;
        }
        String[] parts = s.substring(open + 1, close).split(",");
        if (parts.length < 3) {
            return null;
        }
        int r = channel(parts[0]);
        int g = channel(parts[1]);
        int b = channel(parts[2]);
        int a = 255;
        if (parts.length > 3) {
            a = alpha(parts[3]);
        }
        return new Color(r, g, b, a);
    }

    private static Color parseHsl(String s) {
        int open = s.indexOf('(');
        int close = s.indexOf(')');
        if (open < 0 || close < open) {
            return null;
        }
        String[] parts = s.substring(open + 1, close).split(",");
        if (parts.length < 3) {
            return null;
        }
        float h = Float.parseFloat(parts[0].trim());
        float sat = percent(parts[1]);
        float light = percent(parts[2]);
        float a = parts.length > 3 ? alpha(parts[3]) / 255f : 1f;
        h = ((h % 360f) + 360f) % 360f;
        float c = (1f - Math.abs(2f * light - 1f)) * sat;
        float hp = h / 60f;
        float x = c * (1f - Math.abs((hp % 2f) - 1f));
        float r = 0;
        float g = 0;
        float b = 0;
        if (hp < 1) {
            r = c;
            g = x;
        } else if (hp < 2) {
            r = x;
            g = c;
        } else if (hp < 3) {
            g = c;
            b = x;
        } else if (hp < 4) {
            g = x;
            b = c;
        } else if (hp < 5) {
            r = x;
            b = c;
        } else {
            r = c;
            b = x;
        }
        float m = light - c / 2f;
        return new Color(clamp01(r + m), clamp01(g + m), clamp01(b + m), clamp01(a));
    }

    private static int channel(String raw) {
        String s = raw.trim();
        if (s.endsWith("%")) {
            return Math.max(0, Math.min(255, Math.round(percent(s) * 255f)));
        }
        return Math.max(0, Math.min(255, Math.round(Float.parseFloat(s))));
    }

    private static int alpha(String raw) {
        float v = Float.parseFloat(raw.trim());
        if (v <= 1f) {
            return Math.max(0, Math.min(255, Math.round(v * 255f)));
        }
        return Math.max(0, Math.min(255, Math.round(v)));
    }

    private static float percent(String raw) {
        String s = raw.trim();
        if (s.endsWith("%")) {
            s = s.substring(0, s.length() - 1).trim();
        }
        return Math.max(0f, Math.min(1f, Float.parseFloat(s) / 100f));
    }

    private static float clamp01(float v) {
        if (v < 0) {
            return 0;
        }
        if (v > 1) {
            return 1;
        }
        return v;
    }

    private static boolean isNull(Object v) {
        return v == null || v == JSONObject.NULL;
    }

    static boolean allowed(String url) {
        if (url == null || url.isEmpty()) {
            return false;
        }
        String lower = url.toLowerCase(Locale.US);
        if (lower.contains("api_key=")
                || lower.contains("apikey=")
                || lower.contains("access_token=")
                || lower.contains("token=")
                || lower.contains("key=")) {
            return false;
        }
        try {
            URL u = new URL(url);
            return "https".equalsIgnoreCase(u.getProtocol()) && HOST.equalsIgnoreCase(u.getHost());
        } catch (Exception e) {
            return false;
        }
    }

    private static byte[] httpGet(String url, String accept) {
        if (!allowed(url)) {
            return null;
        }
        HttpURLConnection conn = null;
        try {
            conn = (HttpURLConnection) new URL(url).openConnection();
            conn.setConnectTimeout(CONNECT_TIMEOUT_MS);
            conn.setReadTimeout(READ_TIMEOUT_MS);
            conn.setInstanceFollowRedirects(true);
            conn.setRequestMethod("GET");
            conn.setRequestProperty("User-Agent", BasemapTiles.USER_AGENT);
            conn.setRequestProperty("Accept", accept);
            int code = conn.getResponseCode();
            if (code < 200 || code >= 300) {
                return null;
            }
            InputStream in = conn.getInputStream();
            try {
                ByteArrayOutputStream out = new ByteArrayOutputStream();
                byte[] buf = new byte[8192];
                int n;
                int total = 0;
                while ((n = in.read(buf)) >= 0) {
                    total += n;
                    if (total > MAX_BYTES) {
                        return null;
                    }
                    out.write(buf, 0, n);
                }
                return out.toByteArray();
            } finally {
                in.close();
            }
        } catch (Exception e) {
            return null;
        } finally {
            if (conn != null) {
                conn.disconnect();
            }
        }
    }

    private static byte[] gunzipIfNeeded(byte[] body) {
        if (body == null || body.length < 2) {
            return body;
        }
        if ((body[0] & 0xff) != 0x1f || (body[1] & 0xff) != 0x8b) {
            return body;
        }
        try {
            GZIPInputStream in = new GZIPInputStream(new ByteArrayInputStream(body));
            try {
                ByteArrayOutputStream out = new ByteArrayOutputStream();
                byte[] buf = new byte[8192];
                int n;
                while ((n = in.read(buf)) >= 0) {
                    out.write(buf, 0, n);
                }
                return out.toByteArray();
            } finally {
                in.close();
            }
        } catch (Exception e) {
            return body;
        }
    }

    private static final class ParsedStyle {
        String tileTemplate;
        String vectorSource;
        int maxZoom = DEFAULT_MAX_ZOOM;
        Color background = Color.WHITE;
        final List<LayerPaint> layers = new ArrayList<LayerPaint>();
    }

    private static final class LayerPaint {
        String type;
        String sourceLayer;
        Object colorExpr;
        Object opacityExpr;
        Object widthExpr;
        Object outlineExpr;
        Object filter;
        double minZoom;
        double maxZoom = 24;
    }
}
