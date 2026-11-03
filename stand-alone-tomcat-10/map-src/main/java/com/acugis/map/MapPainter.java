package com.acugis.map;

import java.awt.BasicStroke;
import java.awt.Color;
import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.geom.AffineTransform;
import java.awt.geom.Ellipse2D;
import java.awt.image.BufferedImage;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import org.geotools.geometry.jts.LiteShape;
import org.locationtech.jts.geom.Coordinate;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.geom.LineString;
import org.locationtech.jts.geom.MultiLineString;
import org.locationtech.jts.geom.MultiPoint;
import org.locationtech.jts.geom.MultiPolygon;
import org.locationtech.jts.geom.Point;
import org.locationtech.jts.geom.Polygon;

/** Draws map features with Phase A+B+C symbology. */
final class MapPainter {

    private static final Color FALLBACK_OTHER = new Color(0x9e, 0x9e, 0x9e);
    private static final double[] GRAD_RADII = new double[] {4, 6, 8, 11, 14};
    /** Server proportional: radius = 4 + t * 20. */
    private static final double PROP_MIN_R = 4;
    private static final double PROP_MAX_R = 24;
    /** OpenLayers Heatmap defaults (approx). */
    private static final int HEAT_RADIUS = 10;
    private static final int HEAT_BLUR = 15;

    private MapPainter() {}

    static final class PaintTimings {
        long basemapMs;
        long featureRenderMs;
        long labelRenderMs;
        long legendRenderMs;
        long coordTransformMs;
    }

    static void paint(BufferedImage image, List<MapFeature> features, MapViewport viewport, Symbology sym) {
        paint(image, features, viewport, sym, null, null);
    }

    static void paint(
            BufferedImage image,
            List<MapFeature> features,
            MapViewport viewport,
            Symbology sym,
            String basemap,
            String srs) {
        paintTimed(image, features, viewport, sym, basemap, srs);
    }

    static PaintTimings paintTimed(
            BufferedImage image,
            List<MapFeature> features,
            MapViewport viewport,
            Symbology sym,
            String basemap,
            String srs) {
        PaintTimings timings = new PaintTimings();
        Graphics2D g = image.createGraphics();
        try {
            g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);
            g.setColor(Color.WHITE);
            g.fillRect(0, 0, image.getWidth(), image.getHeight());
            // XYZ underlay (skipped for none / unsupported CRS / fetch failure).
            long basemapStart = System.nanoTime();
            BasemapTiles.paint(g, viewport, basemap, srs);
            timings.basemapMs = (System.nanoTime() - basemapStart) / 1_000_000L;
            if (features == null || features.isEmpty()) {
                return timings;
            }
            if (sym == null) {
                sym = Symbology.defaults();
            }

            long xformStart = System.nanoTime();
            AffineTransform worldToScreen = new AffineTransform();
            worldToScreen.translate(viewport.offsetX, viewport.offsetY);
            worldToScreen.scale(viewport.scaleX, -viewport.scaleY);
            StyleResolver resolver = StyleResolver.build(features, sym);
            timings.coordTransformMs = (System.nanoTime() - xformStart) / 1_000_000L;

            long featureStart = System.nanoTime();
            if (sym.renderer == Symbology.Renderer.HEATMAP) {
                paintHeatmap(image, features, viewport, resolver);
            } else {
                for (MapFeature f : features) {
                    drawFeature(g, f, worldToScreen, viewport, resolver);
                }
            }
            timings.featureRenderMs = (System.nanoTime() - featureStart) / 1_000_000L;

            // Feature labels use loaded attributes only (provider-independent).
            if (sym.labels != null && sym.labels.enabled) {
                long labelStart = System.nanoTime();
                MapLabels.paint(g, features, viewport, sym.labels);
                timings.labelRenderMs = (System.nanoTime() - labelStart) / 1_000_000L;
            }

            if (sym.showLegend && sym.isThematic()) {
                long legendStart = System.nanoTime();
                MapLegend.LegendModel legend = resolver.buildLegend();
                if (legend != null && !legend.rows.isEmpty()) {
                    MapLegend.paint(g, image.getWidth(), image.getHeight(), legend);
                }
                timings.legendRenderMs = (System.nanoTime() - legendStart) / 1_000_000L;
            }
            return timings;
        } finally {
            g.dispose();
        }
    }

    private static void paintHeatmap(
            BufferedImage image, List<MapFeature> features, MapViewport viewport, StyleResolver resolver) {
        int w = image.getWidth();
        int h = image.getHeight();
        float[] intensity = new float[w * h];
        float max = 0f;
        int kernelR = HEAT_RADIUS + HEAT_BLUR;
        double sigma = Math.max(1.0, kernelR / 2.5);

        for (MapFeature f : features) {
            Coordinate c = centroidPixel(f.geometry, viewport);
            if (c == null) {
                continue;
            }
            int px = (int) Math.round(c.x);
            int py = (int) Math.round(c.y);
            float weight = resolver.heatmapWeight(f);
            if (weight <= 0) {
                continue;
            }
            int x0 = Math.max(0, px - kernelR);
            int x1 = Math.min(w - 1, px + kernelR);
            int y0 = Math.max(0, py - kernelR);
            int y1 = Math.min(h - 1, py + kernelR);
            for (int y = y0; y <= y1; y++) {
                for (int x = x0; x <= x1; x++) {
                    double dx = x - px;
                    double dy = y - py;
                    double dist2 = dx * dx + dy * dy;
                    if (dist2 > kernelR * kernelR) {
                        continue;
                    }
                    float add = (float) (weight * Math.exp(-dist2 / (2 * sigma * sigma)));
                    int idx = y * w + x;
                    intensity[idx] += add;
                    if (intensity[idx] > max) {
                        max = intensity[idx];
                    }
                }
            }
        }

        if (max <= 0) {
            return;
        }
        List<Color> ramp = Symbology.heatRampColors(8);
        for (int i = 0; i < intensity.length; i++) {
            float t = intensity[i] / max;
            if (t <= 0.02f) {
                continue;
            }
            t = Math.min(1f, t);
            int ri = Math.min(ramp.size() - 1, Math.round(t * (ramp.size() - 1)));
            Color base = ramp.get(ri);
            int alpha = Math.max(30, Math.min(220, Math.round(40 + t * 180)));
            int x = i % w;
            int y = i / w;
            int rgb = (alpha << 24) | (base.getRed() << 16) | (base.getGreen() << 8) | base.getBlue();
            // Composite over white background already painted.
            int bg = image.getRGB(x, y);
            image.setRGB(x, y, blendOver(bg, rgb));
        }
    }

    private static int blendOver(int dst, int src) {
        int sa = (src >>> 24) & 255;
        if (sa == 0) {
            return dst;
        }
        if (sa == 255) {
            return src;
        }
        int sr = (src >>> 16) & 255;
        int sg = (src >>> 8) & 255;
        int sb = src & 255;
        int dr = (dst >>> 16) & 255;
        int dg = (dst >>> 8) & 255;
        int db = dst & 255;
        int a = sa;
        int inv = 255 - a;
        int r = (sr * a + dr * inv) / 255;
        int g = (sg * a + dg * inv) / 255;
        int b = (sb * a + db * inv) / 255;
        return 0xFF000000 | (r << 16) | (g << 8) | b;
    }

    private static Coordinate centroidPixel(Geometry geom, MapViewport viewport) {
        if (geom == null || geom.isEmpty()) {
            return null;
        }
        Coordinate c;
        try {
            c = geom.getCentroid().getCoordinate();
        } catch (Exception e) {
            c = geom.getCoordinate();
        }
        if (c == null) {
            return null;
        }
        return viewport.toPixel(c);
    }

    private static void drawFeature(
            Graphics2D g,
            MapFeature feature,
            AffineTransform at,
            MapViewport viewport,
            StyleResolver resolver) {
        Geometry geom = feature.geometry;
        if (geom == null || geom.isEmpty()) {
            return;
        }
        ResolvedStyle style = resolver.styleFor(feature);

        // Proportional always draws point symbols (Server behaviour).
        if (resolver.sym.renderer == Symbology.Renderer.PROPORTIONAL) {
            Coordinate pixel = centroidPixel(geom, viewport);
            if (pixel != null) {
                drawPointAt(g, pixel, style);
            }
            return;
        }

        if (geom instanceof Point) {
            drawPoint(g, (Point) geom, viewport, style);
        } else if (geom instanceof MultiPoint) {
            MultiPoint mp = (MultiPoint) geom;
            for (int i = 0; i < mp.getNumGeometries(); i++) {
                drawPoint(g, (Point) mp.getGeometryN(i), viewport, style);
            }
        } else if (geom instanceof LineString || geom instanceof MultiLineString) {
            LiteShape shape = new LiteShape(geom, at, false);
            g.setColor(style.stroke);
            g.setStroke(new BasicStroke(Math.max(0.5f, style.strokeWidth)));
            g.draw(shape);
        } else if (geom instanceof Polygon || geom instanceof MultiPolygon) {
            LiteShape shape = new LiteShape(geom, at, false);
            g.setColor(style.fill);
            g.fill(shape);
            g.setColor(style.stroke);
            g.setStroke(new BasicStroke(Math.max(0.5f, style.strokeWidth)));
            g.draw(shape);
        }
    }

    private static void drawPoint(Graphics2D g, Point point, MapViewport viewport, ResolvedStyle style) {
        drawPointAt(g, viewport.toPixel(point.getCoordinate()), style);
    }

    private static void drawPointAt(Graphics2D g, Coordinate pixel, ResolvedStyle style) {
        if (pixel == null) {
            return;
        }
        double d = style.pointDiameter;
        double x = pixel.x - d / 2.0;
        double y = pixel.y - d / 2.0;
        g.setColor(style.fill);
        g.fill(new Ellipse2D.Double(x, y, d, d));
        if (style.strokeWidth > 0) {
            g.setColor(style.stroke);
            g.setStroke(new BasicStroke(Math.max(0.5f, style.strokeWidth)));
            g.draw(new Ellipse2D.Double(x, y, d, d));
        }
    }

    private static final class ResolvedStyle {
        final Color fill;
        final Color stroke;
        final float strokeWidth;
        final double pointDiameter;

        ResolvedStyle(Color fill, Color stroke, float strokeWidth, double pointDiameter) {
            this.fill = fill;
            this.stroke = stroke;
            this.strokeWidth = strokeWidth;
            this.pointDiameter = pointDiameter;
        }
    }

    private static final class StyleResolver {
        final Symbology sym;
        final Map<String, Color> categoryFills;
        final Map<String, Color> categoryOutlines;
        final Map<String, String> categoryLabels;
        final List<Double> breaks;
        final List<Color> classColors;
        final double valueMin;
        final double valueMax;
        final boolean pointDominant;

        private StyleResolver(
                Symbology sym,
                Map<String, Color> categoryFills,
                Map<String, Color> categoryOutlines,
                Map<String, String> categoryLabels,
                List<Double> breaks,
                List<Color> classColors,
                double valueMin,
                double valueMax,
                boolean pointDominant) {
            this.sym = sym;
            this.categoryFills = categoryFills;
            this.categoryOutlines = categoryOutlines;
            this.categoryLabels = categoryLabels;
            this.breaks = breaks;
            this.classColors = classColors;
            this.valueMin = valueMin;
            this.valueMax = valueMax;
            this.pointDominant = pointDominant;
        }

        static StyleResolver build(List<MapFeature> features, Symbology sym) {
            Map<String, Color> cats = new LinkedHashMap<String, Color>();
            Map<String, Color> catOutlines = new LinkedHashMap<String, Color>();
            Map<String, String> labels = new LinkedHashMap<String, String>();
            List<Double> breaks = Collections.emptyList();
            List<Color> colors = Collections.emptyList();
            double vmin = Double.NaN;
            double vmax = Double.NaN;
            boolean pointDom = dominantIsPoint(features);

            if (sym.renderer == Symbology.Renderer.CATEGORIZED) {
                if (!sym.categories.isEmpty()) {
                    for (Symbology.Category c : sym.categories) {
                        Color fill = c.fill != null ? c.fill : FALLBACK_OTHER;
                        cats.put(c.value, fill);
                        if (c.outline != null) {
                            catOutlines.put(c.value, c.outline);
                        }
                        labels.put(c.value, c.label != null && !c.label.isEmpty() ? c.label : c.value);
                    }
                } else if (sym.field != null && !sym.field.isEmpty()) {
                    List<String> values = new ArrayList<String>();
                    for (MapFeature f : features) {
                        Object v = f.attr(sym.field);
                        if (v == null) {
                            continue;
                        }
                        String key = String.valueOf(v);
                        if (!values.contains(key)) {
                            values.add(key);
                        }
                    }
                    Collections.sort(values);
                    List<Color> ramp = sym.rampColors(Math.max(values.size(), 2));
                    for (int i = 0; i < values.size(); i++) {
                        cats.put(values.get(i), ramp.get(Math.min(i, ramp.size() - 1)));
                        labels.put(values.get(i), values.get(i).isEmpty() ? "(empty)" : values.get(i));
                    }
                }
            } else if (sym.renderer == Symbology.Renderer.GRADUATED
                    || sym.renderer == Symbology.Renderer.CHOROPLETH) {
                List<Double> nums = collectNumeric(features, sym.field);
                if (!nums.isEmpty()) {
                    vmin = nums.get(0);
                    vmax = nums.get(nums.size() - 1);
                }
                breaks = Classification.computeBreaks(nums, sym.classification, sym.classes);
                int nColors = Math.max(breaks.size(), sym.classes);
                colors = sym.rampColors(nColors);
            } else if (sym.renderer == Symbology.Renderer.PROPORTIONAL
                    || sym.renderer == Symbology.Renderer.HEATMAP) {
                List<Double> nums = collectNumeric(features, sym.field);
                if (!nums.isEmpty()) {
                    vmin = nums.get(0);
                    vmax = nums.get(nums.size() - 1);
                    if (vmin == vmax) {
                        vmax = vmin + 1;
                    }
                }
            }
            return new StyleResolver(sym, cats, catOutlines, labels, breaks, colors, vmin, vmax, pointDom);
        }

        private static List<Double> collectNumeric(List<MapFeature> features, String field) {
            List<Double> nums = new ArrayList<Double>();
            if (field == null || field.isEmpty()) {
                return nums;
            }
            for (MapFeature f : features) {
                Double n = toDouble(f.attr(field));
                if (n != null) {
                    nums.add(n);
                }
            }
            Collections.sort(nums);
            return nums;
        }

        private static boolean dominantIsPoint(List<MapFeature> features) {
            int pts = 0;
            int other = 0;
            for (MapFeature f : features) {
                if (f.geometry == null) {
                    continue;
                }
                if (isPointish(f.geometry)) {
                    pts++;
                } else {
                    other++;
                }
            }
            return pts >= other;
        }

        float heatmapWeight(MapFeature f) {
            if (sym.field == null || sym.field.isEmpty() || Double.isNaN(valueMin)) {
                return 1f;
            }
            Double n = toDouble(f.attr(sym.field));
            if (n == null) {
                return 0.5f;
            }
            double t = (n - valueMin) / (valueMax - valueMin);
            return (float) Math.max(0, Math.min(1, t));
        }

        ResolvedStyle styleFor(MapFeature f) {
            Color fill = sym.withOpacity(sym.single.fill);
            Color stroke = sym.single.outline;
            float width = sym.single.outlineWidth;
            double diameter = 6;

            switch (sym.renderer) {
                case CATEGORIZED: {
                    Object v = f.attr(sym.field);
                    String key = v == null ? null : String.valueOf(v);
                    Color c = key == null ? FALLBACK_OTHER : categoryFills.get(key);
                    if (c == null) {
                        c = FALLBACK_OTHER;
                    }
                    fill = sym.withOpacity(c);
                    if (key != null && categoryOutlines.containsKey(key)) {
                        stroke = categoryOutlines.get(key);
                    }
                    break;
                }
                case GRADUATED: {
                    Double n = toDouble(f.attr(sym.field));
                    int idx = n == null ? 0 : Classification.classIndex(n, breaks);
                    Color c = classColors.isEmpty() ? sym.single.fill
                            : classColors.get(Math.min(idx, classColors.size() - 1));
                    fill = sym.withOpacity(c);
                    diameter = graduatedRadius(idx, classColors.size());
                    break;
                }
                case CHOROPLETH: {
                    if (isPointish(f.geometry)) {
                        fill = sym.withOpacity(sym.single.fill);
                    } else {
                        Double n = toDouble(f.attr(sym.field));
                        int idx = n == null ? 0 : Classification.classIndex(n, breaks);
                        Color c = classColors.isEmpty() ? sym.single.fill
                                : classColors.get(Math.min(idx, classColors.size() - 1));
                        fill = sym.withOpacity(c);
                    }
                    break;
                }
                case PROPORTIONAL: {
                    Double n = toDouble(f.attr(sym.field));
                    double t = 0.5;
                    if (n != null && !Double.isNaN(valueMin) && valueMax > valueMin) {
                        t = (n - valueMin) / (valueMax - valueMin);
                        t = Math.max(0, Math.min(1, t));
                    }
                    double radius = PROP_MIN_R + t * (PROP_MAX_R - PROP_MIN_R);
                    diameter = radius * 2;
                    fill = sym.withOpacity(sym.single.fill);
                    break;
                }
                case HEATMAP:
                case SINGLE:
                default:
                    break;
            }
            return new ResolvedStyle(fill, stroke, width, diameter);
        }

        MapLegend.LegendModel buildLegend() {
            MapLegend.LegendModel m = new MapLegend.LegendModel();
            m.title = (sym.field != null && !sym.field.isEmpty()) ? sym.field : sym.renderer.name().toLowerCase();

            switch (sym.renderer) {
                case CATEGORIZED: {
                    if (categoryFills.isEmpty()) {
                        m.add(MapLegend.LegendRow.swatch("No categories defined", FALLBACK_OTHER));
                    } else {
                        for (Map.Entry<String, Color> e : categoryFills.entrySet()) {
                            String label = categoryLabels.containsKey(e.getKey())
                                    ? categoryLabels.get(e.getKey()) : e.getKey();
                            Color fill = e.getValue();
                            if (pointDominant) {
                                m.add(MapLegend.LegendRow.circle(label, fill, sym.single.outline, 6));
                            } else {
                                m.add(MapLegend.LegendRow.swatch(label, fill));
                            }
                        }
                    }
                    break;
                }
                case GRADUATED:
                case CHOROPLETH: {
                    if (breaks.isEmpty()) {
                        m.add(MapLegend.LegendRow.swatch("No classes", FALLBACK_OTHER));
                    } else {
                        Double prev = Double.isNaN(valueMin) ? null : valueMin;
                        for (int i = 0; i < breaks.size(); i++) {
                            double hi = breaks.get(i);
                            Color color = classColors.isEmpty() ? sym.single.fill
                                    : classColors.get(Math.min(i, classColors.size() - 1));
                            String label;
                            if (prev == null) {
                                label = "≤ " + MapLegend.formatNumber(hi);
                            } else {
                                label = MapLegend.formatNumber(prev) + " – " + MapLegend.formatNumber(hi);
                            }
                            if (sym.renderer == Symbology.Renderer.GRADUATED && pointDominant) {
                                double r = graduatedRadius(i, classColors.size()) / 2.0;
                                m.add(MapLegend.LegendRow.circle(label, color, sym.single.outline, r));
                            } else {
                                m.add(MapLegend.LegendRow.swatch(label, color));
                            }
                            prev = hi;
                        }
                    }
                    break;
                }
                case HEATMAP: {
                    m.rampBar = true;
                    m.add(MapLegend.LegendRow.swatch(
                            Double.isNaN(valueMin) ? "—" : MapLegend.formatNumber(valueMin), Color.BLUE));
                    m.add(MapLegend.LegendRow.swatch(
                            Double.isNaN(valueMax) ? "—" : MapLegend.formatNumber(valueMax), Color.RED));
                    break;
                }
                case PROPORTIONAL: {
                    Color fill = sym.withOpacity(sym.single.fill);
                    Color stroke = sym.single.outline;
                    double mid = (!Double.isNaN(valueMin) && !Double.isNaN(valueMax))
                            ? (valueMin + valueMax) / 2.0 : Double.NaN;
                    m.add(MapLegend.LegendRow.circle(
                            Double.isNaN(valueMin) ? "—" : MapLegend.formatNumber(valueMin),
                            fill, stroke, 6));
                    m.add(MapLegend.LegendRow.circle(
                            Double.isNaN(mid) ? "—" : MapLegend.formatNumber(mid),
                            fill, stroke, 12));
                    m.add(MapLegend.LegendRow.circle(
                            Double.isNaN(valueMax) ? "—" : MapLegend.formatNumber(valueMax),
                            fill, stroke, 18));
                    break;
                }
                default:
                    return null;
            }
            return m;
        }

        private static boolean isPointish(Geometry g) {
            return g instanceof Point || g instanceof MultiPoint;
        }

        private static double graduatedRadius(int classIndex, int classCount) {
            int n = Math.max(2, Math.min(GRAD_RADII.length, classCount <= 0 ? GRAD_RADII.length : classCount));
            int idx = Math.max(0, Math.min(n - 1, classIndex));
            double t = n == 1 ? 0 : (double) idx / (double) (n - 1);
            double pos = t * (GRAD_RADII.length - 1);
            int lo = (int) Math.floor(pos);
            int hi = Math.min(GRAD_RADII.length - 1, lo + 1);
            if (lo == hi) {
                return GRAD_RADII[lo] * 2;
            }
            double r = GRAD_RADII[lo] + (GRAD_RADII[hi] - GRAD_RADII[lo]) * (pos - lo);
            return r * 2;
        }

        private static Double toDouble(Object v) {
            if (v == null) {
                return null;
            }
            if (v instanceof Number) {
                return ((Number) v).doubleValue();
            }
            try {
                return Double.valueOf(String.valueOf(v).trim());
            } catch (Exception e) {
                return null;
            }
        }
    }
}
