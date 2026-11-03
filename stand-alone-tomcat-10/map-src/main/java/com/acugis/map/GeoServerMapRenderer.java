package com.acugis.map;

import java.awt.image.BufferedImage;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.Locale;
import javax.imageio.ImageIO;
import org.json.JSONArray;
import org.json.JSONObject;
import org.locationtech.jts.geom.Envelope;

/**
 * GeoServer WMS GetMap renderer for Designer Preview / Report fill.
 *
 * <pre>
 * GeoServerMapRenderer.render(baseUrl, workspace, layer, style, width, height,
 *     connectionName, cql, bbox, srs, zoomToFiltered)
 * GeoServerMapRenderer.render(baseUrl, workspace, layer, style, width, height,
 *     connectionName, cql, bbox, srs, zoomToFiltered, basemap)
 * </pre>
 *
 * <p>{@code connectionName} is the Report Server admin connection name (Designer
 * Connection Manager display name), <em>not</em> the Designer's local UUID.
 * Credentials via {@link GeoServerAuth}. Empty name / anonymous → URL-only GetMap.
 * Failures return a placeholder image.
 */
public final class GeoServerMapRenderer {

    private GeoServerMapRenderer() {}

    public static BufferedImage render(
            String baseUrl,
            String workspace,
            String layer,
            String style,
            int width,
            int height,
            String connectionId,
            String cqlFilter) {
        return render(baseUrl, workspace, layer, style, width, height, connectionId, cqlFilter, null, null, true, null);
    }

    public static BufferedImage render(
            String baseUrl,
            String workspace,
            String layer,
            String style,
            int width,
            int height,
            String connectionId,
            String cqlFilter,
            String bbox,
            String srs) {
        return render(baseUrl, workspace, layer, style, width, height, connectionId, cqlFilter, bbox, srs, true, null);
    }

    public static BufferedImage render(
            String baseUrl,
            String workspace,
            String layer,
            String style,
            int width,
            int height,
            String connectionId,
            String cqlFilter,
            String bbox,
            String srs,
            boolean zoomToFiltered) {
        return render(baseUrl, workspace, layer, style, width, height, connectionId, cqlFilter, bbox, srs, zoomToFiltered, null);
    }

    public static BufferedImage render(
            String baseUrl,
            String workspace,
            String layer,
            String style,
            int width,
            int height,
            String connectionId,
            String cqlFilter,
            String bbox,
            String srs,
            boolean zoomToFiltered,
            String basemap) {
        int w = Math.max(1, width);
        int h = Math.max(1, height);
        try {
            GeoServerAuth.Credentials creds = GeoServerAuth.get(connectionId);
            String root = trimSlash(baseUrl);
            // Prefer snapshotted URL from JRXML; fall back to server connection store URL.
            if (root.isEmpty() && creds != null && creds.baseUrl != null && !creds.baseUrl.isEmpty()) {
                root = trimSlash(creds.baseUrl);
            }
            if (root.isEmpty()) {
                throw new IllegalArgumentException("GeoServer base URL is empty");
            }
            String ws = safe(workspace);
            String lyr = safe(layer);
            if (lyr.isEmpty()) {
                throw new IllegalArgumentException("GeoServer layer is empty");
            }
            String qualified = lyr.contains(":") ? lyr : (ws.isEmpty() ? lyr : ws + ":" + lyr);
            String wmsBase = wmsEndpoint(root, ws);
            String useSrs = safe(srs);
            if (useSrs.isEmpty()) {
                useSrs = "EPSG:4326";
            }
            String cql = cqlFilter == null ? "" : cqlFilter.trim();

            String fallbackBbox = safe(bbox);
            if (fallbackBbox.isEmpty()) {
                fallbackBbox = "-180,-90,180,90";
            }
            String useBbox = fallbackBbox;

            // Zoom GetMap BBOX to filtered features when enabled and CQL is active.
            // Invalid/failed WFS extent → keep fallback so filtered features still show.
            if (zoomToFiltered && !cql.isEmpty()) {
                double[] filtered = fetchFilteredExtent(root, ws, qualified, cql, useSrs, creds);
                if (filtered != null && isUsableExtent(filtered, useSrs)) {
                    filtered = padExtent(filtered, 0.08);
                    filtered = fitExtentToAspect(filtered, w, h);
                    useBbox = extentCsv(filtered);
                }
            }

            boolean wantBasemap = !BasemapTiles.isNone(basemap);

            StringBuilder q = new StringBuilder();
            append(q, "SERVICE", "WMS");
            append(q, "VERSION", "1.1.1");
            append(q, "REQUEST", "GetMap");
            append(q, "LAYERS", qualified);
            append(q, "STYLES", style == null ? "" : style);
            append(q, "SRS", useSrs);
            append(q, "BBOX", useBbox);
            append(q, "WIDTH", String.valueOf(w));
            append(q, "HEIGHT", String.valueOf(h));
            append(q, "FORMAT", "image/png");
            append(q, "TRANSPARENT", wantBasemap ? "true" : "false");
            if (!wantBasemap) {
                append(q, "BGCOLOR", "0xFFFFFF");
            }
            append(q, "EXCEPTIONS", "application/vnd.ogc.se_inimage");
            if (!cql.isEmpty()) {
                append(q, "CQL_FILTER", cql);
            }

            String url = wmsBase + (wmsBase.contains("?") ? "&" : "?") + q.toString();
            byte[] body = httpGet(url, creds);
            BufferedImage overlay = ImageIO.read(new ByteArrayInputStream(body));
            if (overlay == null) {
                String hint = sniffError(body);
                throw new IllegalArgumentException(
                        "GeoServer GetMap did not return a PNG" + (hint.isEmpty() ? "" : ": " + hint));
            }

            if (!wantBasemap) {
                return overlay;
            }

            Envelope world = parseBbox(useBbox);
            BufferedImage base = BasemapTiles.render(world, w, h, basemap, useSrs);
            java.awt.Graphics2D g = base.createGraphics();
            try {
                g.drawImage(overlay, 0, 0, null);
            } finally {
                g.dispose();
            }
            return base;
        } catch (Throwable t) {
            return ErrorImage.create(w, h, t);
        }
    }

    /** Overload without CQL / extent. */
    public static BufferedImage render(
            String baseUrl,
            String workspace,
            String layer,
            String style,
            int width,
            int height,
            String connectionId) {
        return render(baseUrl, workspace, layer, style, width, height, connectionId, null, null, null, true, null);
    }

    private static Envelope parseBbox(String bbox) {
        try {
            String[] p = bbox.split(",");
            if (p.length < 4) {
                return new Envelope(-180, 180, -90, 90);
            }
            double minX = Double.parseDouble(p[0].trim());
            double minY = Double.parseDouble(p[1].trim());
            double maxX = Double.parseDouble(p[2].trim());
            double maxY = Double.parseDouble(p[3].trim());
            return new Envelope(minX, maxX, minY, maxY);
        } catch (Exception e) {
            return new Envelope(-180, 180, -90, 90);
        }
    }

    /**
     * WFS GetFeature + GeoJSON bbox for filtered features (Publisher zoomToFiltered).
     * @return double[]{minX,minY,maxX,maxY} or null
     */
    private static double[] fetchFilteredExtent(
            String root,
            String workspace,
            String typeName,
            String cql,
            String srs,
            GeoServerAuth.Credentials creds) {
        try {
            String wfsBase = wfsEndpoint(root, workspace);
            StringBuilder q = new StringBuilder();
            append(q, "SERVICE", "WFS");
            append(q, "VERSION", "1.1.0");
            append(q, "REQUEST", "GetFeature");
            append(q, "TYPENAME", typeName);
            append(q, "OUTPUTFORMAT", "application/json");
            // GeoJSON is always lon/lat; request CRS84 to avoid axis-order surprises.
            append(q, "SRSNAME", "EPSG:4326");
            append(q, "MAXFEATURES", "5000");
            append(q, "CQL_FILTER", cql);
            String url = wfsBase + (wfsBase.contains("?") ? "&" : "?") + q.toString();
            byte[] body = httpGet(url, creds);
            return geoJsonComputeBbox(body);
        } catch (Throwable ignored) {
            return null;
        }
    }

    /**
     * Prefer feature geometries over FeatureCollection-level bbox (Publisher-aligned).
     */
    private static double[] geoJsonComputeBbox(byte[] body) throws Exception {
        if (body == null || body.length == 0) {
            return null;
        }
        String text = new String(body, StandardCharsets.UTF_8).trim();
        if (text.isEmpty() || (text.charAt(0) != '{' && text.charAt(0) != '[')) {
            return null;
        }
        JSONObject geojson = new JSONObject(text);
        String type = geojson.optString("type", "");
        JSONArray features = geojson.optJSONArray("features");
        boolean hasFeatures = "FeatureCollection".equals(type)
                && features != null
                && features.length() > 0;

        BboxAcc acc = new BboxAcc();
        if (hasFeatures) {
            for (int i = 0; i < features.length(); i++) {
                JSONObject feature = features.optJSONObject(i);
                if (feature == null) {
                    continue;
                }
                JSONArray fbb = feature.optJSONArray("bbox");
                if (fbb != null && fbb.length() >= 4) {
                    acc.include(fbb.getDouble(0), fbb.getDouble(1));
                    acc.include(fbb.getDouble(2), fbb.getDouble(3));
                    continue;
                }
                visitGeometry(feature.optJSONObject("geometry"), acc);
            }
        } else {
            JSONArray bb = geojson.optJSONArray("bbox");
            if (bb != null && bb.length() >= 4) {
                return new double[] {bb.getDouble(0), bb.getDouble(1), bb.getDouble(2), bb.getDouble(3)};
            }
            visitGeometry(geojson, acc);
        }
        return acc.toArray();
    }

    private static void visitGeometry(JSONObject geom, BboxAcc acc) {
        if (geom == null) {
            return;
        }
        String gType = geom.optString("type", "");
        if ("GeometryCollection".equals(gType)) {
            JSONArray geoms = geom.optJSONArray("geometries");
            if (geoms != null) {
                for (int i = 0; i < geoms.length(); i++) {
                    visitGeometry(geoms.optJSONObject(i), acc);
                }
            }
            return;
        }
        visitCoords(geom.optJSONArray("coordinates"), acc);
    }

    private static void visitCoords(JSONArray coords, BboxAcc acc) {
        if (coords == null || coords.length() == 0) {
            return;
        }
        // Position: [x, y, ...]
        Object first = coords.opt(0);
        if (coords.length() >= 2 && first instanceof Number && coords.opt(1) instanceof Number) {
            acc.include(coords.getDouble(0), coords.getDouble(1));
            return;
        }
        for (int i = 0; i < coords.length(); i++) {
            Object child = coords.opt(i);
            if (child instanceof JSONArray) {
                visitCoords((JSONArray) child, acc);
            }
        }
    }

    private static boolean isUsableExtent(double[] b, String srs) {
        if (b == null || b.length < 4) {
            return false;
        }
        if (!(b[0] < b[2]) || !(b[1] < b[3])) {
            return false;
        }
        double w = b[2] - b[0];
        double h = b[3] - b[1];
        if (!(w > 0) || !(h > 0) || Double.isInfinite(w) || Double.isInfinite(h)
                || Double.isNaN(w) || Double.isNaN(h)) {
            return false;
        }
        // Reject obviously broken parses (e.g. property numbers mistaken for coordinates).
        String s = srs == null ? "" : srs.toUpperCase(Locale.US);
        if (s.contains("4326") || s.contains("CRS84") || s.isEmpty()) {
            if (Math.abs(b[0]) > 180.5 || Math.abs(b[2]) > 180.5
                    || Math.abs(b[1]) > 90.5 || Math.abs(b[3]) > 90.5) {
                return false;
            }
        }
        // Degenerate / near-point: still usable after pad, but reject zero-area already handled.
        return true;
    }

    private static double[] padExtent(double[] b, double padFraction) {
        double dx = (b[2] - b[0]) * padFraction;
        double dy = (b[3] - b[1]) * padFraction;
        if (dx <= 0) {
            dx = 0.05;
        }
        if (dy <= 0) {
            dy = 0.05;
        }
        // Minimum pad so tiny multipolygons don't fill the frame edge-to-edge oddly.
        if (dx < 0.05) {
            dx = 0.05;
        }
        if (dy < 0.05) {
            dy = 0.05;
        }
        return new double[] {b[0] - dx, b[1] - dy, b[2] + dx, b[3] + dy};
    }

    private static double[] fitExtentToAspect(double[] b, int width, int height) {
        if (b == null || width < 1 || height < 1) {
            return b;
        }
        double bw = b[2] - b[0];
        double bh = b[3] - b[1];
        if (bw <= 0 || bh <= 0) {
            return b;
        }
        double target = (double) width / (double) height;
        double current = bw / bh;
        if (Math.abs(current - target) < 1e-6) {
            return b;
        }
        double cx = (b[0] + b[2]) / 2.0;
        double cy = (b[1] + b[3]) / 2.0;
        if (current > target) {
            double newH = bw / target;
            return new double[] {b[0], cy - newH / 2.0, b[2], cy + newH / 2.0};
        }
        double newW = bh * target;
        return new double[] {cx - newW / 2.0, b[1], cx + newW / 2.0, b[3]};
    }

    private static String extentCsv(double[] b) {
        return b[0] + "," + b[1] + "," + b[2] + "," + b[3];
    }

    private static final class BboxAcc {
        boolean found;
        double minX;
        double minY;
        double maxX;
        double maxY;

        void include(double x, double y) {
            if (!found) {
                minX = maxX = x;
                minY = maxY = y;
                found = true;
                return;
            }
            minX = Math.min(minX, x);
            minY = Math.min(minY, y);
            maxX = Math.max(maxX, x);
            maxY = Math.max(maxY, y);
        }

        double[] toArray() {
            if (!found) {
                return null;
            }
            return new double[] {minX, minY, maxX, maxY};
        }
    }

    private static String wmsEndpoint(String root, String workspace) throws Exception {
        String lower = root.toLowerCase(Locale.US);
        if (lower.endsWith("/wms") || lower.contains("/wms?")) {
            return root;
        }
        if (!workspace.isEmpty()) {
            return root + "/" + encodePath(workspace) + "/wms";
        }
        return root + "/wms";
    }

    private static String wfsEndpoint(String root, String workspace) throws Exception {
        String lower = root.toLowerCase(Locale.US);
        if (lower.endsWith("/wfs") || lower.contains("/wfs?")) {
            return root;
        }
        if (lower.endsWith("/wms")) {
            return root.substring(0, root.length() - 3) + "wfs";
        }
        if (!workspace.isEmpty()) {
            return root + "/" + encodePath(workspace) + "/wfs";
        }
        return root + "/wfs";
    }

    private static byte[] httpGet(String url, GeoServerAuth.Credentials creds) throws Exception {
        HttpURLConnection conn = (HttpURLConnection) new URL(url).openConnection();
        conn.setConnectTimeout(20000);
        conn.setReadTimeout(60000);
        conn.setInstanceFollowRedirects(true);
        conn.setRequestMethod("GET");
        conn.setRequestProperty("Accept", "image/png,application/json,image/*,*/*");
        if (creds != null && creds.hasUser()) {
            String token = Base64.getEncoder()
                    .encodeToString((creds.username + ":" + creds.password).getBytes(StandardCharsets.UTF_8));
            conn.setRequestProperty("Authorization", "Basic " + token);
        }
        int code = conn.getResponseCode();
        InputStream in = code >= 400 ? conn.getErrorStream() : conn.getInputStream();
        if (in == null) {
            throw new IllegalArgumentException("GeoServer HTTP " + code + " with empty body");
        }
        try {
            byte[] body = readAll(in);
            if (code == 401 || code == 403) {
                throw new IllegalArgumentException(
                        "authentication failed (" + code + "): credentials required or invalid");
            }
            if (code < 200 || code >= 300) {
                throw new IllegalArgumentException(
                        "GeoServer HTTP " + code + ": " + sniffError(body));
            }
            return body;
        } finally {
            in.close();
            conn.disconnect();
        }
    }

    private static byte[] readAll(InputStream in) throws Exception {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        byte[] buf = new byte[8192];
        int n;
        while ((n = in.read(buf)) >= 0) {
            out.write(buf, 0, n);
            if (out.size() > 16 * 1024 * 1024) {
                throw new IllegalArgumentException("GeoServer response too large");
            }
        }
        return out.toByteArray();
    }

    private static String sniffError(byte[] body) {
        if (body == null || body.length == 0) {
            return "";
        }
        String s = new String(body, 0, Math.min(body.length, 400), StandardCharsets.UTF_8)
                .replace('\n', ' ')
                .replace('\r', ' ')
                .trim();
        if (s.length() > 180) {
            s = s.substring(0, 180) + "…";
        }
        return s;
    }

    private static void append(StringBuilder q, String key, String value) throws Exception {
        if (q.length() > 0) {
            q.append('&');
        }
        q.append(URLEncoder.encode(key, "UTF-8"));
        q.append('=');
        q.append(URLEncoder.encode(value == null ? "" : value, "UTF-8"));
    }

    private static String encodePath(String segment) throws Exception {
        return URLEncoder.encode(segment, "UTF-8").replace("+", "%20");
    }

    private static String trimSlash(String s) {
        if (s == null) {
            return "";
        }
        String t = s.trim();
        while (t.endsWith("/")) {
            t = t.substring(0, t.length() - 1);
        }
        return t;
    }

    private static String safe(String s) {
        return s == null ? "" : s.trim();
    }
}
