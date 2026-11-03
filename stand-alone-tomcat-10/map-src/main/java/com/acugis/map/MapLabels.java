package com.acugis.map;

import java.awt.BasicStroke;
import java.awt.Color;
import java.awt.Font;
import java.awt.FontMetrics;
import java.awt.Graphics2D;
import java.awt.RenderingHints;
import java.awt.Shape;
import java.awt.font.FontRenderContext;
import java.awt.font.GlyphVector;
import java.awt.geom.AffineTransform;
import java.awt.geom.Rectangle2D;
import java.util.ArrayList;
import java.util.List;
import org.locationtech.jts.geom.Coordinate;
import org.locationtech.jts.geom.Geometry;
import org.locationtech.jts.geom.LineString;
import org.locationtech.jts.geom.MultiLineString;
import org.locationtech.jts.geom.MultiPoint;
import org.locationtech.jts.geom.Point;
import org.locationtech.jts.linearref.LengthIndexedLine;

/**
 * Draws provider-independent feature labels using already-loaded attributes.
 * Placement is relative to a geometry anchor (point / line midpoint / polygon centroid).
 */
final class MapLabels {

    private static final float POINT_GAP = 4f;

    private MapLabels() {}

    static void paint(
            Graphics2D g,
            List<MapFeature> features,
            MapViewport viewport,
            Symbology.Labels cfg) {
        if (cfg == null || !cfg.enabled) {
            return;
        }
        if (cfg.field == null || cfg.field.isEmpty()) {
            return;
        }
        if (features == null || features.isEmpty()) {
            return;
        }

        g.setRenderingHint(RenderingHints.KEY_TEXT_ANTIALIASING, RenderingHints.VALUE_TEXT_ANTIALIAS_ON);
        g.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON);

        int style = Font.PLAIN;
        if (cfg.bold) {
            style |= Font.BOLD;
        }
        if (cfg.italic) {
            style |= Font.ITALIC;
        }
        String family = cfg.fontFamily == null || cfg.fontFamily.isEmpty() ? "SansSerif" : cfg.fontFamily;
        float size = cfg.fontSize > 0 ? cfg.fontSize : 10f;
        Font font = new Font(family, style, Math.round(size));
        g.setFont(font);
        FontMetrics fm = g.getFontMetrics(font);
        FontRenderContext frc = g.getFontRenderContext();

        List<Rectangle2D> placed = cfg.avoidOverlap ? new ArrayList<Rectangle2D>() : null;

        for (MapFeature f : features) {
            Object raw = f.attr(cfg.field);
            if (raw == null) {
                continue;
            }
            String text = String.valueOf(raw).trim();
            if (text.isEmpty() || "null".equalsIgnoreCase(text)) {
                continue;
            }

            Coordinate anchor = labelAnchorPixel(f.geometry, viewport);
            if (anchor == null) {
                continue;
            }

            float textW = fm.stringWidth(text);
            float textH = fm.getAscent() + fm.getDescent();
            float[] xy = placeBaseline(anchor, textW, textH, fm.getAscent(), cfg);
            float x = xy[0];
            float y = xy[1];

            Rectangle2D bounds = new Rectangle2D.Float(
                    x, y - fm.getAscent(), textW, textH);
            if (placed != null) {
                boolean hits = false;
                for (Rectangle2D r : placed) {
                    if (r.intersects(bounds)) {
                        hits = true;
                        break;
                    }
                }
                if (hits) {
                    continue;
                }
                placed.add(bounds);
            }

            if (cfg.halo && cfg.haloWidth > 0) {
                drawHalo(g, font, frc, text, x, y, cfg.haloColor, cfg.haloWidth);
            }
            g.setColor(cfg.fontColor != null ? cfg.fontColor : Color.BLACK);
            g.drawString(text, x, y);
        }
    }

    private static void drawHalo(
            Graphics2D g, Font font, FontRenderContext frc, String text,
            float x, float y, Color color, float width) {
        GlyphVector gv = font.createGlyphVector(frc, text);
        Shape outline = gv.getOutline(x, y);
        g.setColor(color != null ? color : Color.WHITE);
        g.setStroke(new BasicStroke(width * 2f, BasicStroke.CAP_ROUND, BasicStroke.JOIN_ROUND));
        g.draw(outline);
        // Fill halo outline for denser coverage at small widths.
        AffineTransform at = g.getTransform();
        try {
            for (int dx = -1; dx <= 1; dx++) {
                for (int dy = -1; dy <= 1; dy++) {
                    if (dx == 0 && dy == 0) {
                        continue;
                    }
                    float ox = dx * Math.max(1f, width * 0.5f);
                    float oy = dy * Math.max(1f, width * 0.5f);
                    g.drawString(text, x + ox, y + oy);
                }
            }
        } finally {
            g.setTransform(at);
        }
    }

    /**
     * Returns baseline (x,y) for drawString given anchor and placement/offsets.
     */
    private static float[] placeBaseline(
            Coordinate anchor, float textW, float textH, float ascent, Symbology.Labels cfg) {
        float ax = (float) anchor.x;
        float ay = (float) anchor.y;
        float ox = cfg.offsetX;
        float oy = cfg.offsetY;
        String p = cfg.placement == null ? "right" : cfg.placement;

        float x;
        float y;
        if ("center".equals(p)) {
            x = ax - textW / 2f;
            y = ay + ascent / 2f - (textH - ascent) / 2f;
        } else if ("above".equals(p)) {
            x = ax - textW / 2f;
            y = ay - POINT_GAP;
        } else if ("below".equals(p)) {
            x = ax - textW / 2f;
            y = ay + ascent + POINT_GAP;
        } else if ("left".equals(p)) {
            x = ax - textW - POINT_GAP;
            y = ay + ascent / 2f - (textH - ascent) / 2f;
        } else if ("upperLeft".equals(p)) {
            x = ax - textW - POINT_GAP;
            y = ay - POINT_GAP;
        } else if ("upperRight".equals(p)) {
            x = ax + POINT_GAP;
            y = ay - POINT_GAP;
        } else if ("lowerLeft".equals(p)) {
            x = ax - textW - POINT_GAP;
            y = ay + ascent + POINT_GAP;
        } else if ("lowerRight".equals(p)) {
            x = ax + POINT_GAP;
            y = ay + ascent + POINT_GAP;
        } else {
            // right (default — beside point symbols)
            x = ax + POINT_GAP;
            y = ay + ascent / 2f - (textH - ascent) / 2f;
        }
        return new float[] {x + ox, y + oy};
    }

    static Coordinate labelAnchorPixel(Geometry geom, MapViewport viewport) {
        if (geom == null || geom.isEmpty()) {
            return null;
        }
        if (geom instanceof Point || geom instanceof MultiPoint) {
            return centroidPixel(geom, viewport);
        }
        if (geom instanceof LineString || geom instanceof MultiLineString) {
            return lineMidpointPixel(geom, viewport);
        }
        return centroidPixel(geom, viewport);
    }

    private static Coordinate centroidPixel(Geometry geom, MapViewport viewport) {
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

    private static Coordinate lineMidpointPixel(Geometry geom, MapViewport viewport) {
        try {
            LengthIndexedLine lil = new LengthIndexedLine(geom);
            double len = geom.getLength();
            if (len > 0) {
                Coordinate c = lil.extractPoint(len / 2.0);
                if (c != null) {
                    return viewport.toPixel(c);
                }
            }
        } catch (Exception ignored) {
            // fall through to centroid
        }
        return centroidPixel(geom, viewport);
    }
}
