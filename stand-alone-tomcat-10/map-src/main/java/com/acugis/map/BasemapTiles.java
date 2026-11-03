package com.acugis.map;

import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.image.BufferedImage;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.Locale;
import javax.imageio.ImageIO;
import org.locationtech.jts.geom.Coordinate;
import org.locationtech.jts.geom.Envelope;

/**
 * Fetches and stitches XYZ basemap tiles into a map {@link BufferedImage}.
 *
 * <p>Supported keys (aligned with Report Designer): {@code none}, {@code osm},
 * {@code openfreemap_light}, {@code openfreemap_dark}, {@code esri_imagery}.
 * Saved {@code carto_light} / {@code carto_dark} reports render as OpenFreeMap
 * Positron and Dark. OpenFreeMap styles are public MapLibre documents; no API key.
 *
 * <p>Works when the map CRS is geographic (EPSG:4326 / CRS84) or Web Mercator
 * (EPSG:3857 / 900913). Other projections skip the basemap silently.
 */
public final class BasemapTiles {

    static final String USER_AGENT = "AcuGIS-Report-Designer/1.0 (map basemap)";
    private static final int CONNECT_TIMEOUT_MS = 8000;
    private static final int READ_TIMEOUT_MS = 12000;
    private static final int TILE_SIZE = 256;
    private static final double ORIGIN_SHIFT = 20037508.342789244;
    private static final int MAX_TILES = 64;

    private BasemapTiles() {}

    public static String normalize(String raw) {
        if (raw == null) {
            return "none";
        }
        String k = raw.trim().toLowerCase(Locale.US);
        if (k.isEmpty() || "none".equals(k) || "null".equals(k)) {
            return "none";
        }
        if ("osm".equals(k) || "openstreetmap".equals(k)) {
            return "osm";
        }
        if ("openfreemap_light".equals(k)
                || "openfreemap light".equals(k)
                || "openfreemap_positron".equals(k)
                || "positron".equals(k)
                || "carto_light".equals(k)
                || "carto light".equals(k)
                || "carto".equals(k)) {
            return "openfreemap_light";
        }
        if ("openfreemap_dark".equals(k)
                || "openfreemap dark".equals(k)
                || "carto_dark".equals(k)
                || "carto dark".equals(k)) {
            return "openfreemap_dark";
        }
        if ("esri_imagery".equals(k)
                || "esri imagery".equals(k)
                || "esri_satellite".equals(k)
                || "esri satellite".equals(k)
                || "satellite".equals(k)) {
            return "esri_imagery";
        }
        return "none";
    }

    public static boolean isNone(String key) {
        return "none".equals(normalize(key));
    }

    /**
     * Paint basemap tiles into {@code g} for the given viewport. Returns {@code true}
     * when at least one tile was drawn.
     */
    public static boolean paint(Graphics2D g, MapViewport viewport, String basemapKey, String srs) {
        String key = normalize(basemapKey);
        if ("none".equals(key) || viewport == null || viewport.world == null || viewport.world.isNull()) {
            return false;
        }
        CrsKind crs = crsKind(srs);
        if (crs == CrsKind.UNSUPPORTED) {
            // Heuristic: lon/lat or Web-Mercator-looking extents still get a basemap
            // even when SRS is blank or an unsupported EPSG label from catalog metadata.
            Envelope w = viewport.world;
            if (looksGeographic(w)) {
                crs = CrsKind.GEOGRAPHIC;
            } else if (looksWebMercator(w)) {
                crs = CrsKind.WEBMERCATOR;
            } else {
                return false;
            }
        }

        Envelope lonLat = toLonLatEnvelope(viewport.world, crs);
        if (lonLat == null || lonLat.isNull()) {
            return false;
        }
        clampGeographic(lonLat);

        int zoom = chooseZoom(lonLat, viewport.width, viewport.height);
        Provider provider = Provider.forKey(key);
        if (provider == null) {
            return false;
        }
        if (provider.vectorStyle() != null) {
            OpenFreeMapBasemap.prepare(provider.vectorStyle());
        }
        zoom = clamp(zoom, provider.minZoom(), provider.maxZoom());

        int n = 1 << zoom;
        int x0 = lonToTileX(lonLat.getMinX(), n);
        int x1 = lonToTileX(lonLat.getMaxX(), n);
        int y0 = latToTileY(lonLat.getMaxY(), n); // north → smaller tile Y
        int y1 = latToTileY(lonLat.getMinY(), n);
        x0 = clamp(x0, 0, n - 1);
        x1 = clamp(x1, 0, n - 1);
        y0 = clamp(y0, 0, n - 1);
        y1 = clamp(y1, 0, n - 1);
        if (x1 < x0 || y1 < y0) {
            return false;
        }
        int tileCount = (x1 - x0 + 1) * (y1 - y0 + 1);
        if (tileCount > MAX_TILES) {
            // Zoom out until tile count is manageable.
            while (tileCount > MAX_TILES && zoom > 0) {
                zoom--;
                n = 1 << zoom;
                x0 = clamp(lonToTileX(lonLat.getMinX(), n), 0, n - 1);
                x1 = clamp(lonToTileX(lonLat.getMaxX(), n), 0, n - 1);
                y0 = clamp(latToTileY(lonLat.getMaxY(), n), 0, n - 1);
                y1 = clamp(latToTileY(lonLat.getMinY(), n), 0, n - 1);
                tileCount = (x1 - x0 + 1) * (y1 - y0 + 1);
            }
        }

        boolean any = false;
        Object oldInterp = g.getRenderingHint(RenderingHints.KEY_INTERPOLATION);
        g.setRenderingHint(RenderingHints.KEY_INTERPOLATION, RenderingHints.VALUE_INTERPOLATION_BILINEAR);
        try {
            for (int ty = y0; ty <= y1; ty++) {
                for (int tx = x0; tx <= x1; tx++) {
                    BufferedImage tile = fetchTile(provider, zoom, tx, ty);
                    if (tile == null) {
                        continue;
                    }
                    Envelope tileWorld = tileEnvelopeWorld(tx, ty, zoom, crs);
                    if (tileWorld == null || tileWorld.isNull()) {
                        continue;
                    }
                    Coordinate tl = viewport.toPixel(new Coordinate(tileWorld.getMinX(), tileWorld.getMaxY()));
                    Coordinate br = viewport.toPixel(new Coordinate(tileWorld.getMaxX(), tileWorld.getMinY()));
                    int dx = (int) Math.floor(tl.x);
                    int dy = (int) Math.floor(tl.y);
                    int dw = Math.max(1, (int) Math.ceil(br.x) - dx);
                    int dh = Math.max(1, (int) Math.ceil(br.y) - dy);
                    g.drawImage(tile, dx, dy, dw, dh, null);
                    any = true;
                }
            }
        } finally {
            if (oldInterp != null) {
                g.setRenderingHint(RenderingHints.KEY_INTERPOLATION, oldInterp);
            } else {
                g.setRenderingHint(
                        RenderingHints.KEY_INTERPOLATION, RenderingHints.VALUE_INTERPOLATION_NEAREST_NEIGHBOR);
            }
        }
        return any;
    }

    /** Render basemap alone for a world envelope (GeoServer composite path). */
    public static BufferedImage render(Envelope world, int width, int height, String basemapKey, String srs) {
        int w = Math.max(1, width);
        int h = Math.max(1, height);
        BufferedImage image = new BufferedImage(w, h, BufferedImage.TYPE_INT_ARGB);
        Graphics2D g = image.createGraphics();
        try {
            g.setColor(java.awt.Color.WHITE);
            g.fillRect(0, 0, w, h);
            if (isNone(basemapKey) || world == null || world.isNull()) {
                return image;
            }
            // Stretch world→image like WMS GetMap (no letterboxing) so tiles align with overlay.
            MapViewport viewport = MapViewport.stretchFrame(world, w, h);
            paint(g, viewport, basemapKey, srs);
            return image;
        } finally {
            g.dispose();
        }
    }

    private enum CrsKind {
        GEOGRAPHIC,
        WEBMERCATOR,
        UNSUPPORTED
    }

    private static CrsKind crsKind(String srs) {
        if (srs == null || srs.trim().isEmpty()) {
            return CrsKind.UNSUPPORTED;
        }
        String s = srs.toUpperCase(Locale.US);
        if (s.contains("4326") || s.contains("CRS84") || s.contains("CRS:84")) {
            return CrsKind.GEOGRAPHIC;
        }
        if (s.contains("3857") || s.contains("900913") || s.contains("3785")) {
            return CrsKind.WEBMERCATOR;
        }
        return CrsKind.UNSUPPORTED;
    }

    private static boolean looksGeographic(Envelope w) {
        return Math.abs(w.getMinX()) <= 180.5
                && Math.abs(w.getMaxX()) <= 180.5
                && Math.abs(w.getMinY()) <= 90.5
                && Math.abs(w.getMaxY()) <= 90.5;
    }

    private static boolean looksWebMercator(Envelope w) {
        return Math.abs(w.getMinX()) <= ORIGIN_SHIFT * 1.05
                && Math.abs(w.getMaxX()) <= ORIGIN_SHIFT * 1.05
                && Math.abs(w.getMinY()) <= ORIGIN_SHIFT * 1.05
                && Math.abs(w.getMaxY()) <= ORIGIN_SHIFT * 1.05
                && (Math.abs(w.getMinX()) > 180.5 || Math.abs(w.getMaxX()) > 180.5);
    }

    private static Envelope toLonLatEnvelope(Envelope world, CrsKind crs) {
        if (crs == CrsKind.GEOGRAPHIC) {
            return new Envelope(world);
        }
        if (crs == CrsKind.WEBMERCATOR) {
            double minLon = mercXToLon(world.getMinX());
            double maxLon = mercXToLon(world.getMaxX());
            double minLat = mercYToLat(world.getMinY());
            double maxLat = mercYToLat(world.getMaxY());
            return new Envelope(minLon, maxLon, minLat, maxLat);
        }
        return null;
    }

    private static void clampGeographic(Envelope e) {
        double minX = Math.max(-180, Math.min(180, e.getMinX()));
        double maxX = Math.max(-180, Math.min(180, e.getMaxX()));
        double minY = Math.max(-85.05112878, Math.min(85.05112878, e.getMinY()));
        double maxY = Math.max(-85.05112878, Math.min(85.05112878, e.getMaxY()));
        e.init(minX, maxX, minY, maxY);
    }

    private static Envelope tileEnvelopeWorld(int tx, int ty, int zoom, CrsKind crs) {
        int n = 1 << zoom;
        double lon0 = tileXToLon(tx, n);
        double lon1 = tileXToLon(tx + 1, n);
        double lat0 = tileYToLat(ty + 1, n); // south
        double lat1 = tileYToLat(ty, n); // north
        if (crs == CrsKind.GEOGRAPHIC) {
            return new Envelope(lon0, lon1, lat0, lat1);
        }
        if (crs == CrsKind.WEBMERCATOR) {
            return new Envelope(lonToMercX(lon0), lonToMercX(lon1), latToMercY(lat0), latToMercY(lat1));
        }
        return null;
    }

    private static int chooseZoom(Envelope lonLat, int width, int height) {
        double lonSpan = Math.max(1e-9, lonLat.getWidth());
        double latSpan = Math.max(1e-9, lonLat.getHeight());
        double degPerPxX = lonSpan / Math.max(1, width);
        double degPerPxY = latSpan / Math.max(1, height);
        double degPerPx = Math.min(degPerPxX, degPerPxY);
        // Tile spans 360/2^z degrees of longitude across 256 px.
        double zExact = Math.log(360.0 / (degPerPx * TILE_SIZE)) / Math.log(2.0);
        int z = (int) Math.round(zExact);
        return clamp(z, 0, 18);
    }

    private static BufferedImage fetchTile(Provider provider, int z, int x, int y) {
        if (provider.vectorStyle() != null) {
            try {
                return OpenFreeMapBasemap.renderTile(provider.vectorStyle(), z, x, y);
            } catch (Exception e) {
                return null;
            }
        }
        String url = provider.url(z, x, y);
        HttpURLConnection conn = null;
        try {
            conn = (HttpURLConnection) new URL(url).openConnection();
            conn.setConnectTimeout(CONNECT_TIMEOUT_MS);
            conn.setReadTimeout(READ_TIMEOUT_MS);
            conn.setInstanceFollowRedirects(true);
            conn.setRequestMethod("GET");
            conn.setRequestProperty("User-Agent", USER_AGENT);
            conn.setRequestProperty("Accept", "image/png,image/jpeg,image/*,*/*");
            int code = conn.getResponseCode();
            if (code < 200 || code >= 300) {
                return null;
            }
            InputStream in = conn.getInputStream();
            try {
                return ImageIO.read(in);
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

    private enum Provider {
        OSM {
            @Override
            String url(int z, int x, int y) {
                char host = (char) ('a' + ((x + y) % 3));
                return "https://" + host + ".tile.openstreetmap.org/" + z + "/" + x + "/" + y + ".png";
            }
        },
        OPENFREEMAP_LIGHT {
            @Override
            String url(int z, int x, int y) {
                return OpenFreeMapBasemap.tileURL(vectorStyle(), z, x, y);
            }

            @Override
            String vectorStyle() {
                return OpenFreeMapBasemap.LIGHT_STYLE;
            }

            @Override
            int maxZoom() {
                return OpenFreeMapBasemap.maxZoom(vectorStyle());
            }
        },
        OPENFREEMAP_DARK {
            @Override
            String url(int z, int x, int y) {
                return OpenFreeMapBasemap.tileURL(vectorStyle(), z, x, y);
            }

            @Override
            String vectorStyle() {
                return OpenFreeMapBasemap.DARK_STYLE;
            }

            @Override
            int maxZoom() {
                return OpenFreeMapBasemap.maxZoom(vectorStyle());
            }
        },
        ESRI_IMAGERY {
            @Override
            String url(int z, int x, int y) {
                // Esri MapServer tiles use /tile/{z}/{y}/{x}
                return "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/"
                        + z + "/" + y + "/" + x;
            }
        };

        abstract String url(int z, int x, int y);

        String vectorStyle() {
            return null;
        }

        int minZoom() {
            return 0;
        }

        int maxZoom() {
            return 18;
        }

        static Provider forKey(String key) {
            if ("osm".equals(key)) {
                return OSM;
            }
            if ("openfreemap_light".equals(key)) {
                return OPENFREEMAP_LIGHT;
            }
            if ("openfreemap_dark".equals(key)) {
                return OPENFREEMAP_DARK;
            }
            if ("esri_imagery".equals(key)) {
                return ESRI_IMAGERY;
            }
            return null;
        }
    }

    /** MapLibre style document for an OpenFreeMap key, or null for raster / none. */
    public static String styleEndpoint(String basemapKey) {
        Provider provider = Provider.forKey(normalize(basemapKey));
        if (provider == null) {
            return null;
        }
        return provider.vectorStyle();
    }

    /**
     * Resolved OpenFreeMap vector-tile template, after reading the style and TileJSON.
     * Null for raster basemaps and when the style cannot be loaded.
     */
    public static String tileTemplate(String basemapKey) {
        String style = styleEndpoint(basemapKey);
        if (style == null) {
            return null;
        }
        OpenFreeMapBasemap.prepare(style);
        return OpenFreeMapBasemap.tileTemplate(style);
    }

    /** Raster XYZ URL for OSM / ESRI. Null for OpenFreeMap and none. */
    public static String rasterTileURL(String basemapKey, int z, int x, int y) {
        Provider provider = Provider.forKey(normalize(basemapKey));
        if (provider == null || provider.vectorStyle() != null) {
            return null;
        }
        return provider.url(z, x, y);
    }

    private static int lonToTileX(double lon, int n) {
        return (int) Math.floor((lon + 180.0) / 360.0 * n);
    }

    private static int latToTileY(double lat, int n) {
        double rad = Math.toRadians(lat);
        double y = Math.log(Math.tan(rad) + 1.0 / Math.cos(rad));
        return (int) Math.floor((1.0 - y / Math.PI) / 2.0 * n);
    }

    private static double tileXToLon(int x, int n) {
        return x / (double) n * 360.0 - 180.0;
    }

    private static double tileYToLat(int y, int n) {
        double t = Math.PI * (1.0 - 2.0 * y / (double) n);
        return Math.toDegrees(Math.atan(Math.sinh(t)));
    }

    private static double lonToMercX(double lon) {
        return lon * ORIGIN_SHIFT / 180.0;
    }

    private static double latToMercY(double lat) {
        double clamped = Math.max(-85.05112878, Math.min(85.05112878, lat));
        double rad = Math.toRadians(clamped);
        return Math.log(Math.tan(Math.PI / 4.0 + rad / 2.0)) * ORIGIN_SHIFT / Math.PI;
    }

    private static double mercXToLon(double x) {
        return (x / ORIGIN_SHIFT) * 180.0;
    }

    private static double mercYToLat(double y) {
        double rad = Math.atan(Math.sinh(y * Math.PI / ORIGIN_SHIFT));
        return Math.toDegrees(rad);
    }

    private static int clamp(int v, int lo, int hi) {
        if (v < lo) {
            return lo;
        }
        if (v > hi) {
            return hi;
        }
        return v;
    }
}
