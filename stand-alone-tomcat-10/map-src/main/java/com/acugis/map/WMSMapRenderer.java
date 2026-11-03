package com.acugis.map;

import java.awt.image.BufferedImage;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStreamWriter;
import java.io.Writer;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.Locale;
import javax.imageio.ImageIO;
import org.locationtech.jts.geom.Envelope;
import org.locationtech.jts.geom.Geometry;

/**
 * Generic OGC WMS GetMap renderer for Designer Preview / Report fill.
 *
 * <pre>
 * WMSMapRenderer.render(serviceUrl, layers, styles, width, height,
 *     connectionName, bbox, srs, version, format, basemap)
 * WMSMapRenderer.render(..., basemap, extentConnection, extentSql)
 * </pre>
 *
 * <p>{@code connectionName} is the WMS Connection Manager name (Basic auth via
 * {@link WMSAuth}). {@code extentConnection}/{@code extentSql} compute the GetMap
 * BBOX from report dataset geometries the same way {@link MapRenderer} does
 * (Current Group / dataset envelope). Static {@code bbox} is the Full Layer /
 * Fixed fallback from GetCapabilities.
 */
public final class WMSMapRenderer {

    private WMSMapRenderer() {}

    public static BufferedImage render(
            String serviceUrl,
            String layers,
            String styles,
            int width,
            int height,
            String connectionName,
            String bbox,
            String srs,
            String version,
            String format,
            String basemap) {
        return render(
                serviceUrl, layers, styles, width, height, connectionName, bbox, srs, version, format, basemap,
                null, null);
    }

    public static BufferedImage render(
            String serviceUrl,
            String layers,
            String styles,
            int width,
            int height,
            String connectionName,
            String bbox,
            String srs,
            String version,
            String format,
            String basemap,
            Connection extentConnection,
            String extentSql) {
        int w = Math.max(1, width);
        int h = Math.max(1, height);
        try {
            WMSAuth.Credentials creds = WMSAuth.get(connectionName);
            String root = trimSlash(serviceUrl);
            if (root.isEmpty() && creds != null && creds.baseUrl != null && !creds.baseUrl.isEmpty()) {
                root = trimSlash(creds.baseUrl);
            }
            if (root.isEmpty()) {
                throw new IllegalArgumentException("WMS service URL is empty");
            }
            String lyr = safe(layers);
            if (lyr.isEmpty()) {
                throw new IllegalArgumentException("WMS layer is empty");
            }

            String useVersion = normalizeVersion(version);
            String useSrs = safe(srs);
            if (useSrs.isEmpty()) {
                useSrs = "EPSG:4326";
            }
            String useFormat = normalizeFormat(format);

            // 1) Prefer envelope from report dataset SQL (Current Group / dataset extent),
            //    same geometry load path as MapRenderer.
            // 2) Else static bbox (Full Layer from GetCapabilities / Fixed).
            String designBbox = safe(bbox); // minX,minY,maxX,maxY before WMS 1.3.0 axis swap
            boolean fromDataset = false;
            Envelope groupEnvelope = envelopeFromSQL(extentConnection, extentSql);
            if (groupEnvelope != null && !groupEnvelope.isNull()) {
                logLine("WMS group envelope: "
                        + groupEnvelope.getMinX() + "," + groupEnvelope.getMinY() + ","
                        + groupEnvelope.getMaxX() + "," + groupEnvelope.getMaxY());
                MapViewport viewport = MapViewport.fromEnvelope(groupEnvelope, w, h, 0.08);
                designBbox = envelopeCsv(viewport.world);
                fromDataset = true;
                logLine("WMS GetMap BBOX (from group/dataset, pre-axis): " + designBbox);
            } else {
                if (extentSql != null && !extentSql.trim().isEmpty()) {
                    logLine("WMS group envelope: unavailable (no features); using fallback BBOX");
                }
                if (designBbox.isEmpty()) {
                    designBbox = defaultBbox(useSrs);
                }
                logLine("WMS GetMap BBOX (fallback/full-layer, pre-axis): " + designBbox);
            }

            // Static GetCapabilities extents are geographic degrees. If CRS is projected
            // but BBOX is still geographic, fall back to EPSG:4326. Do not override CRS
            // when BBOX came from dataset geometries (native SRS).
            if (!fromDataset && !isGeographicCRS(useSrs) && looksGeographicBbox(designBbox)) {
                useSrs = "EPSG:4326";
            }

            String useBbox = designBbox;
            // WMS 1.3.0 axis order for geographic CRS: apply only after envelope is final.
            if (is130(useVersion) && isGeographicCRS(useSrs)) {
                useBbox = swapBboxAxis(useBbox);
            }
            logLine("WMS GetMap BBOX (final): " + useBbox + " CRS=" + useSrs + " VERSION=" + useVersion);

            boolean wantBasemap = !BasemapTiles.isNone(basemap);
            boolean png = useFormat.toLowerCase(Locale.US).contains("png");

            StringBuilder q = new StringBuilder();
            append(q, "SERVICE", "WMS");
            append(q, "VERSION", useVersion);
            append(q, "REQUEST", "GetMap");
            append(q, "LAYERS", lyr);
            append(q, "STYLES", styles == null ? "" : styles);
            if (is130(useVersion)) {
                append(q, "CRS", useSrs);
            } else {
                append(q, "SRS", useSrs);
            }
            append(q, "BBOX", useBbox);
            append(q, "WIDTH", String.valueOf(w));
            append(q, "HEIGHT", String.valueOf(h));
            append(q, "FORMAT", useFormat);
            if (png) {
                append(q, "TRANSPARENT", wantBasemap ? "true" : "false");
            }
            if (!wantBasemap && png) {
                append(q, "BGCOLOR", "0xFFFFFF");
            }
            // Omit EXCEPTIONS — GEBCO and many servers reject se_inimage.

            String url = joinQuery(root, q.toString());
            logGetMapURL(url);
            byte[] body = httpGet(url, creds, useFormat);
            BufferedImage overlay = ImageIO.read(new ByteArrayInputStream(body));
            if (overlay == null) {
                String hint = sniffError(body);
                throw new IllegalArgumentException(
                        "WMS GetMap did not return an image" + (hint.isEmpty() ? "" : ": " + hint));
            }

            if (!wantBasemap) {
                return overlay;
            }

            String basemapBbox = safe(designBbox);
            if (basemapBbox.isEmpty()) {
                basemapBbox = defaultBbox(useSrs);
            }
            Envelope world = parseBbox(basemapBbox);
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

    /**
     * Load geometries via {@link GeometryLoader} (same as MapRenderer) and union envelopes.
     * @return envelope or null when unavailable
     */
    private static Envelope envelopeFromSQL(Connection connection, String sql) {
        if (connection == null || sql == null || sql.trim().isEmpty()) {
            return null;
        }
        try {
            List<MapFeature> features = GeometryLoader.loadFeatures(connection, sql.trim());
            if (features == null || features.isEmpty()) {
                return null;
            }
            List<Geometry> geoms = new ArrayList<Geometry>(features.size());
            for (MapFeature f : features) {
                if (f != null && f.geometry != null) {
                    geoms.add(f.geometry);
                }
            }
            Envelope env = MapViewport.envelopeOf(geoms);
            if (env == null || env.isNull()) {
                return null;
            }
            return env;
        } catch (Throwable t) {
            logLine("WMS group envelope SQL failed: " + t.getMessage());
            return null;
        }
    }

    private static String envelopeCsv(Envelope env) {
        if (env == null || env.isNull()) {
            return "";
        }
        return env.getMinX() + "," + env.getMinY() + "," + env.getMaxX() + "," + env.getMaxY();
    }

    /**
     * Writes the exact GetMap URL where the Designer GUI can find it.
     * The packaged Windows exe has no console; PreviewRunner stdout/stderr are
     * captured in-process and discarded on success — so we also append to a
     * temp file: {@code %TEMP%/acugis-wms-getmap.log} (or java.io.tmpdir).
     */
    private static void logGetMapURL(String url) {
        logLine("WMS GetMap URL: " + url);
        try {
            File log = new File(System.getProperty("java.io.tmpdir"), "acugis-wms-getmap.log");
            logLine("WMS GetMap URL also written to: " + log.getAbsolutePath());
        } catch (Exception ignored) {
            // ignore
        }
    }

    private static void logLine(String line) {
        if (line == null) {
            return;
        }
        System.out.println(line);
        System.err.println(line);
        try {
            File log = new File(System.getProperty("java.io.tmpdir"), "acugis-wms-getmap.log");
            try (Writer w = new OutputStreamWriter(new FileOutputStream(log, true), StandardCharsets.UTF_8)) {
                w.write(line);
                w.write(System.lineSeparator());
            }
        } catch (Exception ignored) {
            // logging must never break GetMap
        }
    }

    private static String normalizeVersion(String version) {
        String v = safe(version).toLowerCase(Locale.US);
        if (v.isEmpty() || "auto".equals(v)) {
            return "1.3.0";
        }
        if (v.startsWith("1.1")) {
            return "1.1.1";
        }
        return "1.3.0";
    }

    private static boolean is130(String version) {
        return version != null && version.startsWith("1.3");
    }

    private static String normalizeFormat(String format) {
        String f = safe(format).toLowerCase(Locale.US);
        if (f.contains("jpeg") || f.contains("jpg")) {
            return "image/jpeg";
        }
        return "image/png";
    }

    private static boolean isGeographicCRS(String srs) {
        String s = srs == null ? "" : srs.toUpperCase(Locale.US);
        return s.contains("4326") || s.contains("CRS:84") || s.contains("CRS84") || s.contains("OGC:CRS84");
    }

    /**
     * True when BBOX values look like lon/lat degrees (incl. GEBCO's -180..360 lon),
     * not projected metres (EPSG:3857 ≈ ±2e7).
     */
    private static boolean looksGeographicBbox(String bbox) {
        try {
            String[] p = bbox.split(",");
            if (p.length < 4) {
                return false;
            }
            double minX = Double.parseDouble(p[0].trim());
            double minY = Double.parseDouble(p[1].trim());
            double maxX = Double.parseDouble(p[2].trim());
            double maxY = Double.parseDouble(p[3].trim());
            if (Math.abs(minX) > 360.5 || Math.abs(maxX) > 360.5
                    || Math.abs(minY) > 90.5 || Math.abs(maxY) > 90.5) {
                return false;
            }
            // Mercator metres are orders of magnitude larger.
            return Math.abs(minX) < 1000 && Math.abs(maxX) < 1000
                    && Math.abs(minY) < 1000 && Math.abs(maxY) < 1000;
        } catch (Exception e) {
            return false;
        }
    }

    /** Join base service URL with query params without producing {@code ?&}. */
    private static String joinQuery(String root, String query) {
        if (root == null || root.isEmpty()) {
            return "?" + query;
        }
        if (root.endsWith("?") || root.endsWith("&")) {
            return root + query;
        }
        return root + (root.contains("?") ? "&" : "?") + query;
    }

    private static String defaultBbox(String srs) {
        if (isGeographicCRS(srs)) {
            return "-180,-90,180,90";
        }
        // Web Mercator world approx.
        return "-20037508.34,-20037508.34,20037508.34,20037508.34";
    }

    /** Swap minX,minY,maxX,maxY → minY,minX,maxY,maxX for WMS 1.3.0 geographic CRS. */
    private static String swapBboxAxis(String bbox) {
        try {
            String[] p = bbox.split(",");
            if (p.length < 4) {
                return bbox;
            }
            return p[1].trim() + "," + p[0].trim() + "," + p[3].trim() + "," + p[2].trim();
        } catch (Exception e) {
            return bbox;
        }
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

    private static byte[] httpGet(String url, WMSAuth.Credentials creds, String format) throws Exception {
        HttpURLConnection conn = (HttpURLConnection) new URL(url).openConnection();
        conn.setConnectTimeout(20000);
        conn.setReadTimeout(60000);
        conn.setInstanceFollowRedirects(true);
        conn.setRequestMethod("GET");
        String accept = format + ",image/*,*/*";
        conn.setRequestProperty("Accept", accept);
        if (creds != null && creds.hasUser()) {
            String token = Base64.getEncoder()
                    .encodeToString((creds.username + ":" + creds.password).getBytes(StandardCharsets.UTF_8));
            conn.setRequestProperty("Authorization", "Basic " + token);
        }
        int code = conn.getResponseCode();
        InputStream in = code >= 400 ? conn.getErrorStream() : conn.getInputStream();
        if (in == null) {
            throw new IllegalArgumentException("WMS HTTP " + code + " with empty body");
        }
        try {
            byte[] body = readAll(in);
            if (code == 401 || code == 403) {
                throw new IllegalArgumentException(
                        "authentication failed (" + code + "): credentials required or invalid");
            }
            if (code < 200 || code >= 300) {
                throw new IllegalArgumentException("WMS HTTP " + code + ": " + sniffError(body));
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
                throw new IllegalArgumentException("WMS response too large");
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
